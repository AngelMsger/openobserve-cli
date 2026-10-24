# Getting started: configuration and auth

`openobserve-cli` talks to either a self-hosted OpenObserve (default
`http://localhost:5080`) or OpenObserve Cloud (`https://api.openobserve.ai`).
Authentication is HTTP Basic — email + password — or a pre-generated token.
A context can instead carry a browser-captured session, established by `auth
login --browser`.

## Interactive setup (humans, on a terminal)

```
openobserve-cli config init --pretty   # interactive TUI (recommended for humans)
openobserve-cli config init             # plain line-by-line wizard (works over a pipe)
```

Prompts for the server URL, organization, auth scheme and credentials, verifies
them against the server, then writes a context to
`~/.angelmsger/openobserve/config.yaml` and stores the secret in the OS keychain
(falling back to per-user DPAPI on Windows or a `0600` file on macOS/Linux).
When a config
already exists, re-running it lists the contexts and asks whether to **edit** one
(prefilled), **add** a new one, or **replace** everything; `config init --context
prod` skips that prompt and targets the named context directly. A new context
name is saved trimmed and in lower case (`--context Prod` writes `prod`).

`config get-contexts` lists the contexts with the current one marked.
`config delete-context <name>` removes a context; its stored secret (password,
token or browser session) goes too unless another context on the same server
and scheme still uses it. The only context cannot be deleted (`LAST_CONTEXT`).

`--pretty` renders an interactive TUI and requires a terminal (otherwise it
fails with `PRETTY_NEEDS_TTY`); the plain wizard reads stdin line by line, so it
also works non-interactively for scripted setup.

`auth login` is the lighter form: it re-prompts only for credentials of the
already-configured context.

## For agents and sandboxes

The user has normally configured OpenObserve already. Reuse their host config
under `~/.angelmsger/openobserve/` and OS keychain. If credential resolution
returns `CREDENTIAL_STORE_INACCESSIBLE` or
`CREDENTIAL_NOT_VISIBLE_OR_MISSING` with `recovery.scope=host`, request host
access and retry the same invocation once. Do not run `config init` or `auth
login` inside the sandbox.

Only when the host retry also reports missing credentials should the user
configure them in their terminal or provide environment variables. For a
browser-session context, the user runs `openobserve-cli auth login --browser`
on their own host (it opens a real browser window, so it needs a display and
does not work inside the sandbox). The CLI cannot and must not elevate itself;
`recovery.scope=host` is an instruction to the Agent host or approval layer.

## Headless setup (CI and genuinely unconfigured environments)

Provide everything via environment variables; they take precedence over the
config file and keychain:

```
export OPENOBSERVE_URL=http://localhost:5080
export OPENOBSERVE_ORG=default
export OPENOBSERVE_EMAIL=root@example.com
export OPENOBSERVE_PASSWORD='Complexpass#123'
# …or a pre-generated token instead of email+password:
export OPENOBSERVE_TOKEN='cm9vdEBleGFtcGxlLmNvbTpDb21wbGV4cGFzcw=='
```

`OPENOBSERVE_TOKEN` is the base64 portion of a Basic credential; a full
`Basic …` / `Bearer …` value is also accepted and passed through verbatim.

## Browser sign-in

```
openobserve-cli auth login --browser
```

A browser window opens on the instance's own login page (a one-line notice on
stderr says so, since the command then waits); once you sign in, the CLI
captures the session, verifies it with an authenticated request, stores it in
the OS keychain, and records `auth.scheme: session` plus the captured email on
the active context in `config.yaml` — creating that context when the server
came from `OPENOBSERVE_URL`. Only that context's `auth` block is rewritten. The
CLI can then use the context normally, including `auth status`, `doctor`, and
all read commands. This is the path for instances behind SSO, where neither a
password nor a generated token works.

Requires a Chromium-family browser (Chrome, Chromium, Edge or Brave) and a
graphical session — it does not work over SSH or inside an agent sandbox. Set
`OPENOBSERVE_BROWSER` to choose a specific browser. The browser profile is
remembered under `~/.angelmsger/openobserve/browser-profile` so an identity
provider does not re-prompt on every sign-in; `--fresh-profile` opts out (it
requires `--browser`, and is a `FRESH_PROFILE_NEEDS_BROWSER` usage error
without it), and `auth logout` removes the profile, reporting whether one was
actually there in `browser_profile_removed`.

The capture script is scoped to the instance's own hostname, so an identity
provider on another origin — full-page or in an iframe — never has its
`Authorization` header captured.

Sign in again when a session expires. A malformed or cookie-less captured
session is rejected as `AUTH_BAD_SESSION` instead of being sent as an
unauthenticated request.

## SSO / OAuth (dex, Authentik, Okta, Azure…): use a Service Account

For headless and long-running access, users authenticated through an external
identity provider (dex, Authentik, Okta, Azure AD, …) have **no local password**,
and a captured browser session is not a durable agent credential. The supported
path for programmatic / CLI / agent access alongside SSO is a **Service
Account**: a non-human account that holds a long-lived token and cannot log into
the UI.

1. In OpenObserve, an admin goes to **IAM → Service Accounts → Add Service
   Account**, enters an email + name, and saves. A **token** is generated (shown
   once — copy it).
2. Assign the service account a role under **IAM → Roles**, or it can't read your
   streams.
3. Authenticate the CLI with the service-account **email** + **token** (the token
   is used in the password position of Basic auth):

   ```
   # interactive
   openobserve-cli config init        # scheme: basic, Email: <sa-email>, Password: <token>

   # headless / agent
   export OPENOBSERVE_URL=https://your-openobserve-host
   export OPENOBSERVE_ORG=<org>
   export OPENOBSERVE_EMAIL=<service-account-email>
   export OPENOBSERVE_PASSWORD=<service-account-token>
   ```

Notes:

- OpenObserve also offers an SSO **token-exchange** endpoint that turns an
  IdP-issued JWT into a short-lived (~30 min) Bearer token. The CLI can carry it
  via `OPENOBSERVE_TOKEN='Bearer <jwt>'`, but its expiry makes it unsuitable for a
  long-running CLI — prefer a Service Account's durable token.
- The bootstrap `root` user (`ZO_ROOT_USER_EMAIL` / `ZO_ROOT_USER_PASSWORD`) keeps
  Basic auth even when SSO is on; usable for quick checks, but prefer a
  permission-scoped Service Account over root credentials for agents.

## Verify

```
openobserve-cli auth status     # identity + reachability
openobserve-cli doctor          # config / credentials / connectivity checks
openobserve-cli org list        # confirm which orgs the credential sees
```

## Precedence

Highest wins: command-line flags → environment variables → `.env` file → config
file → built-in defaults. `config show` prints the resolved values and where the
key ones came from.

## Read-only posture

Set `OPENOBSERVE_CLI_READ_ONLY=1` (or `defaults.read_only: true` in the config
file) to assert a read-only session. The CLI has no write commands yet, so this
is a forward-looking guard; `--allow-writes` is the per-call escape hatch.

For preset team services, use `config set-context` and `auth guide` before personal login; see [team setup](team-setup.md).

## Reuse existing authentication

`openobserve-cli --use-context team auth reuse` points a context without a
working login at one this machine already verified for the same complete service
URL and organization; `--dry-run` previews the same decision. It looks, in order,
at contexts under the context's own scheme, then at any long-lived credential
(a password or token, including one stored for this service that no context
points at any more, paired with the usernames recorded on the service), and at a
browser session last. A username under the `session` or `token` scheme is only a
label: it does not stop reuse, and a context with a label only adopts a password
for that same username. A working login, including a working browser session, is
never replaced automatically; `--from-scheme basic` (or `--from-context <name>`)
replaces a session explicitly. The result reports `scheme`, `username` and, when
the source is a context, `source_context`; its `next_steps` name the check to run
next (`auth status`). No secret is copied and the current context does not change.

`AUTH_REUSE_AMBIGUOUS` lists `details.candidates`; choose one with
`--from-context <name>`, `--from-scheme <scheme>`, or both (the stored secret of
that scheme paired with that context's username). Store, network and permission
errors require their named recovery, not a new login. When the result says the
keychain could not be read here, retry on the host before signing in.

`auth login --browser` on a context whose password or token login still works
stops with `BROWSER_LOGIN_REPLACES_LOGIN`; that login is usable as it is. Only the
user decides to pass `--replace`. When that check cannot read the stored password
or token, it stops with `CREDENTIAL_STORE_INACCESSIBLE` before opening a browser;
retry on the host. A browser sign-in never overwrites the username
the context records, so `auth reuse` can return to the password once the session
expires.
