package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Table renders a styled table that truncates gracefully to the terminal
// width. When color is disabled the output degrades to plain text.
type Table struct {
	Headers []string
	Rows    [][]string
	// Width is the maximum render width (defaults to terminal width).
	Width int
}

// Render produces the table as a string (no trailing newline).
func (t Table) Render() string {
	width := t.Width
	if width <= 0 {
		width = TermWidth()
	}
	cols := len(t.Headers)
	widths := make([]int, cols)
	for i, h := range t.Headers {
		widths[i] = lipgloss.Width(h)
	}
	for _, row := range t.Rows {
		for i := 0; i < cols && i < len(row); i++ {
			if w := lipgloss.Width(row[i]); w > widths[i] {
				widths[i] = w
			}
		}
	}

	// Shrink the widest column until the table fits.
	const gap = 2
	total := func() int {
		sum := gap * (cols - 1)
		for _, w := range widths {
			sum += w
		}
		return sum
	}
	for total() > width {
		max, idx := 0, -1
		for i, w := range widths {
			if w > max {
				max, idx = w, i
			}
		}
		if idx < 0 || widths[idx] <= 8 {
			break
		}
		widths[idx]--
	}

	var b strings.Builder
	for i, h := range t.Headers {
		if i > 0 {
			b.WriteString(strings.Repeat(" ", gap))
		}
		b.WriteString(StyleHeader.Render(pad(h, widths[i])))
	}
	for _, row := range t.Rows {
		b.WriteString("\n")
		for i := 0; i < cols; i++ {
			cell := ""
			if i < len(row) {
				cell = row[i]
			}
			if i > 0 {
				b.WriteString(strings.Repeat(" ", gap))
			}
			b.WriteString(pad(cell, widths[i]))
		}
	}
	return b.String()
}

func pad(s string, w int) string {
	s = truncate(s, w)
	if diff := w - lipgloss.Width(s); diff > 0 {
		return s + strings.Repeat(" ", diff)
	}
	return s
}

// ErrorBlock renders a consistent styled error with an optional hint.
func ErrorBlock(msg, hint string) string {
	body := StyleBad.Render(GlyphFail + " " + msg)
	if hint != "" {
		body += "\n" + StyleSubtle.Render("  hint: "+hint)
	}
	return body
}

// UpdateArrow renders "old → new" with the update highlighted.
func UpdateArrow(oldV, newV string) string {
	return StyleWarn.Render(fmt.Sprintf("%s %s %s", oldV, GlyphArrow, newV))
}
