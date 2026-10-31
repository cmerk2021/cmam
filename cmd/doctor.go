package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/cmerk2021/cmam/internal/doctor"
	"github.com/cmerk2021/cmam/internal/ui"
)

func newDoctorCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Diagnose common problems with the cmam setup",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			results := doctor.Run(cmd.Context(), a.paths, a.state, a.registry)
			bad := 0
			for _, r := range results {
				var glyph string
				switch r.Status {
				case doctor.OK:
					glyph = ui.StyleGood.Render(ui.GlyphDone)
				case doctor.Warning:
					glyph = ui.StyleWarn.Render(ui.GlyphWarn)
				case doctor.Error:
					glyph = ui.StyleBad.Render(ui.GlyphFail)
					bad++
				}
				line := fmt.Sprintf("%s %s", glyph, r.Name)
				if r.Detail != "" {
					line += ui.StyleSubtle.Render("  — " + r.Detail)
				}
				fmt.Fprintln(os.Stdout, line)
				if r.Fix != "" && r.Status != doctor.OK {
					fmt.Fprintln(os.Stdout, ui.StyleSubtle.Render("    fix: "+r.Fix))
				}
			}
			if bad > 0 {
				return a.fail(fmt.Sprintf("%d check(s) failed", bad), nil, "apply the fixes above and run `cmam doctor` again")
			}
			fmt.Fprintln(os.Stdout, ui.StyleGood.Render("\nall checks passed"))
			return nil
		},
	}
}
