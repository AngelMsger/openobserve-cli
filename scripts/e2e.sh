#!/usr/bin/env bash
# End-to-end smoke test: run the built binary against the mock OpenObserve
# server and assert the agent-facing contract holds (JSON output, structured
# errors, exit codes). No real credentials or server required.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BIN="$ROOT/bin/openobserve-cli"
ADDR="127.0.0.1:45080"
URL="http://$ADDR"

if [[ ! -x "$BIN" ]]; then
  echo "building binary..."
  (cd "$ROOT" && make build >/dev/null)
fi

TMP="$(mktemp -d)"
cleanup() {
  [[ -n "${MOCK_PID:-}" ]] && kill "$MOCK_PID" 2>/dev/null || true
  rm -rf "$TMP"
}
trap cleanup EXIT

# Build and start the mock server (build to a binary so the trap can kill the
# actual server process — `go run` would leave its compiled child orphaned).
go build -o "$TMP/mockserver" "$ROOT/test/mockserver"
"$TMP/mockserver" "$ADDR" 2>"$TMP/mock.log" &
MOCK_PID=$!

# Wait for it to accept connections.
for _ in $(seq 1 50); do
  if curl -fsS -o /dev/null "$URL/api/organizations" -H 'Authorization: Basic x' 2>/dev/null; then
    break
  fi
  sleep 0.1
done

export OPENOBSERVE_URL="$URL"
export OPENOBSERVE_ORG="default"
export OPENOBSERVE_EMAIL="root@example.com"
export OPENOBSERVE_PASSWORD="pass"

run() { "$BIN" --config "$TMP" "$@"; }

pass=0
check() { # check <label> <expected-substr> -- <command...>
  local label="$1" want="$2"; shift 3
  local out; out="$("$@" 2>/dev/null || true)"
  if grep -q "$want" <<<"$out"; then
    echo "ok   - $label"
    pass=$((pass + 1))
  else
    echo "FAIL - $label (wanted: $want)"
    echo "$out" | head -5
    exit 1
  fi
}

check "org list"        '"default"'      -- run org list
check "stream list"     '"app"'          -- run stream list
check "stream schema"   'full_text_search_keys' -- run stream schema app
check "search run"      '"hits"'         -- run search run --stream app --since 1h --limit 5
check "search ndjson"   '"level":"ERROR"' -- run --format ndjson search run --stream app --since 1h
check "histogram"       '"buckets"'      -- run search histogram --stream app --since 1h --interval 5m
check "doctor healthy"  '"healthy": true' -- run doctor

# Exit-code contract: missing stream -> not_found (6).
set +e
run search run --stream nope-not-real --since 1h >/dev/null 2>&1
# (mock returns rows for any stream, so instead assert a usage error path)
run search run --stream app >/dev/null 2>&1; code=$?
set -e
if [[ "$code" -ne 2 ]]; then
  echo "FAIL - missing time range should exit 2, got $code"; exit 1
fi
echo "ok   - missing-time-range exits 2"
pass=$((pass + 1))

echo ""
echo "e2e: $pass checks passed"
