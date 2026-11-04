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
	*d = DashboardSummary{raw: m}
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
// plus the top-level version and hash. The raw keys are preserved verbatim so
// output round-trips into `dashboard update` without schema coupling or
// numeric drift; only the pieces the CLI itself needs (hash, version, id,
// title) are extracted.
type Dashboard struct {
	raw         map[string]json.RawMessage
	Version     int
	Hash        string
	DashboardID string
	Title       string
}

// UnmarshalJSON decodes the dashboard envelope and extracts the hash plus
// the identifying fields of its version-keyed body.
func (d *Dashboard) UnmarshalJSON(b []byte) error {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		return err
	}
	*d = Dashboard{raw: m}
	_ = json.Unmarshal(m["hash"], &d.Hash)
	var version float64
	_ = json.Unmarshal(m["version"], &version)
	d.Version = int(version)
	if body, ok := d.bodyMap(); ok {
		var fields map[string]json.RawMessage
		_ = json.Unmarshal(body, &fields)
		// Newer dashboard versions use camelCase keys; v1..v2 use snake_case.
		if id, ok := fields["dashboardId"]; ok {
			_ = json.Unmarshal(id, &d.DashboardID)
		}
		if d.DashboardID == "" {
			_ = json.Unmarshal(fields["dashboard_id"], &d.DashboardID)
		}
		_ = json.Unmarshal(fields["title"], &d.Title)
	}
	return nil
}

// MarshalJSON passes the raw envelope through unchanged.
func (d Dashboard) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.raw)
}

// WriteBody returns the version-keyed dashboard definition — the body
// OpenObserve expects for create/update — with the hash/version envelope
// removed. Raw bytes are passed through, so exact JSON numbers survive.
func (d *Dashboard) WriteBody() (json.RawMessage, error) {
	body, ok := d.bodyMap()
	if !ok {
		return nil, cerrors.New(cerrors.CategoryParse, "DASHBOARD_BODY",
			"the response did not contain a version-keyed dashboard body")
	}
	return body, nil
}

// bodyMap locates the v1..v8 definition inside the raw envelope.
func (d *Dashboard) bodyMap() (json.RawMessage, bool) {
	raw, err := json.Marshal(d.raw)
	if err != nil {
		return nil, false
	}
	body, _, err := dashboardBody(raw)
	if err != nil {
		return nil, false
	}
	// Dashboard responses must contain a version envelope, not a bare definition.
	for key := range d.raw {
		if len(key) > 1 && key[0] == 'v' {
			if _, err := strconv.Atoi(key[1:]); err == nil {
				return body, true
			}
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
		q.Set("pageSize", strconv.Itoa(opts.PageSize))
	}
	var env struct {
		Dashboards []DashboardSummary `json:"dashboards"`
	}
	path := "/api/" + url.PathEscape(org) + "/dashboards"
	if err := c.getJSON(ctx, path, q, &env); err != nil {
		return nil, refineDashboardError(err, "")
	}
	return env.Dashboards, nil
}

// GetDashboard returns the full dashboard definition via
// GET /api/{org}/dashboards/{id}, including the current concurrent-edit hash.
func (c *apiClient) GetDashboard(ctx context.Context, org, id, folder string) (*Dashboard, error) {
	path := "/api/" + url.PathEscape(org) + "/dashboards/" + url.PathEscape(id)
	var d Dashboard
	if err := c.getJSON(ctx, path, dashboardFolderQuery(folder), &d); err != nil {
		return nil, refineDashboardError(err, id)
	}
	return &d, nil
}

// CreateDashboard stores body as a new dashboard in folder via
// POST /api/{org}/dashboards and returns it with its assigned ID and hash.
// Body may be a definition or a get backup; the envelope is unwrapped.
func (c *apiClient) CreateDashboard(ctx context.Context, org, folder string, body json.RawMessage) (*Dashboard, error) {
	req := CreateDashboardReq{org, folder, body}
	var d Dashboard
	if err := c.executeDashboardWrite(ctx, req, &d); err != nil {
		return nil, err
	}
	if d.DashboardID == "" {
		return nil, cerrors.MissingWriteIdentity(dashboardWriteTarget(req))
	}
	return &d, nil
}

// UpdateDashboard replaces a dashboard's definition via
// PUT /api/{org}/dashboards/{id}. An explicit hash overrides the backup hash.
// When neither exists, it reads the current revision first. Missing hashes
// prevent the PUT; stale hashes fail with a conflict error.
func (c *apiClient) UpdateDashboard(ctx context.Context, org, id, folder, hash string, body json.RawMessage) (*Dashboard, error) {
	req := UpdateDashboardReq{org, id, folder, hash, body}
	r, err := buildDashboardWrite(req)
	if err != nil {
		return nil, err
	}
	req.Body, req.Hash = r.body, r.query.Get("hash")
	if req.Hash == "" {
		current, err := c.GetDashboard(ctx, org, id, folder)
		if err != nil {
			return nil, err
		}
		req.Hash = current.Hash
		if req.Hash == "" {
			return nil, cerrors.New(cerrors.CategoryParse, "DASHBOARD_HASH_MISSING", "the dashboard response contains no revision hash").
				WithHint("No update was sent. Fetch a valid dashboard revision before retrying.").WithNextSteps("openobserve-cli dashboard get " + id)
		}
	}
	var d Dashboard
	if err := c.executeDashboardWrite(ctx, req, &d); err != nil {
		return nil, err
	}
	return &d, nil
}

// DeleteDashboard removes a dashboard via DELETE /api/{org}/dashboards/{id}.
func (c *apiClient) DeleteDashboard(ctx context.Context, org, id, folder string) error {
	return c.executeDashboardWrite(ctx, DeleteDashboardReq{org, id, folder}, nil)
}

// dashboardFolderQuery builds the folder query parameter shared by every
// dashboard mutation; an empty folder means the server's "default" folder.
func dashboardFolderQuery(folder string) url.Values {
	q := url.Values{}
	q.Set("folder", folderOrDefault(folder))
	return q
}

// refineDashboardError retrargets generic HTTP errors at the dashboard
// workflow: the shared not-found guidance points at streams, and a 409 needs
// the concurrent-edit recovery spelled out.
func refineDashboardError(err error, id string) error {
	var ce *cerrors.CLIError
	if !errors.As(err, &ce) {
		return err
	}
	switch ce.Category {
	case cerrors.CategoryPermission:
		if ce.HTTPStatus == http.StatusForbidden {
			return ce.WithHint("This credential lacks permission for dashboards in the selected folder.").
				WithNextSteps("In OpenObserve IAM, grant the required Dashboards permissions for this folder.", "openobserve-cli dashboard list")
		}
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
				"Fetch the current definition and re-apply your intended changes before updating; do not blindly retry an old backup.",
				"Inspect the current state first: openobserve-cli dashboard get "+id)
	}
	return err
}
