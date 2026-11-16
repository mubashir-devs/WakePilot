package storage

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"wakepilot/internal/device"
)

// HistoryRepo implements device.HistoryRepository on SQLite.
type HistoryRepo struct{ db *sql.DB }

func NewHistoryRepo(db *sql.DB) *HistoryRepo { return &HistoryRepo{db: db} }

var _ device.HistoryRepository = (*HistoryRepo)(nil)

const historyCols = `id, device_id, device_name, mac, broadcast, port, interface, status, error, reachability, reachability_checked_at, requested_at`

func (r *HistoryRepo) Add(ctx context.Context, rec device.WakeRecord) (device.WakeRecord, error) {
	if rec.RequestedAt.IsZero() {
		rec.RequestedAt = time.Now()
	}
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO wake_history (device_id, device_name, mac, broadcast, port, interface, status, error, reachability, requested_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		rec.DeviceID, rec.DeviceName, rec.MAC, rec.Broadcast, rec.Port, rec.Interface, rec.Status, rec.Error, rec.Reachability, formatTime(rec.RequestedAt))
	if err != nil {
		return rec, fmt.Errorf("insert wake record: %w", err)
	}
	rec.ID, err = res.LastInsertId()
	rec.RequestedAt = rec.RequestedAt.UTC()
	return rec, err
}

func (r *HistoryRepo) UpdateReachability(ctx context.Context, id int64, reachability string, at time.Time) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE wake_history SET reachability = ?, reachability_checked_at = ? WHERE id = ?`,
		reachability, formatTime(at), id)
	if err != nil {
		return fmt.Errorf("update wake record: %w", err)
	}
	return nil
}

func (r *HistoryRepo) ExpirePending(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, `UPDATE wake_history SET reachability = 'unknown' WHERE reachability = 'pending'`)
	return err
}

func (r *HistoryRepo) List(ctx context.Context, f device.HistoryFilter) ([]device.WakeRecord, int, error) {
	where, args := "", []any{}
	if f.DeviceID != nil {
		where = " WHERE device_id = ?"
		args = append(args, *f.DeviceID)
	}
	var total int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM wake_history`+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count wake history: %w", err)
	}
	limit := f.Limit
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	args = append(args, limit, max(f.Offset, 0))
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+historyCols+` FROM wake_history`+where+` ORDER BY requested_at DESC, id DESC LIMIT ? OFFSET ?`, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list wake history: %w", err)
	}
	defer rows.Close()
	out := []device.WakeRecord{}
	for rows.Next() {
		var rec device.WakeRecord
		var devID sql.NullInt64
		var checked sql.NullString
		var requested string
		if err := rows.Scan(&rec.ID, &devID, &rec.DeviceName, &rec.MAC, &rec.Broadcast, &rec.Port, &rec.Interface,
			&rec.Status, &rec.Error, &rec.Reachability, &checked, &requested); err != nil {
			return nil, 0, fmt.Errorf("scan wake record: %w", err)
		}
		if devID.Valid {
			rec.DeviceID = &devID.Int64
		}
		if checked.Valid {
			t, err := parseTime(checked.String)
			if err != nil {
				return nil, 0, err
			}
			rec.ReachabilityCheckedAt = &t
		}
		if rec.RequestedAt, err = parseTime(requested); err != nil {
			return nil, 0, err
		}
		out = append(out, rec)
	}
	return out, total, rows.Err()
}
