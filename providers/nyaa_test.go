package providers

import (
	"testing"
)

const sampleNyaaRSS = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:nyaa="https://nyaa.si/xmlns/nyaa" xmlns:atom="https://www.w3.org/2005/Atom">
<channel>
<title>Nyaa - "tokyo ghoul 01" - 1_2</title>
<item>
<title>[SubsPlease] Tokyo Ghoul - 01 (1080p) [1234ABCD].mkv</title>
<link>https://nyaa.si/view/1000000</link>
<guid isPermaLink="true">https://nyaa.si/view/1000000</guid>
<pubDate>Mon, 01 Jan 2024 12:00:00 -0000</pubDate>
<nyaa:seeders>100</nyaa:seeders>
<nyaa:leechers>5</nyaa:leechers>
<nyaa:downloads>500</nyaa:downloads>
<nyaa:infoHash>abc123def456abc123def456abc123def456abc123de</nyaa:infoHash>
<nyaa:categoryId>1_2</nyaa:categoryId>
<nyaa:category>Anime - English-translated</nyaa:category>
<nyaa:size>1.4 GiB</nyaa:size>
<nyaa:comments>10</nyaa:comments>
<nyaa:trusted>Yes</nyaa:trusted>
<nyaa:remake>No</nyaa:remake>
<description><![CDATA[Torrent description]]></description>
</item>
<item>
<title>[Erai-raws] Tokyo Ghoul - 01 [720p] [5678EFAB].mkv</title>
<link>https://nyaa.si/view/1000001</link>
<pubDate>Tue, 02 Jan 2024 12:00:00 -0000</pubDate>
<nyaa:seeders>50</nyaa:seeders>
<nyaa:leechers>2</nyaa:leechers>
<nyaa:downloads>200</nyaa:downloads>
<nyaa:infoHash>def456abc123def456abc123def456abc123def456ab</nyaa:infoHash>
<nyaa:size>700 MiB</nyaa:size>
</item>
<item>
<title>[Judas] Tokyo Ghoul S01 [Batch] [ABCD5678].mkv</title>
<link>https://nyaa.si/view/1000002</link>
<pubDate>Wed, 03 Jan 2024 12:00:00 -0000</pubDate>
<nyaa:seeders>200</nyaa:seeders>
<nyaa:leechers>10</nyaa:leechers>
<nyaa:infoHash>ghi789jkl012ghi789jkl012ghi789jkl012ghi789jk</nyaa:infoHash>
<nyaa:size>15.0 GiB</nyaa:size>
</item>
</channel>
</rss>`

func TestParseNyaaRSS(t *testing.T) {
	results, err := parseNyaaRSS([]byte(sampleNyaaRSS), "Nyaa")
	if err != nil {
		t.Fatalf("parseNyaaRSS error: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	r0 := results[0]
	if r0.Seeders != 200 || r0.Leechers != 10 {
		t.Errorf("result[0] seeders/leechers: %d/%d, want 200/10 (highest sorted)", r0.Seeders, r0.Leechers)
	}
	if !r0.IsBatch {
		t.Error("result[0] should be batch (highest seeders)")
	}
	if r0.Episode != 0 {
		t.Errorf("result[0] batch episode: %d, want 0", r0.Episode)
	}

	r1 := results[1]
	if r1.Seeders != 100 {
		t.Errorf("result[1] seeders: %d, want 100", r1.Seeders)
	}
	if r1.Episode != 1 {
		t.Errorf("result[1] episode: %d, want 1", r1.Episode)
	}
	if r1.Resolution != "1080P" {
		t.Errorf("result[1] resolution: %q, want 1080P", r1.Resolution)
	}
	if r1.MagnetLink == "" {
		t.Error("result[1] magnet link is empty")
	}
	if r1.Provider != "Nyaa" {
		t.Errorf("result[1] provider: %q, want Nyaa", r1.Provider)
	}

	r2 := results[2]
	if r2.Seeders != 50 {
		t.Errorf("result[2] seeders: %d, want 50", r2.Seeders)
	}
	if r2.Resolution != "720P" {
		t.Errorf("result[2] resolution: %q, want 720P", r2.Resolution)
	}

	batchResult := results[0]
	sizeBytes, _ := ParseSize("15.0 GiB")
	if batchResult.SizeBytes != sizeBytes {
		t.Errorf("batch size bytes: %d, want %d", batchResult.SizeBytes, sizeBytes)
	}
}

func TestParseNyaaRSS_Empty(t *testing.T) {
	emptyRSS := `<?xml version="1.0"?><rss version="2.0"><channel></channel></rss>`
	results, err := parseNyaaRSS([]byte(emptyRSS), "Nyaa")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestParseNyaaRSS_SortsBySeeders(t *testing.T) {
	results, err := parseNyaaRSS([]byte(sampleNyaaRSS), "Nyaa")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	for i := 1; i < len(results); i++ {
		if results[i-1].Seeders < results[i].Seeders {
			t.Errorf("results not sorted by seeders at index %d: %d < %d",
				i, results[i-1].Seeders, results[i].Seeders)
		}
	}
}
