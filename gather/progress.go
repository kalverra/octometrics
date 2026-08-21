package gather

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"charm.land/lipgloss/v2"
	"golang.org/x/term"
)

var defaultSpinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

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
	if msg != "" {
		_, _ = fmt.Fprintln(a.writer, msg)
	}
}

// HumanProgressReporter renders animated terminal spinner with elapsed time display.
type HumanProgressReporter struct {
	writer io.Writer
	mu     sync.Mutex

	isTTY        bool
	cancel       func()
	done         chan struct{}
	currentMsg   string
	startTime    time.Time
	spinnerStyle lipgloss.Style
	timeStyle    lipgloss.Style
}

// NewHumanProgressReporter creates a HumanProgressReporter writing to w (defaults to os.Stderr).
func NewHumanProgressReporter(w io.Writer) *HumanProgressReporter {
	if w == nil {
		w = os.Stderr
	}
	var isTTY bool
	if f, ok := w.(*os.File); ok {
		isTTY = term.IsTerminal(int(f.Fd()))
	}
	return &HumanProgressReporter{
		writer:       w,
		isTTY:        isTTY,
		spinnerStyle: lipgloss.NewStyle().Foreground(lipgloss.Color("#F780E2")),
		timeStyle:    lipgloss.NewStyle().Foreground(lipgloss.Color("244")),
	}
}

func (h *HumanProgressReporter) formatLine(frame, msg string, elapsed time.Duration) string {
	d := elapsed.Round(time.Second)
	timeStr := h.timeStyle.Render(fmt.Sprintf("(%s)", d.String()))
	frameStr := h.spinnerStyle.Render(frame)
	return fmt.Sprintf("%s %s %s", frameStr, msg, timeStr)
}

func (h *HumanProgressReporter) formatTitle(msg string, elapsed time.Duration) string {
	d := elapsed.Round(time.Second)
	timeStr := h.timeStyle.Render(fmt.Sprintf("(%s)", d.String()))
	return fmt.Sprintf("%s %s", msg, timeStr)
}

// Start launches or updates the terminal spinner animation with count up timer.
func (h *HumanProgressReporter) Start(msg string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.currentMsg = msg
	if h.startTime.IsZero() {
		h.startTime = time.Now()
	}

	if h.cancel != nil {
		return
	}

	stopCh := make(chan struct{})
	h.done = make(chan struct{})
	h.cancel = func() {
		close(stopCh)
	}

	if !h.isTTY {
		go func() {
			<-stopCh
			close(h.done)
		}()
		return
	}

	go func() {
		defer close(h.done)

		// Hide cursor
		_, _ = fmt.Fprint(h.writer, "\033[?25l")

		ticker := time.NewTicker(80 * time.Millisecond)
		defer ticker.Stop()

		frameIdx := 0
		for {
			h.mu.Lock()
			msg := h.currentMsg
			elapsed := time.Since(h.startTime)
			frame := defaultSpinnerFrames[frameIdx%len(defaultSpinnerFrames)]
			line := h.formatLine(frame, msg, elapsed)
			h.mu.Unlock()

			_, _ = fmt.Fprintf(h.writer, "\r\033[2K%s", line)
			frameIdx++

			select {
			case <-stopCh:
				_, _ = fmt.Fprint(h.writer, "\r\033[2K\033[?25h")
				return
			case <-ticker.C:
			}
		}
	}()
}

// Update updates the spinner title with elapsed time.
func (h *HumanProgressReporter) Update(msg string, elapsed time.Duration) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if msg != "" {
		h.currentMsg = msg
	}
	if elapsed > 0 {
		h.startTime = time.Now().Add(-elapsed)
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
	h.startTime = time.Time{}
	writer := h.writer
	isTTY := h.isTTY
	h.mu.Unlock()

	if isTTY {
		_, _ = fmt.Fprint(writer, "\r\033[2K\033[?25h")
	}

	if msg != "" {
		_, _ = fmt.Fprintln(writer, msg)
	}
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
