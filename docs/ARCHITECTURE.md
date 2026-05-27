# Architecture

## Code Structure

```
suanime/
  main.go                   CLI entry point, subcommands, Kitty terminal check
  providers/
    provider.go             AnimeTorrent struct, Provider interface, SearchAll, parsers
    nyaa.go                 Nyaa.si + Sukebei RSS provider, parseNyaaRSS
    tokyotosho.go           Tokyo Toshokan RSS provider, parseTTRSS
    *_test.go               17 unit + integration tests
  tui/
    model.go                Bubbletea TUI: split-pane layout, search input, key handling
    download.go             DownloadManager, aria2 lifecycle, Poll engine, downloadsView
    aria2.go                JSON-RPC client, progressBar, parseLength, progressPct
    config.go               Config loading/saving, DefaultConfig
    styles.go               Lipgloss theme (dark purple), Navbar, FormatPeers
    kitty.go                Kitty graphics protocol: image display via APC escape sequences
    jikan.go                Jikan API (MyAnimeList) client, fuzzy matching, metadata cache
    *_test.go               5 download integration tests
  docs/
    README.md               Overview, quick start, features
    CLI.md                  CLI command reference
    CONFIG.md               Config file documentation
    COVERART.md             Cover art & metadata feature design
    ARCHITECTURE.md         This file
```

## Data Flow

### Search Flow

```
User types query → TUI handleSearchKeys → searchCmd (goroutine)
  → providers.SearchAll(ctx, query)
    → Nyaa.Search(ctx, query)    ─┐
    → Sukebei.Search(ctx, query) ─┤ goroutines
    → TokyoToshokan.Search(ctx,q) ─┘
  → results merged, sorted by seeders
  → searchDoneMsg → TUI renders results table
```

### Download Flow (Two-Phase Magnet)

```
User presses enter on result → StartDownload(name, magnet)
  → DownloadManager.AddDownload(name, magnet)
    → aria2Client.AddURI(magnet, dir)   [JSON-RPC: aria2.addUri]
    → returns metadata GID ("abc123")
    → dm.items ← {GID:"abc123", Name:name, Status:StatusMeta}

ticker (2s) → Poll()
  → aria2Client.TellActive()    [JSON-RPC: aria2.tellActive]
  → aria2Client.TellWaiting(0,100)
  → aria2Client.TellStopped(0,100)
  → sorted by TotalLength descending
  → findMatching(tsGID, btName, dirName, total):
      1. GID exact match
      2. nameOverlap (btName substring/word match)
      3. File path directory name overlap (>0 bytes)
      4. Single pending item fallback (total > 100KiB threshold)
  → mapStatus(ariaStatus, total, completed, existing):
      - active + total=0      → StatusMeta
      - active + total>0      → StatusRunning
      - active + completed>=total → StatusSeeding
      - complete + total<100K → StatusMeta (metadata phase)
      - complete + total≥100K → StatusCompleted
      - complete, prev running → preserve running (no downgrade)

Phase 1: aria2 returns GID "abc123" (45 KiB metadata)
  → Poll() finds by GID, sets StatusMeta, "fetching metadata..."

Phase 2: aria2 completes metadata, creates GID "def456" (16 GiB files)
  → Poll() sorts def456 before abc123 (16GiB > 45KiB)
  → findMatching matches def456 to existing item by btName/fallback
  → existing.GID = "def456", Status = StatusRunning
  → abc123 also matched by btName, but mapStatus preserves StatusRunning

After download: aria2 seeds to ratio 2.0 (--seed-ratio=2.0)
  → Poll() detects active + completed >= total → StatusSeeding
  → When seeding completes, aria2 marks "complete" → StatusCompleted
```

### Key Types

```go
type AnimeTorrent struct {     // Search result
    Name, MagnetLink, Link string
    Seeders, Leechers int
    Size string; SizeBytes int64
    Provider, InfoHash string
    Episode int; Resolution string
    IsBatch bool
}

type DownloadItem struct {     // Active download
    GID, Name, Magnet string
    Status string              // metadata/running/paused/completed/failed/seeding
    Progress float64; Speed, TotalSize, Completed int64
    Files string; NumFiles int
}

type Provider interface {
    Name() string
    Search(ctx, query) ([]*AnimeTorrent, error)
}
```

### Kitty Image Display Layer

Suanime uses the [Kitty graphics protocol](https://sw.kovidgoyal.net/kitty/graphics-protocol/) to display cover art directly in the terminal. Images are transmitted as APC escape sequences (`\033_G...\033\\`) written to stderr.

```
Jikan cover URL
  → HTTP GET → ~/.cache/suanime/images/<file>
  → base64-encode → kitty APC escape sequence
  → write to stderr with cursor positioning
```

The escape sequence format (cursor-positioned, Z-above-text):
```
\033[s\033[<row>;<col>H\033_Ga=T,f=100,t=d,z=1,i=<id>;<base64_png>\033\\\033[u
```

- `a=T` — transmit and display
- `f=100` — PNG format (non-PNG converted via ImageMagick `convert` to `/tmp/suanime/`)
- `t=d` — direct transmission
- `z=1` — Z-index above text layer (prevents TUI text render from covering image)
- `i=<id>` — unique image ID for deletion

Images are placed at cursor position (no hardcoded pixel coordinates). Cleared on metadata updates via `\033_Ga=d,d=I,i=<id>\033\\` (delete by ID). Layout uses fixed `.Height()` to maintain consistent panel sizes across navigation.

### Jikan Metadata Flow

```
Cursor moves to new anime torrent
  → extractAnimeTitle() parses torrent name
  → jikanSearch() queries Jikan API v4
  → bestMatch() fuzzy-matches result by title overlap
  → AnimeMeta cached per session
  → jikanResultMsg triggers kitty image display
```

## JSON-RPC (aria2)

suanime communicates with aria2 via HTTP JSON-RPC on localhost:

| Method                   | Purpose              |
|--------------------------|----------------------|
| `aria2.addUri`           | Add magnet download  |
| `aria2.tellActive`       | List active downloads|
| `aria2.tellWaiting`      | List queued downloads|
| `aria2.tellStopped`      | List stopped/completed|
| `aria2.pause` / `unpause`| Pause/resume         |
| `aria2.remove`           | Remove active download|
| `aria2.forceRemove`      | Force remove         |
| `aria2.removeDownloadResult`| Clear completed      |
| `aria2.shutdown`         | Stop daemon          |
| `aria2.getVersion`       | Version check        |

**Note**: `TellActive` uses `"params,omitempty"` in the JSON request because Go marshals nil slices as `null`, which aria2's strict parser rejects with `-32602 Invalid params`.

## Provider Architecture

Each provider fetches an RSS/XML feed, parses it with a namespace-agnostic `encoding/xml` tokenizer (not struct-based), and constructs `AnimeTorrent` entries with parsed metadata (episode number, resolution, batch detection).

The tokenizer uses `decoder.Token()` to walk the XML stream, tracking element names by `t.Name.Local` (ignoring namespace prefixes like `nyaa:`). This allows the same parser to work across Nyaa.si and Sukebei which use different namespace URIs.

## Episode Parsing

Two regex patterns cover the most common naming conventions:
- `[-_\\s.](\\d{1,4})` → `- 01`, `_05`, ` 1163`, `.05`
- `[eE](\\d{1,4})` → `E05`, `e01`, `S01E05`

Batch detection: `(batch|complete|season)` keyword match + `01-12` range pattern.
