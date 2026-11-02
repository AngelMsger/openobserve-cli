# Managing alerts

Alerts live in OpenObserve's meta store, so `search` / `metrics` / `trace`
can never see them — `alert` is the only noun that can. Use it to back up,
recreate, silence or clean up the alerting rules that fire on the telemetry
you query with the other nouns.

## Discovery first

`alert list` discovers existing alert IDs and their folders:

```
openobserve-cli alert list                      # summaries across visible folders
openobserve-cli alert list --folder <id>        # another folder
```

Each summary carries `alert_id`, `name`, `stream_name`, `stream_type`,
`enabled`, `owner`, `folder_id` and `folder_name`. Folder IDs are discovered
from these rows; without `--folder` the server lists all visible folders.
For get/update/delete/enable/disable, pass the discovered `--folder` when it
is not `default` (required for folder-scoped permissions). IDs are
case-sensitive; reuse them verbatim. The `stream_name` of a summary (or the
`stream list` output) is the only valid target for a new alert — never invent
one. Empty folders and notification destinations cannot be listed by this CLI;
use a known definition or verify those in OpenObserve's UI/API. Do not assume
an empty alert list means that no folders or destinations exist.

`alert get <id>` returns the full definition — `name`, `description`,
`stream_type`, `stream_name`, `is_real_time`, `query_condition`,
`trigger_condition`, `context_attributes` and `destinations`:

```
openobserve-cli alert get <id> --folder <folder-id> > backup.json
```

The output is directly re-usable by `create` and `update`: both strip the
server-managed fields (`id`, `owner`, `created_at`, `updated_at`,
`last_triggered_at`, `last_satisfied_at`, `last_edited_by`, `folder_id`, `folder_name`, `alert_id`), so a
copy-edit-restore round trip needs no `jq` surgery.

## Writing safely

Every write command supports `--dry-run`, which prints the exact request plan
(`method`, `url`, and for create/update the `body`) and sends nothing.
`--dry-run` works without credentials and under read-only mode. Inspect it
before applying the authorized change; authorization already given in this
session does not need to be requested again. Writes are enabled by default.

```
openobserve-cli alert create --file alert.json --dry-run
openobserve-cli alert create --file alert.json
```

When the session is read-only (`OPENOBSERVE_CLI_READ_ONLY=1` or
`defaults.read_only: true`), every write exits 5 with `READONLY_BLOCKED`
before any request leaves the process. Lift the posture with the root
`--allow-writes` flag only when that override is authorized. It is not required
for ordinary writes when read-only mode is off.

**enable/disable needs no definition file.** Silencing an alert that fires on
a known outage is `alert disable <id>`, not a full update round trip:

```
openobserve-cli alert disable <id>
openobserve-cli alert enable <id>
```

Both are writes and honor the same authorization and read-only gate.

**Deletion needs `--yes`.** Without it the command exits 2
(`CONFIRM_REQUIRED`) without touching the server. Use it only for authorized
deletion. `alert delete <id> --dry-run` previews without `--yes`.

## Typical flows

**Back up every alert in the default folder** — list summaries, then fetch
each definition:

```bash
set -euo pipefail
folder=default
openobserve-cli alert list --folder "$folder" > alerts.json
jq -r '.items[].alert_id' alerts.json | while IFS= read -r id; do
  openobserve-cli alert get "$id" --folder "$folder" > "$id.json"
done
```

**Edit an alert** — get, edit locally, dry-run, then update:

```
openobserve-cli alert get <id> --folder <folder-id> > alert.json
# edit alert.json
openobserve-cli alert update <id> --folder <folder-id> --file alert.json --dry-run
openobserve-cli alert update <id> --folder <folder-id> --file alert.json
```

**Recreate from backup** — the backup body already carries the definition, so
`create` stores a fresh alert from it (server-managed fields are stripped):

```
openobserve-cli alert create --file alert.json
```

**Quiet a known-noisy alert during an incident** — disable, investigate with
the query nouns, re-enable:

```
openobserve-cli alert disable <id> --folder <folder-id>
# … investigate …
openobserve-cli alert enable <id> --folder <folder-id>
```

## Definition gotchas

When authoring or fixing a definition from scratch, keep to the shape the
server accepts: `stream_type` is one of `logs` / `metrics` / `traces`;
`query_condition.conditions` (SQL alerts) or `query_condition.promql`
(metrics alerts) must reference the `stream_name` the alert is attached to;
`trigger_condition.period` is the lookback window in minutes; `threshold`
is compared with the query result using `operator`, not a count of successive
evaluations. `destinations` reference existing alert
destinations (emails / webhooks configured in the OpenObserve UI or API).
Preview a new definition with `--dry-run`, then get it after creation to verify
stored state. Alert evaluation and notification delivery need separate evidence;
neither a dry-run, a 2xx acknowledgement nor a readback proves they work.

## API compatibility

Alert operations use OpenObserve's ID-based v2 API (`/api/v2/{org}/alerts`).
Enable/disable uses `PATCH .../{id}/enable?value=true|false`. Servers exposing
only the older stream/name API are not supported by these commands.

Create/update return compact acknowledgements. Use `alert get` to fetch a full
definition; an update acknowledgement need not contain an ID, so the CLI
reports the ID supplied to the command. Dry-run validates and normalizes JSON
but does not verify streams, destinations, permissions or other server rules.
