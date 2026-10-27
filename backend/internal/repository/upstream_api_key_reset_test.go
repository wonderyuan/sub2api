package repository

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/migrations"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
)

func TestUpstreamAPIKeyResetRepositoryTransaction(t *testing.T) {
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	reset := now.Add(service.RateLimitWindow7d - time.Minute)
	extra, _ := json.Marshal(map[string]any{service.AutoResetAPIKeysOnUpstreamResetExtraKey: true, "codex_7d_reset_at": reset.Format(time.RFC3339), "codex_usage_updated_at": now.Format(time.RFC3339), "codex_7d_used_percent": 10})
	for _, tc := range []struct {
		name     string
		baseline bool
		sources  int
		fail     string
	}{
		{"first observation", false, 1, ""},
		{"reset atomically with outbox", true, 1, ""},
		{"ambiguous group drops baseline", true, 2, ""},
		{"key contention rolls back", true, 1, "lock"},
		{"outbox insertion failure rolls back", true, 1, "outbox"},
		{"baseline write failure rolls back reset and outbox", true, 1, "baseline"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer func() { _ = db.Close() }()
			mock.ExpectBegin()
			mock.ExpectQuery(regexp.QuoteMeta("SELECT pg_try_advisory_xact_lock(1937072737, 7)")).WillReturnRows(sqlmock.NewRows([]string{"locked"}).AddRow(true))
			mock.ExpectExec(regexp.QuoteMeta("SET LOCAL lock_timeout = '500ms'")).WillReturnResult(sqlmock.NewResult(0, 0))
			mock.ExpectQuery("(?s)SELECT platform, type, extra, credentials FROM accounts.*extra -> 'auto_reset_api_keys_on_upstream_reset'").WithArgs(int64(1)).WillReturnRows(sqlmock.NewRows([]string{"platform", "type", "extra", "credentials"}).AddRow("openai", "oauth", extra, []byte(`{}`)))
			groups := sqlmock.NewRows([]string{"group_id", "sources", "source", "reset_at", "observed_at", "used_percent"})
			if tc.baseline {
				groups.AddRow(int64(7), tc.sources, "codex", now.Add(-time.Minute), now.Add(-time.Hour), 90)
			} else {
				groups.AddRow(int64(7), tc.sources, nil, nil, nil, nil)
			}
			mock.ExpectQuery("(?s)SELECT ag.group_id.*LEFT JOIN upstream_api_key_reset_baselines").WithArgs(int64(1)).WillReturnRows(groups)
			if tc.sources > 1 {
				mock.ExpectExec("DELETE FROM upstream_api_key_reset_baselines").WithArgs(int64(1), int64(7)).WillReturnResult(sqlmock.NewResult(0, 1))
			} else {
				if tc.baseline {
					lock := mock.ExpectQuery("(?s)SELECT id FROM api_keys.*ORDER BY id FOR UPDATE NOWAIT").WithArgs(int64(7))
					if tc.fail == "lock" {
						lock.WillReturnError(errors.New("lock not available"))
					} else {
						lock.WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(42))
						update := mock.ExpectQuery("(?s)WITH changed AS.*usage_7d = CASE.*window_7d_start < \\$2.*id = ANY.*INSERT INTO auth_cache_invalidation_outbox \\(cache_key, api_key_id\\).*sha256").WithArgs(int64(7), reset.Add(-service.RateLimitWindow7d), "{42}")
						if tc.fail == "outbox" {
							update.WillReturnError(errors.New("outbox failed"))
						} else {
							update.WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
						}
					}
				}
				if tc.fail != "lock" && tc.fail != "outbox" {
					baseline := mock.ExpectExec("INSERT INTO upstream_api_key_reset_baselines").WithArgs(int64(1), int64(7), "codex", reset, now, 10.0)
					if tc.fail == "baseline" {
						baseline.WillReturnError(errors.New("write failed"))
					} else {
						baseline.WillReturnResult(sqlmock.NewResult(0, 1))
					}
				}
			}
			if tc.fail != "" {
				mock.ExpectRollback()
			} else {
				mock.ExpectCommit()
			}
			result, err := NewUpstreamAPIKeyResetRepository(db).ProcessAccount(context.Background(), 1, now)
			if tc.fail != "" {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				if tc.sources > 1 {
					require.Equal(t, 1, result.SkippedGroups)
				} else if tc.baseline {
					require.Equal(t, 1, result.ResetKeys)
				} else {
					require.Zero(t, result.ResetGroups)
				}
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestUpstreamAPIKeyResetRepositoryBusyGateAndUngrouped(t *testing.T) {
	for _, busy := range []bool{true, false} {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		mock.ExpectBegin()
		mock.ExpectQuery("SELECT pg_try_advisory_xact_lock").WillReturnRows(sqlmock.NewRows([]string{"locked"}).AddRow(!busy))
		if !busy {
			mock.ExpectExec(regexp.QuoteMeta("SET LOCAL lock_timeout = '500ms'")).WillReturnResult(sqlmock.NewResult(0, 0))
			now := time.Now().UTC()
			extra, _ := json.Marshal(map[string]any{"codex_7d_reset_at": now.Add(time.Hour).Format(time.RFC3339), "codex_usage_updated_at": now.Format(time.RFC3339)})
			mock.ExpectQuery("SELECT platform, type, extra, credentials FROM accounts").WithArgs(int64(1)).WillReturnRows(sqlmock.NewRows([]string{"platform", "type", "extra", "credentials"}).AddRow("openai", "oauth", extra, []byte(`{}`)))
			mock.ExpectQuery("SELECT ag.group_id").WithArgs(int64(1)).WillReturnRows(sqlmock.NewRows([]string{"group_id", "sources", "source", "reset_at", "observed_at", "used_percent"}))
			mock.ExpectCommit()
		} else {
			mock.ExpectRollback()
		}
		result, err := NewUpstreamAPIKeyResetRepository(db).ProcessAccount(context.Background(), 1, time.Now())
		require.NoError(t, err)
		require.Equal(t, service.UpstreamAPIKeyResetResult{}, result)
		require.NoError(t, mock.ExpectationsWereMet())
		_ = db.Close()
	}
}

func TestPreserveUpstreamAPIKeyResetSnapshots(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	mock.ExpectQuery("SELECT extra FROM accounts").WithArgs(int64(1)).WillReturnRows(sqlmock.NewRows([]string{"extra"}).AddRow([]byte(`{"codex_7d_reset_at":"database","codex_7d_used_percent":90,"kimi_weekly_reset_at":"database-cn","quota_weekly_reset_at":"old-local"}`)))
	got, err := preserveUpstreamAPIKeyResetSnapshots(context.Background(), client, 1, map[string]any{"codex_7d_reset_at": "forged", "passive_usage_7d_reset": 123, "quota_weekly_reset_at": "new-local"})
	require.NoError(t, err)
	require.Equal(t, "database", got["codex_7d_reset_at"])
	require.Equal(t, "database-cn", got["kimi_weekly_reset_at"])
	require.NotContains(t, got, "passive_usage_7d_reset")
	require.Equal(t, "new-local", got["quota_weekly_reset_at"])
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUpstreamAPIKeyResetMigrationBoundaries(t *testing.T) {
	body, err := migrations.FS.ReadFile("249_upstream_api_key_7d_reset.sql")
	require.NoError(t, err)
	text := string(body)
	for _, required := range []string{"PRIMARY KEY (account_id, group_id)", "ADD COLUMN IF NOT EXISTS api_key_id", "pg_advisory_xact_lock(1937072737, 7)", "'auto_reset_api_keys_on_upstream_reset'", "'chatgpt_account_id'", "'api_key'", "AFTER INSERT OR UPDATE OF account_id, group_id OR DELETE"} {
		require.Contains(t, text, required)
	}
	for _, forbidden := range []string{"'access_token'", "'refresh_token'", "quota_used =", "quota_limit =", "usage_5h =", "usage_1d =", "expires_at ="} {
		require.NotContains(t, text, forbidden)
	}
}
