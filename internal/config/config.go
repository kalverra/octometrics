// Package config provides configuration for the application.
package config

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"time"

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
	Since             time.Time `mapstructure:"since"`
	Until             time.Time `mapstructure:"until"`
	GatherCost        bool      `mapstructure:"gather_cost"`
	DataDir           string    `mapstructure:"data_dir"`
}

const (
	// DefaultLogLevel is the default log level.
	DefaultLogLevel = "silent"
	// DefaultDataDir is the default data directory.
	DefaultDataDir = "data"
)

var (
	// DefaultSince is the default start date for survey.
	DefaultSince = time.Now().AddDate(0, 0, -7)
	// DefaultUntil is the default end date for survey.
	DefaultUntil = time.Now()
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
	v.SetDefault("since", DefaultSince)
	v.SetDefault("until", DefaultUntil)
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
	if err := v.Unmarshal(cfg); err != nil {
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
	if setCount > 1 {
		return errors.New("only one of commit SHA, workflow run ID or pull request number can be provided")
	}
	if setCount == 0 {
		return errors.New("one of commit SHA, workflow run ID or pull request number must be provided")
	}

	if c.GitHubToken == "" {
		fmt.Println("WARNING:GitHub token not provided, will likely hit rate limits quickly")
	}

	return nil
}

// ValidateSurvey validates the configuration for the survey command.
func (c *Config) ValidateSurvey() error {
	if c.Owner == "" {
		return errors.New("owner is required")
	}
	if c.Repo == "" {
		return errors.New("repo is required")
	}

	validEvents := []string{"all", "pull_request", "merge_group", "push"}
	if !slices.Contains(validEvents, c.Event) {
		return fmt.Errorf("invalid event %q: must be one of %s", c.Event, strings.Join(validEvents, ", "))
	}

	if c.Since.IsZero() {
		return errors.New("since is required")
	}
	if c.Until.IsZero() {
		return errors.New("until is required")
	}

	if c.Since.After(c.Until) {
		return errors.New("since must be before until")
	}

	return nil
}
