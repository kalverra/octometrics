package config

import (
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_ConfigFile(t *testing.T) {
	t.Parallel()

	cfg, err := Load(WithConfigFile("testdata/config.valid.yaml"))
	require.NoError(t, err, "failed to load config")
	assert.Equal(t, "debug", cfg.LogLevel)
}

func TestLoad_Env(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "test-token")
	cfg, err := Load()
	require.NoError(t, err, "failed to load config")
	assert.Equal(t, "test-token", cfg.GitHubToken)
}

func TestLoad_Flags(t *testing.T) {
	t.Parallel()

	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	flags.String("log-level", "debug", "log level")
	// simulate parsed flag by setting its value
	err := flags.Set("log-level", "debug")
	require.NoError(t, err, "failed to set flag")
	cfg, err := Load(WithFlags(flags))
	require.NoError(t, err, "failed to load config")
	assert.Equal(t, "debug", cfg.LogLevel)
}

func TestLoad_FlagsOverrideEnv(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "test-token")
	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	flags.String("github-token", "test-token-2", "GitHub token")
	// simulate parsed flag by setting its value
	err := flags.Set("github-token", "test-token-2")
	require.NoError(t, err, "failed to set flag")
	cfg, err := Load(WithFlags(flags))
	require.NoError(t, err, "failed to load config")
	assert.Equal(t, "test-token-2", cfg.GitHubToken)
}
