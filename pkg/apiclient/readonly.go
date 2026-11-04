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

// blocked returns the family's READONLY_BLOCKED error for a refused write. op
// names the operation (e.g. "CreateAlert") so the message says which call was
// refused. Turning read-only mode off is the user's decision, so the error
// names the user's switches without offering to change them.
func blocked(op string) error {
	return cerrors.ReadOnlyBlocked(op, cerrors.ReadOnlyMode{
		Env:        "OPENOBSERVE_CLI_READ_ONLY",
		ConfigFile: "~/.angelmsger/openobserve/config.yaml",
		DryRun:     true,
	})
}

func (c *readOnlyClient) CreateAlert(ctx context.Context, org, folder string, body json.RawMessage) (*Alert, error) {
	return nil, blocked("CreateAlert")
}

func (c *readOnlyClient) UpdateAlert(ctx context.Context, org, id, folder string, body json.RawMessage) (*Alert, error) {
	return nil, blocked("UpdateAlert")
}

func (c *readOnlyClient) DeleteAlert(ctx context.Context, org, id, folder string) error {
	return blocked("DeleteAlert")
}

func (c *readOnlyClient) SetAlertEnabled(ctx context.Context, org, id, folder string, value bool) error {
	return blocked("SetAlertEnabled")
}

func (c *readOnlyClient) CreateDashboard(ctx context.Context, org, folder string, body json.RawMessage) (*Dashboard, error) {
	return nil, blocked("CreateDashboard")
}

func (c *readOnlyClient) UpdateDashboard(ctx context.Context, org, id, folder, hash string, body json.RawMessage) (*Dashboard, error) {
	return nil, blocked("UpdateDashboard")
}

func (c *readOnlyClient) DeleteDashboard(ctx context.Context, org, id, folder string) error {
	return blocked("DeleteDashboard")
}
