package tui

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
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
	StatusSeeding   = "seeding"
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
	RPCErr string
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

	port := fmt.Sprintf("%d", dm.config.Aria2RPCPort)
	args := []string{
		"--enable-rpc",
		"--rpc-listen-port=" + port,
		"--rpc-allow-origin-all",
		"--rpc-listen-all=false",
		"--check-integrity=true",
		"--seed-ratio=0.0",
		"--summary-interval=0",
		"--console-log-level=error",
		"--file-allocation=none",
		"--save-session=" + sessionFile,
		"--save-session-interval=10",
		"--dir=" + dm.config.DownloadDir,
	}
	if _, err := os.Stat(sessionFile); err == nil {
		args = append(args, "--input-file="+sessionFile)
	}
	if dm.config.Aria2RPCSecret != "" {
		args = append(args, "--rpc-secret="+dm.config.Aria2RPCSecret)
	}
	var stderr bytes.Buffer
	cmd := exec.Command("aria2c", args...)
	cmd.Stdout = nil
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("aria2 daemon: %w", err)
	}
	dm.daemon = cmd

	for i := 0; i < 20; i++ {
		time.Sleep(250 * time.Millisecond)
		if _, err := dm.aria2.TellActive(); err == nil {
			return nil
		}
	}
	cmd.Process.Kill()
	return fmt.Errorf("aria2 startup timeout: %s", strings.TrimSpace(stderr.String()))
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

func (dm *DownloadManager) SyncFromAria2() {
	dm.Poll()
}

func (dm *DownloadManager) RecoverOrphans() {
	entries, err := os.ReadDir(dm.config.DownloadDir)
	if err != nil {
		return
	}

	dm.Poll()
	knownDirs := map[string]bool{}
	for _, d := range dm.items {
		knownDirs[d.Name] = true
		if d.Files != "" {
			knownDirs[filepath.Base(filepath.Dir(d.Files))] = true
		}
		if strings.HasPrefix(d.Name, "torrent:") || (len(d.Name) >= 32 && !strings.Contains(d.Name, " ")) {
			dm.mu.Lock()
			for i, item := range dm.items {
				if item.GID == d.GID {
					dm.items = append(dm.items[:i], dm.items[i+1:]...)
					break
				}
			}
			dm.mu.Unlock()
		}
	}

	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".aria2") {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), ".aria2")
		if knownDirs[name] {
			continue
		}

		hash, err := extractInfoHash(filepath.Join(dm.config.DownloadDir, entry.Name()))
		if err != nil || hash == "" {
			continue
		}

		magnet := providers.MagnetFromHash(hash, name)
		gid, err := dm.aria2.AddURI(magnet, dm.config.DownloadDir)
		if err != nil {
			continue
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
		knownDirs[name] = true
	}
}

func extractInfoHash(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	if len(data) < 30 {
		return "", fmt.Errorf("file too short")
	}
	if data[8] == 0x00 && data[9] == 0x14 {
		hash := fmt.Sprintf("%x", data[10:30])
		return hash, nil
	}
	return "", fmt.Errorf("unknown aria2 format")
}

func (dm *DownloadManager) RemoveFilesAndTorrent(gid string) error {
	dm.aria2.Remove(gid)
	dm.aria2.ForceRemove(gid)
	dm.aria2.RemoveResult(gid)

	for i := 0; i < 5; i++ {
		time.Sleep(500 * time.Millisecond)
		dm.Poll()
		found := false
		for _, d := range dm.items {
			if d.GID == gid {
				found = true
				dm.aria2.ForceRemove(d.GID)
				dm.aria2.RemoveResult(d.GID)
				break
			}
		}
		if !found {
			break
		}
	}

	dm.mu.Lock()
	for i, d := range dm.items {
		if d.GID == gid {
			if d.Files != "" {
				dir := filepath.Dir(d.Files)
				entries, _ := os.ReadDir(dir)
				for _, e := range entries {
					if strings.HasSuffix(e.Name(), ".aria2") {
						os.Remove(filepath.Join(dir, e.Name()))
					}
				}
			}
			dm.items = append(dm.items[:i], dm.items[i+1:]...)
			break
		}
	}
	dm.mu.Unlock()
	return nil
}

func (dm *DownloadManager) Poll() {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	if dm.aria2 == nil {
		return
	}

	active, errA := dm.aria2.TellActive()
	waited, errW := dm.aria2.TellWaiting(0, 100)
	stopped, errS := dm.aria2.TellStopped(0, 100)
	if dm.daemon == nil || dm.daemon.Process == nil {
		dm.RPCErr = "aria2 daemon not running"
		return
	}
	if errA != nil || errW != nil || errS != nil {
		dm.RPCErr = fmt.Sprintf("rpc: %v %v %v", errA, errW, errS)
	} else {
		dm.RPCErr = ""
	}

	all := append(active, waited...)
	all = append(all, stopped...)
	sort.Slice(all, func(i, j int) bool {
		return parseLength(all[i].TotalLength) > parseLength(all[j].TotalLength)
	})

	seen := map[string]bool{}
	var updated []*DownloadItem

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
		dirName := ""
		if numFiles > 0 && ts.Files[0].Path != "" {
			dirName = filepath.Base(filepath.Dir(ts.Files[0].Path))
		}

		existing := dm.findMatching(ts.GID, btName, dirName, total)

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
			if dirName != "" {
				existing.Files = ts.Files[0].Path
			}
			existing.Status = dm.mapStatus(ts.Status, total, completed, existing)
			updated = append(updated, existing)
		} else if !(ts.Status == "complete" && total <= 1024*1024 && completed <= 1024*1024) {
			name := btName
			if name == "" && dirName != "" {
				name = dirName
			}
			if name == "" {
				name = "torrent:" + ts.GID[:12]
			}
			item := &DownloadItem{
				GID:       ts.GID,
				Name:      name,
				Started:   time.Now(),
				Status:    dm.mapStatus(ts.Status, total, completed, nil),
				Progress:  pct,
				Speed:     speed,
				TotalSize: total,
				Completed: completed,
				NumFiles:  numFiles,
				Err:       ts.ErrorMessage,
			}
			if dirName != "" {
				item.Files = ts.Files[0].Path
			}
			updated = append(updated, item)
		}
	}

	for _, d := range dm.items {
		if !seen[d.GID] && d.Status != StatusCompleted {
			updated = append(updated, d)
		}
	}

	deduped := updated[:0]
	for _, d := range updated {
		if isHashName(d.Name) && (d.Status == StatusCompleted || d.Status == StatusMeta) {
			continue
		}
		deduped = append(deduped, d)
	}
	merged := mergeByName(deduped)
	deduped = merged[:0]
	for _, d := range merged {
		deduped = append(deduped, d)
	}

	gids := map[string]bool{}
	final := deduped[:0]
	for _, d := range deduped {
		if d.GID != "" && !gids[d.GID] {
			gids[d.GID] = true
			final = append(final, d)
		}
	}
	dm.items = final
}

func mergeByName(items []*DownloadItem) []*DownloadItem {
	if len(items) <= 1 {
		return items
	}
	seen := map[string]*DownloadItem{}
	for _, d := range items {
		if d.Name == "" || isHashName(d.Name) {
			seen[d.GID] = d
			continue
		}
		if prev, ok := seen[d.Name]; ok {
			if prev.TotalSize < d.TotalSize || (prev.Status == StatusMeta && d.Status != StatusMeta) {
				seen[d.Name] = d
			}
		} else {
			seen[d.Name] = d
		}
	}
	out := make([]*DownloadItem, 0, len(seen))
	for _, d := range seen {
		out = append(out, d)
	}
	return out
}

func (dm *DownloadManager) findMatching(tsGID, btName, dirName string, total int64) *DownloadItem {
	for _, d := range dm.items {
		if d.GID == tsGID {
			return d
		}
	}
	if btName != "" {
		for _, d := range dm.items {
			if d.Name != "" && nameOverlap(d.Name, btName) {
				return d
			}
		}
	}
	if dirName != "" && total > 0 {
		for _, d := range dm.items {
			if (d.Status == StatusMeta || d.Status == StatusWaiting) && d.Name != "" {
				if nameOverlap(d.Name, dirName) {
					return d
				}
			}
		}
	}
	if total > 1024*1024 {
		pending := 0
		var last *DownloadItem
		for _, d := range dm.items {
			if d.Status == StatusMeta || d.Status == StatusWaiting {
				pending++
				last = d
			}
		}
		if pending == 1 && last != nil {
			return last
		}
	}
	return nil
}

func (dm *DownloadManager) mapStatus(ariaStatus string, total, completed int64, existing *DownloadItem) string {
	prevTotal := int64(0)
	prevStatus := ""
	if existing != nil {
		prevTotal = existing.TotalSize
		prevStatus = existing.Status
	}
	switch ariaStatus {
	case "active":
		if total == 0 {
			return StatusMeta
		}
		if completed >= total && total > 0 {
			return StatusSeeding
		}
		return StatusRunning
	case "paused":
		return StatusPaused
	case "waiting":
		return StatusWaiting
	case "complete":
		if completed > 0 && total > 0 && completed >= total {
			if total > 1024*1024 || prevTotal > 1024*1024 {
				return StatusSeeding
			}
			if prevStatus == StatusRunning || prevStatus == StatusCompleted || prevStatus == StatusSeeding {
				return prevStatus
			}
			return StatusMeta
		}
		if completed > 0 {
			return StatusRunning
		}
		return StatusFailed
	case "error":
		return StatusFailed
	}
	return StatusMeta
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
		case StatusSeeding:
			bar := progressBar(100, barWidth)
			size := providers.FormatSize(d.TotalSize)
			statusLine = fmt.Sprintf("%s  %s  %s  %s/s",
				goodStyle.Render(bar),
				goodStyle.Render(size),
				accentStyle.Render("seeding"),
				subtleStyle.Render(providers.FormatSize(d.Speed)),
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
	parts = append(parts, mutedStyle.Render("p: pause  r: resume  R: remove files+torrent"))

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

func isHashName(name string) bool {
	if strings.HasPrefix(name, "torrent:") {
		return true
	}
	if len(name) >= 32 && !strings.Contains(name, " ") && !strings.Contains(name, "[") {
		for _, c := range name {
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
				return false
			}
		}
		return true
	}
	return false
}

func RequiresAria2() bool {
	_, err := exec.LookPath("aria2c")
	return err == nil
}
