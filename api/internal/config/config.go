package config

import (
	"fmt"
	"os"
	"reflect"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config holds all configuration for the API server.
type Config struct {
	HA         HomeAssistantConfig `yaml:"ha"`
	Weather    WeatherConfig       `yaml:"weather"`
	Calendar   CalendarConfig      `yaml:"calendar"`
	Departures DeparturesConfig    `yaml:"departures"`
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

	return cfg, nil
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
