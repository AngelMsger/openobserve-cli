# Searching: SQL, time ranges and the map-before-terrain workflow

## The workflow

OpenObserve stores a lot of data. Pulling raw rows blindly is slow and floods
your context. Work in two passes:

1. **Map** — `search histogram` shows how many rows fall in each time bucket, so
   you see *where* the interesting activity is without reading any rows.
2. **Terrain** — `search run` pulls the actual rows, but only for the narrow
   window and filter the histogram pointed you at.

```
# 1. shape of errors over the last 6 hours, 5-minute buckets
openobserve-cli search histogram --stream default --since 6h --interval 5m \
  --where "level = 'ERROR'"

# 2. the spike was ~14:30; pull those rows
openobserve-cli search run --stream default --since 30m \
  --where "level = 'ERROR'" --limit 50
```

## `search run`

Builds and runs a SQL query, or runs one you supply.

- `--stream <name>` builds `SELECT * FROM "<name>" ORDER BY _timestamp DESC`.
- `--where "<cond>"` adds a `WHERE` clause: `--where "code >= 500"`,
  `--where "level = 'ERROR' AND service = 'api'"`.
- `--order asc|desc` controls `_timestamp` ordering (default `desc`, newest first).
- `--sql "<query>"` runs a full query verbatim and ignores `--stream/--where/--order`.
  Use it for aggregations: `--sql 'SELECT code, count(*) FROM "default" GROUP BY code'`.
- `--limit N` caps rows (default 100, max 10000); `--offset N` paginates.

Time range is **required** — pass `--since` or `--from`/`--to` (see below).

Output (JSON) is a summary plus the hits:

```json
{
  "sql": "SELECT * FROM \"default\" WHERE level = 'ERROR' ORDER BY _timestamp DESC",
  "total": 1234, "returned": 50, "took_ms": 18, "scan_size_mb": 4.2,
  "start_micros": 1718524800000000, "end_micros": 1718528400000000,
  "hits": [ { "_timestamp": 1718528391000000, "level": "ERROR", "log": "…" } ]
}
```

`--format ndjson` instead streams the raw hits one per line — ideal for
`| jq` / `| grep`.

## `search histogram`

Runs `histogram(_timestamp, '<interval>')` with `count(*)`, grouped per bucket.

- `--stream <name>` (required) and optional `--where`.
- `--interval` accepts compact widths: `30s`, `1m`, `5m`, `1h`, `1d` (default `1m`).

Returns `{ buckets: [ { bucket, count }, … ] }`.

## Time ranges

Never hand-compute microsecond epochs — the CLI does it. Accepted forms:

- `--since 15m | 1h | 24h | 7d` — look back from now (the easy default).
- `--from` / `--to` each accept: RFC3339 (`2025-06-16T14:00:00Z`), a bare date
  (`2025-06-16`, UTC midnight), an epoch in seconds/millis/micros (auto-detected),
  `now`, or `now-30m` / `now+1h`. A bare duration like `2h` means "2h ago".
- `--to` defaults to now when omitted.

## Tips

- SQL is DataFusion-flavored: standard `SELECT`, `WHERE`, `GROUP BY`, `ORDER BY`,
  and aggregates (`count`, `sum`, `avg`, `min`, `max`).
- Quote stream names with double quotes in raw SQL: `FROM "my-stream"`.
- String literals use single quotes: `WHERE level = 'ERROR'`.
- `_timestamp` is the mandatory time column (microseconds).
- Run `stream schema <name>` first to confirm column names; `full_text_search_keys`
  in the schema tells you which fields support fast text matching.
