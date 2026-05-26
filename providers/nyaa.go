package providers

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type nyaaProvider struct {
	name  string
	base  string
	query string
}

func NewNyaa() Provider {
	return &nyaaProvider{
		name:  "Nyaa",
		base:  "https://nyaa.si",
		query: "?page=rss&q=%s&c=1_0&f=0&s=seeders&o=desc",
	}
}

func NewNyaaSukebei() Provider {
	return &nyaaProvider{
		name:  "Sukebei",
		base:  "https://sukebei.nyaa.si",
		query: "?page=rss&q=%s&f=0&s=seeders&o=desc",
	}
}

func (n *nyaaProvider) Name() string { return n.name }

func (n *nyaaProvider) Search(ctx context.Context, q string) ([]*AnimeTorrent, error) {
	u := n.base + fmt.Sprintf(n.query, url.QueryEscape(q))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "suanime/1.0")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024))
	return parseNyaaRSS(body, n.name)
}

func parseNyaaRSS(data []byte, providerName string) ([]*AnimeTorrent, error) {
	decoder := xml.NewDecoder(strings.NewReader(string(data)))

	type itemFields struct {
		title    string
		link     string
		pubDate  string
		seeders  string
		leechers string
		infoHash string
		size     string
	}

	var results []*AnimeTorrent
	var current *itemFields
	var currentElement string
	var inItem bool

	for {
		token, err := decoder.Token()
		if err != nil {
			break
		}

		switch t := token.(type) {
		case xml.StartElement:
			currentElement = t.Name.Local
			if t.Name.Local == "item" {
				current = &itemFields{}
				inItem = true
			}
		case xml.EndElement:
			if t.Name.Local == "item" && current != nil {
				inItem = false
				name := CleanName(current.title)
				if name == "" {
					current = nil
					continue
				}
				sizeBytes, sizeStr := ParseSize(current.size)
				infoHash := strings.TrimSpace(strings.ToLower(current.infoHash))
				magnet := ""
				if infoHash != "" {
					magnet = MagnetFromHash(infoHash, name)
				}
				seeders := ParseInt(current.seeders)
				leechers := ParseInt(current.leechers)
				date := ""
				if t, err := time.Parse(time.RFC1123Z, current.pubDate); err == nil {
					date = t.Format("2006-01-02")
				} else if t, err := time.Parse(time.RFC1123, current.pubDate); err == nil {
					date = t.Format("2006-01-02")
				}

				results = append(results, &AnimeTorrent{
					Name:       name,
					MagnetLink: magnet,
					Link:       current.link,
					Seeders:    seeders,
					Leechers:   leechers,
					Size:       sizeStr,
					SizeBytes:  sizeBytes,
					Date:       date,
					Provider:   providerName,
					InfoHash:   infoHash,
					Episode:    ParseEpisode(name),
					Resolution: ParseResolution(name),
					IsBatch:    ParseIsBatch(name),
				})
				current = nil
			}
			currentElement = ""
		case xml.CharData:
			if inItem && current != nil {
				val := strings.TrimSpace(string(t))
				switch currentElement {
				case "title":
					current.title = val
				case "link":
					current.link = val
				case "pubDate":
					current.pubDate = val
				case "seeders":
					current.seeders = val
				case "leechers":
					current.leechers = val
				case "infoHash":
					current.infoHash = val
				case "size":
					current.size = val
				}
			}
		}
	}

	return results, nil
}

func ParseInt(s string) int {
	var n int
	fmt.Sscan(s, &n)
	return n
}
