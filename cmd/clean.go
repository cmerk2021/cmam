package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/cmerk2021/cmam/internal/ui"
)

func newCleanCmd(a *app) *cobra.Command {
	var dryRun, registryOnly bool
	cmd := &cobra.Command{
		Use:   "clean",
		Short: "Clear the download and registry caches",
		Long: "Clear the download cache and the cached registry index.\n" +
			"Use --registry to clear only the registry index (forces a refetch on next use).",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			lock, err := a.lock()
			if err != nil {
				return a.friendlyError(err)
			}
			defer lock.Release()

			var freed int64
			var files int
			if size := a.registry.CacheSize(); size > 0 {
				files++
				if dryRun {
					fmt.Fprintf(os.Stderr, "would remove registry cache in %s (%s)\n", a.paths.RegistryCacheDir, ui.FormatBytes(size))
					freed += size
				} else {
					n, err := a.registry.ClearCache()
					freed += n
					if err != nil {
						return a.friendlyError(err)
					}
				}
			}
			if registryOnly {
				return reportFreed(dryRun, freed, files)
			}

			err = filepath.Walk(a.paths.DownloadDir, func(path string, info os.FileInfo, err error) error {
				if err != nil || info.IsDir() {
					return err
				}
				freed += info.Size()
				files++
				if dryRun {
					fmt.Fprintf(os.Stderr, "would remove %s (%s)\n", path, ui.FormatBytes(info.Size()))
					return nil
				}
				return os.Remove(path)
			})
			if err != nil && !os.IsNotExist(err) {
				return a.friendlyError(err)
			}

			return reportFreed(dryRun, freed, files)
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show what would be removed without deleting")
	cmd.Flags().BoolVar(&registryOnly, "registry", false, "only clear the cached registry index")
	return cmd
}

func reportFreed(dryRun bool, freed int64, files int) error {
	verb := "freed"
	if dryRun {
		verb = "would free"
	}
	fmt.Fprintf(os.Stderr, "%s %s %s across %d item(s)\n",
		ui.StyleGood.Render(ui.GlyphDone), verb, ui.StyleBold.Render(ui.FormatBytes(freed)), files)
	return nil
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version, commit, and build date",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println(versionLine())
		},
	}
}
