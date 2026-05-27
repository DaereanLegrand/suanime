# Architecture

## Code Structure (hash: `90f18b8`)

```
suanime/
  main.go                   CLI entry point, subcommands
  providers/
    provider.go             AnimeTorrent struct, Provider interface, SearchAll, parsers
    nyaa.go                 Nyaa.si + Sukebei RSS provider, parseNyaaRSS
    tokyotosho.go           Tokyo Toshokan RSS provider, parseTTRSS
    *_test.go               17 unit + integration tests
  tui/
    model.go                Bubbletea TUI: search input, results list, key handling
    download.go             DownloadManager, aria2 lifecycle, Poll engine, downloadsView
    aria2.go                JSON-RPC client, progressBar, parseLength, progressPct
    config.go               Config loading/saving, DefaultConfig
    styles.go               Lipgloss theme (dark purple), Navbar, FormatPeers
    *_test.go               5 download integration tests
  docs/
    README.md               Overview, quick start, features
    CLI.md                  CLI command reference
    CONFIG.md               Config file documentation
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
