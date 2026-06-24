# openobserve-cli command reference

This index is generated from the CLI command tree — do not edit it by
hand; run `make docs`. The full reference, with every flag and example,
is published at <https://angelmsger.github.io/openobserve-cli/cli/>.

## auth

| Command | Description |
| --- | --- |
| [`openobserve-cli auth`](https://angelmsger.github.io/openobserve-cli/cli/#openobserve-cli-auth) | Log in, check identity and log out |
| [`openobserve-cli auth login`](https://angelmsger.github.io/openobserve-cli/cli/#openobserve-cli-auth-login) | Store credentials for the active context (interactive) |
| [`openobserve-cli auth logout`](https://angelmsger.github.io/openobserve-cli/cli/#openobserve-cli-auth-logout) | Remove the stored credential for the active context |
| [`openobserve-cli auth status`](https://angelmsger.github.io/openobserve-cli/cli/#openobserve-cli-auth-status) | Show the active identity and verify connectivity |

## config

| Command | Description |
| --- | --- |
| [`openobserve-cli config`](https://angelmsger.github.io/openobserve-cli/cli/#openobserve-cli-config) | Set up and inspect configuration and contexts |
| [`openobserve-cli config contexts`](https://angelmsger.github.io/openobserve-cli/cli/#openobserve-cli-config-contexts) | List configured contexts and which one is current |
| [`openobserve-cli config init`](https://angelmsger.github.io/openobserve-cli/cli/#openobserve-cli-config-init) | Interactively configure a context and store credentials |
| [`openobserve-cli config show`](https://angelmsger.github.io/openobserve-cli/cli/#openobserve-cli-config-show) | Show the resolved configuration with field provenance |
| [`openobserve-cli config use-context`](https://angelmsger.github.io/openobserve-cli/cli/#openobserve-cli-config-use-context) | Set the current context |

## doctor

| Command | Description |
| --- | --- |
| [`openobserve-cli doctor`](https://angelmsger.github.io/openobserve-cli/cli/#openobserve-cli-doctor) | Check configuration, credentials and connectivity |

## metrics

| Command | Description |
| --- | --- |
| [`openobserve-cli metrics`](https://angelmsger.github.io/openobserve-cli/cli/#openobserve-cli-metrics) | Query metrics with PromQL (instant and range) |
| [`openobserve-cli metrics query`](https://angelmsger.github.io/openobserve-cli/cli/#openobserve-cli-metrics-query) | Evaluate a PromQL expression at a single instant |
| [`openobserve-cli metrics query-range`](https://angelmsger.github.io/openobserve-cli/cli/#openobserve-cli-metrics-query-range) | Evaluate a PromQL expression across a time window |

## org

| Command | Description |
| --- | --- |
| [`openobserve-cli org`](https://angelmsger.github.io/openobserve-cli/cli/#openobserve-cli-org) | List organizations and set the default one |
| [`openobserve-cli org list`](https://angelmsger.github.io/openobserve-cli/cli/#openobserve-cli-org-list) | List organizations the credential can access |
| [`openobserve-cli org use`](https://angelmsger.github.io/openobserve-cli/cli/#openobserve-cli-org-use) | Set the default organization in the active context |

## search

| Command | Description |
| --- | --- |
| [`openobserve-cli search`](https://angelmsger.github.io/openobserve-cli/cli/#openobserve-cli-search) | Run SQL searches over logs, metrics and traces |
| [`openobserve-cli search histogram`](https://angelmsger.github.io/openobserve-cli/cli/#openobserve-cli-search-histogram) | Return time-bucketed counts (the volume map before raw rows) |
| [`openobserve-cli search run`](https://angelmsger.github.io/openobserve-cli/cli/#openobserve-cli-search-run) | Run a SQL query and return matching rows |
| [`openobserve-cli search tail`](https://angelmsger.github.io/openobserve-cli/cli/#openobserve-cli-search-tail) | Follow a stream live, printing new rows as they arrive |

## skill

| Command | Description |
| --- | --- |
| [`openobserve-cli skill`](https://angelmsger.github.io/openobserve-cli/cli/#openobserve-cli-skill) | Install the companion Skill for coding agents (Claude Code, Codex) |
| [`openobserve-cli skill install`](https://angelmsger.github.io/openobserve-cli/cli/#openobserve-cli-skill-install) | Deploy the embedded Skill into a coding agent's skills directory |
| [`openobserve-cli skill path`](https://angelmsger.github.io/openobserve-cli/cli/#openobserve-cli-skill-path) | Print where the Skill would be installed, and whether it is |
| [`openobserve-cli skill show`](https://angelmsger.github.io/openobserve-cli/cli/#openobserve-cli-skill-show) | Print the embedded SKILL.md to stdout |
| [`openobserve-cli skill status`](https://angelmsger.github.io/openobserve-cli/cli/#openobserve-cli-skill-status) | Report whether the companion Skill is loaded and installed |
| [`openobserve-cli skill uninstall`](https://angelmsger.github.io/openobserve-cli/cli/#openobserve-cli-skill-uninstall) | Remove the companion Skill from a coding agent's skills directory |

## stream

| Command | Description |
| --- | --- |
| [`openobserve-cli stream`](https://angelmsger.github.io/openobserve-cli/cli/#openobserve-cli-stream) | Discover streams and inspect their schema |
| [`openobserve-cli stream get`](https://angelmsger.github.io/openobserve-cli/cli/#openobserve-cli-stream-get) | Get one stream including schema, settings and stats |
| [`openobserve-cli stream list`](https://angelmsger.github.io/openobserve-cli/cli/#openobserve-cli-stream-list) | List streams in the organization (the discovery map) |
| [`openobserve-cli stream schema`](https://angelmsger.github.io/openobserve-cli/cli/#openobserve-cli-stream-schema) | Show just a stream's queryable columns and search settings |
| [`openobserve-cli stream stats`](https://angelmsger.github.io/openobserve-cli/cli/#openobserve-cli-stream-stats) | Show a stream's document count, time range and storage size |

## trace

| Command | Description |
| --- | --- |
| [`openobserve-cli trace`](https://angelmsger.github.io/openobserve-cli/cli/#openobserve-cli-trace) | Search distributed traces and inspect a single trace |
| [`openobserve-cli trace get`](https://angelmsger.github.io/openobserve-cli/cli/#openobserve-cli-trace-get) | Reassemble one trace into a span waterfall |
| [`openobserve-cli trace search`](https://angelmsger.github.io/openobserve-cli/cli/#openobserve-cli-trace-search) | List recent traces in a trace stream |

## version

| Command | Description |
| --- | --- |
| [`openobserve-cli version`](https://angelmsger.github.io/openobserve-cli/cli/#openobserve-cli-version) | Print version information |

