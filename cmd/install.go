package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/cmerk2021/cmam/internal/doctor"
	"github.com/cmerk2021/cmam/internal/installer"
	"github.com/cmerk2021/cmam/internal/ui"
)

func newInstallCmd(a *app) *cobra.Command {
	var refresh, force bool
	cmd := &cobra.Command{
		Use:               "install <app>[@version] [app...]",
		Short:             "Install the latest or a specific version of one or more apps",
		Args:              cobra.MinimumNArgs(1),
		ValidArgsFunction: completeRegistryApps(a),
		RunE: func(cmd *cobra.Command, args []string) error {
			lock, err := a.lock()
			if err != nil {
				return a.friendlyError(err)
			}
			defer lock.Release()

			ctx, cancel := context.WithCancel(cmd.Context())
			defer cancel()
			rep := a.reporter(cancel)

			ins := &installer.Installer{
				Paths: a.paths, GH: a.gh, Registry: a.registry,
				State: a.state, Reporter: rep, Force: force,
			}

			var results []installer.Result
			var firstErr error
			for _, spec := range args {
				name, ver, err := parseSpec(spec)
				if err != nil {
					rep.Warn(err.Error())
					if firstErr == nil {
						firstErr = err
					}
					continue
				}
				res, err := ins.Install(ctx, name, ver, refresh)
				if err != nil {
					if firstErr == nil {
						firstErr = err
					}
					if ctx.Err() != nil {
						break
					}
					continue
				}
				results = append(results, res)
			}
			rep.Close()

			printInstallSummary(a, results)
			if firstErr != nil {
				return a.friendlyError(firstErr)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&refresh, "refresh", false, "force a registry refetch")
	cmd.Flags().BoolVar(&force, "force", false, "overwrite binaries not installed by cmam")
	return cmd
}

func printInstallSummary(a *app, results []installer.Result) {
	installed, skipped := 0, 0
	for _, r := range results {
		if r.Skipped {
			skipped++
			continue
		}
		installed++
		line := fmt.Sprintf("%s %s v%s %s %s",
			ui.StyleGood.Render(ui.GlyphDone),
			ui.StyleAppName.Render(r.Name), r.Version,
			ui.StyleSubtle.Render(ui.GlyphArrow), r.Path)
		if r.Switched {
			line += ui.StyleSubtle.Render(fmt.Sprintf("  (switched from v%s)", r.Previous))
		}
		fmt.Fprintln(os.Stderr, line)
	}
	if installed+skipped > 1 {
		fmt.Fprintln(os.Stderr, ui.StyleSubtle.Render(
			fmt.Sprintf("%d installed, %d already current", installed, skipped)))
	}
	if installed > 0 && !pathHasBinDir(a) {
		fmt.Fprintln(os.Stderr, ui.StyleWarn.Render(ui.GlyphWarn+" "+a.paths.BinDir+" is not on your PATH"))
		fmt.Fprintln(os.Stderr, ui.StyleSubtle.Render("  "+doctor.PathHint(a.paths.BinDir)))
	}
}

func pathHasBinDir(a *app) bool {
	for _, p := range splitPathList(os.Getenv("PATH")) {
		if p == a.paths.BinDir {
			return true
		}
	}
	return false
}
