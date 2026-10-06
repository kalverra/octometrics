package testhelpers

import (
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
)

//nolint:paralleltest // TestSetup verifies sequential cleanup after subtest finishes
func TestSetup(t *testing.T) {
	var (
		log     zerolog.Logger
		testDir string
	)

	t.Run("test dir and logs are deleted on success", func(t *testing.T) {
		log, testDir = Setup(t, Silent())
		log.Info().Msg("test")
		assert.DirExists(t, testDir)
	})

	assert.NoDirExists(t, testDir, "test dir should not still exist after a successful test")

	// This will mark the whole package as failed, which is not what we want. Not sure how to deal with this.
	// Not worth the effort unless you really want to get into it.
	// t.Run("test dir and log file are kept on failure", func(t *testing.T) {
	// 	log, testDir = Setup(t, Silent())
	// 	log.Info().Msg("test")
	// 	assert.FileExists(t, filepath.Join(testDir, testLogFile))
	// 	t.Fail()
	// })

	// assert.DirExists(t, testDir, "test dir should still exist after a failed test")
	// assert.FileExists(t, filepath.Join(testDir, testLogFile), "test log file should still exist after a failed test")
}
