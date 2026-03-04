package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad(t *testing.T) {
	t.Parallel()

	cfg, err := Load(WithConfigFile("testdata/config.valid.yaml"))
	require.NoError(t, err, "failed to load config")
	assert.Equal(t, "debug", cfg.LogLevel)
}
