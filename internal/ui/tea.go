package ui

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// TeaReporter is a Reporter backed by a live Bubble Tea step-list UI with
// spinners and an inline download progress bar. Create it with
// NewTeaReporter and always call Close.
type TeaReporter struct {
	prog   *tea.Program
	done   chan struct{}
	cancel func()
}

type (
	appStartMsg     struct{ app, detail string }
	stepStartMsg    struct{ app, step string }
	stepProgressMsg struct {
		app, step   string
		done, total int64
	}
	stepDoneMsg struct {
		app, step string
		d         time.Duration
	}
	stepFailMsg struct {
		app, step string
		err       error
	}
	noteMsg struct {
		text string
		warn bool
	}
	quitMsg struct{}
)

// NewTeaReporter starts the UI program. cancel is invoked when the user
// presses Ctrl+C so the caller can abort cleanly.
func NewTeaReporter(cancel func()) *TeaReporter {
	m := newStepModel()
	m.cancel = cancel
	p := tea.NewProgram(m, tea.WithOutput(os.Stderr))
	r := &TeaReporter{prog: p, done: make(chan struct{}), cancel: cancel}
	go func() {
		defer close(r.done)
		_, _ = p.Run()
	}()
	return r
}

// AppStart implements Reporter.
func (r *TeaReporter) AppStart(app, detail string) { r.prog.Send(appStartMsg{app, detail}) }

// StepStart implements Reporter.
func (r *TeaReporter) StepStart(app, step string) { r.prog.Send(stepStartMsg{app, step}) }

// StepProgress implements Reporter.
func (r *TeaReporter) StepProgress(app, step string, done, total int64) {
	r.prog.Send(stepProgressMsg{app, step, done, total})
}

// StepDone implements Reporter.
func (r *TeaReporter) StepDone(app, step string, d time.Duration) {
	r.prog.Send(stepDoneMsg{app, step, d})
}

// StepFail implements Reporter.
func (r *TeaReporter) StepFail(app, step string, err error) {
	r.prog.Send(stepFailMsg{app, step, err})
}

// Info implements Reporter.
func (r *TeaReporter) Info(msg string) { r.prog.Send(noteMsg{text: msg}) }

// Warn implements Reporter.
func (r *TeaReporter) Warn(msg string) { r.prog.Send(noteMsg{text: msg, warn: true}) }

// Close implements Reporter: it stops the UI and waits for the final frame.
func (r *TeaReporter) Close() {
	r.prog.Send(quitMsg{})
	<-r.done
}

const (
	stepRunning = iota
	stepDone
	stepFailed
)

type stepState struct {
	name        string
	status      int
	started     time.Time
	dur         time.Duration
	err         error
	doneBytes   int64
	totalBytes  int64
	hasProgress bool
}

type appState struct {
	name, detail string
	steps        []*stepState
}

type stepModel struct {
	apps    []*appState
	notes   []noteMsg
	spin    spinner.Model
	bar     progress.Model
	width   int
	started time.Time
	cancel  func()
}

func newStepModel() *stepModel {
	sp := spinner.New(spinner.WithSpinner(spinner.MiniDot))
	sp.Style = StyleAccent
	bar := progress.New(progress.WithGradient("#6B21A8", "#22D3EE"), progress.WithoutPercentage())
	return &stepModel{spin: sp, bar: bar, width: 100, started: time.Now()}
}

// Init implements tea.Model.
func (m *stepModel) Init() tea.Cmd { return m.spin.Tick }

func (m *stepModel) findApp(name string) *appState {
	for _, a := range m.apps {
		if a.name == name {
			return a
		}
	}
	a := &appState{name: name}
	m.apps = append(m.apps, a)
	return a
}

func (m *stepModel) findStep(app, step string) *stepState {
	a := m.findApp(app)
	for _, s := range a.steps {
		if s.name == step {
			return s
		}
	}
	s := &stepState{name: step, status: stepRunning, started: time.Now()}
	a.steps = append(a.steps, s)
	return s
}

// Update implements tea.Model.
func (m *stepModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.bar.Width = clamp(msg.Width-45, 10, 50)
	case tea.KeyMsg:
		if msg.Type == tea.KeyCtrlC && m.cancel != nil {
			m.cancel() // propagate cancellation; the owner closes the reporter
		}
	case appStartMsg:
		a := m.findApp(msg.app)
		a.detail = msg.detail
	case stepStartMsg:
		m.findStep(msg.app, msg.step)
	case stepProgressMsg:
		s := m.findStep(msg.app, msg.step)
		s.doneBytes, s.totalBytes, s.hasProgress = msg.done, msg.total, true
	case stepDoneMsg:
		s := m.findStep(msg.app, msg.step)
		s.status, s.dur = stepDone, msg.d
	case stepFailMsg:
		s := m.findStep(msg.app, msg.step)
		s.status, s.err = stepFailed, msg.err
	case noteMsg:
		m.notes = append(m.notes, msg)
	case quitMsg:
		return m, tea.Quit
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd
	}
	return m, nil
}

// View implements tea.Model.
func (m *stepModel) View() string {
	var b strings.Builder
	for _, a := range m.apps {
		header := StyleAppName.Render(a.name)
		if a.detail != "" {
			header += " " + StyleSubtle.Render(a.detail)
		}
		b.WriteString(header + "\n")
		for _, s := range a.steps {
			b.WriteString(m.renderStep(s))
		}
	}
	for _, n := range m.notes {
		if n.warn {
			b.WriteString(StyleWarn.Render(GlyphWarn+" "+n.text) + "\n")
		} else {
			b.WriteString(StyleSubtle.Render(n.text) + "\n")
		}
	}
	return b.String()
}

func (m *stepModel) renderStep(s *stepState) string {
	var line string
	switch s.status {
	case stepRunning:
		line = "  " + m.spin.View() + " " + s.name
		if s.hasProgress && s.totalBytes > 0 {
			pct := float64(s.doneBytes) / float64(s.totalBytes)
			elapsed := time.Since(s.started).Seconds()
			speed := float64(s.doneBytes) / maxf(elapsed, 0.001)
			eta := "--"
			if speed > 0 {
				eta = (time.Duration(float64(s.totalBytes-s.doneBytes)/speed) * time.Second).Round(time.Second).String()
			}
			line += "\n    " + m.bar.ViewAs(pct) + " " + StyleSubtle.Render(fmt.Sprintf(
				"%3.0f%%  %s / %s  %s/s  ETA %s",
				pct*100, FormatBytes(s.doneBytes), FormatBytes(s.totalBytes), FormatBytes(int64(speed)), eta))
		} else if s.hasProgress {
			line += StyleSubtle.Render("  " + FormatBytes(s.doneBytes))
		}
	case stepDone:
		line = "  " + StyleGood.Render(GlyphDone) + " " + s.name +
			StyleSubtle.Render("  "+s.dur.Round(time.Millisecond).String())
	case stepFailed:
		line = "  " + StyleBad.Render(GlyphFail+" "+s.name)
		if s.err != nil {
			line += "\n    " + StyleBad.Render(truncate(s.err.Error(), m.width-6))
		}
	}
	return line + "\n"
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func maxf(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func truncate(s string, w int) string {
	if w < 4 || lipgloss.Width(s) <= w {
		return s
	}
	r := []rune(s)
	if len(r) <= w {
		return s
	}
	return string(r[:w-1]) + "…"
}
