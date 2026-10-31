package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/cmerk2021/cmam/internal/selfupdate"
	"github.com/cmerk2021/cmam/internal/ui"
	"github.com/cmerk2021/cmam/internal/version"
)

func newSelfUpdateCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "self-update",
		Short: "Update cmam itself to the latest release",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			lock, err := a.lock()
			if err != nil {
				return a.friendlyError(err)
			}
			defer lock.Release()

			u := &selfupdate.Updater{GH: a.gh, Paths: a.paths}
			latest, newer, err := u.Check(cmd.Context())
			if err != nil {
				return a.friendlyError(err)
			}
			if version.Version == "dev" {
				fmt.Fprintln(os.Stderr, ui.StyleWarn.Render(
					ui.GlyphWarn+" this is a dev build; latest release is v"+latest+" — skipping self-update"))
				return nil
			}
			if !newer {
				fmt.Fprintln(os.Stderr, ui.StyleGood.Render(
					ui.GlyphDone+" cmam v"+version.Version+" is already the latest version"))
				return nil
			}

			fmt.Fprintln(os.Stderr, ui.StyleSubtle.Render("updating cmam v"+version.Version+" "+ui.GlyphArrow+" v"+latest))
			exe, err := u.Apply(cmd.Context(), latest, nil)
			if err != nil {
				return a.friendlyError(err)
			}
			fmt.Fprintf(os.Stderr, "%s cmam updated to v%s (%s)\n", ui.StyleGood.Render(ui.GlyphDone), latest, exe)
			return nil
		},
	}
}
