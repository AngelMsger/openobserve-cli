# Errors and exit codes

Every failure is JSON on **stderr** (stdout stays clean), shaped:

```json
{
  "error": {
    "category": "not_found",
    "code": "STREAM_NOT_FOUND",
    "message": "no stream named \"app\" in org \"default\"",
    "hint": "Stream names are case-sensitive in queries.",
    "next_steps": ["openobserve-cli stream list --org default"],
    "retryable": false,
    "http_status": 404
  }
}
```

Read `next_steps` first — it names the command to run next. `retryable` tells you
whether a retry could help (rate-limit / network / server) or not.

## Category → exit code

| Category     | Exit | Meaning / typical fix |
|--------------|------|-----------------------|
| (success)    | 0    | — |
| `internal`   | 1    | Unexpected bug; retry with `--verbose`. |
| `usage`      | 2    | Bad flag/argument; check `--help`. Missing time range, bad `--order`, etc. |
| `config`     | 3    | Not configured / no server URL → `config init` or set `OPENOBSERVE_URL`. |
| `auth`       | 4    | Credentials rejected → `auth status`, re-run `config init`. |
| `permission` | 5    | Authenticated but not allowed (OpenObserve RBAC) → grant the credential a role; see below. |
| `not_found`  | 6    | Unknown org / stream → `org list`, `stream list`. |
| `rate_limit` | 7    | Too many requests; retryable. Narrow the time range / lower `--limit`. |
| `network`    | 8    | Server unreachable; retryable → `doctor`, check `--base-url`. |
| `server`     | 9    | OpenObserve 5xx; retryable. |
| `parse`      | 10   | Response didn't match expectations; retry with `--verbose`. |
| `conflict`   | 11   | Resource changed since read; re-fetch then retry. |

Scripted use:

```bash
if ! openobserve-cli search run --stream app --since 1h >/tmp/hits.json; then
  case $? in
    3|4) echo "fix auth/config" ;;
    6)   echo "stream missing — run: openobserve-cli stream list" ;;
    7|8|9) echo "transient — retry later" ;;
  esac
fi
```

## Common cases

- **`NO_BASE_URL` (config/3)** — no server configured. Run `config init` or set
  `OPENOBSERVE_URL`.
- **`AUTH_LOGIN_NEEDS_TTY` (auth/4)** — `auth login`/`config init` need a
  terminal. In CI/agents set `OPENOBSERVE_EMAIL`+`OPENOBSERVE_PASSWORD` (or
  `OPENOBSERVE_TOKEN`).
- **`AUTH_BAD_SESSION` (config/3)** — the browser-captured session is malformed
  or contains no cookies. Sign in through o3 again to replace it.
- **`SESSION_BROWSER_MANAGED` (usage/2)** — `config init` or `auth login` was
  used on a session context. Browser sessions are created and refreshed in o3;
  the CLI can consume, inspect, switch, and log out of them.
- **`BAD_TIME_RANGE` (usage/2)** — pass `--since 1h` or `--from`/`--to`.
- **`STREAM_NOT_FOUND` (not_found/6)** — run `stream list`; names are
  case-sensitive.
- **`PROMQL_ERROR` (usage/2)** — a `metrics query` expression was rejected
  (typo'd metric name or bad PromQL). Discover names with
  `stream list --type metrics`; remember metrics are PromQL, not SQL.
- **`TRACE_NOT_FOUND` (not_found/6)** — `trace get` found no spans for that
  `trace_id` in the window; widen `--since` or confirm the id/stream with
  `trace search --stream <name>`.
- **`HTTP_UNAUTHORIZED` (auth/4)** — wrong password/token → re-run `config init`.
- **`HTTP_FORBIDDEN` (permission/5)** — the credential authenticates but has no
  role for the resource. This is the classic **service account** case: under
  OpenObserve RBAC (Enterprise/Cloud) a new user / service account is granted
  nothing by default. `org list` works while `stream list` / `search` 403 for
  exactly this reason. Fix in OpenObserve: **IAM → Roles**, grant a role the
  **Streams** resource (List + Get, for the streams you'll query), then assign
  the user / service account to that role (its **Roles** / **Service Accounts**
  tab).
