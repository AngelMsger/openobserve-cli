# Installation & setup

## 1. Install the CLI

### npm (recommended)

```bash
npm install -g @angelmsger/openobserve-cli
```

The package's `postinstall` downloads the prebuilt binary for your platform from
the matching GitHub Release, verifies its SHA-256 checksum, and installs it.
Upgrade with `npm update -g @angelmsger/openobserve-cli`. Installs done with
`--ignore-scripts` fetch the binary lazily on first run.

### go install

```bash
go install github.com/angelmsger/openobserve-cli/cmd/openobserve-cli@latest   # Go 1.24+
```

### From source

```bash
git clone https://github.com/AngelMsger/openobserve-cli
cd openobserve-cli
make install          # builds and copies to $GOBIN (or $GOPATH/bin)
```

### Prebuilt binary

Download the asset for your platform from the
[Releases page](https://github.com/AngelMsger/openobserve-cli/releases)
(`openobserve-cli-<os>-<arch>`), verify it against `checksums.txt`, and put it
on your `PATH`.

On macOS/Linux, run `chmod +x openobserve-cli-*` before moving the binary. On
Windows PowerShell, download `openobserve-cli-windows-amd64.exe` (or
`windows-arm64.exe`) together with `checksums.txt`, then:

```powershell
$asset = "openobserve-cli-windows-amd64.exe"
$checksumLine = Get-Content .\checksums.txt | Where-Object { $_ -match "\s+$([regex]::Escape($asset))$" } | Select-Object -First 1
if (-not $checksumLine) { throw "No checksum found for $asset" }
$expected = ($checksumLine -split '\s+')[0].ToLowerInvariant()
$actual = (Get-FileHash ".\$asset" -Algorithm SHA256).Hash.ToLowerInvariant()
if ($actual -ne $expected) { throw "SHA-256 mismatch for $asset" }
$binDir = Join-Path $HOME "bin"
New-Item -ItemType Directory -Force $binDir | Out-Null
Move-Item ".\$asset" (Join-Path $binDir "openobserve-cli.exe")
[Environment]::SetEnvironmentVariable("Path", ([Environment]::GetEnvironmentVariable("Path", "User") + ";$binDir"), "User")
```

Open a new PowerShell window after changing `PATH`.

## 2. Enable shell completion (optional)

`openobserve-cli` completes subcommands and enum flag values.

```bash
# bash — current shell
source <(openobserve-cli completion bash)

# zsh — persistent
openobserve-cli completion zsh > "${fpath[1]}/_openobserve-cli"

# fish
openobserve-cli completion fish | source

# PowerShell
openobserve-cli completion powershell | Out-String | Invoke-Expression

# PowerShell — persistent
openobserve-cli completion powershell >> $PROFILE
```

Run `openobserve-cli completion --help` for persistent-install instructions per
shell.

## 3. Install the companion Skill

The `openobserve` Skill is embedded in the binary, so it always matches the CLI
version. `skill install` detects your coding agents — **Claude Code**, **Codex**,
**Cursor**, **Agents** (shared), **Gemini CLI**, **GitHub Copilot**,
**OpenCode**, **Continue**, **Windsurf**, **Grok Build**, **Pi**,
**Kilo Code**, and **Roo Code** — and installs into each:

```bash
openobserve-cli skill install                 # auto-detect, install for each agent
openobserve-cli skill install --agent codex   # target one agent
openobserve-cli skill install --project       # into each agent's project skills dir
openobserve-cli skill uninstall               # remove it
openobserve-cli skill path                     # show where it would install, and status
```

`skill path` prints every agent's resolved location and install status, so use
it rather than memorising the per-agent directories.

After every CLI upgrade, run `openobserve-cli skill install`, then reload the
agent context. `openobserve-cli skill status` compares the loaded, installed,
and embedded versions and reports the next steps when they differ.

Alternatively, install it from the git repository with the `npx skills` workflow:

```bash
npx skills add AngelMsger/openobserve-cli
```

## 4. Configure

Set up a server interactively, or via environment variables for headless use:

```bash
openobserve-cli config init --pretty   # interactive TUI (recommended for humans)
openobserve-cli config init             # plain wizard (works over a pipe / scripts)
```

```bash
export OPENOBSERVE_URL=http://localhost:5080
export OPENOBSERVE_ORG=default
export OPENOBSERVE_EMAIL=root@example.com
export OPENOBSERVE_PASSWORD='Complexpass#123'
# or: export OPENOBSERVE_TOKEN='<base64-or-Basic/Bearer value>'
```

PowerShell uses `$env:` for the same headless setup:

```powershell
$env:OPENOBSERVE_URL = "https://api.openobserve.ai"
$env:OPENOBSERVE_ORG = "default"
$env:OPENOBSERVE_EMAIL = "alice@example.com"
$env:OPENOBSERVE_PASSWORD = "<password-or-service-account-token>"
openobserve-cli doctor
```

Then verify:

```bash
openobserve-cli doctor       # config / credentials / connectivity
openobserve-cli auth status  # identity + reachability
```

Configuration resolves in precedence order (highest first): CLI flags →
environment (`OPENOBSERVE_*`) → `.env` → `~/.angelmsger/openobserve/config.yaml`
→ defaults. Secrets are stored in the OS keychain. If Windows Credential
Manager is unavailable, the fallback file is encrypted with per-user DPAPI;
macOS/Linux retain the `0600` fallback. Secrets are never written to the config
file. See `.env.example` for the full variable list, and the companion Skill's
[getting-started reference](../skills/openobserve/references/getting-started.md)
for auth details, including SSO / Service Accounts.

## Team distribution and personal login

Distribute service settings separately from each member's credentials. An installer
can write a named context without a network connection or access to the keychain:

```bash
openobserve-cli config set-context team \
  --base-url https://service.example.com/deploy --org team \
  --auth-scheme basic --activate

# The member completes personal authentication in a terminal.
openobserve-cli --use-context team auth guide
openobserve-cli --use-context team auth login
```

`config set-context <name>` resolves **flags > environment > `.env` > the named
target context > defaults**. It ignores personal environment fields and secrets,
including secret-based scheme inference. It never verifies connectivity, reads or
writes the keychain, or changes another context's values or shared defaults.
Existing usernames remain unchanged. The first context becomes current;
subsequent calls change the current context only with `--activate`.

Identical presets do not rewrite the file. Conflicting non-empty service fields
return `CONFIG_CONTEXT_CONFLICT` with a `details` object containing each field's
`before` and `after` values. Inspect those differences, then use `--overwrite`
to update the supplied service fields, or use another context name. Unspecified
fields are retained. `--dry-run` uses the same merge and conflict checks and
returns the proposed changes without writing anything; use `--overwrite
--dry-run` to preview a deliberate conflicting update.

A launcher or CI environment can instead inject service settings on every run:

```bash
export OPENOBSERVE_URL=https://service.example.com/deploy
export OPENOBSERVE_ORG=team
export OPENOBSERVE_AUTH_SCHEME=basic
# Optional: a verified page for the team's version or an internal setup guide.
export OPENOBSERVE_CREDENTIAL_URL=https://help.example.com/openobserve/credentials
openobserve-cli auth guide
openobserve-cli auth login
```

Exports must be sourced into the member's shell or injected by a launcher/CI;
an executed child script cannot export values back into its parent shell.
`--auth-scheme` and `--credential-url` override these variables. The optional
page is persisted as `auth.credential_url` by `config set-context` and is
**display-only**: the CLI never sends an API request or credential to it.
Service paths such as `/deploy` are retained when deriving page links.

`auth guide` works offline and emits `server`, `flavor` where applicable,
`scheme`, `credential_url`, `source`, `instructions`, `documentation_url`, and
`next_steps`. Sources are `flag`, `env`, `dotenv`, `file`, `builtin`, or `fallback`.
There is no server-version probe; navigation instructions accompany version-
dependent links.

Basic auth uses the account email and password. Token input is `base64(email:token)` or
a complete Basic/Bearer Authorization value, not a raw service-account secret.
Ingestion-only and RUM tokens do not authorize queries. Service-account availability
depends on edition. For SSO, configure a new context with `--auth-scheme session`, then
run `auth login --browser`. Presets cannot change an existing browser-session context’s
server or scheme; create another context instead.

`auth login` reuses the resolved service, shows the same guide on stderr, asks
only for the missing username and the secret, and verifies authentication before
saving. It saves the username/scheme in the config and the secret in the existing
secure store; an environment-only service becomes a default context if none
exists. A later process can resolve that identity without another username
prompt. A different full service URL (including deployment path) in the selected
context fails with `CONTEXT_BASE_URL_MISMATCH` before storing a credential.

`CREDENTIAL_SAVE_FAILED` means validation succeeded but storage failed.
`LOGIN_CONFIG_WRITE_FAILED` means the credential was stored but its config
identity could not be recorded; its details preserve the server/context/scheme
and `credential_stored: true`. Fix file access and run `auth login` again.
Configuration files are replaced atomically. Normal credential environment
variables remain transient and are never copied by `config set-context`.
Non-interactive users supply credentials through the documented environment
variables rather than piping secrets into `auth login`. `config init` retains its
edit/add/replace flow and now uses target-specific presets and the same guide.

## Reuse an existing login

After preparing a team context, associate an existing personal login without
creating another token or changing the active context:

```sh
openobserve-cli --use-context team auth reuse --dry-run
openobserve-cli --use-context team auth reuse
openobserve-cli --use-context team auth status
```

Reuse matches the complete normalized service URL, authentication scheme and
provider scope (deployment flavor, organization or tenant where applicable).
It verifies the source credential before filling missing destination identity.
A populated destination identity is preserved. Credentials stay in their native
store; no secrets or environment credentials are copied. Both source and unrelated
contexts remain unchanged. Configuration changes during verification stop the write.

The JSON result reports `context`, `state`, `changed`, `verified`, `dry_run` and,
when selected, `source_context`. States are `available` (verified preview),
`reused`, `unchanged`, or `unavailable`. The last two do not establish successful
authentication: always retain the separate `auth status` check. No reusable
identity is a normal no-change result. Network, permission and credential-store
failures retain their structured errors instead of suggesting a fresh login.

Multiple different verified identities return `AUTH_REUSE_AMBIGUOUS`; discover
context names with `config contexts`, then repeat with `--from-context <name>`.
The command does not replace identities or switch authentication schemes. As
with native `auth login`, updating this CLI's own settings remains available
in remote read-only mode; `--dry-run` never changes settings or credentials.

Equivalent service URL overrides preserve the persisted native credential lookup
key without redirecting requests or copying secrets. Logout removes that same
entry. A different complete deployment URL cannot use the retained lookup key.
