package providers

import (
	"context"
	"encoding/xml"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type tokyoToshokanProvider struct {
	name string
	base string
}

func NewTokyoToshokan() Provider {
	return &tokyoToshokanProvider{
		name: "TokyoToshokan",
		base: "https://www.tokyotosho.info",
	}
}

func (t *tokyoToshokanProvider) Name() string { return t.name }

func (t *tokyoToshokanProvider) Search(ctx context.Context, q string) ([]*AnimeTorrent, error) {
	u := t.base + "/rss.php?" + url.Values{
		"filter": {"1"},
		"terms":  {q},
	}.Encode()

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
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
	return parseTTRSS(body, t.name)
}

func parseTTRSS(data []byte, providerName string) ([]*AnimeTorrent, error) {
	decoder := xml.NewDecoder(strings.NewReader(string(data)))

	type itemFields struct {
		title       string
		link        string
		pubDate     string
		description string
	}

	var results []*AnimeTorrent
	var current *itemFields
	var inDesc, inTitle, inLink, inPubDate bool

	for {
		token, err := decoder.Token()
		if err != nil {
			break
		}

		switch t := token.(type) {
		case xml.StartElement:
			if t.Name.Local == "item" {
				current = &itemFields{}
			}
			switch t.Name.Local {
			case "description":
				inDesc = true
			case "title":
				inTitle = true
			case "link":
				inLink = true
			case "pubDate":
				inPubDate = true
			}
		case xml.EndElement:
			if t.Name.Local == "item" && current != nil {
				name := CleanName(current.title)
				if name == "" {
					current = nil
					continue
				}
				magnet := extractMagnetFromText(current.description)
				infoHash := extractHashFromText(current.description)
				if infoHash == "" {
					infoHash = extractHashFromText(current.title)
				}
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
					Date:       date,
					Provider:   providerName,
					InfoHash:   infoHash,
					Episode:    ParseEpisode(name),
					Resolution: ParseResolution(name),
					IsBatch:    ParseIsBatch(name),
				})
				current = nil
			}
			switch t.Name.Local {
			case "description":
				inDesc = false
			case "title":
				inTitle = false
			case "link":
				inLink = false
			case "pubDate":
				inPubDate = false
			}
		case xml.CharData:
			if current != nil {
				val := string(t)
				switch {
				case inDesc:
					current.description += val
				case inTitle:
					current.title += val
				case inLink:
					current.link += val
				case inPubDate:
					current.pubDate += val
				}
			}
		}
	}
	return results, nil
}

func extractMagnetFromText(text string) string {
	m := reMagnetLink.FindString(text)
	return m
}

func extractHashFromText(text string) string {
	m := reInfoHash.FindString(text)
	if m != "" {
		return strings.ToLower(m)
	}
	match := reB32InfoHash.FindStringSubmatch(text)
	if len(match) >= 2 {
		return strings.ToLower(match[1])
	}
	return ""
}
