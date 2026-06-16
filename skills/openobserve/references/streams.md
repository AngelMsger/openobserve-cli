# Streams: discovery and schema

A **stream** is a named collection of records of one type — `logs`, `metrics` or
`traces` — inside an organization. You query streams with `search`; you discover
them and their columns with `stream`.

## List streams (the map)

```
openobserve-cli stream list                 # all types
openobserve-cli stream list --type logs     # just logs
openobserve-cli stream list --schema        # include full field schema (verbose)
```

Each item carries the name, `stream_type`, and storage `stats` (document count,
time range, size). Schema is omitted by default to keep the list a compact map;
use `stream schema` for one stream instead of `--schema` for all.

## Inspect one stream's columns

```
openobserve-cli stream schema <name>
```

Returns just what you need to write correct SQL:

```json
{
  "name": "default",
  "stream_type": "logs",
  "schema": [
    { "name": "_timestamp", "type": "Int64" },
    { "name": "level", "type": "Utf8" },
    { "name": "log", "type": "Utf8" }
  ],
  "settings": {
    "full_text_search_keys": ["log"],
    "partition_keys": {}
  }
}
```

- **`schema`** lists every queryable column and its type — reference these exact
  names in `SELECT` / `WHERE`.
- **`full_text_search_keys`** are the fields optimized for text matching; prefer
  filtering on these for free-text searches.

## Other

```
openobserve-cli stream get <name>     # full stream: schema + settings + stats
openobserve-cli stream stats <name>   # just document count, time range, size
```

`--type logs|metrics|traces` is a hint on any of these; if a name is ambiguous
across types, pass it.

## No dead ends

`search` needs a stream name → `stream list` provides it. A query needs a column
name → `stream schema` provides it. `stream` commands need an org → `org list`
provides it. You never have to guess an identifier the CLI can't tell you.
