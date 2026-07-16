# Getting started: configuration and auth

`openobserve-cli` talks to either a self-hosted OpenObserve (default
`http://localhost:5080`) or OpenObserve Cloud (`https://api.openobserve.ai`).
Authentication is HTTP Basic — email + password — or a pre-generated token.
Contexts created by the o3 desktop can instead carry a browser-captured session.

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
prod` skips that prompt and targets the named context directly.

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
browser-session context, sign in through o3 again instead. The CLI cannot and
must not elevate itself; `recovery.scope=host` is an instruction to the Agent
host or approval layer.

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

## Browser-captured sessions (o3 desktop)

The o3 desktop can sign in through an instance's own browser login page and
store a `session` context in the same config and keychain used by this CLI. The
CLI can use that context normally, including `auth status`, `doctor`, and all
read commands. Select it with `--use-context <name>` or `config use-context`.

Browser sessions cannot be created or refreshed by `config init` or `auth
login`; sign in through o3 again when one expires or needs to change. A malformed
or cookie-less captured session is rejected as `AUTH_BAD_SESSION` instead of
being sent as an unauthenticated request.

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
