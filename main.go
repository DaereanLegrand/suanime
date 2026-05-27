package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"suanime/providers"
	"suanime/tui"

	tea "github.com/charmbracelet/bubbletea"
)

var (
	version = "1.0.0"
	commit  = "dev"
)

var noCover bool

func main() {
	args := os.Args[1:]

	if len(args) == 0 {
		runTUI()
		return
	}

	var filtered []string
	for _, a := range args {
		if a == "--no-cover" {
			noCover = true
			continue
		}
		filtered = append(filtered, a)
	}

	if len(filtered) == 0 {
		runTUI()
		return
	}

	switch filtered[0] {
	case "tui":
		runTUI()
	case "search":
		runSearch(filtered[1:])
	case "get":
		runGet(filtered[1:])
	case "version", "--version", "-v":
		fmt.Printf("suanime %s (%s)\n", version, commit)
	case "--help", "-h", "help":
		printHelp()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", args[0])
		printHelp()
		os.Exit(1)
	}
}

func printHelp() {
	fmt.Printf("suanime %s (%s) - anime torrent search & download\n\n", version, commit)
	fmt.Println(`Usage:
  suanime [--no-cover]               Launch TUI
  suanime [--no-cover] tui           Launch TUI
  suanime search <query>             Search torrents (JSON output)
  suanime get <query>                Search and download
                           --ep N         Filter by episode
                           --resolution R Filter by resolution (720p,1080p)
                           --no-batch     Exclude batch releases
                           --best         Auto-pick best match
  suanime version                    Print version

Flags:
  --no-cover   Skip cover art (works in any terminal, not just kitty)

Examples:
  suanime --no-cover
  suanime search "tokyo ghoul"
  suanime get "tokyo ghoul" --ep 1 --best
  suanime get "tokyo ghoul" --ep 1 --resolution 1080p --no-batch --best

Config: ~/.config/suanime/config.json
Downloads: ~/Downloads/suanime/`)
}

func runSearch(args []string) {
	query := strings.Join(args, " ")
	if query == "" {
		fmt.Fprintln(os.Stderr, "usage: suanime search <query>")
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	results := providers.SearchAll(ctx, query)

	output := struct {
		Query   string                   `json:"query"`
		Count   int                      `json:"count"`
		Results []*providers.AnimeTorrent `json:"results"`
	}{
		Query:   query,
		Count:   len(results),
		Results: results,
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(output); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func runGet(args []string) {
	var queryParts []string
	var epFilter int
	var resFilter string
	var noBatch, autoBest bool

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--ep":
			i++
			if i < len(args) {
				fmt.Sscanf(args[i], "%d", &epFilter)
			}
		case "--resolution":
			i++
			if i < len(args) {
				resFilter = strings.ToUpper(args[i])
			}
		case "--no-batch":
			noBatch = true
		case "--best":
			autoBest = true
		default:
			queryParts = append(queryParts, args[i])
		}
	}

	query := strings.Join(queryParts, " ")
	if query == "" {
		fmt.Fprintln(os.Stderr, "usage: suanime get <query> [--ep N] [--resolution 1080p] [--no-batch] [--best]")
		os.Exit(1)
	}

	cfg, err := tui.LoadConfig()
	if err != nil {
		cfg = tui.DefaultConfig()
	}

	fmt.Fprintf(os.Stderr, "searching: %s\n", query)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	results := providers.SearchAll(ctx, query)
	fmt.Fprintf(os.Stderr, "found %d results\n", len(results))

	candidates := filterResults(results, epFilter, resFilter, noBatch)
	if len(candidates) == 0 {
		fmt.Fprintf(os.Stderr, "no matching results found\n")
		os.Exit(1)
	}

	var chosen *providers.AnimeTorrent
	if autoBest {
		chosen = candidates[0]
	} else {
		chosen = pickInteractive(candidates)
	}

	if chosen == nil {
		fmt.Fprintf(os.Stderr, "no selection made\n")
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "\nchosen: [%s] %s\n", chosen.Provider, TruncateCLI2(chosen.Name, 80))
	fmt.Fprintf(os.Stderr, "  episode: %d, resolution: %s, size: %s\n", chosen.Episode, chosen.Resolution, providers.FormatSize(chosen.SizeBytes))
	fmt.Fprintf(os.Stderr, "  seeders: %d, leechers: %d\n", chosen.Seeders, chosen.Leechers)

	os.MkdirAll(cfg.DownloadDir, 0755)
	dm := tui.NewDownloadManager(cfg)

	if err := dm.StartDaemon(); err != nil {
		fmt.Fprintf(os.Stderr, "error starting aria2: %v\n", err)
		os.Exit(1)
	}
	dm.RecoverOrphans()
	dm.SyncFromAria2()
	defer dm.StopDaemon()

	if err := dm.AddDownload(chosen.Name, chosen.MagnetLink, chosen.Seeders, chosen.Leechers); err != nil {
		fmt.Fprintf(os.Stderr, "error adding download: %v\n", err)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "downloading to: %s\n", cfg.DownloadDir)

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	deadline := time.After(10 * time.Minute)
	metaDeadline := time.After(2 * time.Minute)
	seenFileDownload := false

	for {
		select {
		case <-deadline:
			fmt.Fprintf(os.Stderr, "\ntimed out after 10 minutes\n")
			os.Exit(1)
		case <-metaDeadline:
			items := dm.GetItems()
			allMeta := true
			for _, d := range items {
				if d.Status != "metadata" {
					allMeta = false
				}
			}
			if allMeta && len(items) > 0 {
				fmt.Fprintf(os.Stderr, "\nmetadata timeout (2m) - no seeders available\n")
				os.Exit(1)
			}
		case <-ticker.C:
			dm.Poll()
			items := dm.GetItems()
			for _, d := range items {
				switch d.Status {
				case "running":
					seenFileDownload = true
					fmt.Fprintf(os.Stderr, "\r  %s  %.0f%%  %s/%s  %s/s    ",
						TruncateCLI2(d.Name, 40),
						d.Progress,
						providers.FormatSize(d.Completed),
						providers.FormatSize(d.TotalSize),
						providers.FormatSize(d.Speed),
					)
				case "completed":
					if d.TotalSize > 1*1024*1024 || seenFileDownload {
						fmt.Fprintf(os.Stderr, "\ncompleted: %s (%s)\n", d.Name, providers.FormatSize(d.TotalSize))
						return
					}
				case "failed":
					if d.TotalSize > 0 || d.Err != "" {
						fmt.Fprintf(os.Stderr, "\nfailed: %s - %s\n", d.Name, d.Err)
						os.Exit(1)
					}
				case "metadata":
					fmt.Fprintf(os.Stderr, "\r  fetching metadata...    ")
				}
			}
		}
	}
}

var reOVA = regexp.MustCompile(`(?i)\b(ova|ona|special|movie|film|recap)\b`)

func filterResults(results []*providers.AnimeTorrent, ep int, res string, noBatch bool) []*providers.AnimeTorrent {
	var filtered, ovaFiltered []*providers.AnimeTorrent
	for _, r := range results {
		if r.MagnetLink == "" {
			continue
		}
		if res != "" && !strings.EqualFold(r.Resolution, res) {
			continue
		}
		if noBatch && r.IsBatch {
			continue
		}
		if ep > 0 && r.Episode != ep {
			continue
		}
		if r.Seeders == 0 {
			continue
		}
		if ep > 0 && reOVA.MatchString(r.Name) {
			ovaFiltered = append(ovaFiltered, r)
			continue
		}
		filtered = append(filtered, r)
	}
	if len(filtered) == 0 {
		return ovaFiltered
	}
	return filtered
}

func pickInteractive(candidates []*providers.AnimeTorrent) *providers.AnimeTorrent {
	if len(candidates) == 1 {
		return candidates[0]
	}

	if _, err := exec.LookPath("gum"); err == nil {
		return pickWithGum(candidates)
	}

	fmt.Fprintf(os.Stderr, "gum not found (install for interactive pick), auto-selecting best match\n")
	return candidates[0]
}

func pickWithGum(candidates []*providers.AnimeTorrent) *providers.AnimeTorrent {
	limit := min2(len(candidates), 50)
	var items []string
	for _, c := range candidates[:limit] {
		epStr := fmt.Sprintf("%3d", c.Episode)
		if c.Episode == 0 {
			epStr = "BCH"
		}
		items = append(items, fmt.Sprintf("[%s] Ep:%s S:%d %s %s",
			c.Provider, epStr, c.Seeders,
			providers.FormatSize(c.SizeBytes),
			TruncateCLI2(c.Name, 50),
		))
	}

	cmd := exec.Command("gum", "choose",
		"--header", "Choose a torrent to download:",
		"--height", "20",
	)
	cmd.Stdin = strings.NewReader(strings.Join(items, "\n"))
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		fmt.Fprintf(os.Stderr, "gum cancelled, auto-selecting best match\n")
		return candidates[0]
	}

	choice := strings.TrimSpace(string(out))
	for i, c := range candidates[:limit] {
		epStr := fmt.Sprintf("%3d", c.Episode)
		if c.Episode == 0 {
			epStr = "BCH"
		}
		label := fmt.Sprintf("[%s] Ep:%s S:%d %s %s",
			c.Provider, epStr, c.Seeders,
			providers.FormatSize(c.SizeBytes),
			TruncateCLI2(c.Name, 50),
		)
		if label == choice {
			return candidates[i]
		}
	}
	return candidates[0]
}

func runTUI() {
	if !noCover && !tui.IsKitty() {
		fmt.Fprintf(os.Stderr, "suanime requires the kitty terminal (use --no-cover to skip cover art).\n")
		fmt.Fprintf(os.Stderr, "Current TERM: %s\n", os.Getenv("TERM"))
		os.Exit(1)
	}

	cfg, err := tui.LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: %v\n", err)
		cfg = tui.DefaultConfig()
	}

	if !tui.RequiresAria2() {
		fmt.Fprintf(os.Stderr, "Warning: aria2c not found. Downloads will not work.\n")
		fmt.Fprintf(os.Stderr, "Install: pacman -S aria2 | apt install aria2 | brew install aria2\n\n")
	}

	os.MkdirAll(cfg.DownloadDir, 0755)

	m := tui.NewModel(cfg, noCover)
	dm := m.DownloadManager()

	if err := dm.StartDaemon(); err != nil {
		m.DaemonErr = fmt.Sprintf("aria2: %v", err)
	} else {
		dm.RecoverOrphans()
		dm.SyncFromAria2()
	}
	defer dm.StopDaemon()

	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func TruncateCLI2(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n-1]) + "…"
}

func min2(a, b int) int {
	if a < b {
		return a
	}
	return b
}
