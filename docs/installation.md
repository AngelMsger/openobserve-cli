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
(`openobserve-cli-<os>-<arch>`), `chmod +x` it, and put it on your `PATH`.

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
```

Run `openobserve-cli completion --help` for persistent-install instructions per
shell.

## 3. Install the companion Skill

The `openobserve` Skill is embedded in the binary, so it always matches the CLI
version. `skill install` detects your coding agents (Claude Code, Codex) and
installs into each:

```bash
openobserve-cli skill install                 # auto-detect, install for each agent
openobserve-cli skill install --agent codex   # target one agent
openobserve-cli skill install --project       # into ./.claude/skills, ./.agents/skills
openobserve-cli skill uninstall               # remove it
openobserve-cli skill path                     # show where it would install, and status
```

Re-run `skill install` after upgrading the CLI to keep the Skill version-matched.

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

Then verify:

```bash
openobserve-cli doctor       # config / credentials / connectivity
openobserve-cli auth status  # identity + reachability
```

Configuration resolves in precedence order (highest first): CLI flags →
environment (`OPENOBSERVE_*`) → `.env` → `~/.angelmsger/openobserve/config.yaml`
→ defaults. Secrets are stored in the OS keychain (with a `0600` file fallback)
and never written to the config file. See `.env.example` for the full variable
list, and the companion Skill's
[getting-started reference](../skills/openobserve/references/getting-started.md)
for auth details, including SSO / Service Accounts.
