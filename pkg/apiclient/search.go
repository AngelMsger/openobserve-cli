package apiclient

import (
	"context"
	"net/http"
	"net/url"
)

// Search runs a SQL query against org via POST /api/{org}/_search.
func (c *apiClient) Search(ctx context.Context, org string, req SearchRequest) (*SearchResponse, error) {
	if req.SearchType == "" {
		req.SearchType = "ui"
	}
	path := "/api/" + url.PathEscape(org) + "/_search"
	var resp SearchResponse
	if err := c.doJSON(ctx, http.MethodPost, path, nil, req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
