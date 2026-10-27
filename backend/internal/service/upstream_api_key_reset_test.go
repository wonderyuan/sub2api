package service

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAdvanceUpstreamAPIKeyReset(t *testing.T) {
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	used90, used10 := 90.0, 10.0
	old := UpstreamAPIKeyResetSnapshot{Source: "codex", ResetAt: now.Add(-time.Minute), ObservedAt: now.Add(-time.Hour), Used: &used90}
	fresh := UpstreamAPIKeyResetSnapshot{Source: "codex", ResetAt: now.Add(RateLimitWindow7d - time.Minute), ObservedAt: now, Used: &used10}
	for _, tc := range []struct {
		name        string
		previous    *UpstreamAPIKeyResetSnapshot
		current     UpstreamAPIKeyResetSnapshot
		reset       bool
		wantResetAt time.Time
	}{
		{"first observation only baselines", nil, fresh, false, fresh.ResetAt},
		{"expired first observation is ignored", nil, old, false, time.Time{}},
		{"normal expiry advances", &old, fresh, true, fresh.ResetAt},
		{"same cycle", &fresh, fresh, false, fresh.ResetAt},
		{"older snapshot", &fresh, old, false, fresh.ResetAt},
		{"source change rebaselines", &old, UpstreamAPIKeyResetSnapshot{Source: "kimi", ResetAt: fresh.ResetAt, ObservedAt: now}, false, fresh.ResetAt},
		{"small forward correction", &old, UpstreamAPIKeyResetSnapshot{Source: "codex", ResetAt: old.ResetAt.Add(time.Hour), ObservedAt: now}, false, old.ResetAt.Add(time.Hour)},
		{"old reset time", &fresh, UpstreamAPIKeyResetSnapshot{Source: "codex", ResetAt: fresh.ResetAt.Add(-time.Hour), ObservedAt: now.Add(time.Second)}, false, fresh.ResetAt},
		{"future observation ignored", &old, UpstreamAPIKeyResetSnapshot{Source: "codex", ResetAt: fresh.ResetAt, ObservedAt: now.Add(time.Hour)}, false, old.ResetAt},
		{"unreasonable far future ignored", &old, UpstreamAPIKeyResetSnapshot{Source: "codex", ResetAt: now.Add(30 * 24 * time.Hour), ObservedAt: now}, false, old.ResetAt},
	} {
		t.Run(tc.name, func(t *testing.T) {
			next, reset := AdvanceUpstreamAPIKeyReset(tc.previous, tc.current, now)
			require.Equal(t, tc.reset, reset)
			require.Equal(t, tc.wantResetAt, next.ResetAt)
		})
	}
	t.Run("early reset requires usage drop not time drift", func(t *testing.T) {
		before := old
		before.ResetAt = now.Add(2 * 24 * time.Hour)
		withoutDrop := fresh
		withoutDrop.Used = &used90
		next, reset := AdvanceUpstreamAPIKeyReset(&before, withoutDrop, now)
		require.False(t, reset)
		require.Equal(t, before.ResetAt, next.ResetAt)
		_, reset = AdvanceUpstreamAPIKeyReset(&before, fresh, now)
		require.True(t, reset)
		before.Used = nil
		_, reset = AdvanceUpstreamAPIKeyReset(&before, fresh, now)
		require.False(t, reset)
	})
	t.Run("same cycle observes latest usage for later early reset", func(t *testing.T) {
		before := fresh
		before.Used = &used10
		current := fresh
		current.ObservedAt = now.Add(time.Second)
		current.Used = &used90
		next, reset := AdvanceUpstreamAPIKeyReset(&before, current, now.Add(time.Second))
		require.False(t, reset)
		require.Equal(t, used90, *next.Used)
	})
}

func TestUpstreamAPIKeyResetObservation(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	reset := now.Add(3 * 24 * time.Hour)
	for _, tc := range []struct {
		name, platform, accountType, source string
		credentials, extra                  map[string]any
	}{
		{"codex", PlatformOpenAI, AccountTypeOAuth, "codex", nil, map[string]any{"codex_7d_reset_at": reset.Format(time.RFC3339), "codex_usage_updated_at": now.Format(time.RFC3339), "codex_7d_used_percent": 90}},
		{"anthropic", PlatformAnthropic, AccountTypeOAuth, "anthropic", nil, map[string]any{"passive_usage_7d_reset": reset.Unix(), "passive_usage_sampled_at": now.Format(time.RFC3339), "passive_usage_7d_utilization": 0.9}},
		{"kimi", PlatformKimi, AccountTypeAPIKey, PlatformKimi, map[string]any{"account_mode": "coding", "base_url": "https://api.kimi.com/coding/v1"}, map[string]any{"kimi_weekly_reset_at": reset.Format(time.RFC3339), "kimi_usage_updated_at": now.Format(time.RFC3339), "kimi_weekly_used_percent": 90}},
		{"grok", PlatformGrok, AccountTypeOAuth, "grok", nil, map[string]any{grokBillingExtraKey: map[string]any{"period_type": "weekly", "period_end": reset.Format(time.RFC3339), "weekly_updated_at": now.Format(time.RFC3339), "usage_percent": 90}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			account := &Account{Platform: tc.platform, Type: tc.accountType, Extra: tc.extra, Credentials: tc.credentials}
			got, ok := UpstreamAPIKeyResetObservation(account)
			require.True(t, ok)
			require.Equal(t, tc.source, got.Source)
			require.True(t, reset.Equal(got.ResetAt))
			require.Equal(t, now, got.ObservedAt)
			require.NotNil(t, got.Used)
			require.Equal(t, 90.0, *got.Used)
			parent := int64(1)
			account.ParentAccountID = &parent
			_, ok = UpstreamAPIKeyResetObservation(account)
			require.False(t, ok)
		})
	}
	for _, extra := range []map[string]any{
		{"quota_weekly_reset_at": reset.Format(time.RFC3339)},
		{"codex_7d_reset_at": reset.Format(time.RFC3339)},
		{grokBillingExtraKey: map[string]any{"period_type": "monthly", "period_end": reset.Format(time.RFC3339), "usage_percent": 90, "weekly_updated_at": now.Format(time.RFC3339)}},
	} {
		_, ok := UpstreamAPIKeyResetObservation(&Account{Platform: PlatformGrok, Type: AccountTypeOAuth, Extra: extra})
		require.False(t, ok)
	}
}

func TestUpstreamAPIKeyResetConfigurationAndManagedSnapshots(t *testing.T) {
	for _, value := range []any{"true", 1, nil, 0.0} {
		_, err := normalizeUpstreamAPIKeyResetExtra(map[string]any{AutoResetAPIKeysOnUpstreamResetExtraKey: value})
		require.Error(t, err)
	}
	for _, extra := range []map[string]any{nil, {}, {AutoResetAPIKeysOnUpstreamResetExtraKey: true}, {AutoResetAPIKeysOnUpstreamResetExtraKey: false}} {
		_, err := normalizeUpstreamAPIKeyResetExtra(extra)
		require.NoError(t, err)
	}
	extra := map[string]any{"codex_7d_reset_at": "forged", "kimi_weekly_reset_at": "forged", "passive_usage_7d_reset": 1, "quota_weekly_reset_at": "local", "quota_weekly_limit": 50, AutoResetAPIKeysOnUpstreamResetExtraKey: true}
	copy, err := duplicateAccountExtra(extra)
	require.NoError(t, err)
	require.NotContains(t, copy, "codex_7d_reset_at")
	require.NotContains(t, copy, "kimi_weekly_reset_at")
	require.NotContains(t, copy, "passive_usage_7d_reset")
	require.False(t, IsUpstreamAPIKeyResetSnapshotKey("quota_weekly_reset_at"))
	require.Equal(t, 50.0, copy["quota_weekly_limit"])
	require.Contains(t, extra, "codex_7d_reset_at")
}

func TestUpstreamAPIKeyResetAdminEntryPoints(t *testing.T) {
	ctx := context.Background()
	repo := &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{1: {
		ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive,
		Extra: map[string]any{"codex_7d_reset_at": "database", "codex_7d_used_percent": 90.0},
	}}}
	svc := &adminServiceImpl{accountRepo: repo}
	_, err := svc.CreateAccount(ctx, &CreateAccountInput{Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
		SkipDefaultGroupBind: true, Extra: map[string]any{AutoResetAPIKeysOnUpstreamResetExtraKey: "true"}})
	require.Error(t, err)
	_, err = svc.UpdateAccount(ctx, 1, &UpdateAccountInput{Extra: map[string]any{AutoResetAPIKeysOnUpstreamResetExtraKey: 1}})
	require.Error(t, err)
	updated, err := svc.UpdateAccount(ctx, 1, &UpdateAccountInput{Extra: map[string]any{
		AutoResetAPIKeysOnUpstreamResetExtraKey: true, "codex_7d_reset_at": "forged", "codex_7d_used_percent": 0,
	}})
	require.NoError(t, err)
	require.Equal(t, true, updated.Extra[AutoResetAPIKeysOnUpstreamResetExtraKey])
	require.Equal(t, "database", updated.Extra["codex_7d_reset_at"])
	require.Equal(t, 90.0, updated.Extra["codex_7d_used_percent"])
	updated, err = svc.UpdateAccount(ctx, 1, &UpdateAccountInput{Extra: map[string]any{
		AutoResetAPIKeysOnUpstreamResetExtraKey: false, "custom": "kept",
	}})
	require.NoError(t, err)
	require.Equal(t, false, updated.Extra[AutoResetAPIKeysOnUpstreamResetExtraKey])
	require.Equal(t, "kept", updated.Extra["custom"])
	updated, err = svc.UpdateAccount(ctx, 1, &UpdateAccountInput{Name: "without-extra"})
	require.NoError(t, err)
	require.Equal(t, false, updated.Extra[AutoResetAPIKeysOnUpstreamResetExtraKey])
	require.Equal(t, "kept", updated.Extra["custom"])
	created, err := svc.CreateAccount(ctx, &CreateAccountInput{Name: "new", Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
		SkipDefaultGroupBind: true, Extra: map[string]any{AutoResetAPIKeysOnUpstreamResetExtraKey: true, "codex_7d_reset_at": "injected"}})
	require.NoError(t, err)
	require.Equal(t, true, created.Extra[AutoResetAPIKeysOnUpstreamResetExtraKey])
	require.NotContains(t, created.Extra, "codex_7d_reset_at")
	created, err = svc.CreateAccount(ctx, &CreateAccountInput{Name: "default-off", Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
		SkipDefaultGroupBind: true})
	require.NoError(t, err)
	enabled, _ := created.Extra[AutoResetAPIKeysOnUpstreamResetExtraKey].(bool)
	require.False(t, enabled)
}

type upstreamResetGroupAdminRepo struct {
	*upstreamBillingProbeAccountRepo
	binds int
}

func (r *upstreamResetGroupAdminRepo) BindGroups(context.Context, int64, []int64) error {
	r.binds++
	return nil
}

type upstreamResetGroupRepo struct{ GroupRepository }

func (*upstreamResetGroupRepo) GetByID(_ context.Context, id int64) (*Group, error) {
	return &Group{ID: id}, nil
}

func TestUpstreamAPIKeyResetUnchangedGroupsOnActualAdminSave(t *testing.T) {
	for _, tc := range []struct {
		name      string
		enabled   bool
		groups    []int64
		wantBinds int
	}{
		{"same groups preserves baseline", true, []int64{7, 8}, 0},
		{"changed priority still rebinds", true, []int64{8, 7}, 1},
		{"changed membership still rebinds", true, []int64{7}, 1},
		{"default off unchanged behavior", false, []int64{7, 8}, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &upstreamResetGroupAdminRepo{upstreamBillingProbeAccountRepo: &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{1: {
				ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive,
				Extra:         map[string]any{AutoResetAPIKeysOnUpstreamResetExtraKey: tc.enabled},
				AccountGroups: []AccountGroup{{GroupID: 8, Priority: 2}, {GroupID: 7, Priority: 1}},
			}}}}
			svc := &adminServiceImpl{accountRepo: repo, groupRepo: &upstreamResetGroupRepo{}}
			_, err := svc.UpdateAccount(context.Background(), 1, &UpdateAccountInput{Name: "ordinary-save", GroupIDs: &tc.groups, SkipMixedChannelCheck: true})
			require.NoError(t, err)
			require.Equal(t, tc.wantBinds, repo.binds)
		})
	}
}

type upstreamResetRepoStub struct {
	calls atomic.Int64
	fail  bool
}

func (r *upstreamResetRepoStub) ListEnabledAccountIDs(context.Context) ([]int64, error) {
	return []int64{1, 2}, nil
}
func (r *upstreamResetRepoStub) ProcessAccount(context.Context, int64, time.Time) (UpstreamAPIKeyResetResult, error) {
	r.calls.Add(1)
	if r.fail {
		return UpstreamAPIKeyResetResult{}, errors.New("db failure")
	}
	return UpstreamAPIKeyResetResult{SkippedGroups: 1}, nil
}
func TestUpstreamAPIKeyResetServiceLifecycleAndRetry(t *testing.T) {
	repo := &upstreamResetRepoStub{fail: true}
	svc := NewUpstreamAPIKeyResetService(repo)
	svc.scan(context.Background())
	require.Equal(t, int64(2), repo.calls.Load())
	svc.scan(context.Background())
	require.Equal(t, int64(4), repo.calls.Load())
	svc.Start()
	require.Eventually(t, func() bool { return repo.calls.Load() >= 6 }, time.Second, time.Millisecond)
	svc.Stop()
	svc.Stop()
	require.ErrorIs(t, svc.ctx.Err(), context.Canceled)
}

type upstreamResetBillingStub struct {
	err error
	ids []int64
}

func (s *upstreamResetBillingStub) InvalidateAPIKeyRateLimit(_ context.Context, id int64) error {
	s.ids = append(s.ids, id)
	return s.err
}
func TestUpstreamAPIKeyResetCacheOutboxRetriesBillingAndSafetyPass(t *testing.T) {
	repo := &authInvalidationRepoStub{}
	cache := &authInvalidationCacheStub{}
	billing := &upstreamResetBillingStub{err: errors.New("redis unavailable")}
	worker := NewAuthCacheInvalidationWorker(repo, cache)
	worker.billing = billing
	event := AuthCacheInvalidationEvent{ID: 1, CacheKey: "hash", APIKeyID: 42}
	worker.processEvent(context.Background(), event)
	require.Equal(t, []int64{1}, repo.retried)
	require.Empty(t, repo.scheduled)
	billing.err = nil
	worker.processEvent(context.Background(), event)
	require.Equal(t, []int64{1}, repo.scheduled)
	event.Stage = 1
	worker.processEvent(context.Background(), event)
	require.Equal(t, []int64{1}, repo.deleted)
	require.Equal(t, []int64{42, 42, 42}, billing.ids)
	require.Len(t, cache.published, 3)
}
