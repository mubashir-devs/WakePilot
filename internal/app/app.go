// Package app wires the modules together. Tests reuse it with fake network
// components so the full stack can run without real hardware.
package app

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"sync"

	"wakepilot/internal/api"
	"wakepilot/internal/config"
	"wakepilot/internal/network"
	"wakepilot/internal/service"
	"wakepilot/internal/storage"
	"wakepilot/internal/wol"
	"wakepilot/migrations"
)

// Options override production components (used by tests).
type Options struct {
	Version string
	Sender  wol.Sender
	Lister  network.Lister
	Checker service.Checker
	Static  fs.FS
}

type App struct {
	Handler http.Handler
	db      *sql.DB
	monitor *service.Monitor
	wake    *service.WakeService
	cfg     config.Config

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func New(ctx context.Context, cfg config.Config, log *slog.Logger, opts Options) (*App, error) {
	db, err := storage.Open(cfg.Database.Path)
	if err != nil {
		return nil, err
	}
	if err := storage.Migrate(ctx, db, migrations.FS); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate database: %w", err)
	}

	if opts.Sender == nil {
		opts.Sender = wol.UDPSender{Timeout: cfg.WoL.SendTimeout}
	}
	if opts.Lister == nil {
		opts.Lister = network.SystemLister{}
	}
	if opts.Checker == nil {
		opts.Checker = service.TCPChecker{Ports: cfg.Monitor.Ports, Timeout: cfg.Monitor.ProbeTimeout}
	}
	if opts.Version == "" {
		opts.Version = "dev"
	}

	devices := storage.NewDeviceRepo(db)
	history := storage.NewHistoryRepo(db)
	if err := history.ExpirePending(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("expire pending wake records: %w", err)
	}

	netSvc := network.NewService(opts.Lister, cfg.WoL.DefaultInterface)
	monitor := service.NewMonitor(devices, opts.Checker, cfg.Monitor.Interval, log)
	wakeSvc := service.NewWakeService(devices, history, netSvc, wol.NewEngine(opts.Sender), monitor, service.WakeOptions{
		SendTimeout: cfg.WoL.SendTimeout, VerifyEnabled: cfg.WoL.VerifyEnabled,
		VerifyTimeout: cfg.WoL.VerifyTimeout, VerifyInterval: cfg.WoL.VerifyInterval,
	}, log)
	devSvc := service.NewDeviceService(devices, netSvc, monitor, cfg.WoL.DefaultPort)

	return &App{
		db: db, monitor: monitor, wake: wakeSvc, cfg: cfg,
		Handler: api.New(api.Deps{
			Config: cfg, Version: opts.Version, Devices: devSvc, Wake: wakeSvc,
			Network: netSvc, Log: log, Static: opts.Static,
		}),
	}, nil
}

// Start launches background work (periodic availability monitoring).
func (a *App) Start(ctx context.Context) {
	if !a.cfg.Monitor.Enabled {
		return
	}
	ctx, a.cancel = context.WithCancel(ctx)
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		a.monitor.Run(ctx)
	}()
}

// Close stops background work and closes the database.
func (a *App) Close() error {
	if a.cancel != nil {
		a.cancel()
	}
	a.wg.Wait()
	a.wake.Close()
	return a.db.Close()
}
