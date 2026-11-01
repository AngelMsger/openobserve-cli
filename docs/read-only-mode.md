# Read-only mode

Read-only mode is a session-level safety switch that blocks every mutating
client method **before any HTTP request is sent**. It gives you a "let an agent
explore freely without risk" posture.

## Current status

The **alert commands** (`alert create|update|delete|enable|disable`) are the
first write commands: when the posture is read-only each is blocked before any
HTTP request is sent. Telemetry commands (logs search / histogram / tail,
PromQL metrics, trace search / get) are pure reads and pass through either way.
The remaining planned writes (dashboards, functions / pipelines, users,
ingestion) plug into the same wrapper without rework. This page documents how
the gating works.

## Enabling it

Three layers, in precedence order:

1. **Config file** — `defaults.read_only: true` in
   `~/.angelmsger/openobserve/config.yaml`.
2. **Environment** — `OPENOBSERVE_CLI_READ_ONLY=1`.
3. **Per-invocation override** — the root `--allow-writes` flag flips the posture
   back to read-write for a single command:

   ```bash
   OPENOBSERVE_CLI_READ_ONLY=1 openobserve-cli --allow-writes <write-command>
   ```

`appState.readOnly()` is `defaults.read_only && !--allow-writes`.

## How it works

When the posture is read-only, the API client is wrapped by
`apiclient.NewReadOnly` before any command runs (`pkg/apiclient/readonly.go`).
The wrapper embeds the real client, so every **read** passes straight through.
Each **write** method (`CreateAlert`, `UpdateAlert`, `DeleteAlert` and
`SetAlertEnabled` today) overrides the embedded one to return a structured
error instead of issuing a request:

```json
{
  "error": {
    "category": "permission",
    "code": "READONLY_BLOCKED",
    "message": "read-only mode is active; this is a write operation",
    "hint": "Pass --allow-writes to override for this invocation.",
    "next_steps": ["openobserve-cli --allow-writes <command>"],
    "retryable": false
  }
}
```

`READONLY_BLOCKED` maps to exit code 5 (`permission`). The block happens in the
client, so it is enforced regardless of which command triggered the write.

## Relationship to `--dry-run`

`--dry-run` (on every write command) prints the HTTP request that *would*
be sent without sending it. Because dry-run issues no request, it remains usable
under read-only mode — the read-only wrapper lets the request-description path
through unchanged. So `--dry-run` answers "what would this do?" and read-only
answers "make sure nothing can actually do it"; they compose.
