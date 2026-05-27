# Configuration

Config file location: `~/.config/suanime/config.json`

Created automatically on first run with defaults.

## Schema

```json
{
  "download_dir": "/home/user/Downloads/suanime",
  "aria2_rpc_port": 6800,
  "aria2_rpc_secret": ""
}
```

## Fields

### `download_dir`

Directory where aria2 saves downloaded files. Created if it doesn't exist.

Default: `$HOME/Downloads/suanime`

### `aria2_rpc_port`

TCP port for aria2's JSON-RPC daemon. suanime starts its own aria2 instance on this port. If the port is in use, suanime sends a shutdown RPC to the existing process before starting.

Default: `6800`

### `aria2_rpc_secret`

RPC secret token for aria2 authentication. Leave empty for no authentication.

If set, suanime passes `--rpc-secret` to the aria2 daemon and includes the `X-Aria2-Token` header in all RPC requests.

Default: `""` (no auth)

## Aria2 Behavior

suanime manages its own aria2 daemon process. On startup:
1. Sends `aria2.shutdown` RPC to any process on the configured port
2. Clears stale session files
3. Starts a new aria2 daemon with:
   - `--enable-rpc` on configured port
   - `--seed-ratio=2.0` (auto-seed to ratio 2.0, then stop)
   - `--file-allocation=none` (no preallocation of files)
   - `--rpc-allow-origin-all`
   - `--rpc-listen-all=false` (localhost only)
4. Downloads go to `download_dir`

On shutdown:
1. Sends `aria2.shutdown` RPC
2. Kills the aria2 process

Session files (`download_dir/.suanime-aria2.session`) are cleaned on every startup to prevent stale/corrupt state from previous runs.

## aria2c Binary

suanime requires `aria2c` in PATH. If not found, a warning is printed and downloads won't work.
