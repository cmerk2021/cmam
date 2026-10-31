package ui

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestPlainReporterOutput(t *testing.T) {
	var buf strings.Builder
	r := NewPlainReporter(&buf)

	r.AppStart("wgo", "v1.2.0")
	r.StepStart("wgo", "Download")
	r.StepProgress("wgo", "Download", 50, 100)
	r.StepProgress("wgo", "Download", 100, 100)
	r.StepDone("wgo", "Download", 1500*time.Millisecond)
	r.StepStart("wgo", "Verify")
	r.StepFail("wgo", "Verify", errors.New("checksum mismatch"))
	r.Warn("something odd")
	r.Info("done-ish")
	r.Close()

	out := buf.String()
	for _, want := range []string{
		"==> wgo (v1.2.0)",
		"[1] wgo: Download ...",
		"50%",
		"100%",
		"ok: Download (1.5s)",
		"[2] wgo: Verify ...",
		"FAILED: Verify: checksum mismatch",
		"warning: something odd",
		"done-ish",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q.\nfull output:\n%s", want, out)
		}
	}
	if strings.Contains(out, "\x1b[") {
		t.Error("plain reporter must not emit ANSI escapes")
	}
}

func TestPlainReporterProgressMilestones(t *testing.T) {
	var buf strings.Builder
	r := NewPlainReporter(&buf)
	r.StepStart("wgo", "Download")
	for i := int64(1); i <= 100; i++ {
		r.StepProgress("wgo", "Download", i, 100)
	}
	out := buf.String()
	// Only coarse milestones should print, not 100 lines.
	if n := strings.Count(out, "%"); n > 6 {
		t.Errorf("too many progress lines: %d\n%s", n, out)
	}
}

func TestFormatBytes(t *testing.T) {
	cases := []struct {
		in   int64
		want string
	}{
		{512, "512 B"},
		{2048, "2.0 KB"},
		{5 << 20, "5.0 MB"},
	}
	for _, tc := range cases {
		if got := FormatBytes(tc.in); got != tc.want {
			t.Errorf("FormatBytes(%d) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestTableTruncates(t *testing.T) {
	tbl := Table{
		Headers: []string{"Name", "Description"},
		Rows:    [][]string{{"wgo", strings.Repeat("long description ", 20)}},
		Width:   40,
	}
	out := tbl.Render()
	for _, line := range strings.Split(out, "\n") {
		if len([]rune(line)) > 45 { // small slack for the ellipsis
			t.Errorf("line too wide (%d): %q", len([]rune(line)), line)
		}
	}
}
