package apiclient

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"

	cerrors "github.com/angelmsger/openobserve-cli/pkg/errors"
)

// AlertListOptions narrows ListAlerts. Without any filter the server lists the
// "default" folder; the endpoint is unpaginated.
type AlertListOptions struct {
	Folder string
}

// alertSummaryKeys is the curated, stable subset of a list item rendered by
// `alert list`. The server embeds the full alert definition (query_condition,
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
	if err := json.Unmarshal(b, &m); err != nil {
		return err
	}
	a.raw = m
	a.AlertID, _ = m["id"].(string)
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
	// The summary id is surfaced under the stable "alert_id" key, not the
	// server's unadorned "id".
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

// Alert is a full alert definition as returned by get, create and update:
// name, stream, query condition, trigger condition and so on. The raw map is
// preserved verbatim so output round-trips into `alert update` without schema
// coupling; only the pieces the CLI itself needs (id, name, stream, folder)
// are extracted.
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
	if err := json.Unmarshal(b, &m); err != nil {
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

// ListAlerts returns alert summaries via GET /api/{org}/alerts.
func (c *apiClient) ListAlerts(ctx context.Context, org string, opts AlertListOptions) ([]AlertSummary, error) {
	q := url.Values{}
	if opts.Folder != "" {
		q.Set("folder", opts.Folder)
	}
	var env struct {
		List []AlertSummary `json:"list"`
	}
	path := "/api/" + url.PathEscape(org) + "/alerts"
	if err := c.getJSON(ctx, path, q, &env); err != nil {
		return nil, err
	}
	return env.List, nil
}

// GetAlert returns the full alert definition via
// GET /api/{org}/alerts/{id}.
func (c *apiClient) GetAlert(ctx context.Context, org, id, folder string) (*Alert, error) {
	path := "/api/" + url.PathEscape(org) + "/alerts/" + url.PathEscape(id)
	var a Alert
	if err := c.getJSON(ctx, path, alertFolderQuery(folder), &a); err != nil {
		return nil, refineAlertError(err, org, id)
	}
	return &a, nil
}

// CreateAlert stores body as a new alert in folder via
// POST /api/{org}/alerts and returns it with its assigned ID.
func (c *apiClient) CreateAlert(ctx context.Context, org, folder string, body json.RawMessage) (*Alert, error) {
	path := "/api/" + url.PathEscape(org) + "/alerts"
	var a Alert
	if err := c.doJSON(ctx, http.MethodPost, path, alertFolderQuery(folder), body, &a); err != nil {
		return nil, refineAlertError(err, org, "")
	}
	return &a, nil
}

// UpdateAlert replaces an alert's definition via
// PUT /api/{org}/alerts/{id}.
func (c *apiClient) UpdateAlert(ctx context.Context, org, id, folder string, body json.RawMessage) (*Alert, error) {
	path := "/api/" + url.PathEscape(org) + "/alerts/" + url.PathEscape(id)
	var a Alert
	if err := c.doJSON(ctx, http.MethodPut, path, alertFolderQuery(folder), body, &a); err != nil {
		return nil, refineAlertError(err, org, id)
	}
	return &a, nil
}

// DeleteAlert removes an alert via DELETE /api/{org}/alerts/{id}.
func (c *apiClient) DeleteAlert(ctx context.Context, org, id, folder string) error {
	path := "/api/" + url.PathEscape(org) + "/alerts/" + url.PathEscape(id)
	if err := c.doJSON(ctx, http.MethodDelete, path, alertFolderQuery(folder), nil, nil); err != nil {
		return refineAlertError(err, org, id)
	}
	return nil
}

// SetAlertEnabled enables or disables an alert via
// PATCH /api/{org}/alerts/{id}/enabled?value=<bool>.
func (c *apiClient) SetAlertEnabled(ctx context.Context, org, id, folder string, value bool) error {
	q := alertFolderQuery(folder)
	q.Set("value", strconv.FormatBool(value))
	path := "/api/" + url.PathEscape(org) + "/alerts/" + url.PathEscape(id) + "/enabled"
	if err := c.doJSON(ctx, http.MethodPatch, path, q, nil, nil); err != nil {
		return refineAlertError(err, org, id)
	}
	return nil
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
func refineAlertError(err error, org, id string) error {
	var ce *cerrors.CLIError
	if !errors.As(err, &ce) {
		return err
	}
	switch ce.Category {
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
