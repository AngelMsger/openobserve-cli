# Managing dashboards

Dashboards live in OpenObserve's meta store, so `search` / `metrics` / `trace`
can never see them — `dashboard` is the only noun that can.

## Discovery first

`dashboard list` is the source of every identifier the other commands accept:

```
openobserve-cli dashboard list                      # summaries in the "default" folder
openobserve-cli dashboard list --folder <id>        # another folder
openobserve-cli dashboard list --title traffic      # substring match, across folders
```

Each summary carries `dashboard_id`, `title`, `folder_id`, `folder_name`,
`owner`, `version` and the current `hash`. Folder IDs are discovered from these
rows; the server lists only the `default` folder when no filter is given, so
list a known folder or search by `--title` before concluding a dashboard is
missing. IDs are case-sensitive; reuse them verbatim.

`dashboard get <id>` returns the full definition (the `v<version>` body — tabs,
panels, queries, variables) plus the top-level `version` and `hash`:

```
openobserve-cli dashboard get <id> > backup.json
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
`--dry-run` also works under read-only mode — run it first and show the plan
to the user before mutating anything.

```
openobserve-cli dashboard create --file def.json            # body from file; --file - reads stdin
openobserve-cli dashboard update <id> --file def.json       # backup hash, or prefetch for a bare definition
openobserve-cli dashboard delete <id> --yes                 # permanent; --yes is mandatory
```

- **Read-only mode.** When `defaults.read_only` / `OPENOBSERVE_CLI_READ_ONLY`
  is set, writes fail with `READONLY_BLOCKED` (exit 5) before any request.
  Confirm with the user, then re-run the same command with `--allow-writes`
  in front: `openobserve-cli --allow-writes dashboard update …`.
- **Concurrent-edit protection.** An exported backup retains its hash. The
  server rejects it with `conflict` (exit 7) if another edit has happened since
  export. Do not blind-retry: fetch the current definition, re-apply the intended
  change, then update. `--hash` explicitly overrides the backup revision.
  A bare definition has no saved revision, so the client fetches the current
  hash before PUT; this protects only the interval between those two requests.
  A response missing its hash stops the update without a PUT.
- **Deletion needs `--yes`.** Without it the command exits 2
  (`CONFIRM_REQUIRED`) without touching the server. Never add `--yes` on the
  user's behalf without explicit confirmation in the same session.

## Typical flows

**Backup all dashboards in a folder** — list summaries, then fetch each ID:

```
for id in $(openobserve-cli dashboard list | jq -r '.items[].dashboard_id'); do
  openobserve-cli dashboard get "$id" > "$id.json"
done
```

**Edit a dashboard** — get, edit locally, dry-run, then update:

```
openobserve-cli dashboard get abc123 > abc123.json
# edit abc123.json
openobserve-cli dashboard update abc123 --file abc123.json --dry-run
openobserve-cli --allow-writes dashboard update abc123 --file abc123.json
```

**Recreate from backup** — the backup body already carries its `version`, so
`create` stores it as-is:

```
openobserve-cli dashboard create --file abc123.json
```

## Panel JSON gotchas

When authoring or fixing a definition from scratch, keep to the shape the
server accepts: layouts use a 192-column grid (`x`, `y`, `w`, `h`, `i`); every
panel query needs `fields` with `x`/`y` populated even for custom SQL, using
SELECT aliases as column values; `z` stays empty outside heatmaps,
stacked charts and geo maps; `filter` is an object, not an array. Verify a new
definition with `--dry-run`, then confirm it renders by listing or getting it
after creation — never report a dashboard as working from a 2xx alone.
