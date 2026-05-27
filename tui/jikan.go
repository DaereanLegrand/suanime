package tui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type AnimeMeta struct {
	Title      string
	TitleEng   string
	ImageURL   string
	Score      float64
	Popularity int
	Rank       int
	Year       int
	Episodes   int
	Type       string
	Status     string
	Synopsis   string
	Genres     []string
	Studios    []string
}

type jikanResponse struct {
	Data []jikanAnime `json:"data"`
}

type jikanAnime struct {
	Title    string `json:"title"`
	TitleEng string `json:"title_english"`
	Images   struct {
		WebP struct {
			Large string `json:"large_image_url"`
		} `json:"webp"`
		JPG struct {
			Large string `json:"large_image_url"`
		} `json:"jpg"`
	} `json:"images"`
	Score      float64 `json:"score"`
	Popularity int     `json:"popularity"`
	Rank       int     `json:"rank"`
	Year       int     `json:"year"`
	Episodes   int     `json:"episodes"`
	Type       string  `json:"type"`
	Status     string  `json:"status"`
	Synopsis   string  `json:"synopsis"`
	Genres     []struct {
		Name string `json:"name"`
	} `json:"genres"`
	Studios []struct {
		Name string `json:"name"`
	} `json:"studios"`
}

var (
	jikanCache   = map[string]*AnimeMeta{}
	jikanCacheMu sync.RWMutex
	jikanClient  = &http.Client{Timeout: 10 * time.Second}
)

func jikanSearch(query string) (*AnimeMeta, error) {
	jikanCacheMu.RLock()
	if m, ok := jikanCache[query]; ok {
		jikanCacheMu.RUnlock()
		return m, nil
	}
	jikanCacheMu.RUnlock()

	u := "https://api.jikan.moe/v4/anime?" + url.Values{
		"q":     {query},
		"limit": {"3"},
		"sfw":   {"true"},
	}.Encode()

	req, _ := http.NewRequest("GET", u, nil)
	req.Header.Set("User-Agent", "suanime/1.0")
	resp, err := jikanClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("jikan: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 429 {
		time.Sleep(2 * time.Second)
		return jikanSearch(query)
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("jikan status %d", resp.StatusCode)
	}

	var result jikanResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("jikan parse: %w", err)
	}
	if len(result.Data) == 0 {
		return nil, fmt.Errorf("no results")
	}

	best := bestMatch(query, result.Data)
	if best == nil {
		return nil, fmt.Errorf("no match")
	}

	meta := &AnimeMeta{
		Title:      best.Title,
		TitleEng:   best.TitleEng,
		ImageURL:   best.Images.JPG.Large,
		Score:      best.Score,
		Popularity: best.Popularity,
		Rank:       best.Rank,
		Year:       best.Year,
		Episodes:   best.Episodes,
		Type:       best.Type,
		Status:     best.Status,
		Synopsis:   best.Synopsis,
	}
	if meta.ImageURL == "" {
		meta.ImageURL = best.Images.WebP.Large
	}
	if meta.TitleEng == "" {
		meta.TitleEng = best.Title
	}

	for _, g := range best.Genres {
		meta.Genres = append(meta.Genres, g.Name)
	}
	for _, s := range best.Studios {
		meta.Studios = append(meta.Studios, s.Name)
	}

	jikanCacheMu.Lock()
	jikanCache[query] = meta
	jikanCacheMu.Unlock()

	return meta, nil
}

func bestMatch(query string, candidates []jikanAnime) *jikanAnime {
	ql := normalizeTitle(query)
	var best *jikanAnime
	bestScore := -1

	for i := range candidates {
		c := &candidates[i]
		score := 0
		tl := normalizeTitle(c.Title)
		tel := normalizeTitle(c.TitleEng)

		if tl == ql || tel == ql {
			return c
		}
		if strings.Contains(tl, ql) {
			score += 20
		}
		if strings.Contains(tel, ql) {
			score += 25
		}
		if c.Type == "TV" {
			score += 5
		}
		if best != nil && c.Score > best.Score {
			score += 3
		}
		score += overlapScore(ql, tl)
		score += overlapScore(ql, tel)

		if score > bestScore {
			bestScore = score
			best = c
		}
	}
	return best
}

func overlapScore(query, title string) int {
	qw := strings.Fields(query)
	tw := strings.Fields(title)
	score := 0
	for _, q := range qw {
		if len(q) < 3 {
			continue
		}
		for _, t := range tw {
			if strings.EqualFold(q, t) {
				score += 10
			}
		}
	}
	return score
}

func extractAnimeTitle(torrentName string) string {
	name := strings.TrimSpace(torrentName)
	name = strings.TrimSuffix(name, ".mkv")
	name = strings.TrimSuffix(name, ".mp4")

	if strings.HasPrefix(name, "[") {
		end := strings.IndexByte(name, ']')
		if end > 0 {
			name = strings.TrimSpace(name[end+1:])
		}
	}

	for {
		sep := strings.LastIndex(name, " - ")
		if sep < 0 {
			break
		}
		rest := strings.TrimSpace(name[sep+3:])
		if looksLikeEpisode(rest) || strings.Contains(rest, "[") {
			name = strings.TrimSpace(name[:sep])
			continue
		}
		name = name[:sep] + " " + name[sep+3:]
	}

	idx := strings.Index(name, " [")
	if idx > 0 {
		name = strings.TrimSpace(name[:idx])
	}
	idx = strings.Index(name, " (")
	if idx > 0 {
		name = strings.TrimSpace(name[:idx])
	}
	return strings.TrimSpace(name)
}

func looksLikeEpisode(s string) bool {
	s = strings.TrimSpace(s)
	s = strings.TrimSuffix(s, "v2")
	s = strings.TrimSuffix(s, "v3")
	if len(s) == 0 {
		return true
	}
	first := strings.SplitN(s, " ", 2)[0]
	onlyDigits := true
	for _, c := range first {
		if c < '0' || c > '9' {
			onlyDigits = false
			break
		}
	}
	if onlyDigits {
		return true
	}
	upper := strings.ToUpper(first)
	if upper == "OVA" || upper == "ONA" || upper == "SPECIAL" {
		return true
	}
	return false
}

func normalizeTitle(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, ":", " ")
	s = strings.ReplaceAll(s, "√a", "root a")
	s = strings.ReplaceAll(s, "√A", "root a")
	s = strings.ReplaceAll(s, "!", " ")
	s = strings.ReplaceAll(s, "-", " ")
	s = strings.ReplaceAll(s, "_", " ")
	s = strings.ReplaceAll(s, ".", " ")
	fields := strings.Fields(s)
	return strings.Join(fields, " ")
}
