package apiclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	cerrors "github.com/angelmsger/openobserve-cli/pkg/errors"
)

// DashboardListOptions narrows ListDashboards. Without any filter the server
// lists the "default" folder; Title searches across folders when Folder is
// unset. The server only honors PageSize together with Title; the endpoint is
// otherwise unpaginated.
type DashboardListOptions struct {
	Folder   string
	Title    string
	PageSize int
}

// dashboardSummaryKeys is the curated, stable subset of a list item rendered
// by `dashboard list`. The server also embeds the full dashboard definition
// under a deprecated v1..v8 key; it stays in the raw map but is intentionally
// dropped from output — `dashboard get` fetches definitions.
var dashboardSummaryKeys = []string{
	"dashboard_id", "title", "description", "folder_id", "folder_name",
	"role", "owner", "created", "updatedAt", "version", "hash",
}

// DashboardSummary is one entry of the list endpoint: enough to identify a
// dashboard, its folder and its current concurrent-edit hash. Decoding follows
// the Org pattern — the raw map is kept so fields drifting across O2 versions
// cannot break the client, and only the curated keys are extracted.
type DashboardSummary struct {
	raw         map[string]any
	DashboardID string
	Title       string
	FolderID    string
	FolderName  string
	Version     int
	Hash        string
}

// UnmarshalJSON decodes a list item leniently into its raw map and extracts
// the identifiers the CLI relies on.
func (d *DashboardSummary) UnmarshalJSON(b []byte) error {
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return err
	}
	d.raw = m
	d.DashboardID, _ = m["dashboard_id"].(string)
	d.Title, _ = m["title"].(string)
	d.FolderID, _ = m["folder_id"].(string)
	d.FolderName, _ = m["folder_name"].(string)
	d.Hash, _ = m["hash"].(string)
	if v, ok := m["version"].(float64); ok {
		d.Version = int(v)
	}
	return nil
}

// MarshalJSON renders the curated subset, keeping the summary compact.
func (d DashboardSummary) MarshalJSON() ([]byte, error) {
	out := map[string]any{}
	for _, k := range dashboardSummaryKeys {
		if v, ok := d.raw[k]; ok && v != nil {
			out[k] = v
		}
	}
	if len(out) == 0 {
		out["dashboard_id"] = d.DashboardID
		if d.Title != "" {
			out["title"] = d.Title
		}
		if d.Version != 0 {
			out["version"] = d.Version
		}
		if d.Hash != "" {
			out["hash"] = d.Hash
		}
	}
	return json.Marshal(out)
}

// Dashboard is a full dashboard definition as returned by get, create and
// update: the version-keyed body (v1..v8 — tabs, panels, queries, variables)
// plus the top-level version and hash. The raw map is preserved verbatim so
// output round-trips into `dashboard update` without schema coupling; only the
// pieces the CLI itself needs (hash, version, id, title) are extracted.
type Dashboard struct {
	raw         map[string]any
	Version     int
	Hash        string
	DashboardID string
	Title       string
}

// UnmarshalJSON decodes the dashboard envelope and extracts the hash plus
// the identifying fields of its version-keyed body.
func (d *Dashboard) UnmarshalJSON(b []byte) error {
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return err
	}
	d.raw = m
	d.Hash, _ = m["hash"].(string)
	if v, ok := m["version"].(float64); ok {
		d.Version = int(v)
	}
	if body, ok := d.bodyMap(); ok {
		// Newer dashboard versions use camelCase keys; v1..v2 use snake_case.
		if id, _ := body["dashboardId"].(string); id != "" {
			d.DashboardID = id
		} else if id, _ := body["dashboard_id"].(string); id != "" {
			d.DashboardID = id
		}
		d.Title, _ = body["title"].(string)
	}
	return nil
}

// MarshalJSON passes the raw envelope through unchanged.
func (d Dashboard) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.raw)
}

// WriteBody returns the version-keyed dashboard definition — the body
// OpenObserve expects for create/update — with the hash/version envelope
// removed.
func (d *Dashboard) WriteBody() (json.RawMessage, error) {
	body, ok := d.bodyMap()
	if !ok {
		return nil, cerrors.New(cerrors.CategoryParse, "DASHBOARD_BODY",
			"the response did not contain a version-keyed dashboard body")
	}
	return json.Marshal(body)
}

// bodyMap locates the v1..v8 definition inside the raw envelope.
func (d *Dashboard) bodyMap() (map[string]any, bool) {
	if d.Version != 0 {
		if m, ok := d.raw[fmt.Sprintf("v%d", d.Version)].(map[string]any); ok {
			return m, true
		}
	}
	for i := 1; i <= 8; i++ {
		if m, ok := d.raw[fmt.Sprintf("v%d", i)].(map[string]any); ok {
			return m, true
		}
	}
	return nil, false
}

// ListDashboards returns dashboard summaries via GET /api/{org}/dashboards.
func (c *apiClient) ListDashboards(ctx context.Context, org string, opts DashboardListOptions) ([]DashboardSummary, error) {
	q := url.Values{}
	if opts.Folder != "" {
		q.Set("folder", opts.Folder)
	}
	if opts.Title != "" {
		q.Set("title", opts.Title)
	}
	if opts.PageSize > 0 {
		q.Set("page_size", strconv.Itoa(opts.PageSize))
	}
	var env struct {
		Dashboards []DashboardSummary `json:"dashboards"`
	}
	path := "/api/" + url.PathEscape(org) + "/dashboards"
	if err := c.getJSON(ctx, path, q, &env); err != nil {
		return nil, err
	}
	return env.Dashboards, nil
}

// GetDashboard returns the full dashboard definition via
// GET /api/{org}/dashboards/{id}, including the current concurrent-edit hash.
func (c *apiClient) GetDashboard(ctx context.Context, org, id, folder string) (*Dashboard, error) {
	path := "/api/" + url.PathEscape(org) + "/dashboards/" + url.PathEscape(id)
	var d Dashboard
	if err := c.getJSON(ctx, path, dashboardFolderQuery(folder), &d); err != nil {
		return nil, refineDashboardError(err, org, id)
	}
	return &d, nil
}

// CreateDashboard stores body as a new dashboard in folder via
// POST /api/{org}/dashboards and returns it with its assigned ID and hash.
func (c *apiClient) CreateDashboard(ctx context.Context, org, folder string, body json.RawMessage) (*Dashboard, error) {
	path := "/api/" + url.PathEscape(org) + "/dashboards"
	var d Dashboard
	if err := c.doJSON(ctx, http.MethodPost, path, dashboardFolderQuery(folder), body, &d); err != nil {
		return nil, refineDashboardError(err, org, "")
	}
	return &d, nil
}

// UpdateDashboard replaces a dashboard's definition via
// PUT /api/{org}/dashboards/{id}. hash, when non-empty, feeds the server's
// concurrent-edit check; a stale hash fails with a conflict error.
func (c *apiClient) UpdateDashboard(ctx context.Context, org, id, folder, hash string, body json.RawMessage) (*Dashboard, error) {
	q := dashboardFolderQuery(folder)
	if hash != "" {
		q.Set("hash", hash)
	}
	path := "/api/" + url.PathEscape(org) + "/dashboards/" + url.PathEscape(id)
	var d Dashboard
	if err := c.doJSON(ctx, http.MethodPut, path, q, body, &d); err != nil {
		return nil, refineDashboardError(err, org, id)
	}
	return &d, nil
}

// DeleteDashboard removes a dashboard via DELETE /api/{org}/dashboards/{id}.
func (c *apiClient) DeleteDashboard(ctx context.Context, org, id, folder string) error {
	path := "/api/" + url.PathEscape(org) + "/dashboards/" + url.PathEscape(id)
	if err := c.doJSON(ctx, http.MethodDelete, path, dashboardFolderQuery(folder), nil, nil); err != nil {
		return refineDashboardError(err, org, id)
	}
	return nil
}

// dashboardFolderQuery builds the folder query parameter shared by every
// dashboard mutation; an empty folder means the server's "default" folder.
func dashboardFolderQuery(folder string) url.Values {
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

// refineDashboardError retrargets generic HTTP errors at the dashboard
// workflow: the shared not-found guidance points at streams, and a 409 needs
// the concurrent-edit recovery spelled out.
func refineDashboardError(err error, org, id string) error {
	var ce *cerrors.CLIError
	if !errors.As(err, &ce) {
		return err
	}
	switch ce.Category {
	case cerrors.CategoryNotFound:
		if id == "" {
			// A create/update 404 means the folder (not the dashboard) is missing.
			return ce.WithHint("The target folder does not exist in this organization.").
				WithNextSteps("openobserve-cli dashboard list")
		}
		return ce.WithHint("Dashboard IDs are case-sensitive and folder-scoped; " +
			"pass --folder when the dashboard lives outside the default folder.").
			WithNextSteps("openobserve-cli dashboard list")
	case cerrors.CategoryConflict:
		return ce.WithHint("The dashboard changed since the hash was taken; "+
			"the concurrent-edit check rejected the update.").
			WithNextSteps(
				"Re-run the update — `dashboard update` re-reads the current hash before writing.",
				"Inspect the current state first: openobserve-cli dashboard get "+id)
	}
	return err
}
