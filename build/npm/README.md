# @angelmsger/openobserve-cli

npm distribution of [`openobserve-cli`](https://github.com/AngelMsger/openobserve-cli)
— a command-line tool that searches [OpenObserve](https://openobserve.ai) (O2)
logs, metrics and traces from the terminal, built for coding agents (Claude
Code and others) and humans alike.

```bash
npm install -g @angelmsger/openobserve-cli
openobserve-cli config init --pretty      # interactive TUI: server URL + credentials
openobserve-cli skill install             # deploy the companion agent Skill
openobserve-cli stream list               # discover streams (the map)
```

Installing this package downloads the prebuilt binary for your platform from the
matching GitHub Release and verifies its SHA-256 checksum. If your npm setup
disables install scripts, the binary is fetched on first run instead.

The companion `openobserve` Skill for coding agents is embedded in the binary.
After installing or upgrading the package, run `openobserve-cli skill install` and
reload the agent context. `openobserve-cli skill status` reports version alignment.

See the [project README](https://github.com/AngelMsger/openobserve-cli) and the
[installation guide](https://github.com/AngelMsger/openobserve-cli/blob/main/docs/installation.md)
for full documentation.
