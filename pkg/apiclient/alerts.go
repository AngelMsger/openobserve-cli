package apiclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"

	cerrors "github.com/angelmsger/openobserve-cli/pkg/errors"
)

// AlertListOptions narrows ListAlerts. Without a folder filter, the server
// returns all visible folders. No pagination limit is requested.
type AlertListOptions struct {
	Folder string
}

// alertSummaryKeys is the curated, stable subset of a list item rendered by
// `alert list`. The server embeds condition and trigger metadata (condition,
// trigger_condition, ...) in each item; it stays in the raw map but is
// intentionally dropped from list output — `alert get` fetches definitions.
var alertSummaryKeys = []string{
	"alert_id", "name", "description", "stream_type", "stream_name",
	"is_real_time", "enabled", "owner", "last_triggered_at",
	"folder_id", "folder_name",
}

// AlertSummary is one entry of the list endpoint: enough to identify an alert,
// its target stream and its folder. Decoding follows the Org pattern — the raw
// map is kept so fields drifting across O2 versions cannot break the client,
// and only the curated keys are extracted.
type AlertSummary struct {
	raw        map[string]any
	AlertID    string
	Name       string
	StreamName string
	StreamType string
	FolderID   string
	FolderName string
	Enabled    bool
}

// UnmarshalJSON decodes a list item leniently into its raw map and extracts
// the identifiers the CLI relies on.
func (a *AlertSummary) UnmarshalJSON(b []byte) error {
	var m map[string]any
	decoder := json.NewDecoder(bytes.NewReader(b))
	decoder.UseNumber()
	if err := decoder.Decode(&m); err != nil {
		return err
	}
	a.raw = m
	a.AlertID, _ = m["alert_id"].(string)
	if a.AlertID == "" {
		a.AlertID, _ = m["id"].(string)
	}
	a.Name, _ = m["name"].(string)
	a.StreamName, _ = m["stream_name"].(string)
	a.StreamType, _ = m["stream_type"].(string)
	a.FolderID, _ = m["folder_id"].(string)
	a.FolderName, _ = m["folder_name"].(string)
	a.Enabled, _ = m["enabled"].(bool)
	return nil
}

// MarshalJSON renders the curated subset, keeping the summary compact.
func (a AlertSummary) MarshalJSON() ([]byte, error) {
	out := map[string]any{}
	// Keep the v2 alert_id key; also accept older fixtures using id.
	if a.AlertID != "" {
		out["alert_id"] = a.AlertID
	}
	for _, k := range alertSummaryKeys {
		if v, ok := a.raw[k]; ok && v != nil {
			out[k] = v
		}
	}
	if len(out) == 0 {
		out["alert_id"] = a.AlertID
		if a.Name != "" {
			out["name"] = a.Name
		}
	}
	return json.Marshal(out)
}

// Alert preserves the JSON returned by GetAlert or a write acknowledgement.
// Only GetAlert returns a complete definition. CreateAlert returns an ID/name
// acknowledgement; UpdateAlert may return only a status message. The existing
// return type is retained for public API compatibility. Numeric values are
// preserved exactly for backup/edit/restore.
type Alert struct {
	raw        map[string]any
	AlertID    string
	Name       string
	StreamName string
	FolderID   string
}

// UnmarshalJSON decodes the alert envelope and extracts the identifying
// fields the CLI relies on.
func (a *Alert) UnmarshalJSON(b []byte) error {
	var m map[string]any
	decoder := json.NewDecoder(bytes.NewReader(b))
	decoder.UseNumber()
	if err := decoder.Decode(&m); err != nil {
		return err
	}
	a.raw = m
	a.AlertID, _ = m["id"].(string)
	a.Name, _ = m["name"].(string)
	a.StreamName, _ = m["stream_name"].(string)
	a.FolderID, _ = m["folder_id"].(string)
	return nil
}

// MarshalJSON passes the raw envelope through unchanged.
func (a Alert) MarshalJSON() ([]byte, error) {
	return json.Marshal(a.raw)
}

// ListAlerts returns alert summaries via GET /api/v2/{org}/alerts.
func (c *apiClient) ListAlerts(ctx context.Context, org string, opts AlertListOptions) ([]AlertSummary, error) {
	q := url.Values{}
	if opts.Folder != "" {
		q.Set("folder", opts.Folder)
	}
	var env struct {
		List []AlertSummary `json:"list"`
	}
	path := "/api/v2/" + url.PathEscape(org) + "/alerts"
	if err := c.getJSON(ctx, path, q, &env); err != nil {
		return nil, refineAlertError(err, "")
	}
	return env.List, nil
}

// GetAlert returns the full alert definition via
// GET /api/v2/{org}/alerts/{id}.
func (c *apiClient) GetAlert(ctx context.Context, org, id, folder string) (*Alert, error) {
	path := "/api/v2/" + url.PathEscape(org) + "/alerts/" + url.PathEscape(id)
	var a Alert
	if err := c.getJSON(ctx, path, alertFolderQuery(folder), &a); err != nil {
		return nil, refineAlertError(err, id)
	}
	return &a, nil
}

// CreateAlert stores body as a new alert in folder via
// POST /api/v2/{org}/alerts and returns the acknowledgement with its assigned ID.
func (c *apiClient) CreateAlert(ctx context.Context, org, folder string, body json.RawMessage) (*Alert, error) {
	req := CreateAlertReq{org, folder, body}
	var a Alert
	if err := c.executeAlertWrite(ctx, req, &a); err != nil {
		return nil, err
	}
	if a.AlertID == "" {
		return nil, cerrors.MissingWriteIdentity(alertWriteTarget(req))
	}
	return &a, nil
}

// UpdateAlert replaces a definition using the v2 ID-based API. The result is
// the server's write acknowledgement, not a complete alert definition.
func (c *apiClient) UpdateAlert(ctx context.Context, org, id, folder string, body json.RawMessage) (*Alert, error) {
	var a Alert
	if err := c.executeAlertWrite(ctx, UpdateAlertReq{org, id, folder, body}, &a); err != nil {
		return nil, err
	}
	// The v2 update acknowledgement does not repeat the target ID.
	if a.AlertID == "" {
		a.AlertID = id
	}
	return &a, nil
}

// DeleteAlert removes an alert permanently.
func (c *apiClient) DeleteAlert(ctx context.Context, org, id, folder string) error {
	return c.executeAlertWrite(ctx, DeleteAlertReq{org, id, folder}, nil)
}

// SetAlertEnabled toggles an alert via PATCH /api/v2/{org}/alerts/{id}/enable.
func (c *apiClient) SetAlertEnabled(ctx context.Context, org, id, folder string, value bool) error {
	return c.executeAlertWrite(ctx, SetAlertEnabledReq{org, id, folder, value}, nil)
}

// alertFolderQuery builds the folder query parameter shared by every alert
// read/mutation; an empty folder means the server's "default" folder.
func alertFolderQuery(folder string) url.Values {
	q := url.Values{}
	q.Set("folder", folderOrDefault(folder))
	return q
}

func folderOrDefault(folder string) string {
	if folder == "" {
		return "default"
	}
	return folder
}

// refineAlertError retargets generic HTTP errors at the alert workflow: the
// shared not-found guidance points at streams, and IDs are folder-scoped.
func refineAlertError(err error, id string) error {
	var ce *cerrors.CLIError
	if !errors.As(err, &ce) {
		return err
	}
	switch ce.Category {
	case cerrors.CategoryPermission:
		if ce.HTTPStatus == http.StatusForbidden {
			return ce.WithHint("This credential lacks permission for alerts in the selected folder.").
				WithNextSteps("In OpenObserve IAM, grant the required Alerts permissions for this folder to the user or service account.", "openobserve-cli alert list")
		}
	case cerrors.CategoryNotFound:
		if id == "" {
			// A create/update 404 means the folder (not the alert) is missing.
			return ce.WithHint("The target folder does not exist in this organization.").
				WithNextSteps("openobserve-cli alert list")
		}
		return ce.WithHint("Alert IDs are case-sensitive and folder-scoped; " +
			"pass --folder when the alert lives outside the default folder.").
			WithNextSteps("openobserve-cli alert list")
	}
	return err
}
