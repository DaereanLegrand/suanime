package main

import (
	"fmt"
	"os"

	"suanime/tui"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	cfg, err := tui.LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: %v\n", err)
		cfg = tui.DefaultConfig()
	}
	fmt.Fprintf(os.Stderr, "config: %s\n", tui.ConfigFilePath())

	if !tui.RequiresAria2() {
		fmt.Fprintf(os.Stderr, "Warning: aria2c not found. Downloads will not work.\n")
		fmt.Fprintf(os.Stderr, "Install: pacman -S aria2 | apt install aria2 | brew install aria2\n\n")
	}

	os.MkdirAll(cfg.DownloadDir, 0755)

	m := tui.NewModel(cfg)
	dm := m.DownloadManager()

	if err := dm.StartDaemon(); err != nil {
		fmt.Fprintf(os.Stderr, "warning: aria2 daemon: %v\n", err)
	}
	defer dm.StopDaemon()

	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
