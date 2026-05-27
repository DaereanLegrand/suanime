# suanime

Anime torrent search and download utility. Searches Nyaa.si, Sukebei, and Tokyo Toshokan concurrently, downloads via aria2 with progress tracking, pause/resume/cancel, and seeds to 2.0 ratio.

Build: `go build -ldflags="-X main.commit=$(git rev-parse --short HEAD)" -o suanime .`

## Quick Start

```bash
# Launch TUI
./suanime

# CLI search (JSON)
./suanime search "tokyo ghoul"

# Auto-download best match
./suanime get "tokyo ghoul 01" --ep 1 --resolution 1080p --no-batch --best

# Interactive pick
./suanime get "tokyo ghoul 01" --no-batch
```

## Requirements

- **Go** 1.22+
- **aria2c** (`pacman -S aria2`, `apt install aria2`, `brew install aria2`)
- **gum** (optional, for interactive selection)

## Trackers

| Provider       | Method | Seeders/Leechers | Magnet Links |
|----------------|--------|------------------|-------------|
| Nyaa.si        | RSS    | Yes              | Yes         |
| Nyaa Sukebei   | RSS    | Yes              | Yes         |
| Tokyo Toshokan | RSS    | No (index only)  | Yes         |

All searches run in parallel across providers with a 15-second timeout.

## Architecture

See [ARCHITECTURE.md](ARCHITECTURE.md) for detailed code structure and data flow.

Magnet downloads use a two-phase aria2 flow:
1. **Metadata phase** (GID 1): Fetches .torrent info via DHT (~45 KiB, 2-10 seconds)
2. **File download phase** (GID 2): Downloads actual files (auto-linked by torrent name/overlap)

The `Poll()` engine tracks both phases as a single logical download, preserving the human-readable name from search results.

## CLI Reference

See [CLI.md](CLI.md) for full command reference.

## Configuration

See [CONFIG.md](CONFIG.md) for config file documentation.

## Key Bindings (TUI)

### Search Tab
| Key           | Action                        |
|---------------|-------------------------------|
| `/`           | Focus search bar (clear query)|
| `enter`       | Search / Download selected    |
| `j`/`k`, ↑↓  | Navigate results              |
| `o`           | Open torrent link in browser  |
| `esc`         | Blur search / re-focus        |
| `tab`         | Switch to Downloads tab       |
| `q`           | Quit                          |

### Downloads Tab
| Key           | Action                        |
|---------------|-------------------------------|
| `j`/`k`, ↑↓  | Navigate downloads            |
| `p`           | Pause selected                |
| `r`           | Resume selected               |
| `c`           | Cancel selected               |
| `tab`         | Switch to Search tab          |
| `q`           | Quit                          |

## Test Suite

```bash
# Unit tests (fast, no network)
go test ./... -short

# All tests including integration
go test ./... -count=1 -timeout 120s
```
