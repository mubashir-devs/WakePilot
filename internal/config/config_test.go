package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func write(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "c.yaml")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestDefaultsAreValid(t *testing.T) {
	if err := Default().Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestLoadMissingFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "none.yaml")
	if _, err := Load(p, false); err != nil {
		t.Errorf("optional missing file should yield defaults: %v", err)
	}
	if _, err := Load(p, true); err == nil {
		t.Error("required missing file should fail")
	}
}

func TestLoadOverrides(t *testing.T) {
	t.Setenv("WAKEPILOT_AUTH_TOKEN", "")
	cfg, err := Load(write(t, "server:\n  port: 9090\nwol:\n  default_port: 7\n  send_timeout: 2s\n"), true)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.Port != 9090 || cfg.WoL.DefaultPort != 7 || cfg.WoL.SendTimeout != 2*time.Second || cfg.Server.Host != "127.0.0.1" {
		t.Errorf("unexpected config: %+v", cfg)
	}
}

func TestLoadRejectsUnknownKeys(t *testing.T) {
	if _, err := Load(write(t, "serverr:\n  port: 1\n"), true); err == nil {
		t.Error("unknown key should be rejected")
	}
}

func TestRemoteBindingRequiresAuth(t *testing.T) {
	t.Setenv("WAKEPILOT_AUTH_TOKEN", "")
	_, err := Load(write(t, "server:\n  host: 0.0.0.0\n"), true)
	if err == nil || !strings.Contains(err.Error(), "auth.enabled") {
		t.Fatalf("expected auth requirement error, got %v", err)
	}
	if _, err := Load(write(t, "server:\n  host: 0.0.0.0\nauth:\n  enabled: true\n  token: short\n"), true); err == nil {
		t.Error("short token should be rejected")
	}
	t.Setenv("WAKEPILOT_AUTH_TOKEN", "0123456789abcdef0123")
	if _, err := Load(write(t, "server:\n  host: 0.0.0.0\nauth:\n  enabled: true\n"), true); err != nil {
		t.Errorf("token from env should satisfy validation: %v", err)
	}
}

func TestIsLoopbackHost(t *testing.T) {
	for h, want := range map[string]bool{"127.0.0.1": true, "::1": true, "localhost": true, "0.0.0.0": false, "192.168.1.5": false, "": false, "example.com": false} {
		if IsLoopbackHost(h) != want {
			t.Errorf("IsLoopbackHost(%q) != %v", h, want)
		}
	}
}
