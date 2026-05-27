# TOFIX — Known Issues & Status

## Done

- [x] Search Nyaa.si, Sukebei, Tokyo Toshokan (RSS, parallel)
- [x] Download via aria2 JSON-RPC
- [x] Progress bars with speed, ETA, completed/total
- [x] Pause (`p`), resume (`r`) downloads
- [x] Remove files + torrent (`R`)
- [x] Seed indefinitely (ratio 0.0) until user pauses
- [x] Auto-resume on restart via `--input-file` + `--check-integrity`
- [x] Recover orphan `.aria2` files from download directory
- [x] Merge duplicate entries (metadata GID + file GID → single torrent entry)
- [x] Strip hash names (GID-only entries)
- [x] 1 MiB threshold for metadata vs real file completion
- [x] CLI mode: `search`, `get`, `--best`, `--ep`, `--resolution`, `--no-batch`
- [x] Jikan API metadata: cover art, score, genres, synopsis
- [x] Kitty graphics protocol for cover art
- [x] `--no-cover` flag for any terminal
- [x] Config file: `~/.config/suanime/config.json`
- [x] Build hash embedded (`suanime 1.0.0 (ce88a39)`)
- [x] Test suite: 28 tests (unit + integration + download lifecycle)
- [x] Makefile: build, test, vet, install, release

## Not Done / Needs Fix

- [ ] **File corruption repair**: Corrupt chunks are not detected/repaired
  well enough. `ffprobe` shows 0x00 bytes, EBML header failures, duplicate
  elements. `--check-integrity=true` helps on restart but doesn't catch
  everything. Need explicit repair command that:
  - Re-downloads corrupt pieces (aria2 should do this with `--check-integrity`
    but may need forced re-check via RPC `aria2.changeOption` setting
    `check-integrity=true` per-download, or remove + re-add)
  - Possibly verify all episode files with ffprobe and flag corrupt ones

- [ ] **Resuming status display**: Shows "checking integrity..." during the
  resume/verify phase but could be more granular (show % of files checked)

- [ ] **"." dot entry**: Filtered out empty/single-char names now, but the
  root cause (dirName being ".") could still produce weird entries

- [ ] **No per-file progress**: Torrents with many files don't show which
  file is currently downloading. aria2's `tellStatus` returns `files[]` with
  per-file progress, but we don't display it.

- [ ] **Download scheduling**: No queue management. All downloads run
  concurrently. Should limit concurrent downloads and allow reordering.

- [ ] **Torrent file export**: No way to export the .torrent file from a
  magnet download.

- [ ] **Tracker health**: No display of tracker status, DHT nodes, or
  peer connectivity. Downloads can stall silently.

- [ ] **Disk space check**: No warning before starting download if disk is
  nearly full.

- [ ] **Bandwidth limits**: No way to set download/upload rate limits per
  torrent or globally. aria2 supports `--max-download-limit` and
  `--max-overall-download-limit` but not exposed in config.

- [ ] **Notifications**: No desktop notification when download completes.

- [ ] **Episode deduplication**: If multiple torrents contain the same
  episode, suanime doesn't detect or warn about duplicates.
