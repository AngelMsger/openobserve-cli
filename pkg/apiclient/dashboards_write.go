package apiclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	cerrors "github.com/angelmsger/openobserve-cli/pkg/errors"
)

// CreateDashboardReq describes a new dashboard. Body may be a definition or a
// get backup; normalizeDashboardWrite unwraps the envelope.
type CreateDashboardReq struct {
	Org, Folder string
	Body        json.RawMessage
}

// UpdateDashboardReq describes a replacement definition for an existing
// dashboard. Hash, when non-empty, feeds the server's concurrent-edit check.
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
		r.method, org, r.id, folder, r.body, hash = http.MethodPut, v.Org, v.ID, v.Folder, v.Body, v.Hash
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
	var org string
	switch v := op.(type) {
	case CreateDashboardReq:
		org = v.Org
	case UpdateDashboardReq:
		org = v.Org
	case DeleteDashboardReq:
		org = v.Org
	}
	return refineDashboardError(c.doJSON(ctx, r.method, r.path, r.query, body, out), org, r.id)
}

// PlanDashboardWrite returns a plan without accessing credentials or the
// network. Supported operations are CreateDashboardReq, UpdateDashboardReq
// and DeleteDashboardReq. Dry-run plans use the same builders as execution.
func PlanDashboardWrite(baseURL string, op any) (WriteRequestPlan, error) {
	r, err := buildDashboardWrite(op)
	if err != nil {
		return WriteRequestPlan{}, err
	}
	base, err := NormalizeBaseURL(baseURL)
	if err != nil {
		return WriteRequestPlan{}, err
	}
	return WriteRequestPlan{Method: r.method, URL: base + r.path + "?" + r.query.Encode(), Body: r.body}, nil
}

// NormalizeDashboardBody validates a definition and unwraps a `dashboard get`
// envelope ({v5: {...}, version, hash, updated_at}) down to the body
// OpenObserve expects for create/update. Keys are kept raw so exact numbers
// survive backup/edit/restore for library users as well as CLI callers.
func NormalizeDashboardBody(body json.RawMessage) (json.RawMessage, error) {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, cerrors.Wrap(err, cerrors.CategoryUsage, "BAD_BODY", "the dashboard definition must be a JSON object")
	}
	if m == nil {
		return nil, cerrors.New(cerrors.CategoryUsage, "BAD_BODY", "the dashboard definition must be a JSON object")
	}
	if _, isEnvelope := m["hash"]; isEnvelope {
		for i := 1; i <= 8; i++ {
			if raw, ok := m[fmt.Sprintf("v%d", i)]; ok {
				return raw, nil
			}
		}
	}
	// Already a definition: pass the original bytes through untouched.
	return body, nil
}
