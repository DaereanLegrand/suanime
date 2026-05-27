# Cover Art & Metadata Feature

## Overview

When viewing search results, the right half of the terminal displays cover art and metadata for the anime being searched, fetched from the Jikan API (MyAnimeList).

## Requirements

- **Kitty terminal only by default**: The TUI refuses to start if not running in kitty (`$TERM=xterm-kitty` or `$KITTY_WINDOW_ID` set)
- **`--no-cover` flag**: Skip cover art and work in any terminal. Metadata panel still shows all text info.
- **Always-on 50/50 split**: Left pane = search results, right pane = cover art + metadata
- **Jikan API v4**: Unofficial MyAnimeList API for anime metadata
- **Kitty graphics protocol**: Native terminal image display via APC escape sequences

## Architecture

```
suanime/
  tui/
    jikan.go       ← NEW: Jikan API client, fuzzy matching, caching
    kitty.go       ← NEW: Kitty detection, image display, cleanup
    model.go       ← MODIFIED: split-pane layout, Jikan fetch flow
    styles.go      ← MODIFIED: metadata panel styles, synopsis render
  main.go          ← MODIFIED: Kitty detection at startup
```

## Jikan API (`tui/jikan.go`)

### Endpoint

```
GET https://api.jikan.moe/v4/anime?q=<title>&limit=3
```

### Response Fields Used

| Field      | JSON Path                      | Display             |
|------------|--------------------------------|---------------------|
| Cover URL  | `images.webp.large_image_url` | Kitty image         |
| Score      | `score`                        | ★ 7.79             |
| Year       | `year`                         | 2014               |
| Type       | `type`                         | TV / Movie / OVA   |
| Episodes   | `episodes`                     | 12 episodes        |
| Popularity | `popularity`                   | Popularity: #10    |
| Rank       | `rank`                         | Rank: #1235        |
| Synopsis   | `synopsis`                     | Scrollable text     |
| Genres     | `genres[].name`                | Action · Fantasy   |
| Studios    | `studios[].name`               | Studio Pierrot     |

### Fuzzy Matching

1. Extract candidate title from search query (non-filter tokens)
2. Query Jikan with the candidate
3. Score each result: substring match in title_english > title > synonyms
4. Return highest-scoring match

### Caching

- `map[string]*AnimeMeta` keyed by normalized search query
- Cached per session (no disk cache for metadata)
- Rate limit: 60 req/min — comfortable for interactive use

## Kitty Graphics Protocol (`tui/kitty.go`)

Kitty only supports `f=24` (RGB), `f=32` (RGBA), and `f=100` (PNG) for in-band transmission. Images from Jikan (JPEG/WebP) must be decoded and re-encoded as PNG before sending. Image data is written directly to `/dev/tty` (not stderr) to bypass the TUI framework's output buffer — the same approach used by lf's image preview system.

### Detection

```go
func requiresKitty() bool {
    return os.Getenv("TERM") == "xterm-kitty" || os.Getenv("KITTY_WINDOW_ID") != ""
}
```

On startup, if kitty not detected: print error and `os.Exit(1)`.

### Display Image

Image bytes are decoded (JPEG/WebP → raw pixels) and re-encoded as PNG. The PNG data is then transmitted via a kitty APC escape sequence written directly to `/dev/tty`:

```
\033[s\033[<row>;<col>H\033_Ga=T,f=100,t=d,z=1,i=<id>;<base64_png>\033\\\033[u
```

- `\033[s` / `\033[u` — save/restore cursor position
- `\033[<row>;<col>H` — move cursor to target cell (row+1, col+1)
- `a=T` — transmit and display
- `f=100` — PNG format (always — non-PNG sources converted via ImageMagick `convert`)
- `t=d` — direct transmission
- `z=1` — Z-index above text layer (prevents TUI text render from covering the image)
- `i=<id>` — unique image ID for deletion tracking

Non-PNG images (WebP, JPEG) are converted via `convert <src> png:/tmp/suanime/<name>.png`. Temporary PNGs are cleaned up on exit.

### Clear Image

```
\033_Ga=d,d=I,i=<image_id>\033\\
```

- `a=d` — delete
- `d=I` — delete by image ID
- `<image_id>` — numeric ID assigned on display

### Image Pipeline

```
Jikan JPG/WebP URL
  → HTTP GET to ~/.cache/suanime/images/<filename>
  → Read file bytes
  → If not PNG: decode (image.Decode / jpeg.Decode), re-encode as PNG
  → Base64-encode PNG bytes
  → Write kitty APC escape sequence to /dev/tty
  → Cursor positioned at right pane coordinates
```

### Layout Constraints

The search view uses a 50/50 split. Available content height is `m.height - 4` (nav bar + footer). The results list caps visible items to `maxH - 2`. The meta panel tracks header line count and truncates synopsis to fit within remaining height. Both panels are wrapped in `MaxHeight()` to prevent overflow past the terminal boundary.

## Layout: Always-On 50/50 Split

Search results view:

```
┌──────────────────────────────┬────────────────────────────┐
│ Search results (left 50%)    │ Cover + metadata (right)   │
│                              │                            │
│  > result 1                  │  ┌──────────────────┐     │
│    Ep 1  1080P  S:100 L:5    │  │   COVER ART      │     │
│                              │  │   (kitty image)  │     │
│  > result 2                  │  └──────────────────┘     │
│    Ep 1  720P   S:50  L:2    │                            │
│                              │  ★ 7.79    2014    TV     │
│                              │  #1235    Pop: #10        │
│                              │  12 episodes              │
│                              │  Action · Fantasy · Horror│
│                              │  Studio Pierrot           │
│                              │                            │
│                              │  Synopsis: A sinister     │
│                              │  threat is invading       │
│                              │  Tokyo: flesh-eating      │
│                              │  "ghouls"...              │
├──────────────────────────────┴────────────────────────────┤
│ / search  enter download  j/k move  q quit               │
└──────────────────────────────────────────────────────────┘
```

### Cursor Interaction

- Moving cursor between torrents of the same anime → no panel refresh
- Moving to a different anime → re-fetch metadata and display
- New search → clear panel, fetch new metadata

## Implementation Order

1. `tui/kitty.go` — detection + image display primitives
2. `tui/jikan.go` — API client + parsing + cache
3. `tui/styles.go` — metadata panel styles (score, genres, synopsis)
4. `tui/model.go` — split-pane layout, Jikan integration
5. `main.go` — Kitty check at startup
6. Tests: Jikan parsing, fuzzy matching, image path resolution

## Test Plan

- **Unit**: Jikan response JSON parsing → `AnimeMeta` struct
- **Unit**: Fuzzy title extraction from torrent names
- **Unit**: Image path resolution (local cache, download fallback)
- **Integration**: Live Jikan API call, verify response parsing
- **Integration**: Kitty protocol escape sequence generation
