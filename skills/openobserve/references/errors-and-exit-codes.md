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
whether a retry in the same environment could help (rate-limit / network /
server) or not; a failed write is never retryable, since it may have been
applied (see "Write outcome is unknown" below). Environment changes use the
optional `recovery` object instead.

## Category → exit code

| Category     | Exit | Meaning / typical fix |
|--------------|------|-----------------------|
| (success)    | 0    | — |
| `internal`   | 1    | Unexpected bug; retry with `--verbose`. |
| `usage`      | 2    | Bad flag/argument; check `--help`. Missing time range, bad `--order`, etc. |
| `config`     | 3    | Config/credential resolution failed; inspect `code` and `recovery` before reconfiguring. |
| `auth`       | 4    | Follow `next_steps`: reuse a matching login before signing in again. |
| `permission` | 5    | Authenticated but not allowed (OpenObserve RBAC) → grant the credential a role; see below. |
| `not_found`  | 6    | Unknown org / stream → `org list`, `stream list`. |
| `rate_limit` | 7    | Too many requests; retryable. Narrow the time range / lower `--limit`. |
| `network`    | 8    | Server unreachable; retryable → `doctor`, check `--base-url`. |
| `server`     | 9    | OpenObserve 5xx; retryable. |
| `parse`      | 10   | Response didn't match expectations; retry with `--verbose`. |
| `conflict`   | 11   | Resource changed since read; re-fetch then retry. |

Scripted use:

```bash
if openobserve-cli search run --stream app --since 1h >hits.json; then
  : # consume hits.json
else
  rc=$?
  case "$rc" in
    3|4) echo "fix auth/config" ;;
    6)   echo "stream missing — run: openobserve-cli stream list" ;;
    7|8|9) echo "transient — retry later" ;;
  esac
  exit "$rc"
fi
```

## Common cases

- **"Write outcome is unknown for …"** → an alert or dashboard write failed
  after it may have reached OpenObserve (network error, 429, 5xx, or a
  redirect, a 3xx `http_status` that usually means a login page or single
  sign-on proxy: check the base URL with `doctor`). It is `retryable=false`, whatever its category: do not
  repeat it. Run the read in `next_steps` (`alert get`, `dashboard get`, or
  for a create `alert list --folder` / `dashboard list --folder --title`), and
  repeat the write only if the read shows it did not happen.
- **`WRITE_SUCCEEDED_RESPONSE_INVALID`** → the write happened, but its response
  could not be decoded or named no ID for what it created. Find it with the
  read in `next_steps`; never create it again.
- **`CREDENTIAL_STORE_INACCESSIBLE` / `CREDENTIAL_NOT_VISIBLE_OR_MISSING`
  (config/3)** — when `recovery.scope` is `host`, request host access and retry
  the same invocation once. Repeating it in the same sandbox will not help.
  Only configure credentials when the host retry also reports them missing; for
  a browser session, run `openobserve-cli auth login --browser` on the host
  rather than running `config init` — `--browser` needs a graphical session, so
  sandboxes still use `OPENOBSERVE_EMAIL`/
  `OPENOBSERVE_PASSWORD` or `OPENOBSERVE_TOKEN`.
- **`NO_BASE_URL` (config/3)** — no server configured. Run `config init` or set
  `OPENOBSERVE_URL`.
- **`AUTH_LOGIN_NEEDS_TTY` (auth/4)** — plain `auth login` requires an
  interactive terminal. `config init` can read a pipe, but its `--pretty` mode
  requires a terminal. In CI/agents set `OPENOBSERVE_EMAIL`+`OPENOBSERVE_PASSWORD` (or
  `OPENOBSERVE_TOKEN`).
- **`AUTH_BAD_SESSION` (config/3)** — the browser-captured session is malformed
  or contains no cookies. Sign in again with `auth login --browser` to replace it.
- **`SESSION_BROWSER_MANAGED` (usage/2)** — `config init` or plain `auth login`
  (without `--browser`) was used on a session context. Refresh it with
  `openobserve-cli auth login --browser`; the CLI can also
  consume, inspect, switch, and log out of a session context without
  re-establishing it.
- **`BROWSER_NOT_FOUND` (config/3)** — `auth login --browser` found no
  Chromium-family browser. Install Chrome / Chromium / Edge / Brave, or set
  `OPENOBSERVE_BROWSER=/path/to/browser`; otherwise use `auth login` with a
  password or token.
- **`BROWSER_NO_DISPLAY` (usage/2)** — `--browser` on a Linux host with neither
  `DISPLAY` nor `WAYLAND_DISPLAY`. It needs a graphical session, so over SSH or
  inside a container / agent sandbox use `OPENOBSERVE_EMAIL`+
  `OPENOBSERVE_PASSWORD` or `OPENOBSERVE_TOKEN` instead.
- **`BROWSER_SIGNIN_CANCELLED` (auth/4)** — the sign-in window was closed before
  login completed. Nothing was stored; re-run `auth login --browser`.
- **`BROWSER_SIGNIN_TIMEOUT` (auth/4)** — sign-in did not complete within the
  ten-minute window. Re-run `auth login --browser`, or use a password / token.
- **`BROWSER_LAUNCH_FAILED` (config/3)** — the browser could not be launched or
  driven. The underlying failure is in `message`; the browser's own output is
  not captured. Set `OPENOBSERVE_BROWSER` to a specific executable, or use
  `auth login`.
- **`FRESH_PROFILE_NEEDS_BROWSER` (usage/2)** — `--fresh-profile` was passed
  without `--browser`. It selects a throwaway browser profile and means nothing
  to a password / token login.
- **`CONTEXT_BASE_URL_MISMATCH` (config/3)** — `auth login --browser` captured
  and stored the session, but the active context on disk points at a different
  server, so `auth.scheme: session` was not recorded in it. `OPENOBSERVE_URL` /
  `--base-url` override the server but not the context name. Re-run with a
  context whose `base_url` is the server you signed in to (`--use-context
  <name>`, or `config use-context <name>`); `config get-contexts` lists them, and
  `config init` creates one.
- **`PROFILE_REMOVE_FAILED` (config/3)** — `auth logout` removed the stored
  credential but could not delete
  `~/.angelmsger/openobserve/browser-profile`. The credential is gone; delete
  that directory by hand to clear the remembered browser session.
- **`BAD_TIME_RANGE` (usage/2)** — pass either `--since 1h` or `--from` with an
  optional `--to`. `--since` cannot be combined with `--from`/`--to`, `--to`
  requires `--from`, and the end must be later than the start.
- **`STREAM_NOT_FOUND` (not_found/6)** — run `stream list`; names are
  case-sensitive.
- **`STREAM_AMBIGUOUS` (usage/2)** — discovery found the same name in multiple
  namespaces. Repeat stream get/schema/stats with the discovered `--type`.
- **`BAD_STREAM_TYPE` (usage/2)** — SQL search accepts `--type logs`, `metrics`
  or `traces`; use the `stream_type` from discovery.
- **`DASHBOARD_HASH_MISSING` (parse/10)** — a dashboard update needed to read
  its revision but the response contained no hash. No PUT was sent. Fetch a valid
  definition with `dashboard get <id>` (and its `--folder`) before retrying.
- **`PROMQL_ERROR` (usage/2)** — a `metrics query` expression was rejected
  (typo'd metric name or bad PromQL). Discover names with
  `stream list --type metrics`; `metrics query` accepts PromQL, not SQL.
- **`PROMQL_RESPONSE_INVALID` (parse/10)** — the server answered a metrics query
  without a successful PromQL result (no `status: success`, no result type, or
  a result missing or not shaped like its type), so no result is reported.
  Usually the base URL points past the server root or a proxy answers in its
  place; `openobserve-cli doctor` checks the server and credentials.
- **`TRACE_NOT_FOUND` (not_found/6)** — `trace get` found no spans for that
  `trace_id` in the window; widen `--since` or confirm the id/stream with
  `trace search --stream <name>`.
- **`HTTP_UNAUTHORIZED` (auth/4)** — rejected or expired credentials. Follow
  `next_steps`; use `auth reuse` when offered before fresh sign-in. A session
  context refreshes through `auth login --browser`; preserve the service preset.
- **`HTTP_FORBIDDEN` (permission/5)** — the credential authenticates but has no
  role for the resource. This is the classic **service account** case: under
  OpenObserve RBAC (Enterprise/Cloud) a new user / service account is granted
  nothing by default. `org list` works while `stream list` / `search` 403 for
  exactly this reason. Fix in OpenObserve: **IAM → Roles**, grant a role the
  **Streams** resource (List + Get, for the streams you'll query), then assign
  the user / service account to that role (its **Roles** / **Service Accounts**
  tab).
