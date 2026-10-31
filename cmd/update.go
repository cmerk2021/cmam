package cmd

import (
	"context"
	"fmt"
	"os"
	"sort"

	"github.com/spf13/cobra"

	"github.com/cmerk2021/cmam/internal/github"
	"github.com/cmerk2021/cmam/internal/installer"
	"github.com/cmerk2021/cmam/internal/ui"
)

func newUpdateCmd(a *app) *cobra.Command {
	var refresh bool
	cmd := &cobra.Command{
		Use:               "update [app]",
		Short:             "Update one app, or every installed app when none is given",
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: completeInstalledApps(a),
		RunE: func(cmd *cobra.Command, args []string) error {
			lock, err := a.lock()
			if err != nil {
				return a.friendlyError(err)
			}
			defer lock.Release()

			installed, err := a.state.Load()
			if err != nil {
				return a.friendlyError(err)
			}

			var names []string
			if len(args) == 1 {
				name := args[0]
				if _, ok := installed[name]; !ok {
					return a.fail(name+" is not installed", nil, "run `cmam install "+name+"` first")
				}
				names = []string{name}
			} else {
				for n := range installed {
					names = append(names, n)
				}
				sort.Strings(names)
			}
			if len(names) == 0 {
				fmt.Fprintln(os.Stderr, ui.StyleSubtle.Render("nothing installed yet — try `cmam install <app>`"))
				return nil
			}

			// Check which apps are outdated before touching anything.
			ctx := cmd.Context()
			type plan struct{ name, from, to string }
			var plans []plan
			var checkErr error
			for _, name := range names {
				rec := installed[name]
				latest, err := a.gh.LatestVersion(ctx, rec.Repo)
				if err != nil {
					fmt.Fprintln(os.Stderr, ui.StyleWarn.Render(ui.GlyphWarn+" "+name+": "+err.Error()))
					if checkErr == nil {
						checkErr = err
					}
					continue
				}
				if github.CompareVersions(latest, rec.Version) > 0 {
					plans = append(plans, plan{name, rec.Version, latest})
				}
			}

			if len(plans) == 0 {
				if checkErr != nil {
					return a.friendlyError(checkErr)
				}
				fmt.Fprintln(os.Stderr, ui.StyleGood.Render(ui.GlyphDone+" everything is up to date"))
				return nil
			}

			fmt.Fprintln(os.Stderr, ui.StyleHeader.Render("updates available:"))
			for _, p := range plans {
				fmt.Fprintf(os.Stderr, "  %s  %s\n", ui.StyleAppName.Render(p.name), ui.UpdateArrow("v"+p.from, "v"+p.to))
			}

			cctx, cancel := context.WithCancel(ctx)
			defer cancel()
			rep := a.reporter(cancel)
			ins := &installer.Installer{
				Paths: a.paths, GH: a.gh, Registry: a.registry,
				State: a.state, Reporter: rep,
			}

			updated, failed := 0, 0
			var firstErr error
			for _, p := range plans {
				if _, err := ins.Install(cctx, p.name, p.to, refresh); err != nil {
					failed++
					if firstErr == nil {
						firstErr = err
					}
					if cctx.Err() != nil {
						break
					}
					continue
				}
				updated++
			}
			rep.Close()

			skippedCount := len(names) - len(plans)
			fmt.Fprintln(os.Stderr, ui.StyleSubtle.Render(
				fmt.Sprintf("%d updated, %d skipped, %d failed", updated, skippedCount, failed)))
			if firstErr != nil {
				return a.friendlyError(firstErr)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&refresh, "refresh", false, "force a registry refetch")
	return cmd
}

func newUninstallCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "uninstall <app>",
		Short:             "Remove an installed app",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeInstalledApps(a),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			lock, err := a.lock()
			if err != nil {
				return a.friendlyError(err)
			}
			defer lock.Release()

			ins := &installer.Installer{
				Paths: a.paths, GH: a.gh, Registry: a.registry,
				State: a.state, Reporter: ui.NewPlainReporter(os.Stderr),
			}
			if !a.confirm("uninstall " + name + "?") {
				fmt.Fprintln(os.Stderr, ui.StyleSubtle.Render("aborted"))
				return nil
			}
			rec, err := ins.Uninstall(name)
			if err != nil {
				return a.friendlyError(err)
			}
			fmt.Fprintf(os.Stderr, "%s removed %s v%s (%s)\n",
				ui.StyleGood.Render(ui.GlyphDone), ui.StyleAppName.Render(rec.Name), rec.Version, rec.BinaryPath)
			return nil
		},
	}
	return cmd
}
