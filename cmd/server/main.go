// Command server runs WakePilot: the Wake-on-LAN API and embedded dashboard.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"wakepilot/frontend"
	"wakepilot/internal/app"
	"wakepilot/internal/config"
)

// version is set at build time: -ldflags "-X main.version=1.0.0".
var version = "dev"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "wakepilot:", err)
		os.Exit(1)
	}
}

func run() error {
	cfgPath := flag.String("config", "", "path to YAML config (default: ./config.yaml if present, else built-in defaults)")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Println("wakepilot", version)
		return nil
	}

	required := *cfgPath != ""
	if !required {
		*cfgPath = "config.yaml"
	}
	cfg, err := config.Load(*cfgPath, required)
	if err != nil {
		return fmt.Errorf("configuration: %w", err)
	}

	log := newLogger(cfg.Logging)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	a, err := app.New(ctx, cfg, log, app.Options{Version: version, Static: frontend.Dist()})
	if err != nil {
		return err
	}
	defer a.Close()
	a.Start(ctx)

	srv := &http.Server{
		Addr:              cfg.Server.Addr(),
		Handler:           a.Handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	if !cfg.Server.IsLoopback() {
		log.Warn("listening on a non-loopback address; bearer-token auth is enforced", "addr", srv.Addr)
	}
	log.Info("WakePilot started", "version", version, "url", "http://"+srv.Addr, "auth", cfg.Auth.Enabled)

	errc := make(chan error, 1)
	go func() { errc <- srv.ListenAndServe() }()
	select {
	case err := <-errc:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
		log.Info("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	}
	return nil
}

func newLogger(c config.LoggingConfig) *slog.Logger {
	var lvl slog.Level
	_ = lvl.UnmarshalText([]byte(strings.ToLower(c.Level)))
	opts := &slog.HandlerOptions{Level: lvl}
	if strings.EqualFold(c.Format, "json") {
		return slog.New(slog.NewJSONHandler(os.Stderr, opts))
	}
	return slog.New(slog.NewTextHandler(os.Stderr, opts))
}
