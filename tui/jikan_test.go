package tui

import (
	"testing"
)

func TestExtractAnimeTitle(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"standard", "[SubsPlease] Tokyo Ghoul - 01 (1080p).mkv", "Tokyo Ghoul"},
		{"dash", "[Erai-raws] Mushoku Tensei - 01 [1080p].mkv", "Mushoku Tensei"},
		{"no group", "Tokyo Ghoul - 01 (1080p).mkv", "Tokyo Ghoul"},
		{"batch", "[EMBER] Mushoku Tensei S01+SP [BDRip].mkv", "Mushoku Tensei S01+SP"},
		{"colon", "[SubsPlease] Kono Subarashii - 01 (1080p).mkv", "Kono Subarashii"},
		{"complex", "[Naruto-Kun.Hu] Tokyo Ghoul - OVA 1 - Jack [1080p].mkv", "Tokyo Ghoul"},
		{"no separator", "One Piece 1163 [1080p].mp4", "One Piece 1163"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractAnimeTitle(tt.input)
			if got != tt.expected {
				t.Errorf("extractAnimeTitle(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestBestMatch(t *testing.T) {
	candidates := []jikanAnime{
		{Title: "Tokyo Ghoul", TitleEng: "Tokyo Ghoul", Type: "TV", Score: 7.79},
		{Title: "Tokyo Ghoul √A", TitleEng: "Tokyo Ghoul √A", Type: "TV", Score: 7.03},
		{Title: "Tokyo Ghoul:re", TitleEng: "Tokyo Ghoul:re", Type: "TV", Score: 6.37},
	}

	result := bestMatch("tokyo ghoul", candidates)
	if result == nil {
		t.Fatal("no match")
	}
	if result.Title != "Tokyo Ghoul" {
		t.Errorf("expected 'Tokyo Ghoul', got %q", result.Title)
	}

	result2 := bestMatch("tokyo ghoul re", candidates)
	if result2 == nil {
		t.Fatal("no match")
	}
	if result2.TitleEng != "Tokyo Ghoul:re" {
		t.Errorf("expected 'Tokyo Ghoul:re', got %q", result2.TitleEng)
	}
}

func TestOverlapScore(t *testing.T) {
	a := normalizeTitle("tokyo ghoul")
	b := normalizeTitle("Tokyo Ghoul")
	score := overlapScore(a, b)
	if score < 20 {
		t.Errorf("overlap too low: %d", score)
	}

	a2 := normalizeTitle("tokyo ghoul re")
	b2 := normalizeTitle("Tokyo Ghoul:re")
	score2 := overlapScore(a2, b2)
	if score2 < 20 {
		t.Errorf("overlap too low: %d", score2)
	}
}

func TestJikanSearchIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	meta, err := jikanSearch("tokyo ghoul")
	if err != nil {
		t.Fatalf("jikanSearch: %v", err)
	}
	if meta.Title == "" {
		t.Error("empty title")
	}
	if meta.Score == 0 {
		t.Error("zero score")
	}
	if meta.ImageURL == "" {
		t.Error("no image URL")
	}
	t.Logf("title: %s", meta.Title)
	t.Logf("score: %.2f", meta.Score)
	t.Logf("year: %d", meta.Year)
	t.Logf("image: %s", meta.ImageURL)
	t.Logf("genres: %v", meta.Genres)
}
