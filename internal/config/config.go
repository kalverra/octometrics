// Package config provides configuration for the application.
package config

import (
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

// Config is the configuration for the application.
type Config struct {
	LogLevel string `mapstructure:"log_level"`
}

const (
	// DefaultLogLevel is the default log level.
	DefaultLogLevel = "info"
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
		return v.BindPFlags(flags)
	}
}

// Load loads configuration from file, env vars, and optionally flags.
func Load(opts ...LoadOption) (*Config, error) {
	v := viper.New()

	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath(".")

	v.SetDefault("log_level", DefaultLogLevel)

	for _, opt := range opts {
		if err := opt(v); err != nil {
			return nil, err
		}
	}

	if err := v.ReadInConfig(); err != nil {
		return nil, err
	}

	cfg := &Config{}
	if err := v.Unmarshal(cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}
