package tui

import (
	"errors"
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

var kittySeqMu sync.Mutex

func IsKitty() bool {
	return os.Getenv("TERM") == "xterm-kitty" || os.Getenv("KITTY_WINDOW_ID") != ""
}

func Cleanup() {}

func runIcat(path string, col, row, widthCells, heightCells int) error {
	// kitten icat --stdin no --transfer-mode file
	//   --place "${w}x${h}@${x}x${y}" "$file" < /dev/null > /dev/tty
	place := fmt.Sprintf("%dx%d@%dx%d", widthCells, heightCells, col, row)

	cmd := exec.Command("kitten", "icat",
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
	cmd.Stderr = tty
	return cmd.Run()
}

var nextImageID int
var displayedImageID int
var ttyFd *os.File

func kittyTTY() *os.File {
	if ttyFd != nil {
		return ttyFd
	}
	f, err := os.OpenFile("/dev/tty", os.O_WRONLY, 0)
	if err != nil {
		f = os.Stderr
	}
	ttyFd = f
	return f
}

func clearViaEscape() {
	kittySeqMu.Lock()
	defer kittySeqMu.Unlock()
	if displayedImageID > 0 {
		fmt.Fprintf(kittyTTY(), "\033_Ga=d,d=I,i=%d\033\\", displayedImageID)
		displayedImageID = 0
	}
}

func clearAllViaEscape() {
	kittySeqMu.Lock()
	defer kittySeqMu.Unlock()
	if displayedImageID > 0 {
		fmt.Fprintf(kittyTTY(), "\033_Ga=d,d=I,i=%d\033\\", displayedImageID)
		displayedImageID = 0
	}
	// clear all just in case
	fmt.Fprint(kittyTTY(), "\033_Ga=d,d=A\033\\")
}

func kittyShowImage(path string, col, row, widthCells, heightCells int) error {
	if err := runIcat(path, col, row, widthCells, heightCells); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return fmt.Errorf("kitten not found; %w", err)
		}
		return err
	}
	return nil
}

func kittyClearImage() {
	clearAllViaEscape()
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

		if err := kittyShowImage(cachePath, col, row, w, h); err != nil {
			return ErrMsg(fmt.Sprintf("image: %v", err))
		}
		return nil
	}
}
