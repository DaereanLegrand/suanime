<!-- Cover art: Tokyo Ghoul (MAL #22319) -->
<p align="center">
  <img src="https://cdn.myanimelist.net/images/anime/1498/134443l.webp" width="300" alt="Tokyo Ghoul"/>
</p>

<p align="center">
  <strong>suanime</strong> — anime torrent search &amp; download in your terminal
</p>

<p align="center">
  <img src="https://img.shields.io/badge/go-1.22+-00ADD8?logo=go" alt="Go"/>
  <img src="https://img.shields.io/badge/terminal-kitty-d18fee?logo=windowsterminal" alt="kitty"/>
  <img src="https://img.shields.io/badge/downloads-aria2-6db33f?logo=transmission" alt="aria2"/>
  <img src="https://img.shields.io/badge/metadata-Jikan%20API-2e51a2?logo=myanimelist" alt="Jikan"/>
</p>

---

Searches **Nyaa.si**, **Sukebei**, and **Tokyo Toshokan** concurrently. Downloads via **aria2** with progress tracking, pause/resume/cancel, and seeds to 2.0 ratio. Shows **cover art &amp; metadata** (score, year, genres, synopsis) from MyAnimeList via the Jikan API.

## Features

- **3 trackers** active by default — all searched in parallel (15s timeout)
- **aria2 RPC backend** — real progress bars, speed, ETA, pause/resume/cancel
- **Seed to 2.0 ratio** — automatic, configurable in `config.json`
- **Kitty graphics protocol** — cover art displayed natively in the terminal
- **Jikan metadata** — fuzzy-matched covers, scores, genres, synopses
- **CLI mode** — `search` (JSON), `get` (auto-download), `version`
- **Split-pane TUI** — search results on the left, cover + metadata on the right
- **Config file** — `~/.config/suanime/config.json`
- **Test suite** — 28 tests (unit + integration + download lifecycle)

## Quick Start

```bash
git clone https://github.com/DaereanLegrand/suanime
cd suanime
go build -ldflags="-X main.commit=$(git rev-parse --short HEAD)" -o suanime .
./suanime
```

## Requirements

- **Go** 1.22+
- **aria2c** — `pacman -S aria2` | `apt install aria2` | `brew install aria2`
- **kitty terminal** — required (native image display)
- **gum** (optional) — for `suanime get` interactive selection

## Usage

```bash
# TUI (default)
./suanime

# CLI search — JSON to stdout
./suanime search "tokyo ghoul"

# Auto-download best match
./suanime get "tokyo ghoul 01" --ep 1 --resolution 1080p --no-batch --best

# Interactive pick (gum)
./suanime get "tokyo ghoul" --no-batch
```

## TUI Keybindings

| Key | Search Tab | Downloads Tab |
|-----|-----------|---------------|
| `/` | Focus search | — |
| `enter` | Search / Download | — |
| `j`/`k` | Navigate results | Navigate downloads |
| `d` | Download selected | Remove |
| `p` | — | Pause |
| `r` | — | Resume |
| `c` | — | Cancel |
| `tab` | Downloads tab | Search tab |
| `q` | Quit | Quit |

## Configuration

`~/.config/suanime/config.json` (created on first run):

```json
{
  "download_dir": "/home/user/Downloads/suanime",
  "aria2_rpc_port": 6800,
  "aria2_rpc_secret": ""
}
```

## Architecture

```
search query
  → Nyaa RSS ────────┐
  → Sukebei RSS ─────┤ goroutines → merged, sorted by seeders
  → TokyoToshokan RSS ┘
  → TUI renders results table (left 50%)
  → Jikan API → cover art + metadata (right 50%)
  → enter/d → aria2.addUri RPC
  → 2-phase magnet: metadata GID → file download GID (auto-linked)
  → Poll() every 2s: speed, ETA, progress bar
  → Complete → seed to 2.0 ratio
```

## Directory

```
suanime/
  main.go              CLI + TUI entry point
  providers/
    provider.go        AnimeTorrent, SearchAll, parsers
    nyaa.go            Nyaa.si + Sukebei RSS
    tokyotosho.go      Tokyo Toshokan RSS
  tui/
    model.go           Bubbletea TUI, split-pane, keybindings
    download.go        DownloadManager, Poll engine, aria2 lifecycle
    aria2.go           JSON-RPC client
    jikan.go           Jikan API v4, fuzzy matching, caching
    kitty.go           Kitty graphics protocol, image display
    config.go          Config loading/saving
    styles.go          Dark purple theme
  docs/
    README.md, CLI.md, CONFIG.md, ARCHITECTURE.md, COVERART.md
```

## Tests

```bash
go test ./... -count=1 -timeout 120s
# 28 tests: unit + integration + download lifecycle
```

## License

MIT
