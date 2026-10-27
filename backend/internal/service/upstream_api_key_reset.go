package service

import (
	"context"
	"log/slog"
	"math"
	"strings"
	"sync"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const AutoResetAPIKeysOnUpstreamResetExtraKey = "auto_reset_api_keys_on_upstream_reset"

// UpstreamAPIKeyResetSnapshot contains only the authoritative weekly observation,
// never credentials or a locally configured account quota window.
type UpstreamAPIKeyResetSnapshot struct {
	Source     string
	ResetAt    time.Time
	ObservedAt time.Time
	Used       *float64
}

type UpstreamAPIKeyResetResult struct {
	ResetGroups   int
	ResetKeys     int
	SkippedGroups int
}

// The repository owns observation, baseline advancement and key resets in one
// database transaction. Keeping this narrow avoids widening AccountRepository.
type UpstreamAPIKeyResetRepository interface {
	ListEnabledAccountIDs(context.Context) ([]int64, error)
	ProcessAccount(context.Context, int64, time.Time) (UpstreamAPIKeyResetResult, error)
}

func normalizeUpstreamAPIKeyResetExtra(extra map[string]any) (map[string]any, error) {
	if extra == nil {
		return nil, nil
	}
	if value, exists := extra[AutoResetAPIKeysOnUpstreamResetExtraKey]; exists {
		if _, ok := value.(bool); !ok {
			return nil, infraerrors.BadRequest("INVALID_AUTO_RESET_API_KEYS_ON_UPSTREAM_RESET", "auto_reset_api_keys_on_upstream_reset must be a boolean")
		}
	}
	return extra, nil
}

// Ordinary account saves resend group_ids. Do not delete/recreate identical
// bindings for enabled accounts: that would erase their observation baseline.
// Priority/order changes still use the existing BindGroups behavior.
func upstreamAPIKeyResetGroupsUnchanged(account *Account, groupIDs []int64) bool {
	if account == nil {
		return false
	}
	if enabled, _ := account.Extra[AutoResetAPIKeysOnUpstreamResetExtraKey].(bool); !enabled {
		return false
	}
	if len(account.AccountGroups) != len(groupIDs) {
		return false
	}
	priorities := make(map[int64]int, len(account.AccountGroups))
	for _, binding := range account.AccountGroups {
		priorities[binding.GroupID] = binding.Priority
	}
	for i, id := range groupIDs {
		if priority, ok := priorities[id]; !ok || priority != i+1 {
			return false
		}
		delete(priorities, id)
	}
	return len(priorities) == 0
}

// These snapshot fields are written only by upstream observers. Admin edits may
// round-trip them but cannot fabricate a reset. Baselines live outside extra.
func IsUpstreamAPIKeyResetSnapshotKey(key string) bool {
	switch key {
	case "codex_7d_reset_at", "codex_7d_used_percent", "codex_usage_updated_at",
		"passive_usage_7d_reset", "passive_usage_7d_utilization", "passive_usage_sampled_at", grokBillingExtraKey:
		return true
	}
	for _, provider := range []string{PlatformKimi, PlatformZhipu, PlatformMiniMax, PlatformOpenCodeGo, PlatformCommandCode, PlatformCline} {
		for _, suffix := range []string{cnExtraSuffixWeeklyReset, cnExtraSuffixWeeklyUsed, cnExtraSuffixUsageUpdated} {
			if key == cnExtraKey(provider, suffix) {
				return true
			}
		}
	}
	return false
}

func stripUpstreamAPIKeyResetSnapshots(extra map[string]any) {
	for key := range extra {
		if IsUpstreamAPIKeyResetSnapshotKey(key) {
			delete(extra, key)
		}
	}
}

// UpstreamAPIKeyResetObservation deliberately does not use accountSevenDayResetAt:
// that manual alignment helper also accepts local quota_weekly_reset_at.
func UpstreamAPIKeyResetObservation(account *Account) (UpstreamAPIKeyResetSnapshot, bool) {
	if account == nil || account.IsShadow() {
		return UpstreamAPIKeyResetSnapshot{}, false
	}
	extra := account.Extra
	var snapshot UpstreamAPIKeyResetSnapshot
	var used any
	switch {
	case account.Platform == PlatformOpenAI && account.Type == AccountTypeOAuth:
		snapshot.Source = "codex"
		snapshot.ResetAt = parseExtraTime(extra["codex_7d_reset_at"])
		snapshot.ObservedAt = parseExtraTime(extra["codex_usage_updated_at"])
		used = extra["codex_7d_used_percent"]
	case account.Platform == PlatformAnthropic:
		snapshot.Source = "anthropic"
		snapshot.ResetAt = parseUnixSecondsTime(extra["passive_usage_7d_reset"])
		snapshot.ObservedAt = parseExtraTime(extra["passive_usage_sampled_at"])
		if value, ok := parseOpenAIAutoResetThreshold(extra["passive_usage_7d_utilization"]); ok {
			used = value * 100
		}
	case account.GetCodingPlanProvider() != "":
		provider := account.GetCodingPlanProvider()
		snapshot.Source = provider
		snapshot.ResetAt = parseExtraTime(extra[cnExtraKey(provider, cnExtraSuffixWeeklyReset)])
		snapshot.ObservedAt = parseExtraTime(extra[cnExtraKey(provider, cnExtraSuffixUsageUpdated)])
		used = extra[cnExtraKey(provider, cnExtraSuffixWeeklyUsed)]
	case account.Platform == PlatformGrok:
		billing, err := grokBillingSnapshotFromExtra(extra)
		if err != nil || billing == nil || strings.ToLower(strings.TrimSpace(billing.PeriodType)) != "weekly" {
			return snapshot, false
		}
		snapshot.Source = "grok"
		snapshot.ResetAt = parseExtraTime(billing.PeriodEnd)
		// A monthly-only refresh must not make an old weekly snapshot newer.
		snapshot.ObservedAt = parseExtraTime(billing.WeeklyUpdatedAt)
		if billing.UsagePercent != nil {
			used = *billing.UsagePercent
		}
	}
	if value, ok := parseOpenAIAutoResetThreshold(used); ok && value >= 0 && !math.IsNaN(value) && !math.IsInf(value, 0) {
		snapshot.Used = &value
	}
	return snapshot, snapshot.Source != "" && !snapshot.ResetAt.IsZero() && !snapshot.ObservedAt.IsZero()
}

// AdvanceUpstreamAPIKeyReset ignores stale observations and small clock fixes.
// A large future jump before expiry is insufficient without a usage decrease.
// Rejected jumps do not overwrite the last trusted reset boundary.
func AdvanceUpstreamAPIKeyReset(previous *UpstreamAPIKeyResetSnapshot, current UpstreamAPIKeyResetSnapshot, now time.Time) (next UpstreamAPIKeyResetSnapshot, reset bool) {
	if previous != nil {
		next = *previous
	}
	if current.ResetAt.IsZero() || current.ObservedAt.IsZero() || current.ObservedAt.After(now.Add(time.Minute)) ||
		!current.ResetAt.After(current.ObservedAt) || current.ResetAt.After(current.ObservedAt.Add(RateLimitWindow7d+10*time.Minute)) {
		return next, false
	}
	if previous == nil || current.Source != previous.Source {
		if !current.ResetAt.After(now) {
			return next, false
		}
		return current, false
	}
	if !current.ObservedAt.After(previous.ObservedAt) || current.ResetAt.Before(previous.ResetAt) {
		return next, false
	}
	advance := current.ResetAt.Sub(previous.ResetAt)
	if advance <= 24*time.Hour {
		return current, false
	}
	normal := !current.ObservedAt.Before(previous.ResetAt)
	early := current.Used != nil && previous.Used != nil && *current.Used+0.5 < *previous.Used
	if current.ResetAt.After(now) && (normal || early) {
		return current, true
	}
	return next, false
}

type UpstreamAPIKeyResetService struct {
	repo   UpstreamAPIKeyResetRepository
	ctx    context.Context
	cancel context.CancelFunc
	start  sync.Once
	stop   sync.Once
	wg     sync.WaitGroup
}

func NewUpstreamAPIKeyResetService(repo UpstreamAPIKeyResetRepository) *UpstreamAPIKeyResetService {
	ctx, cancel := context.WithCancel(context.Background())
	return &UpstreamAPIKeyResetService{repo: repo, ctx: ctx, cancel: cancel}
}

func (s *UpstreamAPIKeyResetService) Start() {
	if s == nil || s.repo == nil {
		return
	}
	s.start.Do(func() {
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			ticker := time.NewTicker(time.Minute)
			defer ticker.Stop()
			for {
				s.scan(s.ctx)
				select {
				case <-s.ctx.Done():
					return
				case <-ticker.C:
				}
			}
		}()
	})
}

func (s *UpstreamAPIKeyResetService) Stop() {
	if s == nil {
		return
	}
	s.stop.Do(func() { s.cancel(); s.wg.Wait() })
}

func (s *UpstreamAPIKeyResetService) scan(parent context.Context) {
	ctx, cancel := context.WithTimeout(parent, 50*time.Second)
	defer cancel()
	ids, err := s.repo.ListEnabledAccountIDs(ctx)
	if err != nil {
		if ctx.Err() == nil {
			slog.Warn("upstream_api_key_reset_scan_failed")
		}
		return
	}
	for _, id := range ids {
		if ctx.Err() != nil {
			return
		}
		result, err := s.repo.ProcessAccount(ctx, id, time.Now().UTC())
		if err != nil {
			// Do not log driver errors: SQL parameters can contain account extra.
			slog.Warn("upstream_api_key_reset_transaction_failed", "account_id", id)
			continue
		}
		if result.SkippedGroups > 0 {
			slog.Warn("upstream_api_key_reset_ambiguous_groups", "account_id", id, "groups", result.SkippedGroups)
		}
		if result.ResetGroups > 0 {
			slog.Info("upstream_api_key_reset_applied", "account_id", id, "groups", result.ResetGroups, "keys", result.ResetKeys)
		}
	}
}
