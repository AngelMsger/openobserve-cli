package apiclient

// Org is one OpenObserve organization. Identifier is the value used in the
// /api/{org}/… path; Name is the human label.
type Org struct {
	ID         int64  `json:"id,omitempty"`
	Identifier string `json:"identifier"`
	Name       string `json:"name,omitempty"`
	Type       string `json:"type,omitempty"`
	Plan       string `json:"plan,omitempty"`
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
