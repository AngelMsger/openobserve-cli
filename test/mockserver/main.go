// Command mockserver is a tiny stand-in for an OpenObserve API, used by
// scripts/e2e.sh to exercise openobserve-cli end-to-end without real
// credentials. It serves canned organizations, streams and search responses.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

func main() {
	addr := "127.0.0.1:45080"
	if len(os.Args) > 1 {
		addr = os.Args[1]
	}

	mux := http.NewServeMux()

	mux.HandleFunc("/api/organizations", func(w http.ResponseWriter, r *http.Request) {
		if !requireAuth(w, r) {
			return
		}
		// Mirror a real self-hosted payload: many extra fields, and `plan` as a
		// number (it is a string in other builds) — the client must tolerate it.
		writeJSON(w, map[string]any{
			"data": []map[string]any{
				{"id": 1, "identifier": "default", "name": "Default", "type": "default",
					"plan": 0, "ingest_threshold": 9383939382, "search_threshold": 9383939382,
					"UserObj": map[string]any{"first_name": "", "last_name": ""}},
				{"id": 2, "identifier": "team-a", "name": "Team A", "type": "custom", "plan": 0},
			},
		})
	})

	mux.HandleFunc("/api/default/streams", func(w http.ResponseWriter, r *http.Request) {
		if !requireAuth(w, r) {
			return
		}
		writeJSON(w, map[string]any{
			"list": []map[string]any{
				{
					"name": "app", "stream_type": "logs",
					"stats":    map[string]any{"doc_num": 1234, "storage_size": 12.5},
					"schema":   []map[string]any{{"name": "_timestamp", "type": "Int64"}, {"name": "level", "type": "Utf8"}, {"name": "log", "type": "Utf8"}},
					"settings": map[string]any{"full_text_search_keys": []string{"log"}},
				},
				{"name": "web", "stream_type": "logs", "stats": map[string]any{"doc_num": 42}},
			},
		})
	})

	mux.HandleFunc("/api/default/_search", func(w http.ResponseWriter, r *http.Request) {
		if !requireAuth(w, r) {
			return
		}
		raw, _ := io.ReadAll(r.Body)
		var req struct {
			Query struct {
				SQL string `json:"sql"`
			} `json:"query"`
		}
		_ = json.Unmarshal(raw, &req)
		if strings.Contains(req.Query.SQL, "histogram(") {
			writeJSON(w, map[string]any{
				"took":  3,
				"total": 2,
				"hits": []map[string]any{
					{"zo_sql_key": 1700000000000000, "zo_sql_num": 5},
					{"zo_sql_key": 1700000300000000, "zo_sql_num": 9},
				},
			})
			return
		}
		writeJSON(w, map[string]any{
			"took": 5, "total": 1, "scan_size": 1.5,
			"hits": []map[string]any{
				{"_timestamp": 1700000000000000, "level": "ERROR", "log": "boom"},
			},
		})
	})

	fmt.Fprintf(os.Stderr, "mockserver listening on %s\n", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func requireAuth(w http.ResponseWriter, r *http.Request) bool {
	if r.Header.Get("Authorization") == "" {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"missing credentials"}`))
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
