// Package apiclient is the OpenObserve API surface used by the CLI. It builds
// org-scoped requests, decodes normalized models, and converts non-2xx
// responses into structured *errors.CLIError values.
package apiclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	cerrors "github.com/angelmsger/openobserve-cli/internal/errors"
	"github.com/angelmsger/openobserve-cli/internal/transport"
)

// Client is the OpenObserve API surface used by the CLI.
type Client interface {
	BaseURL() string
	// DefaultOrg returns the organization identifier the client was built with.
	DefaultOrg() string

	// Ping verifies connectivity and credentials by listing organizations.
	Ping(ctx context.Context) ([]Org, error)
	// ListOrgs returns the organizations the credential can see.
	ListOrgs(ctx context.Context) ([]Org, error)

	// ListStreams returns the streams in org. streamType ("logs"/"metrics"/
	// "traces") narrows the result; "" returns all types. fetchSchema includes
	// each stream's field schema.
	ListStreams(ctx context.Context, org, streamType string, fetchSchema bool) ([]Stream, error)
	// GetStream returns a single stream by name, including its schema.
	GetStream(ctx context.Context, org, name, streamType string) (*Stream, error)

	// Search runs a SQL query against org and returns matching hits.
	Search(ctx context.Context, org string, req SearchRequest) (*SearchResponse, error)

	// QueryMetricsInstant runs an instant PromQL query at a point in time.
	QueryMetricsInstant(ctx context.Context, org, promql string, timeSec float64) (*PromQLResponse, error)
	// QueryMetricsRange runs a PromQL query over a [start,end] window at step
	// resolution. start/end are Unix seconds; step is a Prometheus duration.
	QueryMetricsRange(ctx context.Context, org, promql string, startSec, endSec float64, step string) (*PromQLResponse, error)

	// LatestTraces returns recent traces in a trace stream, newest first.
	LatestTraces(ctx context.Context, org, stream string, startMicros, endMicros int64, from, size int, filter string) (*TraceSearchResponse, error)
}

// apiClient is the single Client implementation.
type apiClient struct {
	baseURL    string // server root, no trailing slash
	defaultOrg string
	http       *transport.Client
}

// Config configures a Client.
type Config struct {
	BaseURL   string
	Org       string
	Transport *transport.Client
}

// New builds a Client. The transport must already carry the auth decorator.
func New(cfg Config) Client {
	return &apiClient{
		baseURL:    strings.TrimRight(cfg.BaseURL, "/"),
		defaultOrg: cfg.Org,
		http:       cfg.Transport,
	}
}

func (c *apiClient) BaseURL() string    { return c.baseURL }
func (c *apiClient) DefaultOrg() string { return c.defaultOrg }

// getJSON performs a GET and decodes the JSON body into out.
func (c *apiClient) getJSON(ctx context.Context, path string, query url.Values, out any) error {
	return c.doJSON(ctx, http.MethodGet, path, query, nil, out)
}

// doJSON performs an HTTP request and decodes a JSON response into out.
// Non-2xx responses are converted into structured *errors.CLIError values.
func (c *apiClient) doJSON(ctx context.Context, method, path string, query url.Values, body any, out any) error {
	endpoint := c.baseURL + path
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}

	var reqBody io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return cerrors.Wrap(err, cerrors.CategoryInternal, "ENCODE", "failed to encode request body")
		}
		reqBody = bytes.NewReader(raw)
	}

	req, err := http.NewRequest(method, endpoint, reqBody)
	if err != nil {
		return cerrors.Wrap(err, cerrors.CategoryUsage, "BAD_REQUEST", "failed to build request")
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(ctx, req)
	if err != nil {
		return cerrors.Wrap(err, cerrors.CategoryNetwork, "NETWORK",
			fmt.Sprintf("request to %s failed", endpoint))
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return c.httpError(resp)
	}
	if out == nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}
	rawResp, _ := io.ReadAll(resp.Body)
	return decodeJSON(rawResp, out)
}

// decodeJSON unmarshals a server response body into out, surfacing a snippet on
// failure so a shape mismatch is diagnosable.
func decodeJSON(body []byte, out any) error {
	if err := json.Unmarshal(body, out); err != nil {
		snippet := strings.TrimSpace(string(body))
		if len(snippet) > 200 {
			snippet = snippet[:200] + "…"
		}
		return cerrors.Wrap(err, cerrors.CategoryParse, "DECODE",
			fmt.Sprintf("could not decode the server response: %v", err)).
			WithHint("The server's JSON did not match what openobserve-cli expected; "+
				"this is likely a client bug, not a failed request.").
			WithNextSteps(
				"Retry with --verbose to inspect the raw response.",
				"Report it with this snippet: "+snippet)
	}
	return nil
}

// httpError turns a non-2xx response into a classified CLIError.
func (c *apiClient) httpError(resp *http.Response) error {
	cat := cerrors.FromHTTPStatus(resp.StatusCode)
	snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
	msg := fmt.Sprintf("OpenObserve returned HTTP %d", resp.StatusCode)
	if detail := extractAPIMessage(snippet); detail != "" {
		msg += ": " + detail
	}
	// A 403 on an org-scoped resource is almost always OpenObserve RBAC: the
	// credential authenticates but its user / service account has not been
	// granted a role for the resource. Point straight at the fix — a generic
	// "lack permission" hint leaves an agent (and the user) stuck.
	if resp.StatusCode == http.StatusForbidden {
		return cerrors.New(cat, "HTTP_FORBIDDEN", msg).
			WithHTTPStatus(resp.StatusCode).
			WithHint("Authenticated, but this credential lacks permission for this resource. "+
				"Under OpenObserve RBAC (Enterprise/Cloud) a new user or service account is granted nothing by default.").
			WithNextSteps(
				"In OpenObserve open IAM → Roles: grant a role the Streams resource (List + Get), then assign your user / service account to that role (its Roles or Service Accounts tab).",
				"Confirm the organization is correct: openobserve-cli org list")
	}
	return cerrors.New(cat, "HTTP_"+statusSlug(resp.StatusCode), msg).
		WithHTTPStatus(resp.StatusCode)
}

func statusSlug(status int) string {
	if t := http.StatusText(status); t != "" {
		return strings.ToUpper(strings.ReplaceAll(t, " ", "_"))
	}
	return fmt.Sprintf("%d", status)
}

// extractAPIMessage best-effort extracts a human message from an OpenObserve
// JSON error body. O2 uses "message" (and sometimes "error"/"error_detail").
func extractAPIMessage(raw []byte) string {
	var v struct {
		Message     string `json:"message"`
		Error       string `json:"error"`
		ErrorDetail string `json:"error_detail"`
	}
	if json.Unmarshal(raw, &v) == nil {
		switch {
		case v.Message != "":
			return v.Message
		case v.ErrorDetail != "":
			return v.ErrorDetail
		case v.Error != "":
			return v.Error
		}
	}
	return ""
}
