package apiclient

// readOnlyClient is the client the session read-only posture
// (defaults.read_only / OPENOBSERVE_CLI_READ_ONLY / --allow-writes) hands out.
// Every method today is a read, so it passes them all through.
//
// The first write method (dashboards, alerts, functions, users, ingest) must
// not be added to Client and overridden here: an embedded Client lets a write
// without an override pass through unblocked. Split Client the way the family
// does (jira-cli first): Client = Reader + Writer, this wrapper embeds Reader
// only and implements each Writer method with a READONLY_BLOCKED error, so a
// write it does not block fails to compile.
type readOnlyClient struct {
	Client
}

// NewReadOnly returns a read-only view of c.
func NewReadOnly(c Client) Client {
	return &readOnlyClient{Client: c}
}
