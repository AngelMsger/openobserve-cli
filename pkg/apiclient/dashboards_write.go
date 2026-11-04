package apiclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	cerrors "github.com/angelmsger/openobserve-cli/pkg/errors"
)

// CreateDashboardReq describes a new dashboard. Body may be a definition or a
// get backup; NormalizeDashboardBody unwraps the envelope.
type CreateDashboardReq struct {
	Org, Folder string
	Body        json.RawMessage
}

// UpdateDashboardReq describes a replacement definition for an existing
// dashboard. Hash overrides the backup revision; when neither is available, execution
// fetches the current hash and the offline plan describes that prefetch.
type UpdateDashboardReq struct {
	Org, ID, Folder, Hash string
	Body                  json.RawMessage
}

// DeleteDashboardReq identifies the dashboard to remove.
type DeleteDashboardReq struct{ Org, ID, Folder string }

type dashboardWriteRequest struct {
	method, path, id string
	query            url.Values
	body             json.RawMessage
}

// buildDashboardWrite is shared by execution and dry-run; API paths,
// parameters and payload normalization have a single owner in the public
// client layer.
func buildDashboardWrite(op any) (dashboardWriteRequest, error) {
	var r dashboardWriteRequest
	var org, folder, hash string
	switch v := op.(type) {
	case CreateDashboardReq:
		r.method, org, folder, r.body = http.MethodPost, v.Org, v.Folder, v.Body
	case UpdateDashboardReq:
		normalized, backupHash, err := dashboardBody(v.Body)
		if err != nil {
			return r, badDashboardBody(err)
		}
		r.method, org, r.id, folder, r.body, hash = http.MethodPut, v.Org, v.ID, v.Folder, normalized, v.Hash
		if hash == "" {
			hash = backupHash
		}
	case DeleteDashboardReq:
		r.method, org, r.id, folder = http.MethodDelete, v.Org, v.ID, v.Folder
	default:
		return r, cerrors.Newf(cerrors.CategoryInternal, "UNKNOWN_WRITE_OP", "unsupported write operation %T", op)
	}
	if r.method != http.MethodPost && r.id == "" {
		return r, cerrors.New(cerrors.CategoryUsage, "DASHBOARD_ID_REQUIRED", "a dashboard ID is required").
			WithNextSteps("openobserve-cli dashboard list")
	}
	r.path = "/api/" + url.PathEscape(org) + "/dashboards"
	if r.id != "" {
		r.path += "/" + url.PathEscape(r.id)
	}
	r.query = dashboardFolderQuery(folder)
	if hash != "" {
		r.query.Set("hash", hash)
	}
	if r.method == http.MethodPost || r.method == http.MethodPut {
		body, err := NormalizeDashboardBody(r.body)
		if err != nil {
			return r, err
		}
		r.body = body
	}
	return r, nil
}

func (c *apiClient) executeDashboardWrite(ctx context.Context, op any, out any) error {
	r, err := buildDashboardWrite(op)
	if err != nil {
		return err
	}
	// An absent body must remain nil rather than a typed nil RawMessage,
	// which doJSON would encode as a literal JSON null.
	var body any
	if r.body != nil {
		body = r.body
	}
	return cerrors.FromWrite(refineDashboardError(c.doJSON(ctx, r.method, r.path, r.query, body, out), r.id),
		dashboardWriteTarget(op))
}

// PlanDashboardWrite returns a plan without accessing credentials or the
// network. Supported operations are CreateDashboardReq, UpdateDashboardReq
// and DeleteDashboardReq. Dry-run plans use the same builders as execution.
func PlanDashboardWrite(baseURL string, op any) (WriteRequestPlan, error) {
	r, err := buildDashboardWrite(op)
	if err != nil {
		return WriteRequestPlan{}, err
	}
	plan, err := planWriteRequest(baseURL, r.method, r.path, r.query, r.body)
	if err != nil {
		return WriteRequestPlan{}, err
	}
	if r.method == http.MethodPut && r.query.Get("hash") == "" {
		plan.Prefetch = &WriteRequestPlan{Method: http.MethodGet, URL: plan.URL}
	}
	return plan, nil
}

// NormalizeDashboardBody validates a definition and unwraps a `dashboard get`
// envelope ({v5: {...}, version, hash, updated_at}) down to the body
// OpenObserve expects for create/update. Keys are kept raw so exact numbers
// survive backup/edit/restore for library users as well as CLI callers.
func NormalizeDashboardBody(body json.RawMessage) (json.RawMessage, error) {
	normalized, _, err := dashboardBody(body)
	if err != nil {
		return nil, badDashboardBody(err)
	}
	return normalized, nil
}

func badDashboardBody(err error) error {
	return cerrors.Wrap(err, cerrors.CategoryUsage, "BAD_BODY", "invalid dashboard definition").
		WithHint("Use a JSON object or an exported dashboard with one valid active version body.")
}

// dashboardBody selects the version named by the envelope, ignoring null
// inactive versions. Without version metadata, exactly one object is accepted.
// Raw bytes preserve large numbers and unknown fields.
func dashboardBody(body json.RawMessage) (json.RawMessage, string, error) {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, "", err
	}
	if m == nil {
		return nil, "", fmt.Errorf("expected a JSON object")
	}
	keys := make([]string, 0)
	for key := range m {
		if strings.HasPrefix(key, "v") {
			if n, err := strconv.Atoi(key[1:]); err == nil && n > 0 {
				keys = append(keys, key)
			}
		}
	}
	_, hasHash := m["hash"]
	if len(keys) == 0 && !hasHash {
		return body, "", nil
	}
	var hash string
	if raw, ok := m["hash"]; ok {
		if err := json.Unmarshal(raw, &hash); err != nil {
			return nil, "", fmt.Errorf("invalid backup hash: %w", err)
		}
	}
	var selected json.RawMessage
	if raw, ok := m["version"]; ok {
		var version int
		if err := json.Unmarshal(raw, &version); err != nil || version < 1 {
			return nil, "", fmt.Errorf("invalid envelope version")
		}
		selected = m[fmt.Sprintf("v%d", version)]
	} else {
		for _, key := range keys {
			if bytes.Equal(bytes.TrimSpace(m[key]), []byte("null")) {
				continue
			}
			if selected != nil {
				return nil, "", fmt.Errorf("multiple active dashboard versions")
			}
			selected = m[key]
		}
	}
	var definition map[string]json.RawMessage
	if err := json.Unmarshal(selected, &definition); err != nil || definition == nil {
		return nil, "", fmt.Errorf("active dashboard version must contain an object")
	}
	return selected, hash, nil
}
