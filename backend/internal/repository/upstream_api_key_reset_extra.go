package repository

import (
	"context"
	"encoding/json"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// Caller already holds accounts' row lock. Preserve only the fields consumed by
// the reset observer; local quota settings are deliberately not managed here.
func preserveUpstreamAPIKeyResetSnapshots(ctx context.Context, client *dbent.Client, id int64, incoming map[string]any) (map[string]any, error) {
	rows, err := client.QueryContext(ctx, `SELECT extra FROM accounts WHERE id = $1 AND deleted_at IS NULL`, id)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, service.ErrAccountNotFound
	}
	var raw []byte
	if err := rows.Scan(&raw); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	var current map[string]any
	if err := json.Unmarshal(raw, &current); err != nil {
		return nil, err
	}
	for key := range incoming {
		if service.IsUpstreamAPIKeyResetSnapshotKey(key) {
			delete(incoming, key)
		}
	}
	for key, value := range current {
		if service.IsUpstreamAPIKeyResetSnapshotKey(key) {
			incoming[key] = value
		}
	}
	return incoming, nil
}
