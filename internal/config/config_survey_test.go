package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestValidateSurvey(t *testing.T) {
	t.Parallel()

	t.Run("valid config", func(t *testing.T) {
		t.Parallel()
		cfg := &Config{
			Owner:       "test-owner",
			Repo:        "test-repo",
			Event:       "pull_request",
			Since:       time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			Until:       time.Date(2025, 1, 7, 0, 0, 0, 0, time.UTC),
			GitHubToken: "token",
		}
		assert.NoError(t, cfg.ValidateSurvey())
	})

	t.Run("missing owner", func(t *testing.T) {
		t.Parallel()
		cfg := &Config{Repo: "test-repo", GitHubToken: "token"}
		assert.ErrorContains(t, cfg.ValidateSurvey(), "owner is required")
	})

	t.Run("missing repo", func(t *testing.T) {
		t.Parallel()
		cfg := &Config{Owner: "test-owner", GitHubToken: "token"}
		assert.ErrorContains(t, cfg.ValidateSurvey(), "repo is required")
	})

	t.Run("invalid event", func(t *testing.T) {
		t.Parallel()
		cfg := &Config{Owner: "o", Repo: "r", Event: "invalid", GitHubToken: "token"}
		assert.ErrorContains(t, cfg.ValidateSurvey(), "invalid event")
	})

	t.Run("valid events", func(t *testing.T) {
		t.Parallel()
		for _, event := range []string{"all", "pull_request", "merge_group", "push"} {
			cfg := &Config{
				Owner:       "o",
				Repo:        "r",
				Event:       event,
				GitHubToken: "token",
				Since:       time.Now().AddDate(0, 0, -1),
				Until:       time.Now().AddDate(0, 0, 1),
			}
			assert.NoError(t, cfg.ValidateSurvey(), "event %q should be valid", event)
		}
	})

	t.Run("invalid since date", func(t *testing.T) {
		t.Parallel()
		cfg := &Config{Owner: "o", Repo: "r", Until: time.Now().AddDate(0, 0, -1), GitHubToken: "token", Event: "all"}
		assert.ErrorContains(t, cfg.ValidateSurvey(), "since is required")
	})

	t.Run("invalid until date", func(t *testing.T) {
		t.Parallel()
		cfg := &Config{Owner: "o", Repo: "r", Since: time.Now().AddDate(0, 0, 1), GitHubToken: "token", Event: "all"}
		assert.ErrorContains(t, cfg.ValidateSurvey(), "until is required")
	})

	t.Run("since after until", func(t *testing.T) {
		t.Parallel()
		cfg := &Config{
			Owner:       "o",
			Repo:        "r",
			Since:       time.Date(2025, 1, 7, 0, 0, 0, 0, time.UTC),
			Until:       time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			GitHubToken: "token",
			Event:       "all",
		}
		assert.ErrorContains(t, cfg.ValidateSurvey(), "since must be before until")
	})
}
