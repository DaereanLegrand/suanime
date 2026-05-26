package tui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"suanime/providers"

	"github.com/charmbracelet/lipgloss"
)

const (
	StatusRunning   = "running"
	StatusMeta      = "metadata"
	StatusPaused    = "paused"
	StatusCompleted = "completed"
	StatusFailed    = "failed"
	StatusWaiting   = "waiting"
)

type DownloadItem struct {
	GID       string
	Name      string
	Magnet    string
	Started   time.Time
	Status    string
	Progress  float64
	Speed     int64
	TotalSize int64
	Completed int64
	Err       string
	Files     string
	NumFiles  int
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
	dm.killExistingAria2()

	os.MkdirAll(dm.config.DownloadDir, 0755)
	sessionFile := dm.sessionFile()
	os.Remove(sessionFile)
	os.Remove(sessionFile + "_old")

	port := fmt.Sprintf("%d", dm.config.Aria2RPCPort)
	args := []string{
		"--enable-rpc",
		"--rpc-listen-port=" + port,
		"--rpc-allow-origin-all",
		"--rpc-listen-all=false",
		"--seed-time=0",
		"--summary-interval=0",
		"--console-log-level=error",
		"--file-allocation=none",
		"--save-session=" + sessionFile,
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

func (dm *DownloadManager) killExistingAria2() {
	c := newAria2Client(dm.config.Aria2RPCPort, dm.config.Aria2RPCSecret)
	c.call("aria2.shutdown", []interface{}{})
	time.Sleep(200 * time.Millisecond)
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
		Status:  StatusMeta,
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

func (dm *DownloadManager) FindGID(gid string) *DownloadItem {
	dm.mu.Lock()
	defer dm.mu.Unlock()
	for _, d := range dm.items {
		if d.GID == gid {
			return d
		}
	}
	return nil
}

func (dm *DownloadManager) Cancel(gid string) error {
	if err := dm.aria2.Remove(gid); err != nil {
		if err2 := dm.aria2.ForceRemove(gid); err2 != nil {
			return fmt.Errorf("remove: %w / force: %w", err, err2)
		}
	}
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
	btNames := map[string]*DownloadItem{}

	for _, ts := range all {
		seen[ts.GID] = true
		completed := parseLength(ts.CompletedLength)
		total := parseLength(ts.TotalLength)
		speed := parseLength(ts.DownloadSpeed)
		pct := progressPct(completed, total)

		numFiles := len(ts.Files)
		btName := ""
		if ts.Bittorrent != nil && ts.Bittorrent.Info.Name != "" {
			btName = ts.Bittorrent.Info.Name
		}

		var existing *DownloadItem
		for _, d := range dm.items {
			if d.GID == ts.GID {
				existing = d
				break
			}
		}

		if existing == nil && btName != "" {
			for _, d := range dm.items {
				if d.Name != "" && nameOverlap(d.Name, btName) {
					existing = d
					break
				}
			}
		}

		if existing == nil && total > 0 && len(ts.Files) > 0 {
			dirName := filepath.Base(filepath.Dir(ts.Files[0].Path))
			for _, d := range dm.items {
				if (d.Status == StatusMeta || d.Status == StatusWaiting) && d.Name != "" {
					if nameOverlap(d.Name, dirName) {
						existing = d
						break
					}
				}
			}
		}

		if existing == nil && total > 0 {
			pending := 0
			var lastPending *DownloadItem
			for _, d := range dm.items {
				if d.Status == StatusMeta || d.Status == StatusWaiting {
					pending++
					lastPending = d
				}
			}
			if pending == 1 && lastPending != nil {
				existing = lastPending
			}
		}

		if existing != nil {
			existing.GID = ts.GID
			if btName != "" {
				existing.Name = btName
			}
			existing.Speed = speed
			existing.TotalSize = total
			existing.Completed = completed
			existing.NumFiles = numFiles
			existing.Progress = pct
			if numFiles > 0 && ts.Files[0].Path != "" {
				existing.Files = ts.Files[0].Path
			}

			switch ts.Status {
			case "active":
				if total == 0 {
					existing.Status = StatusMeta
				} else {
					existing.Status = StatusRunning
				}
			case "paused":
				existing.Status = StatusPaused
			case "waiting":
				existing.Status = StatusWaiting
			case "complete":
				if completed > 0 && total > 0 && completed >= total {
					if total > 100*1024 || existing.TotalSize > 100*1024 {
						existing.Status = StatusCompleted
						existing.Progress = 100
					}
				} else if completed == 0 && total == 0 {
					existing.Status = StatusFailed
					existing.Err = "no data (no seeders?)"
				}
			case "error":
				existing.Status = StatusFailed
				existing.Err = ts.ErrorMessage
			case "removed":
			}
			updated = append(updated, existing)
			if btName != "" {
				btNames[btName] = existing
			}
		} else {
			name := btName
			if name == "" {
				name = ts.GID
			}
			status := StatusMeta
			switch ts.Status {
			case "active":
				if total == 0 {
					status = StatusMeta
				} else {
					status = StatusRunning
				}
			case "paused":
				status = StatusPaused
			case "waiting":
				status = StatusWaiting
			case "complete":
				if completed > 0 && total > 0 && completed >= total && total > 100*1024 {
					status = StatusCompleted
				} else if total <= 100*1024 {
					status = StatusMeta
				}
			case "error":
				status = StatusFailed
			}
			item := &DownloadItem{
				GID:       ts.GID,
				Name:      name,
				Started:   time.Now(),
				Status:    status,
				Progress:  pct,
				Speed:     speed,
				TotalSize: total,
				Completed: completed,
				NumFiles:  numFiles,
				Err:       ts.ErrorMessage,
			}
			if numFiles > 0 && ts.Files[0].Path != "" {
				item.Files = ts.Files[0].Path
			}
			updated = append(updated, item)
			if btName != "" {
				btNames[btName] = item
			}
		}
	}

	for _, d := range dm.items {
		if !seen[d.GID] {
			if d.Status == StatusMeta || d.Status == StatusRunning || d.Status == StatusWaiting || d.Status == StatusPaused {
				updated = append(updated, d)
			}
		}
	}

	deduped := make([]*DownloadItem, 0, len(updated))
	gids := map[string]bool{}
	names := map[string]bool{}
	for _, d := range updated {
		if d.GID != "" && !gids[d.GID] {
			gids[d.GID] = true
			names[d.Name] = true
			deduped = append(deduped, d)
		}
	}
	dm.items = deduped
	_ = btNames
	_ = names
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
			cursor = accentStyle.Render(">")
		}

		name := Truncate(d.Name, max(30, m.width-10))
		barWidth := min(30, m.width-40)

		var statusLine string
		switch d.Status {
		case StatusMeta:
			statusLine = fmt.Sprintf("%s  %s",
				warnStyle.Render(strings.Repeat("░", barWidth)),
				loadingStyle.Render("fetching metadata..."),
			)
		case StatusRunning:
			bar := progressBar(d.Progress, barWidth)
			speedStr := ""
			if d.Speed > 0 {
				speedStr = providers.FormatSize(d.Speed) + "/s"
			}
			eta := ""
			if d.Speed > 0 && d.TotalSize > d.Completed {
				remaining := d.TotalSize - d.Completed
				etaSec := remaining / d.Speed
				eta = fmt.Sprintf("ETA %s", time.Duration(etaSec)*time.Second)
			}
			done := providers.FormatSize(d.Completed)
			total := providers.FormatSize(d.TotalSize)
			statusLine = fmt.Sprintf("%s  %s/%s  %s  %s  %s",
				goodStyle.Render(bar),
				goodStyle.Render(done),
				subtleStyle.Render(total),
				subtleStyle.Render(speedStr),
				subtleStyle.Render(eta),
				accentStyle.Render(fmt.Sprintf("%.0f%%", d.Progress)),
			)
		case StatusPaused:
			bar := progressBar(d.Progress, barWidth)
			statusLine = fmt.Sprintf("%s  %.0f%%  %s",
				warnStyle.Render(bar),
				d.Progress,
				warnStyle.Render("paused"),
			)
		case StatusWaiting:
			statusLine = fmt.Sprintf("%s  %s",
				subtleStyle.Render(strings.Repeat("░", barWidth)),
				warnStyle.Render("waiting in queue"),
			)
		case StatusCompleted:
			bar := progressBar(100, barWidth)
			size := providers.FormatSize(d.TotalSize)
			statusLine = fmt.Sprintf("%s  %s  %s",
				goodStyle.Render(bar),
				goodStyle.Render(size),
				goodStyle.Render("completed"),
			)
		case StatusFailed:
			bar := progressBar(d.Progress, barWidth)
			err := d.Err
			if err == "" {
				err = "unknown error"
			}
			statusLine = fmt.Sprintf("%s  %s",
				badStyle.Render(bar),
				badStyle.Render("failed: "+Truncate(err, 40)),
			)
		}

		entry := fmt.Sprintf("%s %s\n  %s", cursor, name, statusLine)

		if i == m.downCursor {
			parts = append(parts, selectedStyle.Render(entry))
		} else {
			parts = append(parts, normalStyle.Render(entry))
		}
	}

	parts = append(parts, "")
	parts = append(parts, mutedStyle.Render("p: pause  r: resume  c: cancel  d: remove  esc: back"))

	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func nameOverlap(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	aLower := strings.ToLower(a)
	bLower := strings.ToLower(b)
	if strings.Contains(aLower, bLower) || strings.Contains(bLower, aLower) {
		return true
	}
	aWords := strings.FieldsFunc(aLower, func(r rune) bool {
		return r == ' ' || r == '-' || r == '_' || r == '.' || r == '[' || r == ']' || r == '(' || r == ')'
	})
	bWords := strings.FieldsFunc(bLower, func(r rune) bool {
		return r == ' ' || r == '-' || r == '_' || r == '.' || r == '[' || r == ']' || r == '(' || r == ')'
	})
	if len(aWords) >= 2 && len(bWords) >= 2 {
		match := 0
		for _, aw := range aWords {
			for _, bw := range bWords {
				if aw == bw && len(aw) > 2 {
					match++
					break
				}
			}
		}
		return match >= 2
	}
	return false
}

func RequiresAria2() bool {
	_, err := exec.LookPath("aria2c")
	return err == nil
}
