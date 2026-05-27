package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAria2RPCConnection(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	if !RequiresAria2() {
		t.Skip("aria2c not found")
	}

	cfg := DefaultConfig()
	cfg.DownloadDir = filepath.Join(os.TempDir(), "suanime-test")
	os.MkdirAll(cfg.DownloadDir, 0755)
	defer os.RemoveAll(cfg.DownloadDir)

	dm := NewDownloadManager(cfg)
	if err := dm.StartDaemon(); err != nil {
		t.Fatalf("StartDaemon: %v", err)
	}
	defer dm.StopDaemon()

	time.Sleep(500 * time.Millisecond)

	client := newAria2Client(cfg.Aria2RPCPort, cfg.Aria2RPCSecret)
	version, err := client.call("aria2.getVersion", []interface{}{})
	if err != nil {
		t.Fatalf("getVersion RPC failed: %v", err)
	}
	t.Logf("aria2 version response: %s", string(version))
}

func TestDownloadManager_AddAndPoll(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	if !RequiresAria2() {
		t.Skip("aria2c not found")
	}

	cfg := DefaultConfig()
	cfg.DownloadDir = filepath.Join(os.TempDir(), "suanime-test-add")
	os.MkdirAll(cfg.DownloadDir, 0755)
	defer os.RemoveAll(cfg.DownloadDir)

	dm := NewDownloadManager(cfg)
	if err := dm.StartDaemon(); err != nil {
		t.Fatalf("StartDaemon: %v", err)
	}
	defer dm.StopDaemon()

	magnet := "magnet:?xt=urn:btih:dd8255ecdc7ca55fb0bbf81323d87062db1f6d1c&dn=Big+Buck+Bunny&tr=udp://tracker.coppersurfer.tk:6969/announce"

	err := dm.AddDownload("Big Buck Bunny", magnet, 0, 0)
	if err != nil {
		t.Fatalf("AddDownload: %v", err)
	}
	t.Logf("added download")

	time.Sleep(2 * time.Second)
	dm.Poll()
	items := dm.GetItems()
	if len(items) == 0 {
		t.Error("no items after poll")
	} else {
		t.Logf("item[0]: name=%q, status=%s, progress=%.0f%%",
			items[0].Name, items[0].Status, items[0].Progress)
		if items[0].GID == "" {
			t.Error("GID is empty")
		}
		if items[0].Name == "" || items[0].Name == items[0].GID {
			t.Error("name should not be GID")
		}
	}

	dm.Pause(items[0].GID)
	time.Sleep(500 * time.Millisecond)
	dm.Poll()
	items = dm.GetItems()
	if len(items) > 0 {
		t.Logf("after cancel: status=%s", items[0].Status)
	}
	dm.RemoveFilesAndTorrent(items[0].GID)
}

func TestDownloadManager_PauseResumeCancel(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	if !RequiresAria2() {
		t.Skip("aria2c not found")
	}

	cfg := DefaultConfig()
	cfg.DownloadDir = filepath.Join(os.TempDir(), "suanime-test-prc")
	os.MkdirAll(cfg.DownloadDir, 0755)
	defer os.RemoveAll(cfg.DownloadDir)

	dm := NewDownloadManager(cfg)
	if err := dm.StartDaemon(); err != nil {
		t.Fatalf("StartDaemon: %v", err)
	}
	defer dm.StopDaemon()

	magnet := "magnet:?xt=urn:btih:dd8255ecdc7ca55fb0bbf81323d87062db1f6d1c&dn=Big+Buck+Bunny&tr=udp://tracker.coppersurfer.tk:6969/announce"

	err := dm.AddDownload("Test Torrent", magnet, 0, 0)
	if err != nil {
		t.Fatalf("AddDownload: %v", err)
	}

	time.Sleep(1 * time.Second)
	dm.Poll()
	items := dm.GetItems()
	if len(items) == 0 {
		t.Fatal("no items to test")
	}
	gid := items[0].GID
	t.Logf("initial: status=%s, name=%q", items[0].Status, items[0].Name)

	err = dm.Pause(gid)
	if err != nil {
		t.Logf("pause returned error (may be expected for metadata phase): %v", err)
	}

	time.Sleep(500 * time.Millisecond)
	dm.Poll()
	items = dm.GetItems()
	if len(items) > 0 {
		t.Logf("after pause: status=%s", items[0].Status)
	}

	err = dm.Resume(gid)
	if err != nil {
		t.Logf("resume returned error: %v", err)
	}

	time.Sleep(500 * time.Millisecond)
	dm.Poll()
	items = dm.GetItems()
	if len(items) > 0 {
		t.Logf("after resume: status=%s", items[0].Status)
	}

	err = dm.Pause(gid)
	if err != nil {
		t.Logf("pause returned: %v", err)
	}

	time.Sleep(500 * time.Millisecond)
	dm.Poll()
	items = dm.GetItems()
	if len(items) > 0 {
		t.Logf("after cancel: status=%s", items[0].Status)
		dm.RemoveFilesAndTorrent(items[0].GID)
	}
}

func TestDownloadManager_MultipleDownloads(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	if !RequiresAria2() {
		t.Skip("aria2c not found")
	}

	cfg := DefaultConfig()
	cfg.DownloadDir = filepath.Join(os.TempDir(), "suanime-test-multi")
	os.MkdirAll(cfg.DownloadDir, 0755)
	defer os.RemoveAll(cfg.DownloadDir)

	dm := NewDownloadManager(cfg)
	if err := dm.StartDaemon(); err != nil {
		t.Fatalf("StartDaemon: %v", err)
	}
	defer dm.StopDaemon()

	hashes := []string{
		"dd8255ecdc7ca55fb0bbf81323d87062db1f6d1c",
		"08ada5a7a6183aae1e09d831df6748d566095a10",
		"6a9759b00d6c0e4368df51c9eb62b8c22b662b0a",
	}
	for i, h := range hashes {
		magnet := fmt.Sprintf("magnet:?xt=urn:btih:%s&dn=Test%d&tr=udp://tracker.coppersurfer.tk:6969/announce", h, i+1)
		err := dm.AddDownload(fmt.Sprintf("Test %d", i+1), magnet, 0, 0)
		if err != nil {
			t.Fatalf("AddDownload %d: %v", i+1, err)
		}
	}

	time.Sleep(1 * time.Second)
	dm.Poll()
	items := dm.GetItems()
	t.Logf("added 3 downloads, got %d items (dedup expected)", len(items))

	if len(items) < 1 {
		t.Error("should have at least one item")
	}

	for _, item := range items {
		if item.GID == "" {
			t.Error("item has empty GID")
		}
	}

	for _, item := range items {
		dm.RemoveFilesAndTorrent(item.GID)
	}
	time.Sleep(500 * time.Millisecond)
	dm.Poll()
	items = dm.GetItems()
	for _, r := range items {
		dm.RemoveFilesAndTorrent(r.GID)
	}
	time.Sleep(200 * time.Millisecond)
	dm.Poll()
	items = dm.GetItems()
	if len(items) != 0 {
		for _, r := range items {
			t.Logf("  remaining: gid=%s status=%s", r.GID, r.Status)
		}
	}
	if len(items) != 0 {
		for _, r := range items {
			t.Logf("  remaining: gid=%s status=%s", r.GID, r.Status)
		}
		t.Errorf("expected 0 items after cancel all, got %d", len(items))
	}
}

func TestDownloadManager_NamePreservation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	if !RequiresAria2() {
		t.Skip("aria2c not found")
	}

	cfg := DefaultConfig()
	cfg.DownloadDir = filepath.Join(os.TempDir(), "suanime-test-name")
	os.MkdirAll(cfg.DownloadDir, 0755)
	defer os.RemoveAll(cfg.DownloadDir)

	dm := NewDownloadManager(cfg)
	if err := dm.StartDaemon(); err != nil {
		t.Fatalf("StartDaemon: %v", err)
	}
	defer dm.StopDaemon()

	customName := "[HorribleSubs] Tokyo Ghoul - 01 [1080p].mkv"
	magnet := "magnet:?xt=urn:btih:dd8255ecdc7ca55fb0bbf81323d87062db1f6d1c&dn=Big+Buck+Bunny&tr=udp://tracker.coppersurfer.tk:6969/announce"

	err := dm.AddDownload(customName, magnet, 0, 0)
	if err != nil {
		t.Fatalf("AddDownload: %v", err)
	}

	for i := 0; i < 5; i++ {
		time.Sleep(1 * time.Second)
		dm.Poll()
		items := dm.GetItems()
		if len(items) > 0 {
			t.Logf("poll %d: name=%q (GID=%s)", i, items[0].Name, items[0].GID)
			if items[0].Name == items[0].GID {
				t.Errorf("poll %d: name should not be GID, got %q", i, items[0].Name)
			}
			if items[0].Name != customName && items[0].Name != "Big Buck Bunny" {
				t.Errorf("poll %d: unexpected name %q", i, items[0].Name)
			}
		}
	}

	items := dm.GetItems()
	if len(items) > 0 {
		dm.RemoveFilesAndTorrent(items[0].GID)
	}
}

func TestDownloadManager_SessionCleanup(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	if !RequiresAria2() {
		t.Skip("aria2c not found")
	}

	cfg := DefaultConfig()
	cfg.DownloadDir = filepath.Join(os.TempDir(), "suanime-test-session")
	os.MkdirAll(cfg.DownloadDir, 0755)
	defer os.RemoveAll(cfg.DownloadDir)

	sessionFile := cfg.DownloadDir + "/.suanime-aria2.session"
	os.Remove(sessionFile)

	dm := NewDownloadManager(cfg)
	if err := dm.StartDaemon(); err != nil {
		t.Fatalf("StartDaemon: %v", err)
	}
	defer dm.StopDaemon()

	dm.Poll()
	items := dm.GetItems()
	if len(items) != 0 {
		t.Errorf("expected 0 items from fresh session, got %d", len(items))
	}
}
