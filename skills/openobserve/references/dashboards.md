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
expects, so a copy-edit-restore round trip needs no `jq` surgery.

## Writing safely

All three write commands support `--dry-run`, which prints the exact request
plan (`method`, `url`, `body` — for updates also the read step that resolves
the current hash) and sends nothing. `--dry-run` works under read-only mode;
run it first and show the plan to the user before mutating anything.

```
openobserve-cli dashboard create --file def.json            # body from file; --file - reads stdin
openobserve-cli dashboard update <id> --file def.json       # read-modify-write, built in
openobserve-cli dashboard delete <id> --yes                 # permanent; --yes is mandatory
```

- **Read-only mode.** When `defaults.read_only` / `OPENOBSERVE_CLI_READ_ONLY`
  is set, writes fail with `READONLY_BLOCKED` (exit 5) before any request.
  Confirm with the user, then re-run the same command with `--allow-writes`
  in front: `openobserve-cli --allow-writes dashboard update …`.
- **Concurrent-edit protection.** `update` re-reads the current hash and sends
  it with the PUT; the server rejects a stale dashboard with a `conflict`
  error (exit 7). Do not blind-retry: `dashboard get` the current definition,
  re-apply the intended change, and update again. Pass `--hash` explicitly only
  to pin a specific known revision.
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
