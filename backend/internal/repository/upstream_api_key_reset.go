package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

type upstreamAPIKeyResetRepository struct{ db *sql.DB }

func NewUpstreamAPIKeyResetRepository(db *sql.DB) service.UpstreamAPIKeyResetRepository {
	return &upstreamAPIKeyResetRepository{db: db}
}

func (r *upstreamAPIKeyResetRepository) ListEnabledAccountIDs(ctx context.Context) ([]int64, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id FROM accounts
		WHERE deleted_at IS NULL AND parent_account_id IS NULL
		AND extra -> 'auto_reset_api_keys_on_upstream_reset' = 'true'::jsonb ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (r *upstreamAPIKeyResetRepository) ProcessAccount(ctx context.Context, id int64, now time.Time) (result service.UpstreamAPIKeyResetResult, err error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return result, err
	}
	defer func() { _ = tx.Rollback() }()
	// Do not lock accounts: billing writes key then account, so taking an
	// account lock before resetting keys would invert the hot-path lock order.
	// Configuration/membership triggers serialize through this same gate.
	var locked bool
	if err := tx.QueryRowContext(ctx, `SELECT pg_try_advisory_xact_lock(1937072737, 7)`).Scan(&locked); err != nil {
		return result, err
	}
	if !locked {
		return result, nil
	}
	// Foreign-key checks can wait on account/group deletion locks even though
	// key locks use NOWAIT. Bound those implicit waits and retry the whole cycle.
	if _, err := tx.ExecContext(ctx, `SET LOCAL lock_timeout = '500ms'`); err != nil {
		return result, err
	}
	var account service.Account
	var extra, credentials []byte
	account.ID = id
	err = tx.QueryRowContext(ctx, `SELECT platform, type, extra, credentials FROM accounts
		WHERE id = $1 AND deleted_at IS NULL AND parent_account_id IS NULL
		AND extra -> 'auto_reset_api_keys_on_upstream_reset' = 'true'::jsonb`, id).
		Scan(&account.Platform, &account.Type, &extra, &credentials)
	if errors.Is(err, sql.ErrNoRows) {
		return result, nil
	}
	if err != nil {
		return result, err
	}
	if err := json.Unmarshal(extra, &account.Extra); err != nil {
		return result, err
	}
	if len(credentials) > 0 {
		if err := json.Unmarshal(credentials, &account.Credentials); err != nil {
			return result, err
		}
	}
	current, ok := service.UpstreamAPIKeyResetObservation(&account)
	if !ok {
		return result, nil
	}
	// Configuration/membership triggers use the same advisory lock. Other
	// accounts are read without row locks, so no account-lock inversion occurs.
	rows, err := tx.QueryContext(ctx, `SELECT ag.group_id,
		(SELECT COUNT(*) FROM account_groups members JOIN accounts a ON a.id = members.account_id
		 WHERE members.group_id = ag.group_id AND a.deleted_at IS NULL AND a.parent_account_id IS NULL
		 AND a.extra -> 'auto_reset_api_keys_on_upstream_reset' = 'true'::jsonb),
		b.source, b.reset_at, b.observed_at, b.used_percent
		FROM account_groups ag JOIN groups g ON g.id = ag.group_id AND g.deleted_at IS NULL
		LEFT JOIN upstream_api_key_reset_baselines b ON b.account_id = ag.account_id AND b.group_id = ag.group_id
		WHERE ag.account_id = $1 ORDER BY ag.group_id`, id)
	if err != nil {
		return result, err
	}
	type groupBaseline struct {
		id       int64
		sources  int
		previous *service.UpstreamAPIKeyResetSnapshot
	}
	var groups []groupBaseline
	for rows.Next() {
		var group groupBaseline
		var source sql.NullString
		var resetAt, observedAt sql.NullTime
		var used sql.NullFloat64
		if err := rows.Scan(&group.id, &group.sources, &source, &resetAt, &observedAt, &used); err != nil {
			_ = rows.Close()
			return result, err
		}
		if source.Valid && resetAt.Valid && observedAt.Valid {
			group.previous = &service.UpstreamAPIKeyResetSnapshot{Source: source.String, ResetAt: resetAt.Time, ObservedAt: observedAt.Time}
			if used.Valid {
				group.previous.Used = &used.Float64
			}
		}
		groups = append(groups, group)
	}
	rowErr := rows.Err()
	_ = rows.Close()
	if rowErr != nil {
		return result, rowErr
	}
	for _, group := range groups {
		if group.sources != 1 {
			result.SkippedGroups++
			// Drop rather than retain an actionable old cycle during ambiguity.
			if _, err := tx.ExecContext(ctx, `DELETE FROM upstream_api_key_reset_baselines WHERE account_id = $1 AND group_id = $2`, id, group.id); err != nil {
				return result, err
			}
			continue
		}
		next, reset := service.AdvanceUpstreamAPIKeyReset(group.previous, current, now)
		if next.Source == "" {
			continue
		}
		if reset {
			// Billing may hold a key while waiting for an admin-held account.
			// Never wait on keys with the configuration gate held: retry instead.
			keyRows, err := tx.QueryContext(ctx, `SELECT id FROM api_keys
				WHERE group_id = $1 AND deleted_at IS NULL ORDER BY id FOR UPDATE NOWAIT`, group.id)
			if err != nil {
				return result, err
			}
			var keyIDs []int64
			for keyRows.Next() {
				var keyID int64
				if err := keyRows.Scan(&keyID); err != nil {
					_ = keyRows.Close()
					return result, err
				}
				keyIDs = append(keyIDs, keyID)
			}
			keyErr := keyRows.Err()
			_ = keyRows.Close()
			if keyErr != nil {
				return result, keyErr
			}
			// All keys in the group, including currently disabled/exhausted keys;
			// no status/quota/expiry/5h/1d column is changed. The CTE enqueues only
			// hashes and IDs, atomically with the reset and baseline advancement.
			var changed int
			err = tx.QueryRowContext(ctx, `WITH changed AS (
				UPDATE api_keys SET usage_7d = CASE
					WHEN window_7d_start IS NULL OR window_7d_start < $2 THEN 0 ELSE usage_7d END,
					window_7d_start = $2, updated_at = NOW()
				WHERE group_id = $1 AND id = ANY($3::bigint[]) AND deleted_at IS NULL RETURNING id, key
			), enqueued AS (
				INSERT INTO auth_cache_invalidation_outbox (cache_key, api_key_id)
				SELECT encode(sha256(convert_to(key, 'UTF8')), 'hex'), id FROM changed RETURNING id
			) SELECT COUNT(*) FROM enqueued`, group.id, next.ResetAt.Add(-service.RateLimitWindow7d), pq.Array(keyIDs)).Scan(&changed)
			if err != nil {
				return result, err
			}
			result.ResetGroups++
			result.ResetKeys += changed
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO upstream_api_key_reset_baselines
			(account_id, group_id, source, reset_at, observed_at, used_percent) VALUES ($1,$2,$3,$4,$5,$6)
			ON CONFLICT (account_id, group_id) DO UPDATE SET source = EXCLUDED.source,
			reset_at = EXCLUDED.reset_at, observed_at = EXCLUDED.observed_at, used_percent = EXCLUDED.used_percent`,
			id, group.id, next.Source, next.ResetAt, next.ObservedAt, next.Used); err != nil {
			return result, err
		}
	}
	if err := tx.Commit(); err != nil {
		return service.UpstreamAPIKeyResetResult{}, err
	}
	return result, nil
}
