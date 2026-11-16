// Package config loads and validates the YAML application configuration.
package config

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Auth     AuthConfig     `yaml:"auth"`
	Database DatabaseConfig `yaml:"database"`
	WoL      WoLConfig      `yaml:"wol"`
	Monitor  MonitorConfig  `yaml:"monitor"`
	Logging  LoggingConfig  `yaml:"logging"`
}

type ServerConfig struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
	// AllowedHosts restricts the Host header (DNS-rebinding protection). When
	// empty and the server is bound to loopback, only localhost names are allowed.
	AllowedHosts []string `yaml:"allowed_hosts"`
}

type AuthConfig struct {
	Enabled bool `yaml:"enabled"`
	// Token is the bearer token. Prefer the WAKEPILOT_AUTH_TOKEN env var.
	Token string `yaml:"token"`
}

type DatabaseConfig struct {
	Path string `yaml:"path"`
}

type WoLConfig struct {
	DefaultPort      int           `yaml:"default_port"`
	DefaultInterface string        `yaml:"default_interface"`
	SendTimeout      time.Duration `yaml:"send_timeout"`
	VerifyEnabled    bool          `yaml:"verify_enabled"`
	VerifyTimeout    time.Duration `yaml:"verify_timeout"`
	VerifyInterval   time.Duration `yaml:"verify_interval"`
}

type MonitorConfig struct {
	Enabled      bool          `yaml:"enabled"`
	Interval     time.Duration `yaml:"interval"`
	ProbeTimeout time.Duration `yaml:"probe_timeout"`
	Ports        []int         `yaml:"ports"`
}

type LoggingConfig struct {
	Level  string `yaml:"level"`  // debug, info, warn, error
	Format string `yaml:"format"` // text, json
}

// Default returns a safe configuration: loopback only, no auth required.
func Default() Config {
	return Config{
		Server:   ServerConfig{Host: "127.0.0.1", Port: 8080},
		Database: DatabaseConfig{Path: "wakepilot.db"},
		WoL: WoLConfig{
			DefaultPort:    9,
			SendTimeout:    5 * time.Second,
			VerifyEnabled:  true,
			VerifyTimeout:  90 * time.Second,
			VerifyInterval: 3 * time.Second,
		},
		Monitor: MonitorConfig{
			Enabled:      true,
			Interval:     30 * time.Second,
			ProbeTimeout: 1500 * time.Millisecond,
			Ports:        []int{22, 80, 135, 139, 443, 445, 3389, 5900},
		},
		Logging: LoggingConfig{Level: "info", Format: "text"},
	}
}

// Load reads the YAML file at path. A missing file is only an error when
// required is true; otherwise defaults are used. Environment overrides:
// WAKEPILOT_AUTH_TOKEN, WAKEPILOT_DB.
func Load(path string, required bool) (Config, error) {
	cfg := Default()
	data, err := os.ReadFile(path)
	switch {
	case err == nil:
		dec := yaml.NewDecoder(strings.NewReader(string(data)))
		dec.KnownFields(true)
		if err := dec.Decode(&cfg); err != nil && !errors.Is(err, io.EOF) {
			return cfg, fmt.Errorf("parse %s: %w", path, err)
		}
	case errors.Is(err, os.ErrNotExist) && !required:
	default:
		return cfg, fmt.Errorf("read config: %w", err)
	}
	if v := os.Getenv("WAKEPILOT_AUTH_TOKEN"); v != "" {
		cfg.Auth.Token = v
	}
	if v := os.Getenv("WAKEPILOT_DB"); v != "" {
		cfg.Database.Path = v
	}
	return cfg, cfg.Validate()
}

// Addr is the listen address.
func (s ServerConfig) Addr() string {
	return net.JoinHostPort(s.Host, fmt.Sprint(s.Port))
}

// IsLoopback reports whether the server only accepts local connections.
func (s ServerConfig) IsLoopback() bool { return IsLoopbackHost(s.Host) }

func IsLoopbackHost(h string) bool {
	if strings.EqualFold(h, "localhost") {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}

const minTokenLen = 16

func (c Config) Validate() error {
	var errs []error
	add := func(format string, a ...any) { errs = append(errs, fmt.Errorf(format, a...)) }

	if c.Server.Host == "" {
		add("server.host must not be empty")
	}
	if c.Server.Port < 1 || c.Server.Port > 65535 {
		add("server.port must be 1-65535")
	}
	if !c.Server.IsLoopback() && !c.Auth.Enabled {
		add("server.host %q is not a loopback address: auth.enabled must be true (remote access without authentication is refused)", c.Server.Host)
	}
	if c.Auth.Enabled && len(c.Auth.Token) < minTokenLen {
		add("auth.token (or WAKEPILOT_AUTH_TOKEN) must be at least %d characters when auth is enabled", minTokenLen)
	}
	if c.Database.Path == "" {
		add("database.path must not be empty")
	}
	if c.WoL.DefaultPort < 1 || c.WoL.DefaultPort > 65535 {
		add("wol.default_port must be 1-65535")
	}
	if c.WoL.SendTimeout <= 0 {
		add("wol.send_timeout must be positive")
	}
	if c.WoL.VerifyEnabled && (c.WoL.VerifyInterval <= 0 || c.WoL.VerifyTimeout < c.WoL.VerifyInterval) {
		add("wol.verify_interval must be positive and not exceed wol.verify_timeout")
	}
	if c.Monitor.Enabled && (c.Monitor.Interval <= 0 || c.Monitor.ProbeTimeout <= 0) {
		add("monitor.interval and monitor.probe_timeout must be positive")
	}
	if len(c.Monitor.Ports) == 0 {
		add("monitor.ports must list at least one TCP port")
	}
	for _, p := range c.Monitor.Ports {
		if p < 1 || p > 65535 {
			add("monitor.ports contains invalid port %d", p)
		}
	}
	switch strings.ToLower(c.Logging.Level) {
	case "debug", "info", "warn", "error":
	default:
		add("logging.level must be debug, info, warn or error")
	}
	switch strings.ToLower(c.Logging.Format) {
	case "text", "json":
	default:
		add("logging.format must be text or json")
	}
	return errors.Join(errs...)
}
