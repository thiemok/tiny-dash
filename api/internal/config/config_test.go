package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func loadYAML(t *testing.T, yaml string) (*Config, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CONFIG_PATH", path)
	return Load()
}

func TestRefreshDefaults(t *testing.T) {
	cfg, err := loadYAML(t, "ha:\n  baseUrl: http://ha.example.com\n")
	if err != nil {
		t.Fatal(err)
	}
	want := RefreshConfig{
		Min: 2 * time.Minute, Max: time.Hour, Retry: 2 * time.Minute,
		QuietHours: QuietHoursConfig{Min: time.Hour, Max: 4 * time.Hour},
	}
	if cfg.Refresh != want {
		t.Fatalf("Refresh = %+v, want %+v", cfg.Refresh, want)
	}
	if cfg.Refresh.QuietHours.Enabled() {
		t.Fatal("quiet hours enabled without a window")
	}
}

func TestRefreshFromYAML(t *testing.T) {
	cfg, err := loadYAML(t, `
refresh:
  min: 3m
  max: 30m
  quietHours:
    start: "22:30"
    end: "06:00"
    max: 2h
`)
	if err != nil {
		t.Fatal(err)
	}
	r := cfg.Refresh
	if r.Min != 3*time.Minute || r.Max != 30*time.Minute || r.Retry != 2*time.Minute {
		t.Fatalf("bounds = %v %v %v", r.Min, r.Max, r.Retry)
	}
	q := r.QuietHours
	if !q.Enabled() || q.Start != "22:30" || q.End != "06:00" || q.Min != time.Hour || q.Max != 2*time.Hour {
		t.Fatalf("quietHours = %+v", q)
	}
}

func TestRefreshInvalid(t *testing.T) {
	tests := []struct {
		name, yaml, wantErr string
	}{
		{"min above max", "refresh:\n  min: 2h\n  max: 1h\n", "exceeds max"},
		{"quiet min above max", "refresh:\n  quietHours:\n    start: \"23:00\"\n    end: \"06:00\"\n    min: 5h\n", "exceeds max"},
		{"malformed time", "refresh:\n  quietHours:\n    start: \"11pm\"\n    end: \"06:00\"\n", "HH:MM"},
		{"only start", "refresh:\n  quietHours:\n    start: \"23:00\"\n", "HH:MM"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := loadYAML(t, tt.yaml)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Load error = %v, want one containing %q", err, tt.wantErr)
			}
		})
	}
}
