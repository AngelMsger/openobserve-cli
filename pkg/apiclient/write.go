package apiclient

import (
	"context"
	"encoding/json"
	"net/url"
)

// WriteRequestPlan describes an offline preview. If Prefetch is present, the
// dashboard PUT hash is resolved from that GET at execution time.
type WriteRequestPlan struct {
	Method   string            `json:"method"`
	URL      string            `json:"url"`
	Body     json.RawMessage   `json:"body,omitempty"`
	Prefetch *WriteRequestPlan `json:"prefetch,omitempty"`
}

// DescribeWrite previews alert and dashboard *Req operations without network
// or credential access. It is also available through read-only clients.
func (c *apiClient) DescribeWrite(_ context.Context, op any) (WriteRequestPlan, error) {
	switch op.(type) {
	case CreateDashboardReq, UpdateDashboardReq, DeleteDashboardReq:
		return PlanDashboardWrite(c.baseURL, op)
	}
	r, err := buildAlertWrite(op)
	if err != nil {
		return WriteRequestPlan{}, err
	}
	return planWriteRequest(c.baseURL, r.method, r.path, r.query, r.body)
}

func planWriteRequest(baseURL, method, path string, query url.Values, body json.RawMessage) (WriteRequestPlan, error) {
	base, err := NormalizeBaseURL(baseURL)
	if err != nil {
		return WriteRequestPlan{}, err
	}
	endpoint := base + path
	if encoded := query.Encode(); encoded != "" {
		endpoint += "?" + encoded
	}
	return WriteRequestPlan{Method: method, URL: endpoint, Body: body}, nil
}
