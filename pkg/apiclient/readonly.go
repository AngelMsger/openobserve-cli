package apiclient

// readOnlyClient wraps a Client and blocks every mutating method before a
// request leaves the process. The v0.1 surface is entirely read-only, so the
// wrapper currently only embeds the underlying Client and passes reads through.
//
// It exists so the session read-only posture (defaults.read_only /
// OPENOBSERVE_CLI_READ_ONLY / --allow-writes) is wired end-to-end now: when P1
// adds write methods (dashboards, alerts, functions, users, ingest) to Client,
// each gets an override here that returns a structured READONLY_BLOCKED error,
// with no other plumbing to change.
type readOnlyClient struct {
	Client
}

// NewReadOnly returns a read-only view of c.
func NewReadOnly(c Client) Client {
	return &readOnlyClient{Client: c}
}
