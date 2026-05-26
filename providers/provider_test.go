package providers

import (
	"testing"
)

func TestParseEpisode(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int
	}{
		{"standard", "[SubsPlease] Tokyo Ghoul - 01 (1080p)", 1},
		{"dash", "[Erai-raws] Tokyo Ghoul - 12 [1080p]", 12},
		{"dot", "[HorribleSubs] Tokyo.Ghoul.S01E05.720p", 5},
		{"underscore", "[Commie] Tokyo_Ghoul_-_03_[720p]", 3},
		{"double digits", "[SubsPlease] Tokyo Ghoul - 24 (1080p)", 24},
		{"with suffix", "[Judas] Tokyo Ghoul S01E05v2 [1080p]", 5},
		{"no episode", "[EMBER] Tokyo Ghoul S01 1080p BDRip", 0},
		{"batch", "[Judas] Tokyo Ghoul - S01 [1080p][Batch]", 0},
		{"odd format", "[Sick-Fansubs] One Piece 1163 [1080p].mp4", 1163},
		{"three digits", "[Erai-raws] One Piece - 1163 [1080p]", 1163},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseEpisode(tt.input)
			if got != tt.expected {
				t.Errorf("ParseEpisode(%q) = %d, want %d", tt.input, got, tt.expected)
			}
		})
	}
}

func TestParseResolution(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"1080p", "[SubsPlease] Tokyo Ghoul - 01 (1080p)", "1080P"},
		{"720p", "[Erai-raws] Tokyo Ghoul - 01 [720p]", "720P"},
		{"2160p", "[NanakoRaws] One Piece - 971 (4K 2160p)", "2160P"},
		{"480p", "[Judas] Tokyo Ghoul - 01 [480p]", "480P"},
		{"no resolution", "[EMBER] Tokyo Ghoul S01 BDRip", ""},
		{"lowercase", "tokyo ghoul 01 1080p", "1080P"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseResolution(tt.input)
			if got != tt.expected {
				t.Errorf("ParseResolution(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestParseIsBatch(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{"batch keyword", "[Judas] Tokyo Ghoul S01 [Batch]", true},
		{"complete keyword", "[Judas] Tokyo Ghoul Complete Series", true},
		{"season keyword", "[LostYears] Jujutsu Kaisen Season 1 (BD 1080p)", true},
		{"episode range", "[EMBER] Tokyo Ghoul 01-12 [1080p]", true},
		{"single ep", "[SubsPlease] Tokyo Ghoul - 01 (1080p)", false},
		{"not batch", "[Erai-raws] One Piece - 1163 [1080p]", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseIsBatch(tt.input)
			if got != tt.expected {
				t.Errorf("ParseIsBatch(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestParseSize(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantBytes  int64
		wantString string
	}{
		{"GiB", "1.4 GiB", int64(1503238553), "1.4 GiB"},
		{"MiB", "541.45 MiB", int64(567751475), "541.45 MiB"},
		{"KiB", "512 KiB", 512 * 1024, "512 KiB"},
		{"TiB", "1.0 TiB", 1024 * 1024 * 1024 * 1024, "1.0 TiB"},
		{"bytes", "1234 B", 1234, "1234 B"},
		{"empty", "", 0, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotBytes, gotStr := ParseSize(tt.input)
			if gotBytes != tt.wantBytes {
				t.Errorf("ParseSize(%q) bytes = %d, want %d", tt.input, gotBytes, tt.wantBytes)
			}
			if gotStr != tt.wantString {
				t.Errorf("ParseSize(%q) string = %q, want %q", tt.input, gotStr, tt.wantString)
			}
		})
	}
}

func TestCleanName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"bracket hash", "[SubsPlease] Tokyo Ghoul - 01 (1080p) [ABCD1234].mkv", "[SubsPlease] Tokyo Ghoul - 01 (1080p) .mkv"},
		{"bracket hash 2", "[Erai-raws] Tokyo Ghoul - 01 [1080p] [ABCD1234].mkv", "[Erai-raws] Tokyo Ghoul - 01 [1080p] .mkv"},
		{"paren hash", "[Erai-raws] One Piece - 1163 (AAC)[ABCD1234].mkv", "[Erai-raws] One Piece - 1163 (AAC).mkv"},
		{"no hash", "[SubsPlease] Tokyo Ghoul - 01 (1080p).mkv", "[SubsPlease] Tokyo Ghoul - 01 (1080p).mkv"},
		{"parentheses hash", "[EMBER] Show S01 (ABCD1234).mkv", "[EMBER] Show S01 .mkv"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CleanName(tt.input)
			if got != tt.expected {
				t.Errorf("CleanName(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestFormatSize(t *testing.T) {
	tests := []struct {
		bytes    int64
		expected string
	}{
		{0, "0 B"},
		{1024, "1.0 KiB"},
		{1024 * 1024, "1.0 MiB"},
		{1024 * 1024 * 1024, "1.0 GiB"},
		{1024 * 1024 * 1024 * 1024, "1.0 TiB"},
		{int64(1503238553), "1.4 GiB"},
		{int64(567751475), "541.4 MiB"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			got := FormatSize(tt.bytes)
			if got != tt.expected {
				t.Errorf("FormatSize(%d) = %q, want %q", tt.bytes, got, tt.expected)
			}
		})
	}
}

func TestMagnetFromHash(t *testing.T) {
	magnet := MagnetFromHash("abc123def456", "Test Show - 01")
	if magnet == "" {
		t.Fatal("magnet should not be empty")
	}
	if !contains(magnet, "urn:btih:abc123def456") {
		t.Errorf("magnet should contain info hash, got: %s", magnet)
	}
	if !contains(magnet, "Test+Show+-+01") {
		t.Errorf("magnet should contain escaped name, got: %s", magnet)
	}
}

func TestEscapeMagnet(t *testing.T) {
	result := EscapeMagnet("Tokyo Ghoul - 01")
	if result != "Tokyo+Ghoul+-+01" {
		t.Errorf("EscapeMagnet = %q, want %q", result, "Tokyo+Ghoul+-+01")
	}
}

func TestParseInt(t *testing.T) {
	if got := ParseInt("42"); got != 42 {
		t.Errorf("ParseInt(42) = %d", got)
	}
	if got := ParseInt("0"); got != 0 {
		t.Errorf("ParseInt(0) = %d", got)
	}
	if got := ParseInt("invalid"); got != 0 {
		t.Errorf("ParseInt(invalid) = %d, want 0", got)
	}
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
