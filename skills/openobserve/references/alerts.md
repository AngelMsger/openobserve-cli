# Managing alerts

Alerts live in OpenObserve's meta store, so `search` / `metrics` / `trace`
can never see them — `alert` is the only noun that can. Use it to back up,
recreate, silence or clean up the alerting rules that fire on the telemetry
you query with the other nouns.

## Discovery first

`alert list` is the source of every identifier the other commands accept:

```
openobserve-cli alert list                      # summaries in the "default" folder
openobserve-cli alert list --folder <id>        # another folder
```

Each summary carries `alert_id`, `name`, `stream_name`, `stream_type`,
`enabled`, `owner`, `folder_id` and `folder_name`. Folder IDs are discovered
from these rows; the server lists only the `default` folder when no filter is
given, so list a known folder before concluding an alert is missing. IDs are
case-sensitive; reuse them verbatim. The `stream_name` of a summary (or the
`stream list` output) is the only valid target for a new alert — never invent
one.

`alert get <id>` returns the full definition — `name`, `description`,
`stream_type`, `stream_name`, `is_real_time`, `query_condition`,
`trigger_condition`, `context_attributes` and `destinations`:

```
openobserve-cli alert get <id> > backup.json
```

The output is directly re-usable by `create` and `update`: both strip the
server-managed fields (`id`, `owner`, `created_at`, `updated_at`,
`last_triggered_at`, `last_satisfied_at`, `folder_id`, `folder_name`), so a
copy-edit-restore round trip needs no `jq` surgery.

## Writing safely

Every write command supports `--dry-run`, which prints the exact request plan
(`method`, `url`, and for create/update the `body`) and sends nothing.
`--dry-run` works under read-only mode; run it first and show the plan to the
user before mutating anything.

```
openobserve-cli alert create --file alert.json --dry-run
openobserve-cli --allow-writes alert create --file alert.json
```

When the session is read-only (`OPENOBSERVE_CLI_READ_ONLY=1` or
`defaults.read_only: true`), every write exits 5 with `READONLY_BLOCKED`
before any request leaves the process. Lift the posture with the root
`--allow-writes` flag — never silently, always with the user's explicit
confirmation in the same session.

**enable/disable needs no definition file.** Silencing an alert that fires on
a known outage is `alert disable <id>`, not a full update round trip:

```
openobserve-cli alert disable <id>
openobserve-cli alert enable <id>
```

Both are writes: they are still gated by read-only mode, so pass
`--allow-writes` only after confirming with the user.

**Deletion needs `--yes`.** Without it the command exits 2
(`CONFIRM_REQUIRED`) without touching the server. Never add `--yes` on the
user's behalf without explicit confirmation in the same session.

## Typical flows

**Back up every alert in the default folder** — list summaries, then fetch
each definition:

```
for id in $(openobserve-cli alert list | jq -r '.items[].alert_id'); do
  openobserve-cli alert get "$id" > "$id.json"
done
```

**Edit an alert** — get, edit locally, dry-run, then update:

```
openobserve-cli alert get <id> > alert.json
# edit alert.json
openobserve-cli alert update <id> --file alert.json --dry-run
openobserve-cli --allow-writes alert update <id> --file alert.json
```

**Recreate from backup** — the backup body already carries the definition, so
`create` stores a fresh alert from it (server-managed fields are stripped):

```
openobserve-cli alert create --file alert.json
```

**Quiet a known-noisy alert during an incident** — disable, investigate with
the query nouns, re-enable:

```
openobserve-cli --allow-writes alert disable <id>
# … investigate …
openobserve-cli --allow-writes alert enable <id>
```

## Definition gotchas

When authoring or fixing a definition from scratch, keep to the shape the
server accepts: `stream_type` is one of `logs` / `metrics` / `traces`;
`query_condition.conditions` (SQL alerts) or `query_condition.promql`
(metrics alerts) must reference the `stream_name` the alert is attached to;
`trigger_condition.period` is minutes and `threshold` is the number of
triggered evaluations before firing; `destinations` reference existing alert
destinations (emails / webhooks configured in the OpenObserve UI or API).
Verify a new definition with `--dry-run`, then confirm it landed by listing or
getting it after creation — never report an alert as working from a 2xx
alone.
