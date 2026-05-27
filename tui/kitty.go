package tui

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	tea "github.com/charmbracelet/bubbletea"
)

var kittyMu sync.Mutex

func IsKitty() bool {
	return os.Getenv("TERM") == "xterm-kitty" || os.Getenv("KITTY_WINDOW_ID") != ""
}

func Cleanup() {}

func kittyTTY() *os.File {
	f, err := os.OpenFile("/dev/tty", os.O_WRONLY, 0)
	if err != nil {
		return os.Stderr
	}
	return f
}

func clearAllImages() {
	tty := kittyTTY()
	kittyMu.Lock()
	fmt.Fprint(tty, "\033_Ga=d,d=A\033\\")
	kittyMu.Unlock()
}

func displayViaIcat(path string, col, row, widthCells, heightCells int) error {
	place := fmt.Sprintf("%dx%d@%dx%d", widthCells, heightCells, col, row)
	cmd := exec.Command("kitten", "icat",
		"--silent",
		"--stdin", "no",
		"--transfer-mode", "file",
		"--place", place,
		path,
	)
	tty, err := os.OpenFile("/dev/tty", os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	cmd.Stdin = nil
	cmd.Stdout = tty
	cmd.Stderr = nil
	return cmd.Run()
}

func KittyShowCmd(url string, col, row, w, h int) tea.Cmd {
	return func() tea.Msg {
		dir, _ := os.UserCacheDir()
		cacheDir := filepath.Join(dir, "suanime", "images")
		os.MkdirAll(cacheDir, 0755)

		uParts := strings.Split(url, "/")
		fname := uParts[len(uParts)-1]
		if !strings.HasSuffix(fname, ".jpg") && !strings.HasSuffix(fname, ".webp") && !strings.HasSuffix(fname, ".png") {
			fname += ".jpg"
		}
		cachePath := filepath.Join(cacheDir, fname)

		if _, err := os.Stat(cachePath); os.IsNotExist(err) {
			resp, err := http.Get(url)
			if err != nil {
				return ErrMsg(fmt.Sprintf("image fetch: %v", err))
			}
			defer resp.Body.Close()
			if resp.StatusCode != 200 {
				return ErrMsg(fmt.Sprintf("image HTTP %d", resp.StatusCode))
			}
			data, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
			if err != nil {
				return ErrMsg(fmt.Sprintf("image read: %v", err))
			}
			os.WriteFile(cachePath, data, 0644)
		}

		clearAllImages()
		if err := displayViaIcat(cachePath, col, row, w, h); err != nil {
			return ErrMsg(fmt.Sprintf("image: %v", err))
		}
		return nil
	}
}
