// Package cmd implements the cmam command-line interface.
package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/cmerk2021/cmam/internal/config"
	"github.com/cmerk2021/cmam/internal/github"
	"github.com/cmerk2021/cmam/internal/registry"
	"github.com/cmerk2021/cmam/internal/selfupdate"
	"github.com/cmerk2021/cmam/internal/state"
	"github.com/cmerk2021/cmam/internal/ui"
	"github.com/cmerk2021/cmam/internal/version"
)

// app carries shared dependencies for all commands.
type app struct {
	paths    config.Paths
	gh       *github.Client
	registry *registry.Client
	state    *state.Store

	noColor bool
	verbose bool
	yes     bool

	updateNotice func() string
}

// interactive reports whether stdout is a terminal.
func (a *app) interactive() bool { return ui.IsTTY(os.Stdout) }

// reporter builds the right Reporter for the current terminal.
func (a *app) reporter(cancel func()) ui.Reporter {
	if a.interactive() && ui.ColorEnabled(a.noColor) {
		return ui.NewTeaReporter(cancel)
	}
	return ui.NewPlainReporter(os.Stderr)
}

// lock serializes mutating commands across cmam processes.
func (a *app) lock() (*state.Lock, error) { return state.Acquire(a.paths.LockFile) }

// fail prints a styled error block (with raw details when --verbose) and
// returns a silent error so Cobra does not reprint it.
func (a *app) fail(msg string, err error, hint string) error {
	if a.verbose && err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
	}
	fmt.Fprintln(os.Stderr, ui.ErrorBlock(msg, hint))
	return errSilent
}

var errSilent = errors.New("cmam: error already reported")

// friendlyError turns common errors into a styled block with a hint.
func (a *app) friendlyError(err error) error {
	if err == nil || errors.Is(err, errSilent) {
		return err
	}
	var rl *github.RateLimitError
	hint := ""
	msg := err.Error()
	switch {
	case errors.As(err, &rl):
		msg = rl.Error()
		hint = "export GITHUB_TOKEN=<personal access token> and retry"
	case errors.Is(err, context.Canceled):
		msg = "operation cancelled"
	}
	return a.fail(msg, err, hint)
}

// NewRootCmd builds the full cmam command tree.
func NewRootCmd() *cobra.Command {
	a := &app{}

	root := &cobra.Command{
		Use:           "cmam",
		Short:         "cmam installs and manages prebuilt Go CLI tools",
		Long:          "cmam is a tiny package manager for prebuilt Go CLIs,\ninstalling verified release binaries into ~/.local/bin.",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			paths, err := config.ResolvePaths()
			if err != nil {
				return err
			}
			a.paths = paths
			if err := paths.EnsureDirs(); err != nil {
				return err
			}
			if !ui.ColorEnabled(a.noColor) {
				ui.DisableColors()
			}
			a.gh = github.NewClient()
			a.state = &state.Store{Path: paths.StateFile()}
			a.registry = registry.NewClient(
				config.RegistryIndexURL(),
				fmt.Sprintf("https://api.github.com/repos/%s/contents/apps?ref=%s", config.RegistryRepo, config.RegistryBranch),
				config.RegistryManifestURL,
				paths.RegistryCacheDir,
			)
			a.updateNotice = selfupdate.StartBackgroundCheck(cmd.Context(), paths, a.interactive())
			return nil
		},
		PersistentPostRun: func(cmd *cobra.Command, args []string) {
			if cmd.Name() == "self-update" || a.updateNotice == nil {
				return
			}
			if n := a.updateNotice(); n != "" {
				fmt.Fprintln(os.Stderr, ui.StyleSubtle.Render(n))
			}
		},
	}

	root.PersistentFlags().BoolVar(&a.noColor, "no-color", false, "disable colored output")
	root.PersistentFlags().BoolVarP(&a.verbose, "verbose", "v", false, "show raw error details and extra logs")
	root.PersistentFlags().BoolVarP(&a.yes, "yes", "y", false, "assume yes for confirmation prompts")

	root.AddCommand(
		newInstallCmd(a),
		newUpdateCmd(a),
		newUninstallCmd(a),
		newListCmd(a),
		newSearchCmd(a),
		newInfoCmd(a),
		newSelfUpdateCmd(a),
		newDoctorCmd(a),
		newCleanCmd(a),
		newVersionCmd(),
	)
	return root
}

// Execute runs the CLI, wiring signal-based cancellation, and returns the
// process exit code.
func Execute() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	root := NewRootCmd()
	if err := root.ExecuteContext(ctx); err != nil {
		if !errors.Is(err, errSilent) {
			fmt.Fprintln(os.Stderr, ui.ErrorBlock(err.Error(), ""))
		}
		return 1
	}
	return 0
}

// completeRegistryApps completes app names from the cached registry.
func completeRegistryApps(a *app) cobra.CompletionFunc {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return a.registry.CachedNames(), cobra.ShellCompDirectiveNoFileComp
	}
}

// completeInstalledApps completes app names from installed state.
func completeInstalledApps(a *app) cobra.CompletionFunc {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return a.state.Names(), cobra.ShellCompDirectiveNoFileComp
	}
}

// confirm asks a y/N question on the terminal, honoring --yes.
func (a *app) confirm(prompt string) bool {
	if a.yes {
		return true
	}
	if !a.interactive() {
		return false
	}
	fmt.Fprintf(os.Stderr, "%s [y/N] ", prompt)
	var answer string
	_, _ = fmt.Scanln(&answer)
	return answer == "y" || answer == "Y" || answer == "yes"
}

// Version info shared with the version command.
func versionLine() string {
	return fmt.Sprintf("cmam %s (commit %s, built %s)", version.Version, version.Commit, version.Date)
}
