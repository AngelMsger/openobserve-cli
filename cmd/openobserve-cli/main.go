// Command openobserve-cli lets Coding Agents query an OpenObserve (O2)
// observability backend: discover streams, run SQL searches over logs / metrics
// / traces, and inspect histograms — all with agent-friendly JSON output and
// structured errors.
package main

import (
	"os"

	"github.com/angelmsger/openobserve-cli/internal/app"
)

func main() {
	os.Exit(app.Execute())
}
