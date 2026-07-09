package gather

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEstablishPRChecksConclusion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		current string
		new     string
		want    string
	}{
		{name: "failure beats success", current: "success", new: "failure", want: "failure"},
		{name: "success does not beat failure", current: "failure", new: "success", want: "failure"},
		{name: "timed_out beats in_progress", current: "in_progress", new: "timed_out", want: "timed_out"},
		{name: "in_progress beats success", current: "success", new: "in_progress", want: "in_progress"},
		{name: "empty current takes new", current: "", new: "success", want: "success"},
		{name: "unknown status keeps current", current: "success", new: "weird", want: "success"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, establishPRChecksConclusion(tt.current, tt.new))
		})
	}
}

func TestBillableMinutes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		durationMS int64
		want       int64
	}{
		{name: "sub minute rounds up", durationMS: 59_000, want: 1},
		{name: "exact minute", durationMS: 60_000, want: 1},
		{name: "over minute", durationMS: 90_000, want: 2},
		{name: "zero duration", durationMS: 0, want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, billableMinutes(tt.durationMS))
		})
	}
}
