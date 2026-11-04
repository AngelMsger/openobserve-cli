package apiclient

import (
	"context"
	"net/http"
	"net/url"

	"github.com/angelmsger/openobserve-cli/pkg/transport"
)

// Search runs a SQL query against org via POST /api/{org}/_search. The POST
// only reads, so the transport retries it and follows its redirect as it
// does a GET (transport.AsRead).
func (c *apiClient) Search(ctx context.Context, org string, req SearchRequest) (*SearchResponse, error) {
	if req.SearchType == "" {
		req.SearchType = "ui"
	}
	path := "/api/" + url.PathEscape(org) + "/_search"
	q := url.Values{}
	if req.StreamType != "" {
		q.Set("type", req.StreamType)
	}
	var resp SearchResponse
	if err := c.doJSON(transport.AsRead(ctx), http.MethodPost, path, q, req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
