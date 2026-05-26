package providers

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type AnimeTorrent struct {
	Name       string
	MagnetLink string
	Link       string
	Seeders    int
	Leechers   int
	Size       string
	SizeBytes  int64
	Date       string
	Provider   string
	InfoHash   string
	Episode    int
	Resolution string
	IsBatch    bool
}

type Provider interface {
	Name() string
	Search(ctx context.Context, query string) ([]*AnimeTorrent, error)
}

var All = []Provider{
	NewNyaa(),
	NewNyaaSukebei(),
	NewTokyoToshokan(),
}

func SearchAll(ctx context.Context, query string) []*AnimeTorrent {
	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		results []*AnimeTorrent
	)

	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	for _, p := range All {
		wg.Add(1)
		go func(p Provider) {
			defer wg.Done()
			items, err := p.Search(ctx, query)
			if err != nil {
				return
			}
			mu.Lock()
			results = append(results, items...)
			mu.Unlock()
		}(p)
	}

	wg.Wait()
	sort.Slice(results, func(i, j int) bool {
		return results[i].Seeders > results[j].Seeders
	})
	return results
}

var (
	reEpDash      = regexp.MustCompile(`(?i)[-_\s.](\d{1,4})(?:v\d+)?(?:\b|[._\s\-\]\[\(])`)
	reEpSxE       = regexp.MustCompile(`(?i)(?:[sS]\d{1,4})?[eE](\d{1,4})(?:v\d+)?(?:\b|[._\s\-\]\[\(])`)
	reBatch       = regexp.MustCompile(`(?i)[-_\s.](?:0[1-9])\s*[-~]\s*(?:[1-9]\d{1,3})\b`)
	reRes         = regexp.MustCompile(`(?i)(\d{3,4}p)`)
	reInfoHash    = regexp.MustCompile(`(?i)\b([0-9a-fA-F]{40})\b`)
	reB32InfoHash = regexp.MustCompile(`(?i)btih:([A-Z2-7]{32})`)
	reMagnetLink  = regexp.MustCompile(`(?i)(magnet:\?[^\s"'<>]+)`)
	reBatchKwd    = regexp.MustCompile(`(?i)\b(batch|complete|season)\b`)
)

func ParseEpisode(name string) int {
	if m := reEpSxE.FindStringSubmatch(name); m != nil {
		n, _ := strconv.Atoi(m[1])
		return n
	}
	if m := reEpDash.FindStringSubmatch(name); m != nil {
		if n, err := strconv.Atoi(m[1]); err == nil && n > 0 && n < 2000 {
			return n
		}
	}
	return 0
}

func ParseResolution(name string) string {
	m := reRes.FindStringSubmatch(name)
	if m != nil {
		return strings.ToUpper(m[1])
	}
	return ""
}

func ParseIsBatch(name string) bool {
	if reBatchKwd.MatchString(name) {
		return true
	}
	if reBatch.MatchString(name) {
		return true
	}
	return false
}

func ParseSize(sizeStr string) (int64, string) {
	sizeStr = strings.TrimSpace(sizeStr)
	parts := strings.Fields(sizeStr)
	if len(parts) == 0 {
		return 0, ""
	}
	val, err := strconv.ParseFloat(parts[0], 64)
	if err != nil {
		return 0, sizeStr
	}
	unit := strings.ToUpper(strings.TrimSpace(strings.Join(parts[1:], "")))
	var multiplier int64 = 1
	switch {
	case strings.HasPrefix(unit, "T"):
		multiplier = 1024 * 1024 * 1024 * 1024
	case strings.HasPrefix(unit, "G"):
		multiplier = 1024 * 1024 * 1024
	case strings.HasPrefix(unit, "M"):
		multiplier = 1024 * 1024
	case strings.HasPrefix(unit, "K"):
		multiplier = 1024
	}
	return int64(val * float64(multiplier)), sizeStr
}

func FormatSize(bytes int64) string {
	switch {
	case bytes >= 1024*1024*1024*1024:
		return fmt.Sprintf("%.1f TiB", float64(bytes)/(1024*1024*1024*1024))
	case bytes >= 1024*1024*1024:
		return fmt.Sprintf("%.1f GiB", float64(bytes)/(1024*1024*1024))
	case bytes >= 1024*1024:
		return fmt.Sprintf("%.1f MiB", float64(bytes)/(1024*1024))
	case bytes >= 1024:
		return fmt.Sprintf("%.1f KiB", float64(bytes)/(1024))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

func CleanName(name string) string {
	name = strings.TrimSpace(name)
	name = regexp.MustCompile(`\[[0-9a-fA-F]{8}\]`).ReplaceAllString(name, "")
	name = regexp.MustCompile(`\([0-9a-fA-F]{8}\)`).ReplaceAllString(name, "")
	return strings.TrimSpace(name)
}

func MagnetFromHash(hash, name string) string {
	hash = strings.TrimSpace(hash)
	if !strings.HasPrefix(hash, "urn:btih:") {
		hash = "urn:btih:" + hash
	}
	m := fmt.Sprintf("magnet:?xt=%s&dn=%s", hash, EscapeMagnet(name))
	m += "&tr=http://nyaa.tracker.wf:7777/announce"
	m += "&tr=udp://tracker.opentrackr.org:1337/announce"
	m += "&tr=udp://tracker.coppersurfer.tk:6969/announce"
	m += "&tr=udp://9.rarbg.to:2710/announce"
	m += "&tr=udp://tracker.internetwarriors.net:1337/announce"
	m += "&tr=udp://tracker.leechers-paradise.org:6969/announce"
	return m
}

func EscapeMagnet(s string) string {
	return strings.ReplaceAll(s, " ", "+")
}
