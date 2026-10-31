package ui

import (
	"os"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"golang.org/x/term"
)

// Adaptive palette: one cohesive set of colors that works on both light and
// dark terminals.
var (
	ColorPrimary = lipgloss.AdaptiveColor{Light: "#6B21A8", Dark: "#C084FC"}
	ColorAccent  = lipgloss.AdaptiveColor{Light: "#0E7490", Dark: "#22D3EE"}
	ColorGood    = lipgloss.AdaptiveColor{Light: "#15803D", Dark: "#4ADE80"}
	ColorWarn    = lipgloss.AdaptiveColor{Light: "#A16207", Dark: "#FACC15"}
	ColorBad     = lipgloss.AdaptiveColor{Light: "#B91C1C", Dark: "#F87171"}
	ColorSubtle  = lipgloss.AdaptiveColor{Light: "#6B7280", Dark: "#9CA3AF"}
)

// Styles used across the CLI, defined once for a cohesive look.
var (
	StyleBrand   = lipgloss.NewStyle().Bold(true).Foreground(ColorPrimary)
	StyleAccent  = lipgloss.NewStyle().Foreground(ColorAccent)
	StyleGood    = lipgloss.NewStyle().Foreground(ColorGood)
	StyleWarn    = lipgloss.NewStyle().Foreground(ColorWarn)
	StyleBad     = lipgloss.NewStyle().Foreground(ColorBad)
	StyleSubtle  = lipgloss.NewStyle().Foreground(ColorSubtle)
	StyleBold    = lipgloss.NewStyle().Bold(true)
	StyleHeader  = lipgloss.NewStyle().Bold(true).Foreground(ColorAccent)
	StyleCard    = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(ColorSubtle).Padding(0, 2)
	StyleErrBox  = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(ColorBad).Padding(0, 1)
	StyleAppName = lipgloss.NewStyle().Bold(true).Foreground(ColorAccent)
)

// Glyphs used for step and check states.
const (
	GlyphPending = "○"
	GlyphDone    = "✔"
	GlyphFail    = "✖"
	GlyphWarn    = "⚠"
	GlyphArrow   = "→"
)

// IsTTY reports whether the given file is a terminal.
func IsTTY(f *os.File) bool { return term.IsTerminal(int(f.Fd())) }

// ColorEnabled reports whether colored output should be used, honoring
// NO_COLOR and the --no-color flag.
func ColorEnabled(noColorFlag bool) bool {
	if noColorFlag || os.Getenv("NO_COLOR") != "" {
		return false
	}
	return IsTTY(os.Stdout)
}

// DisableColors globally disables lipgloss styling.
func DisableColors() { lipgloss.SetColorProfile(termenv.Ascii) }

// TermWidth returns the terminal width or a sensible default.
func TermWidth() int {
	if w, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && w > 0 {
		return w
	}
	return 100
}

// Brand returns the CMAM header/brand line.
func Brand(version string) string {
	return StyleBrand.Render("◆ cmam") + " " + StyleSubtle.Render("v"+version)
}
