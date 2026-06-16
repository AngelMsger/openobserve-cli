# Getting started: configuration and auth

`openobserve-cli` talks to either a self-hosted OpenObserve (default
`http://localhost:5080`) or OpenObserve Cloud (`https://api.openobserve.ai`).
Authentication is HTTP Basic — email + password — or a pre-generated token.

## Interactive setup (humans, on a terminal)

```
openobserve-cli config init
```

Prompts for the server URL, organization, auth scheme and credentials, verifies
them against the server, then writes a context to
`~/.angelmsger/openobserve/config.yaml` and stores the secret in the OS keychain
(falling back to a `0600` file when no keychain is available). Re-run it any time
to add or update a context; `config init --context prod` names the context.

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
