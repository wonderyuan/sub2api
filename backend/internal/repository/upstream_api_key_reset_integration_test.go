//go:build integration

package repository

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

type upstreamResetFixture struct {
	client                            *dbent.Client
	accountID, groupID, keyID, userID int64
	now                               time.Time
}

func newUpstreamResetFixture(t *testing.T) upstreamResetFixture {
	t.Helper()
	ctx := context.Background()
	client := testEntClient(t)
	suffix := fmt.Sprint(time.Now().UnixNano())
	u, err := client.User.Create().SetEmail("upstream-reset-" + suffix + "@test.invalid").SetPasswordHash("test").Save(ctx)
	require.NoError(t, err)
	g, err := client.Group.Create().SetName("upstream-reset-" + suffix).SetPlatform(service.PlatformOpenAI).Save(ctx)
	require.NoError(t, err)
	now := time.Now().UTC().Truncate(time.Second)
	a, err := client.Account.Create().SetName("upstream-reset-" + suffix).SetPlatform(service.PlatformOpenAI).SetType(service.AccountTypeOAuth).
		SetCredentials(map[string]any{"chatgpt_account_id": "stable-id", "access_token": "test-only"}).
		SetExtra(map[string]any{service.AutoResetAPIKeysOnUpstreamResetExtraKey: true, "codex_7d_reset_at": now.Add(time.Minute).Format(time.RFC3339), "codex_usage_updated_at": now.Add(-time.Minute).Format(time.RFC3339), "codex_7d_used_percent": 90}).Save(ctx)
	require.NoError(t, err)
	_, err = client.AccountGroup.Create().SetAccountID(a.ID).SetGroupID(g.ID).Save(ctx)
	require.NoError(t, err)
	k, err := client.APIKey.Create().SetUserID(u.ID).SetGroupID(g.ID).SetKey("test-upstream-reset-" + suffix).SetName("reset-test").
		SetQuota(120).SetQuotaUsed(35).SetUsage5h(2).SetUsage1d(4).SetUsage7d(8).
		SetWindow7dStart(now.Add(-service.RateLimitWindow7d)).SetExpiresAt(now.Add(24 * time.Hour)).SetStatus(service.StatusAPIKeyQuotaExhausted).Save(ctx)
	require.NoError(t, err)
	f := upstreamResetFixture{client: client, accountID: a.ID, groupID: g.ID, keyID: k.ID, userID: u.ID, now: now}
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(ctx, `DELETE FROM api_keys WHERE id = $1`, k.ID)
		_, _ = integrationDB.ExecContext(ctx, `DELETE FROM accounts WHERE id = $1`, a.ID)
		_, _ = integrationDB.ExecContext(ctx, `DELETE FROM groups WHERE id = $1`, g.ID)
		_, _ = integrationDB.ExecContext(ctx, `DELETE FROM users WHERE id = $1`, u.ID)
		_, _ = integrationDB.ExecContext(ctx, `DELETE FROM auth_cache_invalidation_outbox WHERE api_key_id = $1`, k.ID)
	})
	return f
}

func (f upstreamResetFixture) baseline(t *testing.T) {
	t.Helper()
	repo := NewUpstreamAPIKeyResetRepository(integrationDB)
	result, err := repo.ProcessAccount(context.Background(), f.accountID, f.now)
	require.NoError(t, err)
	require.Zero(t, result.ResetKeys)
	var count int
	require.NoError(t, integrationDB.QueryRow(`SELECT COUNT(*) FROM upstream_api_key_reset_baselines WHERE account_id=$1`, f.accountID).Scan(&count))
	require.Equal(t, 1, count)
}
func (f upstreamResetFixture) advance(t *testing.T) {
	t.Helper()
	_, err := integrationDB.Exec(`UPDATE accounts SET extra = extra || jsonb_build_object('codex_7d_reset_at',$2::text,'codex_usage_updated_at',$3::text,'codex_7d_used_percent',0) WHERE id=$1`, f.accountID, f.now.Add(service.RateLimitWindow7d).Format(time.RFC3339), f.now.Format(time.RFC3339))
	require.NoError(t, err)
}

func TestUpstreamAPIKeyResetIntegrationAtomicIdempotentAndNarrow(t *testing.T) {
	f := newUpstreamResetFixture(t)
	f.baseline(t)
	f.advance(t)
	repo := NewUpstreamAPIKeyResetRepository(integrationDB)
	var wg sync.WaitGroup
	errs := make([]error, 4)
	results := make([]service.UpstreamAPIKeyResetResult, 4)
	for i := range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i], errs[i] = repo.ProcessAccount(context.Background(), f.accountID, f.now)
		}()
	}
	wg.Wait()
	resetKeys := 0
	for i := range errs {
		require.NoError(t, errs[i])
		resetKeys += results[i].ResetKeys
	}
	require.Equal(t, 1, resetKeys)
	got, err := f.client.APIKey.Get(context.Background(), f.keyID)
	require.NoError(t, err)
	require.Zero(t, got.Usage7d)
	require.True(t, f.now.Equal(*got.Window7dStart))
	require.Equal(t, 35.0, got.QuotaUsed)
	require.Equal(t, 120.0, got.Quota)
	require.Equal(t, 2.0, got.Usage5h)
	require.Equal(t, 4.0, got.Usage1d)
	require.Equal(t, service.StatusAPIKeyQuotaExhausted, got.Status)
	require.True(t, f.now.Add(24*time.Hour).Equal(*got.ExpiresAt))
	var count int
	require.NoError(t, integrationDB.QueryRow(`SELECT COUNT(*) FROM auth_cache_invalidation_outbox WHERE api_key_id=$1`, f.keyID).Scan(&count))
	require.Equal(t, 1, count)
	_, err = integrationDB.Exec(`UPDATE api_keys SET usage_7d=3 WHERE id=$1`, f.keyID)
	require.NoError(t, err)
	result, err := repo.ProcessAccount(context.Background(), f.accountID, f.now)
	require.NoError(t, err)
	require.Zero(t, result.ResetKeys)
	got, err = f.client.APIKey.Get(context.Background(), f.keyID)
	require.NoError(t, err)
	require.Equal(t, 3.0, got.Usage7d)
}

func TestUpstreamAPIKeyResetIntegrationPreservesLazyNewCycleAndRetriesLocks(t *testing.T) {
	f := newUpstreamResetFixture(t)
	f.baseline(t)
	f.advance(t)
	_, err := integrationDB.Exec(`UPDATE api_keys SET usage_7d=3, window_7d_start=$2 WHERE id=$1`, f.keyID, f.now.Add(time.Second))
	require.NoError(t, err)
	tx, err := integrationDB.BeginTx(context.Background(), nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	var id int64
	require.NoError(t, tx.QueryRow(`SELECT id FROM api_keys WHERE id=$1 FOR UPDATE`, f.keyID).Scan(&id))
	repo := NewUpstreamAPIKeyResetRepository(integrationDB)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err = repo.ProcessAccount(ctx, f.accountID, f.now)
	require.Error(t, err, "NOWAIT must roll back, not wait holding configuration gate")
	require.NoError(t, tx.Rollback())
	result, err := repo.ProcessAccount(context.Background(), f.accountID, f.now)
	require.NoError(t, err)
	require.Equal(t, 1, result.ResetKeys)
	got, err := f.client.APIKey.Get(context.Background(), f.keyID)
	require.NoError(t, err)
	require.Equal(t, 3.0, got.Usage7d, "new cycle usage must not be erased by the later scanner")
	require.True(t, f.now.Equal(*got.Window7dStart))
}

func TestUpstreamAPIKeyResetIntegrationAmbiguousSourcesRequireFreshBaseline(t *testing.T) {
	f := newUpstreamResetFixture(t)
	f.baseline(t)
	ctx := context.Background()
	other, err := f.client.Account.Create().SetName(fmt.Sprint("second-reset-source-", time.Now().UnixNano())).
		SetPlatform(service.PlatformOpenAI).SetType(service.AccountTypeOAuth).
		SetExtra(map[string]any{service.AutoResetAPIKeysOnUpstreamResetExtraKey: true}).Save(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = integrationDB.Exec(`DELETE FROM accounts WHERE id=$1`, other.ID) })
	_, err = f.client.AccountGroup.Create().SetAccountID(other.ID).SetGroupID(f.groupID).Save(ctx)
	require.NoError(t, err)
	f.advance(t)
	repo := NewUpstreamAPIKeyResetRepository(integrationDB)
	result, err := repo.ProcessAccount(ctx, f.accountID, f.now)
	require.NoError(t, err)
	require.Equal(t, 1, result.SkippedGroups)
	require.Zero(t, result.ResetKeys)
	_, err = integrationDB.Exec(`UPDATE accounts SET extra='{}'::jsonb WHERE id=$1`, other.ID)
	require.NoError(t, err)
	result, err = repo.ProcessAccount(ctx, f.accountID, f.now)
	require.NoError(t, err)
	require.Zero(t, result.ResetKeys, "ambiguity resolution must rebaseline, not replay a historical reset")
	got, err := f.client.APIKey.Get(ctx, f.keyID)
	require.NoError(t, err)
	require.Equal(t, 8.0, got.Usage7d)
}

func TestUpstreamAPIKeyResetIntegrationEnableEpochAndIdentity(t *testing.T) {
	f := newUpstreamResetFixture(t)
	count := func() int {
		var n int
		require.NoError(t, integrationDB.QueryRow(`SELECT COUNT(*) FROM upstream_api_key_reset_baselines WHERE account_id=$1`, f.accountID).Scan(&n))
		return n
	}
	f.baseline(t)
	_, err := integrationDB.Exec(`UPDATE accounts SET extra=extra || '{"quota_weekly_reset_at":"local-window","custom":"kept"}'::jsonb,
		credentials=credentials || '{"access_token":"rotated","refresh_token":"rotated","expires_at":123,"model_mapping":{"a":"b"}}'::jsonb WHERE id=$1`, f.accountID)
	require.NoError(t, err)
	require.Equal(t, 1, count(), "token rotation must not erase baseline")
	beforeIdentityChange, err := f.client.Account.Get(context.Background(), f.accountID)
	require.NoError(t, err)
	require.Contains(t, beforeIdentityChange.Extra, "codex_usage_updated_at", "token rotation must retain the upstream observation")
	_, err = integrationDB.Exec(`UPDATE accounts SET credentials=credentials || '{"chatgpt_account_id":"different"}'::jsonb WHERE id=$1`, f.accountID)
	require.NoError(t, err)
	require.Zero(t, count(), "actual upstream identity replacement must rebaseline")
	afterIdentityChange, err := f.client.Account.Get(context.Background(), f.accountID)
	require.NoError(t, err)
	for _, key := range []string{"codex_7d_reset_at", "codex_7d_used_percent", "codex_usage_updated_at"} {
		require.NotContains(t, afterIdentityChange.Extra, key, "old identity observations must not survive replacement")
	}
	require.Equal(t, "local-window", afterIdentityChange.Extra["quota_weekly_reset_at"])
	require.Equal(t, "kept", afterIdentityChange.Extra["custom"])
	repo := NewUpstreamAPIKeyResetRepository(integrationDB)
	result, err := repo.ProcessAccount(context.Background(), f.accountID, f.now)
	require.NoError(t, err)
	require.Zero(t, result.ResetKeys)
	require.Zero(t, count(), "scanner must wait for the new identity's first real observation")
	// Relative to the previous identity this is a >24h advance and usage drop.
	// It must only establish a baseline, never clear the group's existing usage.
	f.advance(t)
	f.baseline(t)
	keyAfterFirstObservation, err := f.client.APIKey.Get(context.Background(), f.keyID)
	require.NoError(t, err)
	require.Equal(t, 8.0, keyAfterFirstObservation.Usage7d)
	tx, err := integrationDB.BeginTx(context.Background(), nil)
	require.NoError(t, err)
	for _, enabled := range []bool{false, true} {
		_, err = tx.Exec(`UPDATE accounts SET extra=jsonb_set(extra,'{auto_reset_api_keys_on_upstream_reset}',to_jsonb($2::boolean)) WHERE id=$1`, f.accountID, enabled)
		require.NoError(t, err)
	}
	require.NoError(t, tx.Commit())
	require.Zero(t, count(), "off-on between scan ticks must clear baseline")
	afterReenable, err := f.client.Account.Get(context.Background(), f.accountID)
	require.NoError(t, err)
	require.Equal(t, f.now.Format(time.RFC3339), afterReenable.Extra["codex_usage_updated_at"], "ordinary off-on must retain valid observations")
	f.advance(t)
	result, err = NewUpstreamAPIKeyResetRepository(integrationDB).ProcessAccount(context.Background(), f.accountID, f.now)
	require.NoError(t, err)
	require.Zero(t, result.ResetKeys, "reenable only baselines current cycle")
	_, err = integrationDB.Exec(`DELETE FROM account_groups WHERE account_id=$1`, f.accountID)
	require.NoError(t, err)
	require.Zero(t, count(), "membership change must clear baseline")
	result, err = NewUpstreamAPIKeyResetRepository(integrationDB).ProcessAccount(context.Background(), f.accountID, f.now)
	require.NoError(t, err)
	require.Zero(t, result.ResetKeys, "ungrouped account must never touch ungrouped keys")
}

func TestUpstreamAPIKeyResetIntegrationBoundsImplicitForeignKeyLocks(t *testing.T) {
	f := newUpstreamResetFixture(t)
	ctx := context.Background()
	tx, err := integrationDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	var id int64
	require.NoError(t, tx.QueryRow(`SELECT id FROM groups WHERE id=$1 FOR UPDATE`, f.groupID).Scan(&id))
	repo := NewUpstreamAPIKeyResetRepository(integrationDB)
	bounded, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	_, err = repo.ProcessAccount(bounded, f.accountID, f.now)
	require.Error(t, err, "baseline foreign-key check must time out rather than block the configuration gate")
	require.NoError(t, bounded.Err(), "database lock_timeout should fire before the outer deadline")
	require.NoError(t, tx.Rollback())
	f.baseline(t)
}
