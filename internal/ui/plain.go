package ui

import (
	"fmt"
	"io"
	"sync"
	"time"
)

// PlainReporter writes line-based output with no ANSI sequences, spinners,
// or redraws. It is used when stdout is not a TTY (pipes, CI) and as the
// fallback when Bubble Tea cannot run.
type PlainReporter struct {
	// Out receives all output (normally stderr so stdout stays clean).
	Out io.Writer

	mu       sync.Mutex
	app      string
	step     int
	lastPct  int
	lastStep string
}

// NewPlainReporter builds a PlainReporter writing to out.
func NewPlainReporter(out io.Writer) *PlainReporter { return &PlainReporter{Out: out, lastPct: -1} }

// AppStart implements Reporter.
func (r *PlainReporter) AppStart(app, detail string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.app = app
	r.step = 0
	if detail != "" {
		fmt.Fprintf(r.Out, "==> %s (%s)\n", app, detail)
	} else {
		fmt.Fprintf(r.Out, "==> %s\n", app)
	}
}

// StepStart implements Reporter.
func (r *PlainReporter) StepStart(app, step string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.step++
	r.lastPct = -1
	r.lastStep = step
	fmt.Fprintf(r.Out, "[%d] %s: %s ...\n", r.step, app, step)
}

// StepProgress implements Reporter. It prints coarse milestones (every 25%)
// to stay readable in logs.
func (r *PlainReporter) StepProgress(app, step string, done, total int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if total <= 0 {
		return
	}
	pct := int(done * 100 / total)
	milestone := pct / 25 * 25
	if milestone > r.lastPct {
		r.lastPct = milestone
		fmt.Fprintf(r.Out, "    %s: %d%% (%s / %s)\n", step, pct, FormatBytes(done), FormatBytes(total))
	}
}

// StepDone implements Reporter.
func (r *PlainReporter) StepDone(app, step string, d time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	fmt.Fprintf(r.Out, "    ok: %s (%s)\n", step, d.Round(time.Millisecond))
}

// StepFail implements Reporter.
func (r *PlainReporter) StepFail(app, step string, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	fmt.Fprintf(r.Out, "    FAILED: %s: %v\n", step, err)
}

// Info implements Reporter.
func (r *PlainReporter) Info(msg string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	fmt.Fprintln(r.Out, msg)
}

// Warn implements Reporter.
func (r *PlainReporter) Warn(msg string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	fmt.Fprintf(r.Out, "warning: %s\n", msg)
}

// Close implements Reporter.
func (r *PlainReporter) Close() {}

// FormatBytes renders a byte count in a human-friendly unit.
func FormatBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}
