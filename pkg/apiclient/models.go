package apiclient

import "encoding/json"

// Org is one OpenObserve organization. Identifier is the value used in the
// /api/{org}/… path; Name is the human label.
//
// The org object OpenObserve returns varies across versions and editions —
// fields appear, disappear, and even change JSON type (e.g. `plan` is a string
// in some builds and a number in others). To stay robust we decode the whole
// object into a raw map (which can never fail on a type mismatch) and pull the
// two fields we rely on out of it. Output is a lean, curated projection so an
// agent isn't flooded with thresholds and user objects it doesn't need.
type Org struct {
	Identifier string
	Name       string
	raw        map[string]any
}

// orgOutputKeys are the fields surfaced by `org list`, in a stable order. Values
// pass through with whatever type the server used, so no type drift can break
// rendering.
var orgOutputKeys = []string{"identifier", "name", "type", "id"}

// UnmarshalJSON decodes an org leniently: everything lands in a map, and the
// fields used for path scoping and display are extracted from it.
func (o *Org) UnmarshalJSON(b []byte) error {
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return err
	}
	o.raw = m
	o.Identifier, _ = m["identifier"].(string)
	o.Name, _ = m["name"].(string)
	return nil
}

// MarshalJSON emits the curated subset of the raw object (falling back to the
// extracted fields when the org was constructed without a raw map).
func (o Org) MarshalJSON() ([]byte, error) {
	out := map[string]any{}
	for _, k := range orgOutputKeys {
		if v, ok := o.raw[k]; ok {
			out[k] = v
		}
	}
	if len(out) == 0 {
		out["identifier"] = o.Identifier
		if o.Name != "" {
			out["name"] = o.Name
		}
	}
	return json.Marshal(out)
}

// SchemaField is one column of a stream's schema.
type SchemaField struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// StreamStats summarizes a stream's stored data.
type StreamStats struct {
	DocTimeMin     int64   `json:"doc_time_min,omitempty"`
	DocTimeMax     int64   `json:"doc_time_max,omitempty"`
	DocNum         int64   `json:"doc_num,omitempty"`
	FileNum        int64   `json:"file_num,omitempty"`
	StorageSize    float64 `json:"storage_size,omitempty"`
	CompressedSize float64 `json:"compressed_size,omitempty"`
}

// StreamSettings holds the query-relevant configuration of a stream.
type StreamSettings struct {
	PartitionKeys      any      `json:"partition_keys,omitempty"`
	FullTextSearchKeys []string `json:"full_text_search_keys,omitempty"`
	BloomFilterFields  []string `json:"bloom_filter_fields,omitempty"`
}

// Stream is a logs / metrics / traces stream within an organization.
type Stream struct {
	Name        string          `json:"name"`
	StorageType string          `json:"storage_type,omitempty"`
	StreamType  string          `json:"stream_type"`
	Stats       *StreamStats    `json:"stats,omitempty"`
	Schema      []SchemaField   `json:"schema,omitempty"`
	Settings    *StreamSettings `json:"settings,omitempty"`
}

// SearchQuery is the inner query block of a search request. Times are Unix
// microseconds, as OpenObserve requires.
type SearchQuery struct {
	SQL       string `json:"sql"`
	StartTime int64  `json:"start_time"`
	EndTime   int64  `json:"end_time"`
	From      int    `json:"from"`
	Size      int    `json:"size"`
}

// SearchRequest is the body POSTed to /api/{org}/_search.
type SearchRequest struct {
	Query      SearchQuery `json:"query"`
	SearchType string      `json:"search_type,omitempty"`
}

// SearchResponse is the search API's reply.
type SearchResponse struct {
	Took     int              `json:"took"`
	From     int              `json:"from"`
	Size     int              `json:"size"`
	ScanSize float64          `json:"scan_size"`
	Total    int64            `json:"total"`
	Hits     []map[string]any `json:"hits"`
}

// PromQLResponse is the Prometheus-compatible reply from OpenObserve's PromQL
// endpoints (/api/{org}/prometheus/api/v1/query{,_range}). The envelope is
// stable but `data` is kept raw: its shape depends on the result type
// (matrix / vector / scalar / string), so decoding it eagerly would couple the
// client to a shape it doesn't need to understand. On a query error the API
// usually replies with a non-2xx status (handled by the client's httpError);
// the Status/Error fields cover the rarer 200-with-error case.
type PromQLResponse struct {
	Status    string          `json:"status"`
	Data      json.RawMessage `json:"data,omitempty"`
	ErrorType string          `json:"errorType,omitempty"`
	Error     string          `json:"error,omitempty"`
}

// TraceSummary is one trace returned by GET /api/{org}/{stream}/traces/latest.
// Fields are kept loose (raw maps / slices) because the trace payload carries
// OpenTelemetry-derived attributes that vary by instrumentation.
type TraceSummary struct {
	TraceID     string           `json:"trace_id"`
	Duration    json.RawMessage  `json:"duration,omitempty"`
	StartTime   int64            `json:"start_time,omitempty"`
	EndTime     int64            `json:"end_time,omitempty"`
	FirstEvent  map[string]any   `json:"first_event,omitempty"`
	ServiceName []map[string]any `json:"service_name,omitempty"`
	Spans       json.RawMessage  `json:"spans,omitempty"`
}

// TraceSearchResponse is the reply from the latest-traces endpoint.
type TraceSearchResponse struct {
	Total int64          `json:"total"`
	Hits  []TraceSummary `json:"hits"`
}
