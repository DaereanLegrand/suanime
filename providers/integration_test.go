package providers

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestIntegration_SearchTokyoGhoul(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	results := SearchAll(ctx, "tokyo ghoul")
	if len(results) == 0 {
		t.Error("SearchAll returned no results for 'tokyo ghoul'")
	}
	t.Logf("SearchAll 'tokyo ghoul': %d results", len(results))

	for _, r := range results[:minInt(5, len(results))] {
		t.Logf("  [%s] S:%d L:%d Ep:%d Res:%s Batch:%v Name:%s",
			r.Provider, r.Seeders, r.Leechers, r.Episode, r.Resolution, r.IsBatch, Truncate2(r.Name, 60))
	}

	found := false
	for _, r := range results {
		if r.MagnetLink != "" {
			found = true
			break
		}
	}
	if !found {
		t.Error("no results have magnet links")
	}
}

func TestIntegration_SearchTokyoGhoulEpisode1(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	results := SearchAll(ctx, "tokyo ghoul 01")

	var ep1 []*AnimeTorrent
	for _, r := range results {
		if r.Episode == 1 && !r.IsBatch {
			ep1 = append(ep1, r)
		}
	}

	if len(ep1) == 0 {
		t.Log("no exact episode 1 matches found, trying broader search")
		for _, r := range results {
			if r.Episode == 1 || !r.IsBatch {
				ep1 = append(ep1, r)
			}
		}
	}

	t.Logf("Episode 1 candidates: %d", len(ep1))
	for _, r := range ep1[:minInt(3, len(ep1))] {
		t.Logf("  [%s] Ep:%d Res:%s S:%d L:%d %s",
			r.Provider, r.Episode, r.Resolution, r.Seeders, r.Leechers, Truncate2(r.Name, 60))
		t.Logf("    Magnet: %s...", Truncate2(r.MagnetLink, 80))
	}

	if len(ep1) == 0 {
		t.Skip("no episode 1 matches to test")
	}

	best := ep1[0]
	if best.MagnetLink == "" {
		t.Error("best match has no magnet link")
	}
	if best.Seeders < 0 {
		t.Error("negative seeders")
	}
}

func TestIntegration_ProviderSearch(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	for _, p := range All {
		t.Run(p.Name(), func(t *testing.T) {
			t.Parallel()
			results, err := p.Search(ctx, "tokyo ghoul")
			if err != nil {
				t.Logf("  %s error: %v", p.Name(), err)
				return
			}
			if len(results) == 0 {
				t.Logf("  %s: 0 results", p.Name())
				return
			}
			t.Logf("  %s: %d results", p.Name(), len(results))
			if len(results) > 0 {
				r := results[0]
				t.Logf("    first: S:%d L:%d Ep:%d Mag:%v",
					r.Seeders, r.Leechers, r.Episode, r.MagnetLink != "")
			}
		})
	}
}

func TestIntegration_FindBestTokyoGhoulEp1(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	results := SearchAll(ctx, "tokyo ghoul 01")

	var best *AnimeTorrent
	for _, r := range results {
		if r.Episode == 1 && !r.IsBatch && r.MagnetLink != "" {
			if best == nil || r.Seeders > best.Seeders {
				best = r
			}
		}
	}

	if best == nil {
		for _, r := range results {
			if !r.IsBatch && r.MagnetLink != "" {
				if best == nil || r.Seeders > best.Seeders {
					best = r
				}
			}
		}
	}

	if best == nil {
		t.Fatal("no downloadable results found for 'tokyo ghoul 01'")
	}

	t.Logf("Best match: [%s] %s", best.Provider, best.Name)
	t.Logf("  Episode: %d, Resolution: %s", best.Episode, best.Resolution)
	t.Logf("  Seeders: %d, Leechers: %d", best.Seeders, best.Leechers)
	t.Logf("  Size: %s", FormatSize(best.SizeBytes))
	t.Logf("  Magnet: %s", best.MagnetLink)

	fmt.Println("--- BEST MATCH ---")
	fmt.Printf("Name:     %s\n", best.Name)
	fmt.Printf("Provider: %s\n", best.Provider)
	fmt.Printf("Episode:  %d\n", best.Episode)
	fmt.Printf("Size:     %s\n", FormatSize(best.SizeBytes))
	fmt.Printf("Seeders:  %d\n", best.Seeders)
	fmt.Printf("Leechers: %d\n", best.Leechers)
	fmt.Printf("Magnet:   %s\n", best.MagnetLink)
	fmt.Println("---")
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func Truncate2(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n-1]) + "…"
}
