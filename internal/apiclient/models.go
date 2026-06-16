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
