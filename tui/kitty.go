package tui

import (
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

var kittySeqMu sync.Mutex

func IsKitty() bool {
	return os.Getenv("TERM") == "xterm-kitty" || os.Getenv("KITTY_WINDOW_ID") != ""
}

var nextImageID int
var displayedImageID int

func kittyShowImage(path string, col, row, widthCells, heightCells int) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read image: %w", err)
	}
	b64 := base64.StdEncoding.EncodeToString(data)

	id := nextImageID
	nextImageID++

	colPx := col * 10
	rowPx := row * 20
	wPx := widthCells * 10
	hPx := heightCells * 20

	kittySeqMu.Lock()
	defer kittySeqMu.Unlock()

	if displayedImageID > 0 {
		fmt.Fprintf(os.Stderr, "\033_Ga=d,d=I,i=%d\033\\", displayedImageID)
	}

	cmd := fmt.Sprintf("\033_Ga=T,f=100,t=d,s=%d,v=%d,c=%d,r=%d,i=%d;%s\033\\",
		wPx, hPx, colPx, rowPx, id, b64)
	fmt.Fprint(os.Stderr, cmd)

	displayedImageID = id
	return nil
}

func kittyClearImage() {
	kittySeqMu.Lock()
	defer kittySeqMu.Unlock()
	if displayedImageID > 0 {
		fmt.Fprintf(os.Stderr, "\033_Ga=d,d=I,i=%d\033\\", displayedImageID)
		displayedImageID = 0
	}
}

func kittyShowImageFromURL(url string, col, row, widthCells, heightCells int) error {
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
			return fmt.Errorf("fetch image: %w", err)
		}
		defer resp.Body.Close()
		data, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
		if err != nil {
			return fmt.Errorf("read image: %w", err)
		}
		os.WriteFile(cachePath, data, 0644)
	}

	return kittyShowImage(cachePath, col, row, widthCells, heightCells)
}
