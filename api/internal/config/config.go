package config

import (
	"fmt"
	"os"
	"reflect"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config holds all configuration for the API server.
type Config struct {
	HA         HomeAssistantConfig `yaml:"ha"`
	Weather    WeatherConfig       `yaml:"weather"`
	Calendar   CalendarConfig      `yaml:"calendar"`
	Departures DeparturesConfig    `yaml:"departures"`
	Refresh    RefreshConfig       `yaml:"refresh"`
}

type HomeAssistantConfig struct {
	BaseURL string `yaml:"baseUrl" env:"HA_BASE_URL"`
	Token   string `yaml:"token"   env:"HA_TOKEN"`
}

type WeatherConfig struct {
	EntityID string `yaml:"entityId" env:"WEATHER_ENTITY"`
}

type CalendarConfig struct {
	EntityIDs []string `yaml:"entityIds"`
}

type DeparturesConfig struct {
	EntityIDs []string `yaml:"entityIds"`
}

// RefreshConfig bounds the delay the image response asks the device to wait
// before its next fetch. Within the bounds, the delay runs until the next
// moment a dashboard section changes.
type RefreshConfig struct {
	// Min and Max bound the delay outside quiet hours.
	Min time.Duration `yaml:"min"`
	Max time.Duration `yaml:"max"`
	// Retry is the delay after a section failed to load.
	Retry      time.Duration    `yaml:"retry"`
	QuietHours QuietHoursConfig `yaml:"quietHours"`
}

// QuietHoursConfig is a daily window, in the server's local time zone, with
// its own delay bounds and an hourly clock. Start and End are "15:04"; a
// window may cross midnight. Leaving Start or End empty disables it.
type QuietHoursConfig struct {
	Start string        `yaml:"start"`
	End   string        `yaml:"end"`
	Min   time.Duration `yaml:"min"`
	Max   time.Duration `yaml:"max"`
}

// Enabled reports whether a quiet hours window is configured.
func (q QuietHoursConfig) Enabled() bool { return q.Start != "" && q.End != "" }

// Defaults for unset RefreshConfig fields. The device clamps the delay to
// its own bounds as well.
const (
	defaultRefreshMin   = 2 * time.Minute
	defaultRefreshMax   = time.Hour
	defaultRefreshRetry = 2 * time.Minute
	defaultQuietMin     = time.Hour
	defaultQuietMax     = 4 * time.Hour
)

const envPrefix = "TINYDASH_"

// Load reads configuration from a YAML file (if it exists) and applies
// environment variable overrides. Env vars use the prefix TINYDASH_ followed
// by the env tag value (e.g. TINYDASH_HA_BASE_URL).
func Load() (*Config, error) {
	cfg := &Config{}

	configPath := os.Getenv("CONFIG_PATH")
	if configPath == "" {
		configPath = "config.yaml"
	}

	data, err := os.ReadFile(configPath)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("reading config file %s: %w", configPath, err)
	}
	if err == nil {
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("parsing config file %s: %w", configPath, err)
		}
	}

	applyEnvOverrides(cfg)

	if err := cfg.Refresh.applyDefaults(); err != nil {
		return nil, fmt.Errorf("config refresh: %w", err)
	}

	return cfg, nil
}

func (r *RefreshConfig) applyDefaults() error {
	setDefault(&r.Min, defaultRefreshMin)
	setDefault(&r.Max, defaultRefreshMax)
	setDefault(&r.Retry, defaultRefreshRetry)
	setDefault(&r.QuietHours.Min, defaultQuietMin)
	setDefault(&r.QuietHours.Max, defaultQuietMax)
	if r.Min > r.Max {
		return fmt.Errorf("min %v exceeds max %v", r.Min, r.Max)
	}
	q := r.QuietHours
	if q.Start == "" && q.End == "" {
		return nil
	}
	if q.Min > q.Max {
		return fmt.Errorf("quietHours: min %v exceeds max %v", q.Min, q.Max)
	}
	for _, v := range []string{q.Start, q.End} {
		if _, err := time.Parse("15:04", v); err != nil {
			return fmt.Errorf("quietHours: start and end must both be HH:MM, got %q", v)
		}
	}
	return nil
}

func setDefault(d *time.Duration, def time.Duration) {
	if *d <= 0 {
		*d = def
	}
}

// applyEnvOverrides walks the config struct and overrides any field that has
// an `env` tag with the corresponding TINYDASH_-prefixed environment variable.
func applyEnvOverrides(cfg *Config) {
	v := reflect.ValueOf(cfg).Elem()
	applyEnvToStruct(v)
}

func applyEnvToStruct(v reflect.Value) {
	t := v.Type()
	for i := range t.NumField() {
		field := v.Field(i)
		fieldType := t.Field(i)

		if field.Kind() == reflect.Struct {
			applyEnvToStruct(field)
			continue
		}

		envTag := fieldType.Tag.Get("env")
		if envTag == "" {
			continue
		}

		envKey := envPrefix + strings.ToUpper(envTag)
		if val, ok := os.LookupEnv(envKey); ok && field.CanSet() && field.Kind() == reflect.String {
			field.SetString(val)
		}
	}
}
