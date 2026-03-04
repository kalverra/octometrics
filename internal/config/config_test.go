package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLoad(t *testing.T) {
	t.Parallel()

	cfg, err := Load(WithConfigFile("testdata/config.valid.yaml"))
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}
	assert.Equal(t, "debug", cfg.LogLevel)
}
