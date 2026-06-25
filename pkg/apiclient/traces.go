package apiclient

import (
	"context"
	"net/url"
	"strconv"
)

// LatestTraces lists recent traces in a trace stream via
// GET /api/{org}/{stream}/traces/latest. start/end are Unix microseconds (as the
// trace endpoint, like _search, works in microseconds). filter is an optional
// SQL-style predicate scoping which traces are returned.
func (c *apiClient) LatestTraces(ctx context.Context, org, stream string, startMicros, endMicros int64, from, size int, filter string) (*TraceSearchResponse, error) {
	q := url.Values{}
	q.Set("start_time", strconv.FormatInt(startMicros, 10))
	q.Set("end_time", strconv.FormatInt(endMicros, 10))
	q.Set("from", strconv.Itoa(from))
	q.Set("size", strconv.Itoa(size))
	if filter != "" {
		q.Set("filter", filter)
	}
	path := "/api/" + url.PathEscape(org) + "/" + url.PathEscape(stream) + "/traces/latest"
	var resp TraceSearchResponse
	if err := c.getJSON(ctx, path, q, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
