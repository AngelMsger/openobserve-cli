package apiclient

import (
	"context"
	"encoding/json"

	cerrors "github.com/angelmsger/openobserve-cli/pkg/errors"
)

// readOnlyClient wraps a Client and blocks every mutating method before a
// request leaves the process. Reads pass through the embedded Client; each
// write method is overridden to return a structured READONLY_BLOCKED error
// when the session read-only posture (defaults.read_only /
// OPENOBSERVE_CLI_READ_ONLY / --allow-writes) is active.
type readOnlyClient struct {
	Client
}

// NewReadOnly returns a read-only view of c.
func NewReadOnly(c Client) Client {
	return &readOnlyClient{Client: c}
}

// readonlyBlocked is the structured error every write returns under the
// read-only posture. Keep the wording in sync with docs/read-only-mode.md.
func readonlyBlocked() error {
	return cerrors.New(cerrors.CategoryPermission, "READONLY_BLOCKED",
		"read-only mode is active; this is a write operation").
		WithHint("Pass --allow-writes to override for this invocation.").
		WithNextSteps("openobserve-cli --allow-writes <command>")
}

func (c *readOnlyClient) CreateAlert(ctx context.Context, org, folder string, body json.RawMessage) (*Alert, error) {
	return nil, readonlyBlocked()
}

func (c *readOnlyClient) UpdateAlert(ctx context.Context, org, id, folder string, body json.RawMessage) (*Alert, error) {
	return nil, readonlyBlocked()
}

func (c *readOnlyClient) DeleteAlert(ctx context.Context, org, id, folder string) error {
	return readonlyBlocked()
}

func (c *readOnlyClient) SetAlertEnabled(ctx context.Context, org, id, folder string, value bool) error {
	return readonlyBlocked()
}
