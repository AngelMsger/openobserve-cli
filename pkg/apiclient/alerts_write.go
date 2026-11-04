package apiclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"

	cerrors "github.com/angelmsger/openobserve-cli/pkg/errors"
)

// CreateAlertReq describes a new alert. Body may be a definition or a get backup.
type CreateAlertReq struct {
	Org, Folder string
	Body        json.RawMessage
}

// UpdateAlertReq describes a replacement definition for an existing alert.
type UpdateAlertReq struct {
	Org, ID, Folder string
	Body            json.RawMessage
}

// DeleteAlertReq identifies the alert to remove.
type DeleteAlertReq struct{ Org, ID, Folder string }

// SetAlertEnabledReq identifies the alert and its desired enabled state.
type SetAlertEnabledReq struct {
	Org, ID, Folder string
	Enabled         bool
}

type alertWriteRequest struct {
	method, path, id string
	query            url.Values
	body             json.RawMessage
}

// buildAlertWrite is shared by execution and dry-run; API paths, parameters
// and payload normalization have a single owner in the public client layer.
func buildAlertWrite(op any) (alertWriteRequest, error) {
	var r alertWriteRequest
	var org, folder string
	switch v := op.(type) {
	case CreateAlertReq:
		r.method, org, folder, r.body = http.MethodPost, v.Org, v.Folder, v.Body
	case UpdateAlertReq:
		r.method, org, r.id, folder, r.body = http.MethodPut, v.Org, v.ID, v.Folder, v.Body
	case DeleteAlertReq:
		r.method, org, r.id, folder = http.MethodDelete, v.Org, v.ID, v.Folder
	case SetAlertEnabledReq:
		r.method, org, r.id, folder = http.MethodPatch, v.Org, v.ID, v.Folder
	default:
		return r, cerrors.Newf(cerrors.CategoryInternal, "UNKNOWN_WRITE_OP", "unsupported write operation %T", op)
	}
	if r.method != http.MethodPost && r.id == "" {
		return r, cerrors.New(cerrors.CategoryUsage, "ALERT_ID_REQUIRED", "an alert ID is required").
			WithNextSteps("openobserve-cli alert list")
	}
	r.path = "/api/v2/" + url.PathEscape(org) + "/alerts"
	r.query = alertFolderQuery(folder)
	if r.id != "" {
		r.path += "/" + url.PathEscape(r.id)
	}
	if v, ok := op.(SetAlertEnabledReq); ok {
		r.path += "/enable"
		r.query.Set("value", strconv.FormatBool(v.Enabled))
	}
	if r.method == http.MethodPost || r.method == http.MethodPut {
		body, err := NormalizeAlertBody(r.body)
		if err != nil {
			return r, err
		}
		r.body = body
	}
	return r, nil
}

func (c *apiClient) executeAlertWrite(ctx context.Context, op any, out any) error {
	r, err := buildAlertWrite(op)
	if err != nil {
		return err
	}
	// An absent body must remain nil rather than a typed nil RawMessage,
	// which doJSON would encode as a literal JSON null.
	var body any
	if r.body != nil {
		body = r.body
	}
	return cerrors.FromWrite(refineAlertError(c.doJSON(ctx, r.method, r.path, r.query, body, out), r.id),
		alertWriteTarget(op))
}

// NormalizeAlertBody validates a definition and removes response metadata.
// Unknown definition fields and exact numbers survive backup/edit/restore.
// Folder selection belongs to the request query, not the backup's folder_id.
func NormalizeAlertBody(body json.RawMessage) (json.RawMessage, error) {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, cerrors.Wrap(err, cerrors.CategoryUsage, "BAD_BODY", "the alert definition must be a JSON object")
	}
	if m == nil {
		return nil, cerrors.New(cerrors.CategoryUsage, "BAD_BODY", "the alert definition must be a JSON object")
	}
	for _, k := range []string{"id", "alert_id", "owner", "created_at", "updated_at", "last_triggered_at", "last_satisfied_at", "last_edited_by", "folder_id", "folder_name"} {
		delete(m, k)
	}
	return json.Marshal(m)
}
