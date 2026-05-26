package tui

import (
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"

	"suanime/providers"

	"github.com/charmbracelet/lipgloss"
)

const (
	StatusRunning   = "running"
	StatusPaused    = "paused"
	StatusCompleted = "completed"
	StatusFailed    = "failed"
	StatusWaiting   = "waiting"
)

type DownloadItem struct {
	GID         string
	Name        string
	Magnet      string
	Started     time.Time
	Status      string
	Progress    float64
	Speed       int64
	TotalSize   int64
	Completed   int64
	Err         string
	Files       string
}

type DownloadManager struct {
	aria2  *aria2Client
	items  []*DownloadItem
	mu     sync.Mutex
	config Config
	daemon *exec.Cmd
}

func NewDownloadManager(cfg Config) *DownloadManager {
	return &DownloadManager{
		aria2:  newAria2Client(cfg.Aria2RPCPort, cfg.Aria2RPCSecret),
		items:  []*DownloadItem{},
		config: cfg,
	}
}

func (dm *DownloadManager) StartDaemon() error {
	port := fmt.Sprintf("%d", dm.config.Aria2RPCPort)
	args := []string{
		"--enable-rpc",
		"--rpc-listen-port=" + port,
		"--rpc-allow-origin-all",
		"--rpc-listen-all=false",
		"--seed-time=0",
		"--summary-interval=0",
		"--console-log-level=error",
		"--save-session=" + dm.sessionFile(),
		"--save-session-interval=10",
		"--dir=" + dm.config.DownloadDir,
	}
	if dm.config.Aria2RPCSecret != "" {
		args = append(args, "--rpc-secret="+dm.config.Aria2RPCSecret)
	}
	cmd := exec.Command("aria2c", args...)
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("aria2 daemon: %w", err)
	}
	dm.daemon = cmd
	time.Sleep(500 * time.Millisecond)
	return nil
}

func (dm *DownloadManager) sessionFile() string {
	return dm.config.DownloadDir + "/.suanime-aria2.session"
}

func (dm *DownloadManager) StopDaemon() {
	if dm.daemon != nil && dm.daemon.Process != nil {
		dm.aria2.call("aria2.shutdown", []interface{}{})
		dm.daemon.Process.Kill()
	}
}

func (dm *DownloadManager) AddDownload(name, magnet string) error {
	gid, err := dm.aria2.AddURI(magnet, dm.config.DownloadDir)
	if err != nil {
		return err
	}
	dm.mu.Lock()
	dm.items = append(dm.items, &DownloadItem{
		GID:     gid,
		Name:    name,
		Magnet:  magnet,
		Started: time.Now(),
		Status:  StatusRunning,
	})
	dm.mu.Unlock()
	return nil
}

func (dm *DownloadManager) Pause(gid string) error {
	return dm.aria2.Pause(gid)
}

func (dm *DownloadManager) Resume(gid string) error {
	return dm.aria2.Unpause(gid)
}

func (dm *DownloadManager) Cancel(gid string) error {
	dm.aria2.Remove(gid)
	dm.aria2.RemoveResult(gid)
	dm.mu.Lock()
	defer dm.mu.Unlock()
	for i, d := range dm.items {
		if d.GID == gid {
			dm.items = append(dm.items[:i], dm.items[i+1:]...)
			break
		}
	}
	return nil
}

func (dm *DownloadManager) Poll() {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	if dm.aria2 == nil {
		return
	}

	active, _ := dm.aria2.TellActive()
	waited, _ := dm.aria2.TellWaiting(0, 100)
	stopped, _ := dm.aria2.TellStopped(0, 100)

	all := append(active, waited...)
	all = append(all, stopped...)

	seen := map[string]bool{}
	var updated []*DownloadItem

	for _, ts := range all {
		seen[ts.GID] = true
		completed := parseLength(ts.CompletedLength)
		total := parseLength(ts.TotalLength)
		speed := parseLength(ts.DownloadSpeed)
		pct := progressPct(completed, total)

		name := ts.GID
		if ts.Bittorrent != nil && ts.Bittorrent.Info.Name != "" {
			name = ts.Bittorrent.Info.Name
		}
		files := ""
		if len(ts.Files) > 0 {
			files = ts.Files[0].Path
		}

		var existing *DownloadItem
		for _, d := range dm.items {
			if d.GID == ts.GID {
				existing = d
				break
			}
		}

		if existing != nil {
			if name != existing.GID {
				existing.Name = name
			}
			existing.Progress = pct
			existing.Speed = speed
			existing.TotalSize = total
			existing.Completed = completed
			existing.Files = files
			switch ts.Status {
			case "active":
				existing.Status = StatusRunning
			case "paused":
				existing.Status = StatusPaused
			case "waiting":
				existing.Status = StatusWaiting
			case "complete":
				existing.Status = StatusCompleted
			case "error":
				existing.Status = StatusFailed
				existing.Err = ts.ErrorMessage
			case "removed":
			}
			updated = append(updated, existing)
		} else {
			status := StatusRunning
			switch ts.Status {
			case "paused":
				status = StatusPaused
			case "waiting":
				status = StatusWaiting
			case "complete":
				status = StatusCompleted
			case "error":
				status = StatusFailed
			}
			updated = append(updated, &DownloadItem{
				GID:       ts.GID,
				Name:      name,
				Started:   time.Now(),
				Status:    status,
				Progress:  pct,
				Speed:     speed,
				TotalSize: total,
				Completed: completed,
				Files:     files,
				Err:       ts.ErrorMessage,
			})
		}
	}

	for _, d := range dm.items {
		if !seen[d.GID] && d.Status == StatusCompleted {
			updated = append(updated, d)
		}
	}

	dm.items = updated
}

func (dm *DownloadManager) GetItems() []*DownloadItem {
	dm.mu.Lock()
	defer dm.mu.Unlock()
	out := make([]*DownloadItem, len(dm.items))
	copy(out, dm.items)
	return out
}

func (m *Model) downloadsView() string {
	items := m.dlManager.GetItems()

	var parts []string
	parts = append(parts, accentStyle.Bold(true).Render("Downloads"))
	parts = append(parts, "")

	if len(items) == 0 {
		parts = append(parts, subtleStyle.Render("no active downloads"))
		return lipgloss.JoinVertical(lipgloss.Left, parts...)
	}

	for i, d := range items {
		cursor := " "
		if i == m.downCursor {
			cursor = ">"
		}

		name := Truncate(d.Name, max(30, m.width-10))
		var statusLine string
		switch d.Status {
		case StatusRunning:
			bar := progressBar(d.Progress, 30)
			speedStr := providers.FormatSize(d.Speed) + "/s"
			eta := ""
			if d.Speed > 0 && d.TotalSize > d.Completed {
				remaining := d.TotalSize - d.Completed
				etaSec := remaining / d.Speed
				eta = fmt.Sprintf("ETA %s", time.Duration(etaSec)*time.Second)
			}
			statusLine = fmt.Sprintf("%s %s  %5.1f%%  %s  %s",
				goodStyle.Render(bar),
				subtleStyle.Render(speedStr),
				d.Progress,
				subtleStyle.Render(eta),
				accentStyle.Render("active"),
			)
		case StatusPaused:
			bar := progressBar(d.Progress, 30)
			statusLine = fmt.Sprintf("%s  %5.1f%%  %s",
				warnStyle.Render(bar),
				d.Progress,
				warnStyle.Render("paused"),
			)
		case StatusWaiting:
			statusLine = fmt.Sprintf("%s  %s",
				subtleStyle.Render(strings.Repeat(" ", 30)),
				warnStyle.Render("waiting"),
			)
		case StatusCompleted:
			bar := progressBar(100, 30)
			statusLine = fmt.Sprintf("%s  %s  %s",
				goodStyle.Render(bar),
				goodStyle.Render("100%"),
				goodStyle.Render("completed"),
			)
		case StatusFailed:
			bar := progressBar(d.Progress, 30)
			statusLine = fmt.Sprintf("%s  %s",
				badStyle.Render(bar),
				badStyle.Render("failed: "+d.Err),
			)
		}

		entry := fmt.Sprintf("%s  %s\n    %s", cursor, name, statusLine)

		if i == m.downCursor {
			parts = append(parts, selectedStyle.Render(entry))
		} else {
			parts = append(parts, normalStyle.Render(entry))
		}
	}

	parts = append(parts, "")
	parts = append(parts, subtleStyle.Render("p: pause  r: resume  c: cancel  d: remove  esc: back"))

	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func RequiresAria2() bool {
	_, err := exec.LookPath("aria2c")
	return err == nil
}
