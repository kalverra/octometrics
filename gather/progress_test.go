package gather

import (
	"bytes"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestProgressReporter_AI(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	reporter := NewAIProgressReporter(&buf)

	reporter.Start("Waiting for workflow run 123 to complete")
	assert.Contains(t, buf.String(), "Waiting for workflow run 123 to complete")

	buf.Reset()
	reporter.Update("Waiting for workflow run 123 to complete", 15*time.Second)
	assert.Empty(t, buf.String(), "AI progress reporter should not produce output on periodic updates")

	buf.Reset()
	reporter.Stop("Workflow run 123 completed")
	assert.Contains(t, buf.String(), "Workflow run 123 completed")
}

func TestProgressReporter_Noop(t *testing.T) {
	t.Parallel()

	reporter := &NoopProgressReporter{}
	reporter.Start("msg")
	reporter.Update("msg", time.Second)
	reporter.Stop("msg")
}

func TestNewAutoProgressReporter(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	r1 := NewAutoProgressReporter("human", false, &buf)
	_, isHuman := r1.(*HumanProgressReporter)
	assert.True(t, isHuman, "style=human should return HumanProgressReporter")

	r2 := NewAutoProgressReporter("ai", true, &buf)
	_, isAI := r2.(*AIProgressReporter)
	assert.True(t, isAI, "style=ai should return AIProgressReporter")

	r3 := NewAutoProgressReporter("none", true, &buf)
	_, isNoop := r3.(*NoopProgressReporter)
	assert.True(t, isNoop, "style=none should return NoopProgressReporter")

	r4 := NewAutoProgressReporter("auto", true, &buf)
	_, isHumanAuto := r4.(*HumanProgressReporter)
	assert.True(t, isHumanAuto, "style=auto + isTTY=true should return HumanProgressReporter")

	r5 := NewAutoProgressReporter("auto", false, &buf)
	_, isAIAuto := r5.(*AIProgressReporter)
	assert.True(t, isAIAuto, "style=auto + isTTY=false should return AIProgressReporter")
}

func TestProgressReporter_Human_Transitions(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	reporter := NewHumanProgressReporter(&buf)

	reporter.Start("Collecting data (commit abc1234)")
	assert.Equal(t, "Collecting data (commit abc1234)", reporter.currentMsg)

	reporter.Start("Building observation (commit abc1234)")
	assert.Equal(t, "Building observation (commit abc1234)", reporter.currentMsg)

	reporter.Stop("Done ✅")
	assert.Contains(t, buf.String(), "Done ✅")
}

func TestProgressReporter_Human_StopEmpty(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	reporter := NewHumanProgressReporter(&buf)

	reporter.Start("Collecting data (workflow run 123)")
	reporter.Stop("")
	assert.NotContains(t, buf.String(), "\n\n")
}

func TestProgressReporter_Human_StopClearsLine(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	reporter := NewHumanProgressReporter(&buf)
	reporter.isTTY = true

	reporter.Stop("")
	assert.Contains(
		t,
		buf.String(),
		"\r\033[2K\033[?25h",
		"Stop should clear terminal line and restore cursor when TTY",
	)
}

func TestProgressReporter_Human_ConsistentCountUpDurationFormat(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	reporter := NewHumanProgressReporter(&buf)

	reporter.Start("Collecting data (commit abc1234)")
	initialStartTime := reporter.startTime
	assert.False(t, initialStartTime.IsZero())

	assert.Contains(t, reporter.formatTitle("Collecting data (commit abc1234)", 0), "(0s)")
	assert.Contains(t, reporter.formatTitle("Collecting data (commit abc1234)", 5*time.Second), "(5s)")
	assert.Contains(t, reporter.formatTitle("Collecting data (commit abc1234)", 62*time.Second), "(1m2s)")

	reporter.startTime = time.Now().Add(-10 * time.Second)
	preservedStartTime := reporter.startTime

	reporter.Start("Building observation (commit abc1234)")
	assert.Equal(t, "Building observation (commit abc1234)", reporter.currentMsg)
	assert.Equal(
		t,
		preservedStartTime,
		reporter.startTime,
		"Start should preserve overall process start time for continuous count up",
	)

	reporter.Start("Building comparison (commit abc1234 vs def5678)")
	assert.Equal(t, "Building comparison (commit abc1234 vs def5678)", reporter.currentMsg)
	assert.Equal(
		t,
		preservedStartTime,
		reporter.startTime,
		"Start should preserve overall process start time across all phases",
	)

	reporter.Stop("")
	assert.True(t, reporter.startTime.IsZero(), "Stop should reset start time")
}
