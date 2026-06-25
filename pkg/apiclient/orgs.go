package apiclient

import (
	"context"
	"encoding/json"
)

// ListOrgs returns the organizations the credential can see, via
// GET /api/organizations.
func (c *apiClient) ListOrgs(ctx context.Context) ([]Org, error) {
	// Decode into a raw message first so we tolerate either the documented
	// {"data": [...]} envelope or a bare array.
	var raw json.RawMessage
	if err := c.getJSON(ctx, "/api/organizations", nil, &raw); err != nil {
		return nil, err
	}
	var env struct {
		Data []Org `json:"data"`
	}
	if err := json.Unmarshal(raw, &env); err == nil && env.Data != nil {
		return env.Data, nil
	}
	var list []Org
	if err := json.Unmarshal(raw, &list); err == nil {
		return list, nil
	}
	// Neither shape matched; surface a decode error with the snippet.
	return nil, decodeJSON(raw, &env)
}

// Ping verifies connectivity and credentials by listing organizations.
func (c *apiClient) Ping(ctx context.Context) ([]Org, error) {
	return c.ListOrgs(ctx)
}
