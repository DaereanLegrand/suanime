# CLI Commands

## `suanime` (default)

Launches the TUI.

```bash
./suanime
./suanime tui
```

---

## `suanime search <query>`

Searches all trackers concurrently and outputs JSON to stdout.

```bash
./suanime search "tokyo ghoul"
```

Output format:
```json
{
  "query": "tokyo ghoul",
  "count": 227,
  "results": [
    {
      "Name": "[SubsPlease] Tokyo Ghoul - 01 (1080p)",
      "MagnetLink": "magnet:?xt=urn:btih:...",
      "Link": "https://nyaa.si/view/...",
      "Seeders": 100,
      "Leechers": 5,
      "Size": "1.4 GiB",
      "SizeBytes": 1503238553,
      "Date": "2024-01-01",
      "Provider": "Nyaa",
      "InfoHash": "abc123...",
      "Episode": 1,
      "Resolution": "1080P",
      "IsBatch": false
    }
  ]
}
```

Pipe to `jq` for filtering:
```bash
./suanime search "tokyo ghoul 01" | jq '.results[] | select(.Episode == 1 and .IsBatch == false)'
```

---

## `suanime get <query>`

Searches and downloads the best match.

### Options

| Flag            | Description                            |
|-----------------|----------------------------------------|
| `--ep N`        | Filter by episode number               |
| `--resolution R`| Filter by resolution (e.g. `1080p`)    |
| `--no-batch`    | Exclude batch/season releases          |
| `--best`        | Auto-select highest-seeder match       |

Without `--best`, uses `gum choose` for interactive selection (falls back to best match if gum not installed).

### Filtering

Results are filtered in this order:
1. Non-empty magnet link required
2. Resolution match (if `--resolution` is set)
3. Batch exclusion (if `--no-batch`)
4. Episode match (if `--ep` is set)
5. Non-zero seeders required
6. OVA/ONA/Special/Movie excluded if non-OVA results exist; included as fallback
7. Sorted by seeders descending

### Examples

```bash
# Download Tokyo Ghoul episode 1, best available
./suanime get "tokyo ghoul 01" --ep 1 --no-batch --best

# Interactive pick for Tokyo Ghoul season 1 batch
./suanime get "tokyo ghoul" --resolution 1080p

# Download with specific resolution
./suanime get "tokyo ghoul 01" --ep 1 --resolution 720p --best
```

### Download Loop

The get command:
1. Searches all trackers
2. Filters and selects best match (or prompts with gum)
3. Starts aria2 daemon on configured RPC port
4. Adds magnet via JSON-RPC (`aria2.addUri`)
5. Polls every 2 seconds for progress
6. Shows speed, ETA, completed/total bytes
7. Times out after 10 minutes (metadata timeout: 2 minutes)
8. Seeds to ratio 2.0 automatically

---

## `suanime version`

```bash
./suanime version
# suanime 1.0.0 (90f18b8)
```

The commit hash in parentheses matches the git commit used to build the binary, cross-referencing with this documentation.

---

## `suanime --help`

Prints usage summary.
