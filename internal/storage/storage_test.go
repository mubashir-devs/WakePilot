package storage

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"wakepilot/internal/device"
	"wakepilot/migrations"
)

func newDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := Migrate(context.Background(), db, migrations.FS); err != nil {
		t.Fatal(err)
	}
	return db
}

func sample(mac string) device.Device {
	return device.Device{Name: "NAS", MAC: mac, IP: "192.168.1.5", Broadcast: "192.168.1.255", Port: 9, Interface: "eth0", Description: "d"}
}

func TestMigrateIsIdempotent(t *testing.T) {
	db := newDB(t)
	if err := Migrate(context.Background(), db, migrations.FS); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&n)
	if n != 1 {
		t.Errorf("schema_migrations rows = %d, want 1", n)
	}
}

func TestDeviceCRUD(t *testing.T) {
	ctx := context.Background()
	repo := NewDeviceRepo(newDB(t))

	created, err := repo.Create(ctx, sample("aa:bb:cc:dd:ee:01"))
	if err != nil {
		t.Fatal(err)
	}
	if created.ID == 0 || created.CreatedAt.IsZero() {
		t.Fatalf("bad created device: %+v", created)
	}

	got, err := repo.Get(ctx, created.ID)
	if err != nil || got.Name != "NAS" || got.Broadcast != "192.168.1.255" {
		t.Fatalf("Get = %+v, %v", got, err)
	}

	got.Name = "Renamed"
	got.Port = 7
	upd, err := repo.Update(ctx, got)
	if err != nil || upd.Name != "Renamed" || upd.Port != 7 {
		t.Fatalf("Update = %+v, %v", upd, err)
	}
	if !upd.UpdatedAt.After(created.UpdatedAt) && !upd.UpdatedAt.Equal(created.UpdatedAt) {
		t.Error("updated_at went backwards")
	}

	second, _ := repo.Create(ctx, device.Device{Name: "alpha", MAC: "aa:bb:cc:dd:ee:02", Port: 9})
	list, err := repo.List(ctx)
	if err != nil || len(list) != 2 || list[0].ID != second.ID {
		t.Fatalf("List (sorted by name) = %+v, %v", list, err)
	}

	if err := repo.Delete(ctx, created.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Get(ctx, created.ID); !errors.Is(err, device.ErrNotFound) {
		t.Errorf("Get after delete: %v", err)
	}
	if err := repo.Delete(ctx, created.ID); !errors.Is(err, device.ErrNotFound) {
		t.Errorf("second delete: %v", err)
	}
	if _, err := repo.Update(ctx, device.Device{ID: 999, MAC: "aa:bb:cc:dd:ee:09", Name: "x", Port: 9}); !errors.Is(err, device.ErrNotFound) {
		t.Errorf("update missing: %v", err)
	}
}

func TestDeviceDuplicateMAC(t *testing.T) {
	ctx := context.Background()
	repo := NewDeviceRepo(newDB(t))
	if _, err := repo.Create(ctx, sample("aa:bb:cc:dd:ee:01")); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Create(ctx, sample("aa:bb:cc:dd:ee:01")); !errors.Is(err, device.ErrDuplicateMAC) {
		t.Errorf("duplicate create: %v", err)
	}
	other, _ := repo.Create(ctx, sample("aa:bb:cc:dd:ee:02"))
	other.MAC = "aa:bb:cc:dd:ee:01"
	if _, err := repo.Update(ctx, other); !errors.Is(err, device.ErrDuplicateMAC) {
		t.Errorf("duplicate update: %v", err)
	}
}

func TestHistory(t *testing.T) {
	ctx := context.Background()
	db := newDB(t)
	devs, hist := NewDeviceRepo(db), NewHistoryRepo(db)
	d, _ := devs.Create(ctx, sample("aa:bb:cc:dd:ee:01"))

	base := time.Now().Add(-time.Hour)
	for i := 0; i < 3; i++ {
		_, err := hist.Add(ctx, device.WakeRecord{
			DeviceID: &d.ID, DeviceName: d.Name, MAC: d.MAC, Broadcast: "192.168.1.255", Port: 9,
			Status: device.StatusSent, Reachability: device.ReachPending, RequestedAt: base.Add(time.Duration(i) * time.Minute),
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	failed, _ := hist.Add(ctx, device.WakeRecord{DeviceName: "gone", MAC: "aa:bb:cc:dd:ee:99", Status: device.StatusFailed, Error: "boom", Reachability: device.ReachNotChecked})

	recs, total, err := hist.List(ctx, device.HistoryFilter{Limit: 2})
	if err != nil || total != 4 || len(recs) != 2 || recs[0].ID != failed.ID {
		t.Fatalf("List = %d recs total %d, %v (first %+v)", len(recs), total, err, recs)
	}
	if recs[0].DeviceID != nil || recs[0].Error != "boom" {
		t.Errorf("failed record fields wrong: %+v", recs[0])
	}

	filtered, total, _ := hist.List(ctx, device.HistoryFilter{DeviceID: &d.ID, Limit: 10})
	if total != 3 || len(filtered) != 3 || !filtered[0].RequestedAt.After(filtered[2].RequestedAt) {
		t.Errorf("filtered = %d total %d", len(filtered), total)
	}

	if err := hist.UpdateReachability(ctx, filtered[0].ID, device.ReachReachable, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := hist.ExpirePending(ctx); err != nil {
		t.Fatal(err)
	}
	after, _, _ := hist.List(ctx, device.HistoryFilter{DeviceID: &d.ID, Limit: 10})
	if after[0].Reachability != device.ReachReachable || after[0].ReachabilityCheckedAt == nil || after[1].Reachability != device.ReachUnknown {
		t.Errorf("reachability after expire: %+v", after)
	}

	// Deleting the device keeps history (device_id becomes NULL).
	if err := devs.Delete(ctx, d.ID); err != nil {
		t.Fatal(err)
	}
	_, total, _ = hist.List(ctx, device.HistoryFilter{Limit: 10})
	if total != 4 {
		t.Errorf("history lost on device delete: total=%d", total)
	}
}
