# Getting started: configuration and auth

`openobserve-cli` talks to either a self-hosted OpenObserve (default
`http://localhost:5080`) or OpenObserve Cloud (`https://api.openobserve.ai`).
Authentication is HTTP Basic — email + password — or a pre-generated token.

## Interactive setup (humans, on a terminal)

```
openobserve-cli config init --pretty   # interactive TUI (recommended for humans)
openobserve-cli config init             # plain line-by-line wizard (works over a pipe)
```

Prompts for the server URL, organization, auth scheme and credentials, verifies
them against the server, then writes a context to
`~/.angelmsger/openobserve/config.yaml` and stores the secret in the OS keychain
(falling back to a `0600` file when no keychain is available). Re-run it any time
to add or update a context; `config init --context prod` names the context.

`--pretty` renders an interactive TUI and requires a terminal (otherwise it
fails with `PRETTY_NEEDS_TTY`); the plain wizard reads stdin line by line, so it
also works non-interactively for scripted setup.

`auth login` is the lighter form: it re-prompts only for credentials of the
already-configured context.

## Headless setup (CI, agent sandboxes)

There is no TTY, so `config init` / `auth login` fail fast with a structured
error instead of hanging. Provide everything via environment variables — they
take precedence over the config file and keychain:

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

## SSO / OAuth (dex, Authentik, Okta, Azure…): use a Service Account

When OpenObserve logs users in through an external identity provider (dex,
Authentik, Okta, Azure AD, …), those users have **no local password**, so they
cannot authenticate the CLI directly — every OpenObserve API call, including this
CLI's, uses HTTP Basic auth, not the browser OAuth flow. The supported path for
programmatic / CLI / agent access alongside SSO is a **Service Account**: a
non-human account that holds a long-lived token and cannot log into the UI.

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
file) to assert a read-only session. v0.1 has no write commands, so this is a
forward-looking guard; `--allow-writes` is the per-call escape hatch.
