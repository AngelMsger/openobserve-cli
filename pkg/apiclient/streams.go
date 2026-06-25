package apiclient

import (
	"context"
	"net/url"
	"strings"

	cerrors "github.com/angelmsger/openobserve-cli/pkg/errors"
)

// streamListEnvelope is the shape of GET /api/{org}/streams.
type streamListEnvelope struct {
	List []Stream `json:"list"`
}

// ListStreams returns the streams in org. streamType narrows the result;
// fetchSchema includes each stream's field schema.
func (c *apiClient) ListStreams(ctx context.Context, org, streamType string, fetchSchema bool) ([]Stream, error) {
	q := url.Values{}
	if streamType != "" {
		q.Set("type", streamType)
	}
	if fetchSchema {
		q.Set("fetchSchema", "true")
	}
	var env streamListEnvelope
	path := "/api/" + url.PathEscape(org) + "/streams"
	if err := c.getJSON(ctx, path, q, &env); err != nil {
		return nil, err
	}
	return env.List, nil
}

// GetStream returns a single stream by name including its schema. It lists with
// fetchSchema and filters client-side, so the result carries the same schema /
// settings shape the list endpoint provides.
func (c *apiClient) GetStream(ctx context.Context, org, name, streamType string) (*Stream, error) {
	streams, err := c.ListStreams(ctx, org, streamType, true)
	if err != nil {
		return nil, err
	}
	for i := range streams {
		if strings.EqualFold(streams[i].Name, name) {
			return &streams[i], nil
		}
	}
	return nil, cerrors.Newf(cerrors.CategoryNotFound, "STREAM_NOT_FOUND",
		"no stream named %q in org %q", name, org).
		WithHint("Stream names are case-sensitive in queries.").
		WithNextSteps("openobserve-cli stream list --org " + org)
}
