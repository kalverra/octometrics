package gather

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"charm.land/huh/v2/spinner"
	"charm.land/lipgloss/v2"
)

// ProgressReporter handles wait status reporting for human vs AI consumers.
type ProgressReporter interface {
	Start(msg string)
	Update(msg string, elapsed time.Duration)
	Stop(msg string)
}

// NoopProgressReporter suppresses all progress output.
type NoopProgressReporter struct{}

// Start implements ProgressReporter for NoopProgressReporter.
func (*NoopProgressReporter) Start(_ string) {}

// Update implements ProgressReporter for NoopProgressReporter.
func (*NoopProgressReporter) Update(_ string, _ time.Duration) {}

// Stop implements ProgressReporter for NoopProgressReporter.
func (*NoopProgressReporter) Stop(_ string) {}

// AIProgressReporter renders super minimal single-line status messages without terminal animations.
type AIProgressReporter struct {
	writer io.Writer
	mu     sync.Mutex
}

// NewAIProgressReporter creates an AIProgressReporter writing to w (defaults to os.Stderr).
func NewAIProgressReporter(w io.Writer) *AIProgressReporter {
	if w == nil {
		w = os.Stderr
	}
	return &AIProgressReporter{writer: w}
}

// Start prints the initial status message for AI output.
func (a *AIProgressReporter) Start(msg string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	_, _ = fmt.Fprintln(a.writer, msg+"...")
}

// Update keeps periodic updates silent for minimal AI output.
func (a *AIProgressReporter) Update(_ string, _ time.Duration) {
	// AI output mode keeps periodic updates completely silent to avoid log noise
}

// Stop prints the completion status message for AI output.
func (a *AIProgressReporter) Stop(msg string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	_, _ = fmt.Fprintln(a.writer, msg)
}

// HumanProgressReporter renders animated terminal spinner with elapsed time display.
type HumanProgressReporter struct {
	writer io.Writer
	mu     sync.Mutex

	spin      *spinner.Spinner
	cancel    context.CancelFunc
	done      chan struct{}
	lastTitle string
	timeStyle lipgloss.Style
}

// NewHumanProgressReporter creates a HumanProgressReporter writing to w (defaults to os.Stderr).
func NewHumanProgressReporter(w io.Writer) *HumanProgressReporter {
	if w == nil {
		w = os.Stderr
	}
	return &HumanProgressReporter{
		writer:    w,
		timeStyle: lipgloss.NewStyle().Foreground(lipgloss.Color("244")),
	}
}

// Start launches the terminal spinner animation.
func (h *HumanProgressReporter) Start(msg string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.lastTitle = msg
	ctx, cancel := context.WithCancel(context.Background())
	h.cancel = cancel
	h.done = make(chan struct{})

	h.spin = spinner.New().
		Title(msg).
		WithOutput(h.writer).
		Action(func() {
			<-ctx.Done()
		})

	go func() {
		_ = h.spin.Run()
		close(h.done)
	}()
}

// Update updates the spinner title with elapsed time.
func (h *HumanProgressReporter) Update(msg string, elapsed time.Duration) {
	h.mu.Lock()
	defer h.mu.Unlock()

	mins := int(elapsed.Minutes())
	secs := int(elapsed.Seconds()) % 60
	timeStr := h.timeStyle.Render(fmt.Sprintf("[%02d:%02d]", mins, secs))

	h.lastTitle = fmt.Sprintf("%s %s", msg, timeStr)
	if h.spin != nil {
		h.spin.Title(h.lastTitle)
	}
}

// Stop stops the spinner animation and prints final completion text.
func (h *HumanProgressReporter) Stop(msg string) {
	h.mu.Lock()
	if h.cancel != nil {
		h.cancel()
		<-h.done
		h.cancel = nil
	}
	writer := h.writer
	h.mu.Unlock()

	_, _ = fmt.Fprintln(writer, msg)
}

// NewAutoProgressReporter constructs a ProgressReporter based on style ("auto", "human", "ai", "none")
// and whether the output stream is attached to an interactive terminal (isTTY).
func NewAutoProgressReporter(style string, isTTY bool, w io.Writer) ProgressReporter {
	if w == nil {
		w = os.Stderr
	}

	switch strings.ToLower(style) {
	case "human":
		return NewHumanProgressReporter(w)
	case "ai":
		return NewAIProgressReporter(w)
	case "none":
		return &NoopProgressReporter{}
	case "auto":
		fallthrough
	default:
		if isTTY {
			return NewHumanProgressReporter(w)
		}
		return NewAIProgressReporter(w)
	}
}
