// Package config provides configuration for the application.
package config

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/go-viper/mapstructure/v2"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

// Config is the configuration for the application.
type Config struct {
	LogLevel          string    `mapstructure:"log_level"`
	GitHubToken       string    `mapstructure:"github_token"`
	Owner             string    `mapstructure:"owner"`
	Repo              string    `mapstructure:"repo"`
	CommitSHA         string    `mapstructure:"commit_sha"`
	WorkflowRunID     int64     `mapstructure:"workflow_run_id"`
	PullRequestNumber int       `mapstructure:"pull_request_number"`
	ForceUpdate       bool      `mapstructure:"force_update"`
	NoObserve         bool      `mapstructure:"no_observe"`
	Event             string    `mapstructure:"event"`
	From              time.Time `mapstructure:"from"`
	To                time.Time `mapstructure:"to"`
	GatherCost        bool      `mapstructure:"gather_cost"`
	DataDir           string    `mapstructure:"data_dir"`
}

const (
	// DefaultLogLevel is the default log level.
	DefaultLogLevel = "silent"
	// DefaultDataDir is the default data directory.
	DefaultDataDir = "data"
)

// LoadOption is a function that can be used to load configuration.
type LoadOption func(*viper.Viper) error

// WithConfigFile sets a specific config file to load.
func WithConfigFile(path string) LoadOption {
	return func(v *viper.Viper) error {
		v.SetConfigFile(path)
		return nil
	}
}

// WithFlags binds flags to the viper instance.
func WithFlags(flags *pflag.FlagSet) LoadOption {
	return func(v *viper.Viper) error {
		var err error
		flags.VisitAll(func(f *pflag.Flag) {
			configName := strings.ReplaceAll(f.Name, "-", "_")
			if bindErr := v.BindPFlag(configName, f); bindErr != nil && err == nil {
				err = bindErr
			}
		})
		return err
	}
}

// Load loads configuration from file, env vars, and optionally flags.
func Load(opts ...LoadOption) (*Config, error) {
	v := viper.New()

	v.SetDefault("log_level", DefaultLogLevel)
	v.SetDefault("data_dir", DefaultDataDir)

	// Bind all configuration fields to environment variables
	typ := reflect.TypeFor[Config]()
	for field := range typ.Fields() {
		tag := field.Tag.Get("mapstructure")
		if tag != "" {
			if err := v.BindEnv(tag); err != nil {
				return nil, err
			}
		}
	}

	for _, opt := range opts {
		if err := opt(v); err != nil {
			return nil, err
		}
	}

	v.AutomaticEnv()

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok { // If the config file is not found, we don't need to return an error
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}
	}

	cfg := &Config{}
	if err := v.Unmarshal(cfg, viper.DecodeHook(
		mapstructure.ComposeDecodeHookFunc(
			mapstructure.StringToTimeDurationHookFunc(),
			mapstructure.StringToSliceHookFunc(","),
			stringToTimeHookFunc(),
		),
	)); err != nil {
		return nil, err
	}

	return cfg, nil
}

// ValidateGather validates the configuration for the gather command.
func (c *Config) ValidateGather() error {
	if c.Owner == "" {
		return errors.New("owner is required")
	}
	if c.Repo == "" {
		return errors.New("repo is required")
	}

	setCount := 0
	if c.CommitSHA != "" {
		setCount++
	}
	if c.WorkflowRunID != 0 {
		setCount++
	}
	if c.PullRequestNumber != 0 {
		setCount++
	}
	if !c.From.IsZero() || !c.To.IsZero() {
		setCount++
	}

	if setCount > 1 {
		return errors.New(
			"only one of commit SHA, workflow run ID, pull request number, or a time range can be provided",
		)
	}

	if setCount == 0 {
		return errors.New(
			"one of commit SHA, workflow run ID, pull request number, or a time range (--from and --to) must be provided",
		)
	}

	if !c.From.IsZero() && !c.To.IsZero() && c.From.After(c.To) {
		return errors.New("from date must be before to date")
	}

	return nil
}

// ValidateCompare validates the configuration for the compare command.
func (c *Config) ValidateCompare() error {
	if c.Owner == "" {
		return errors.New("owner is required")
	}
	if c.Repo == "" {
		return errors.New("repo is required")
	}
	return nil
}

var timeLayouts = []string{time.RFC3339, "2006-01-02"}

func stringToTimeHookFunc() mapstructure.DecodeHookFuncType {
	return func(f reflect.Type, t reflect.Type, data any) (any, error) {
		if f.Kind() != reflect.String || t != reflect.TypeFor[time.Time]() {
			return data, nil
		}
		str, ok := data.(string)
		if !ok || str == "" {
			return time.Time{}, nil
		}
		for _, layout := range timeLayouts {
			if parsed, err := time.Parse(layout, str); err == nil {
				return parsed, nil
			}
		}
		return nil, fmt.Errorf("unable to parse time %q, expected formats: %v", str, timeLayouts)
	}
}
