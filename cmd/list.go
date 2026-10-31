package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"

	"github.com/spf13/cobra"

	"github.com/cmerk2021/cmam/internal/github"
	"github.com/cmerk2021/cmam/internal/state"
	"github.com/cmerk2021/cmam/internal/ui"
)

type listEntry struct {
	Name            string `json:"name"`
	Installed       string `json:"installed"`
	Latest          string `json:"latest,omitempty"`
	UpdateAvailable bool   `json:"update_available"`
	Path            string `json:"path"`
	Repo            string `json:"repo"`
}

func newListCmd(a *app) *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List installed apps, versions, and available updates",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			installed, err := a.state.Load()
			if err != nil {
				return a.friendlyError(err)
			}
			if len(installed) == 0 {
				if jsonOut {
					fmt.Println("[]")
					return nil
				}
				fmt.Fprintln(os.Stderr, ui.StyleSubtle.Render("nothing installed yet — try `cmam install <app>`"))
				return nil
			}

			entries := buildListEntries(cmd, a, installed)
			if jsonOut {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(entries)
			}

			t := ui.Table{Headers: []string{"Name", "Installed", "Latest", "Path"}}
			for _, e := range entries {
				latest := e.Latest
				if latest == "" {
					latest = "?"
				} else if e.UpdateAvailable {
					latest = ui.UpdateArrow(e.Installed, e.Latest)
				}
				t.Rows = append(t.Rows, []string{ui.StyleAppName.Render(e.Name), e.Installed, latest, ui.StyleSubtle.Render(e.Path)})
			}
			fmt.Println(t.Render())
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "output JSON")
	return cmd
}

func buildListEntries(cmd *cobra.Command, a *app, installed map[string]state.Record) []listEntry {
	names := make([]string, 0, len(installed))
	for n := range installed {
		names = append(names, n)
	}
	sort.Strings(names)

	entries := make([]listEntry, len(names))
	var wg sync.WaitGroup
	for i, name := range names {
		rec := installed[name]
		entries[i] = listEntry{Name: name, Installed: rec.Version, Path: rec.BinaryPath, Repo: rec.Repo}
		wg.Add(1)
		go func(i int, repo string) {
			defer wg.Done()
			if latest, err := a.gh.LatestVersion(cmd.Context(), repo); err == nil {
				entries[i].Latest = latest
				entries[i].UpdateAvailable = github.CompareVersions(latest, entries[i].Installed) > 0
			}
		}(i, rec.Repo)
	}
	wg.Wait()
	return entries
}

func newSearchCmd(a *app) *cobra.Command {
	var jsonOut, refresh bool
	cmd := &cobra.Command{
		Use:   "search [query]",
		Short: "Browse or search the registry",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			res, err := a.registry.Load(cmd.Context(), refresh)
			if err != nil {
				return a.friendlyError(err)
			}
			if res.Stale {
				fmt.Fprintln(os.Stderr, ui.StyleSubtle.Render(
					fmt.Sprintf("using cached registry from %s ago (network unavailable)", res.Age.Round(1e9))))
			}

			installed, _ := a.state.Load()
			query := ""
			if len(args) == 1 {
				query = strings.ToLower(args[0])
			}

			type hit struct {
				Name        string `json:"name"`
				Description string `json:"description"`
				Repo        string `json:"repo"`
				Installed   string `json:"installed,omitempty"`
			}
			var hits []hit
			for _, m := range res.Apps {
				if query != "" && !strings.Contains(strings.ToLower(m.Name), query) &&
					!strings.Contains(strings.ToLower(m.Description), query) {
					continue
				}
				h := hit{Name: m.Name, Description: m.Description, Repo: m.Repo}
				if rec, ok := installed[m.Name]; ok {
					h.Installed = rec.Version
				}
				hits = append(hits, h)
			}

			if jsonOut {
				if hits == nil {
					hits = []hit{}
				}
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(hits)
			}
			if len(hits) == 0 {
				fmt.Fprintln(os.Stderr, ui.StyleSubtle.Render("no apps match"))
				return nil
			}
			t := ui.Table{Headers: []string{"Name", "Installed", "Repo", "Description"}}
			for _, h := range hits {
				inst := ""
				if h.Installed != "" {
					inst = ui.StyleGood.Render("v" + h.Installed)
				}
				t.Rows = append(t.Rows, []string{ui.StyleAppName.Render(h.Name), inst, ui.StyleSubtle.Render(h.Repo), h.Description})
			}
			fmt.Println(t.Render())
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "output JSON")
	cmd.Flags().BoolVar(&refresh, "refresh", false, "force a registry refetch")
	return cmd
}

func newInfoCmd(a *app) *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:               "info <app>",
		Short:             "Show details about an app",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeRegistryApps(a),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			m, err := a.registry.Get(cmd.Context(), name, false)
			if err != nil {
				return a.fail(err.Error(), err, "run `cmam search` to browse available apps")
			}
			latest, latestErr := a.gh.LatestVersion(cmd.Context(), m.Repo)

			installed, _ := a.state.Load()
			rec, isInstalled := installed[name]

			if jsonOut {
				out := map[string]any{
					"name":        m.Name,
					"description": m.Description,
					"repo":        m.Repo,
					"installed":   isInstalled,
				}
				if latestErr == nil {
					out["latest"] = latest
				}
				if isInstalled {
					out["installed_version"] = rec.Version
					out["path"] = rec.BinaryPath
				}
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(out)
			}

			var b strings.Builder
			b.WriteString(ui.StyleAppName.Render(m.Name) + "\n")
			b.WriteString(m.Description + "\n\n")
			b.WriteString(ui.StyleSubtle.Render("repo    ") + "https://github.com/" + m.Repo + "\n")
			if latestErr == nil {
				b.WriteString(ui.StyleSubtle.Render("latest  ") + "v" + latest + "\n")
			} else {
				b.WriteString(ui.StyleSubtle.Render("latest  ") + ui.StyleWarn.Render("unknown: "+latestErr.Error()) + "\n")
			}
			if isInstalled {
				status := ui.StyleGood.Render("installed v" + rec.Version)
				if latestErr == nil && github.CompareVersions(latest, rec.Version) > 0 {
					status += "  " + ui.UpdateArrow("v"+rec.Version, "v"+latest)
				}
				b.WriteString(ui.StyleSubtle.Render("status  ") + status + "\n")
				b.WriteString(ui.StyleSubtle.Render("path    ") + rec.BinaryPath)
			} else {
				b.WriteString(ui.StyleSubtle.Render("status  ") + "not installed")
			}
			fmt.Println(ui.StyleCard.Render(b.String()))
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "output JSON")
	return cmd
}
