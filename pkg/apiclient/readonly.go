package apiclient

import (
	"context"
	"encoding/json"

	cerrors "github.com/angelmsger/openobserve-cli/pkg/errors"
)

// readOnlyClient embeds only Reader. Writer methods must be blocked explicitly.
// Never embed Client here: newly added writes would otherwise pass through.
type readOnlyClient struct {
	Reader
}

var _ Client = (*readOnlyClient)(nil)

// NewReadOnly returns a read-only view of c.
func NewReadOnly(c Client) Client {
	return &readOnlyClient{Reader: c}
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

func (c *readOnlyClient) CreateDashboard(ctx context.Context, org, folder string, body json.RawMessage) (*Dashboard, error) {
	return nil, readonlyBlocked()
}

func (c *readOnlyClient) UpdateDashboard(ctx context.Context, org, id, folder, hash string, body json.RawMessage) (*Dashboard, error) {
	return nil, readonlyBlocked()
}

func (c *readOnlyClient) DeleteDashboard(ctx context.Context, org, id, folder string) error {
	return readonlyBlocked()
}
