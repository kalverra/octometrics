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
