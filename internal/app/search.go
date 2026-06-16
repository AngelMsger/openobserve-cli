package app

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/angelmsger/openobserve-cli/internal/apiclient"
	cerrors "github.com/angelmsger/openobserve-cli/internal/errors"
	"github.com/angelmsger/openobserve-cli/internal/output"
	"github.com/angelmsger/openobserve-cli/internal/timeutil"
	"github.com/angelmsger/openobserve-cli/pkg/constants"
	"github.com/spf13/cobra"
)

func newSearchCmd(s *appState) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "search",
		Short: "Run SQL searches over logs, metrics and traces",
		Long: "Search is the heart of the CLI. `search run` returns matching rows;\n" +
			"`search histogram` returns time-bucketed counts so you can see volume\n" +
			"and shape before pulling raw rows; `search tail` follows a stream live.\n" +
			"Time ranges are given in human form (--since 1h, or --from/--to); the CLI\n" +
			"converts them to the microsecond epochs OpenObserve needs.",
	}
	cmd.AddCommand(newSearchRunCmd(s), newSearchHistogramCmd(s), newSearchTailCmd(s))
	return cmd
}

// timeFlags holds the shared time-range flags.
type timeFlags struct {
	since string
	from  string
	to    string
}

func addTimeFlags(cmd *cobra.Command, t *timeFlags) {
	f := cmd.Flags()
	f.StringVar(&t.since, "since", "", "look back this far from now, e.g. 15m, 1h, 24h, 7d")
	f.StringVar(&t.from, "from", "", "range start: RFC3339, an epoch, 2006-01-02, or now-1h")
	f.StringVar(&t.to, "to", "", "range end (default now)")
}

func (t timeFlags) resolve() (start, end int64, err error) {
	r := timeutil.Range{Since: t.since, From: t.from, To: t.to}
	start, end, rerr := r.Resolve()
	if rerr != nil {
		return 0, 0, cerrors.Wrap(rerr, cerrors.CategoryUsage, "BAD_TIME_RANGE", rerr.Error()).
			WithHint("Pass --since (e.g. 1h) or --from/--to.").
			WithNextSteps("openobserve-cli search run --stream <name> --since 1h --limit 10")
	}
	return start, end, nil
}

func newSearchRunCmd(s *appState) *cobra.Command {
	var (
		tf      timeFlags
		stream  string
		sql     string
		where   string
		order   string
		limit   int
		offset  int
		all     bool
		maxRows int
	)
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run a SQL query and return matching rows",
		Long: "Provide --stream to query a stream with an auto-built SELECT, optionally\n" +
			"narrowed by --where; or provide a full --sql query (use --sql @file or\n" +
			"--sql @- to read a long query from a file or stdin). The time range is\n" +
			"required (--since or --from/--to). JSON output returns a summary plus the\n" +
			"hits; --format ndjson streams one hit per line for piping. Use --all to\n" +
			"page through every matching row as ndjson (bound it with --max).",
		Example: "  # last hour of errors from the 'default' log stream\n" +
			"  openobserve-cli search run --stream default --where \"level = 'ERROR'\" --since 1h --limit 20\n\n" +
			"  # a full SQL query read from a file, over an explicit window\n" +
			"  openobserve-cli search run --sql @query.sql --since 24h\n\n" +
			"  # page through every matching row (streamed as ndjson)\n" +
			"  openobserve-cli search run --stream default --since 24h --all --max 50000",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			start, end, err := tf.resolve()
			if err != nil {
				return err
			}
			resolvedSQL, err := readInlineOrFile(sql)
			if err != nil {
				return err
			}
			query, err := buildRunSQL(resolvedSQL, stream, where, order)
			if err != nil {
				return err
			}

			ctx, cancel := cmdContext(s)
			defer cancel()
			client, err := s.newClient()
			if err != nil {
				return err
			}

			if all {
				return s.runSearchAll(ctx, client, query, start, end, offset, limit, maxRows)
			}

			size := limit
			if size <= 0 {
				size = constants.DefaultSearchLimit
			}
			if size > constants.MaxSearchLimit {
				size = constants.MaxSearchLimit
			}
			resp, err := client.Search(ctx, s.org(), apiclient.SearchRequest{
				Query: apiclient.SearchQuery{
					SQL:       query,
					StartTime: start,
					EndTime:   end,
					From:      offset,
					Size:      size,
				},
			})
			if err != nil {
				return err
			}
			// ndjson streams the raw hits, one per line — ideal for piping.
			if s.cfg().Defaults.Format == "ndjson" {
				return s.emitList(resp.Hits, pageInfo{})
			}
			return s.emit(map[string]any{
				"sql":          query,
				"total":        resp.Total,
				"returned":     len(resp.Hits),
				"took_ms":      resp.Took,
				"scan_size_mb": resp.ScanSize,
				"start_micros": start,
				"end_micros":   end,
				"hits":         resp.Hits,
			})
		},
	}
	f := cmd.Flags()
	f.StringVar(&stream, "stream", "", "stream to query (builds SELECT * FROM \"<stream>\")")
	f.StringVar(&sql, "sql", "", "full SQL query (overrides --stream/--where/--order)")
	f.StringVar(&where, "where", "", "WHERE clause to filter the auto-built query, e.g. \"level = 'ERROR'\"")
	f.StringVar(&order, "order", "desc", "_timestamp ordering for the auto-built query: desc or asc")
	f.IntVar(&limit, "limit", 0, fmt.Sprintf("max rows to return (default %d, max %d); with --all, the page size", constants.DefaultSearchLimit, constants.MaxSearchLimit))
	f.IntVar(&offset, "offset", 0, "row offset for pagination (maps to the search 'from')")
	f.BoolVar(&all, "all", false, "page through every matching row, streamed as ndjson")
	f.IntVar(&maxRows, "max", 0, "with --all, stop after this many rows (0 = no cap); a cap is reported on stderr")
	addTimeFlags(cmd, &tf)
	enumComplete(cmd, "order", "desc", "asc")
	return cmd
}

// runSearchAll pages through every matching row, streaming each page as ndjson.
// Buffering the whole result (or wrapping it in a single JSON envelope) would
// defeat the purpose, so output is always ndjson here. Paging is from/size based;
// a future large-scan path can use OpenObserve's _search_partition. When --max is
// reached, the truncation is announced on stderr — never silently.
func (s *appState) runSearchAll(ctx context.Context, client apiclient.Client, query string, start, end int64, from, limit, maxRows int) error {
	pageSize := constants.MaxSearchLimit
	if limit > 0 && limit < pageSize {
		pageSize = limit
	}
	fetched := 0
	for {
		size := pageSize
		if maxRows > 0 && maxRows-fetched < size {
			size = maxRows - fetched
		}
		if size <= 0 {
			break
		}
		resp, err := client.Search(ctx, s.org(), apiclient.SearchRequest{
			Query: apiclient.SearchQuery{SQL: query, StartTime: start, EndTime: end, From: from, Size: size},
		})
		if err != nil {
			return err
		}
		if len(resp.Hits) == 0 {
			break
		}
		if err := s.emitStream(resp.Hits); err != nil {
			return err
		}
		fetched += len(resp.Hits)
		from += len(resp.Hits)
		if len(resp.Hits) < size {
			break // short page → no more rows
		}
		if maxRows > 0 && fetched >= maxRows {
			output.EmitNotice(os.Stderr, map[string]any{"_notice": map[string]any{"truncated": map[string]any{
				"reason": "--max reached", "rows": fetched,
				"hint": "increase --max, or narrow the query / time range",
			}}})
			break
		}
	}
	return nil
}

func newSearchTailCmd(s *appState) *cobra.Command {
	var (
		stream   string
		where    string
		interval string
		backfill string
	)
	cmd := &cobra.Command{
		Use:   "tail",
		Short: "Follow a stream live, printing new rows as they arrive",
		Long: "Polls the stream on an interval and prints newly-arrived rows as ndjson,\n" +
			"like `tail -f` for logs. It runs until interrupted (Ctrl-C). Use --since to\n" +
			"backfill a window first; otherwise only rows arriving after start are shown.",
		Example: "  # follow errors in the 'app' stream\n" +
			"  openobserve-cli search tail --stream app --where \"level = 'ERROR'\"\n\n" +
			"  # backfill the last 5 minutes, then follow\n" +
			"  openobserve-cli search tail --stream app --since 5m --interval 3s",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if strings.TrimSpace(stream) == "" {
				return cerrors.New(cerrors.CategoryUsage, "NO_STREAM",
					"--stream is required for tail").
					WithNextSteps("openobserve-cli stream list")
			}
			pollEvery, err := time.ParseDuration(strings.TrimSpace(interval))
			if err != nil || pollEvery <= 0 {
				return cerrors.Newf(cerrors.CategoryUsage, "BAD_INTERVAL",
					"invalid --interval %q (want e.g. 2s, 5s, 1m)", interval)
			}
			query, err := buildRunSQL("", stream, where, "asc")
			if err != nil {
				return err
			}

			// A fresh, signal-cancellable context: tail runs indefinitely, so the
			// per-command request timeout would cut it off. Each poll gets its own
			// bounded child context.
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()

			client, err := s.newClient()
			if err != nil {
				return err
			}

			watermark := timeutil.ToMicros(time.Now())
			if b := strings.TrimSpace(backfill); b != "" {
				start, _, rerr := timeFlags{since: b}.resolve()
				if rerr != nil {
					return rerr
				}
				watermark = start
			}

			for {
				now := timeutil.ToMicros(time.Now())
				if now > watermark {
					reqCtx, cancel := context.WithTimeout(ctx, s.timeout())
					resp, serr := client.Search(reqCtx, s.org(), apiclient.SearchRequest{
						Query: apiclient.SearchQuery{SQL: query, StartTime: watermark, EndTime: now, Size: constants.MaxSearchLimit},
					})
					cancel()
					if serr != nil {
						if ctx.Err() != nil {
							return nil // interrupted mid-request
						}
						return serr
					}
					if len(resp.Hits) > 0 {
						if eerr := s.emitStream(resp.Hits); eerr != nil {
							return eerr
						}
						for _, h := range resp.Hits {
							if ts, ok := spanNumber(h, "_timestamp"); ok && int64(ts) >= watermark {
								watermark = int64(ts) + 1
							}
						}
					} else {
						watermark = now + 1
					}
				}
				select {
				case <-ctx.Done():
					return nil
				case <-time.After(pollEvery):
				}
			}
		},
	}
	f := cmd.Flags()
	f.StringVar(&stream, "stream", "", "stream to follow (required)")
	f.StringVar(&where, "where", "", "WHERE clause to filter, e.g. \"level = 'ERROR'\"")
	f.StringVar(&interval, "interval", "2s", "poll interval, e.g. 2s, 5s, 1m")
	f.StringVar(&backfill, "since", "", "backfill this window before following, e.g. 5m")
	return cmd
}

func newSearchHistogramCmd(s *appState) *cobra.Command {
	var (
		tf       timeFlags
		stream   string
		where    string
		interval string
	)
	cmd := &cobra.Command{
		Use:   "histogram",
		Short: "Return time-bucketed counts (the volume map before raw rows)",
		Long: "Runs a histogram(_timestamp, <interval>) aggregation so you can see how\n" +
			"many rows fall in each time bucket — the shape of the data — before\n" +
			"deciding which window to pull raw rows from with `search run`.",
		Example: "  openobserve-cli search histogram --stream default --since 6h --interval 5m",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if stream == "" {
				return cerrors.New(cerrors.CategoryUsage, "NO_STREAM",
					"--stream is required for histogram").
					WithNextSteps("openobserve-cli stream list")
			}
			start, end, err := tf.resolve()
			if err != nil {
				return err
			}
			oiv, err := convertInterval(interval)
			if err != nil {
				return err
			}
			query := buildHistogramSQL(stream, where, oiv)

			ctx, cancel := cmdContext(s)
			defer cancel()
			client, err := s.newClient()
			if err != nil {
				return err
			}
			resp, err := client.Search(ctx, s.org(), apiclient.SearchRequest{
				Query: apiclient.SearchQuery{
					SQL:       query,
					StartTime: start,
					EndTime:   end,
					Size:      0,
				},
			})
			if err != nil {
				return err
			}
			buckets := make([]map[string]any, 0, len(resp.Hits))
			for _, h := range resp.Hits {
				buckets = append(buckets, map[string]any{
					"bucket": h["zo_sql_key"],
					"count":  h["zo_sql_num"],
				})
			}
			return s.emit(map[string]any{
				"sql":          query,
				"interval":     oiv,
				"took_ms":      resp.Took,
				"start_micros": start,
				"end_micros":   end,
				"buckets":      buckets,
			})
		},
	}
	f := cmd.Flags()
	f.StringVar(&stream, "stream", "", "stream to aggregate (required)")
	f.StringVar(&where, "where", "", "WHERE clause to filter, e.g. \"level = 'ERROR'\"")
	f.StringVar(&interval, "interval", "1m", "bucket width: 30s, 1m, 5m, 1h, 1d")
	addTimeFlags(cmd, &tf)
	return cmd
}

// buildRunSQL returns the SQL to execute. An explicit --sql wins; otherwise a
// SELECT is assembled from --stream / --where / --order.
func buildRunSQL(sql, stream, where, order string) (string, error) {
	if strings.TrimSpace(sql) != "" {
		return sql, nil
	}
	if strings.TrimSpace(stream) == "" {
		return "", cerrors.New(cerrors.CategoryUsage, "NO_QUERY",
			"provide --stream (with optional --where) or a full --sql").
			WithNextSteps("openobserve-cli stream list",
				"openobserve-cli search run --stream <name> --since 1h --limit 10")
	}
	ord := strings.ToLower(strings.TrimSpace(order))
	if ord == "" {
		ord = "desc"
	}
	if ord != "asc" && ord != "desc" {
		return "", cerrors.Newf(cerrors.CategoryUsage, "BAD_ORDER",
			"invalid --order %q (want asc or desc)", order)
	}
	var b strings.Builder
	fmt.Fprintf(&b, `SELECT * FROM "%s"`, stream)
	if w := strings.TrimSpace(where); w != "" {
		fmt.Fprintf(&b, " WHERE %s", w)
	}
	fmt.Fprintf(&b, " ORDER BY _timestamp %s", strings.ToUpper(ord))
	return b.String(), nil
}

// buildHistogramSQL assembles the time-bucket aggregation query.
func buildHistogramSQL(stream, where, oInterval string) string {
	var b strings.Builder
	fmt.Fprintf(&b, `SELECT histogram(_timestamp, '%s') AS zo_sql_key, count(*) AS zo_sql_num FROM "%s"`, oInterval, stream)
	if w := strings.TrimSpace(where); w != "" {
		fmt.Fprintf(&b, " WHERE %s", w)
	}
	b.WriteString(" GROUP BY zo_sql_key ORDER BY zo_sql_key")
	return b.String()
}

var intervalRe = regexp.MustCompile(`^(\d+)\s*([smhdw])$`)

var intervalWords = map[string]string{
	"s": "second", "m": "minute", "h": "hour", "d": "day", "w": "week",
}

// convertInterval turns a compact bucket width ("5m") into OpenObserve's
// histogram interval form ("5 minute"). A value already in word form is passed
// through.
func convertInterval(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "1 minute", nil
	}
	if m := intervalRe.FindStringSubmatch(s); m != nil {
		n, _ := strconv.Atoi(m[1])
		return fmt.Sprintf("%d %s", n, intervalWords[m[2]]), nil
	}
	// Accept an already-worded interval like "10 second".
	if strings.ContainsAny(s, " ") {
		return s, nil
	}
	return "", cerrors.Newf(cerrors.CategoryUsage, "BAD_INTERVAL",
		"invalid --interval %q (want e.g. 30s, 1m, 5m, 1h)", s)
}
