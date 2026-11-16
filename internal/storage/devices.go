package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"wakepilot/internal/device"
)

// DeviceRepo implements device.Repository on SQLite.
type DeviceRepo struct{ db *sql.DB }

func NewDeviceRepo(db *sql.DB) *DeviceRepo { return &DeviceRepo{db: db} }

var _ device.Repository = (*DeviceRepo)(nil)

const deviceCols = `id, name, mac, ip, broadcast, port, interface, description, created_at, updated_at`

type scanner interface{ Scan(dest ...any) error }

func scanDevice(s scanner) (device.Device, error) {
	var d device.Device
	var created, updated string
	if err := s.Scan(&d.ID, &d.Name, &d.MAC, &d.IP, &d.Broadcast, &d.Port, &d.Interface, &d.Description, &created, &updated); err != nil {
		return d, err
	}
	var err error
	if d.CreatedAt, err = parseTime(created); err != nil {
		return d, err
	}
	d.UpdatedAt, err = parseTime(updated)
	return d, err
}

func isUnique(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}

func (r *DeviceRepo) Create(ctx context.Context, d device.Device) (device.Device, error) {
	now := time.Now().UTC()
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO devices (name, mac, ip, broadcast, port, interface, description, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		d.Name, d.MAC, d.IP, d.Broadcast, d.Port, d.Interface, d.Description, formatTime(now), formatTime(now))
	if err != nil {
		if isUnique(err) {
			return device.Device{}, device.ErrDuplicateMAC
		}
		return device.Device{}, fmt.Errorf("insert device: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return device.Device{}, err
	}
	return r.Get(ctx, id)
}

func (r *DeviceRepo) Get(ctx context.Context, id int64) (device.Device, error) {
	d, err := scanDevice(r.db.QueryRowContext(ctx, `SELECT `+deviceCols+` FROM devices WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return d, device.ErrNotFound
	}
	if err != nil {
		return d, fmt.Errorf("get device: %w", err)
	}
	return d, nil
}

func (r *DeviceRepo) List(ctx context.Context) ([]device.Device, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+deviceCols+` FROM devices ORDER BY name COLLATE NOCASE, id`)
	if err != nil {
		return nil, fmt.Errorf("list devices: %w", err)
	}
	defer rows.Close()
	out := []device.Device{}
	for rows.Next() {
		d, err := scanDevice(rows)
		if err != nil {
			return nil, fmt.Errorf("scan device: %w", err)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (r *DeviceRepo) Update(ctx context.Context, d device.Device) (device.Device, error) {
	res, err := r.db.ExecContext(ctx,
		`UPDATE devices SET name=?, mac=?, ip=?, broadcast=?, port=?, interface=?, description=?, updated_at=? WHERE id=?`,
		d.Name, d.MAC, d.IP, d.Broadcast, d.Port, d.Interface, d.Description, formatTime(time.Now()), d.ID)
	if err != nil {
		if isUnique(err) {
			return device.Device{}, device.ErrDuplicateMAC
		}
		return device.Device{}, fmt.Errorf("update device: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return device.Device{}, device.ErrNotFound
	}
	return r.Get(ctx, d.ID)
}

func (r *DeviceRepo) Delete(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM devices WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete device: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return device.ErrNotFound
	}
	return nil
}
