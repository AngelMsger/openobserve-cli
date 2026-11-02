# Managing dashboards

Dashboards live in OpenObserve's meta store, so `search` / `metrics` / `trace`
can never see them — `dashboard` is the only noun that can.

## Discovery first

`dashboard list` discovers existing dashboard IDs and their folders:

```
openobserve-cli dashboard list                      # summaries in the "default" folder
openobserve-cli dashboard list --folder <id>        # another folder
openobserve-cli dashboard list --title traffic      # substring match, across folders
```

Each summary carries `dashboard_id`, `title`, `folder_id`, `folder_name`,
`owner`, `version` and the current `hash`. Folder IDs are discovered from these
rows; the server lists only the `default` folder when no filter is given, so
list a known folder or search by `--title` before concluding a dashboard is
missing. IDs are case-sensitive; reuse them verbatim. Pass the discovered
`--folder` on get/update/delete, too; otherwise those commands target `default`.
Empty folders cannot be discovered by this CLI; verify their IDs in the UI/API.

`dashboard get <id>` returns the full definition (the `v<version>` body — tabs,
panels, queries, variables) plus the top-level `version` and `hash`:

```
openobserve-cli dashboard get <id> --folder <folder-id> > backup.json
```

The output is directly re-usable by `create` and `update`: both unwrap a
get-response envelope (`{v5: {...}, hash: ...}`) down to the body OpenObserve
expects, selecting the active version and ignoring inactive `vN: null` entries.
Malformed or ambiguous envelopes are rejected; exact numeric values are retained.

## Writing safely

All three write commands support `--dry-run`, which prints the exact request
plan (`method`, `url`, `body`) and sends nothing; plans are built offline by
the API client, so they work without credentials and report an invalid
`base_url`. Update uses the explicit `--hash`, then the backup hash. If neither
is available, `request.prefetch` describes a GET whose response supplies the
PUT hash at execution time; the preview itself makes no requests.
`--dry-run` also works under read-only mode. Inspect it before applying the
authorized change; existing authorization need not be requested again. Writes
are enabled by default. Dry-run does not validate server-side schemas, permissions,
query execution or dashboard rendering.

```
openobserve-cli dashboard create --file def.json            # body from file; --file - reads stdin
openobserve-cli dashboard update <id> --file def.json       # backup hash, or prefetch for a bare definition
openobserve-cli dashboard delete <id> --yes                 # permanent; --yes is mandatory
```

- **Read-only mode.** When `defaults.read_only` / `OPENOBSERVE_CLI_READ_ONLY`
  is set, writes fail with `READONLY_BLOCKED` (exit 5) before any request.
  Only when overriding that posture is authorized, re-run the same command with
  `--allow-writes`: `openobserve-cli --allow-writes dashboard update …`.
- **Concurrent-edit protection.** An exported backup retains its hash. The
  server rejects it with `conflict` (exit 11) if another edit has happened since
  export. Do not blind-retry: fetch the current definition, re-apply the intended
  change, then update. `--hash` explicitly overrides the backup revision.
  A bare definition has no saved revision, so the client fetches the current
  hash before PUT; this protects only the interval between those two requests.
  A response missing its hash stops the update without a PUT.
- **Deletion needs `--yes`.** Without it the command exits 2
  (`CONFIRM_REQUIRED`) without touching the server. Use it only for authorized
  deletion. A deletion preview with `--dry-run` does not require `--yes`.

## Typical flows

**Backup all dashboards in a folder** — list summaries, then fetch each ID:

```bash
set -euo pipefail
folder=default
openobserve-cli dashboard list --folder "$folder" > dashboards.json
jq -r '.items[].dashboard_id' dashboards.json | while IFS= read -r id; do
  openobserve-cli dashboard get "$id" --folder "$folder" > "$id.json"
done
```

**Edit a dashboard** — get, edit locally, dry-run, then update:

```
openobserve-cli dashboard get <id> --folder <folder-id> > dashboard.json
# edit dashboard.json, keeping its hash
openobserve-cli dashboard update <id> --folder <folder-id> --file dashboard.json --dry-run
openobserve-cli dashboard update <id> --folder <folder-id> --file dashboard.json
```

**Recreate from backup** — the backup body already carries its `version`, so
`create` stores it as-is:

```
openobserve-cli dashboard create --folder <folder-id> --file dashboard.json
```

## Panel JSON gotchas

Panel schemas and layout conventions vary by dashboard version and panel type.
Start from a known-good `dashboard get` export, preserve its version and panel
structure, and change only the fields needed for the task. Discover streams
and schema before editing panel queries. Read back the saved definition to
verify persistence; rendering must be checked in OpenObserve's UI separately.
