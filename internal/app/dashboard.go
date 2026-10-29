package app

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"

	"github.com/angelmsger/openobserve-cli/pkg/apiclient"
	cerrors "github.com/angelmsger/openobserve-cli/pkg/errors"
	"github.com/spf13/cobra"
)

// newDashboardCmd wires the dashboard group — the first write surface of the
// CLI. Reads (`list`, `get`) pass through the read-only wrapper like any other
// command; writes (`create`, `update`, `delete`) are blocked by it with a
// READONLY_BLOCKED error unless the invocation lifted the posture. All writes
// support --dry-run (a request plan, nothing sent), and the destructive
// `delete` requires --yes.
func newDashboardCmd(s *appState) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dashboard",
		Short: "List, inspect and manage OpenObserve dashboards",
		Long: "Dashboards live in OpenObserve's meta store, so the query commands\n" +
			"cannot see them. `dashboard list` discovers IDs, folders and hashes;\n" +
			"`get` fetches a definition; `create`, `update` and `delete` manage it.\n" +
			"Writes support --dry-run, `delete` requires --yes, and all three are\n" +
			"blocked by read-only mode (defaults.read_only / OPENOBSERVE_CLI_READ_ONLY)\n" +
			"unless you pass the root --allow-writes flag.",
	}
	cmd.AddCommand(
		newDashboardListCmd(s),
		newDashboardGetCmd(s),
		newDashboardCreateCmd(s),
		newDashboardUpdateCmd(s),
		newDashboardDeleteCmd(s),
	)
	return cmd
}

func newDashboardListCmd(s *appState) *cobra.Command {
	var folder, title string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List dashboards in the organization",
		Long: "Lists dashboard summaries: dashboard_id, title, folder, owner, schema\n" +
			"version and the current hash. Without filters the server lists the\n" +
			"\"default\" folder; --folder picks another, and --title matches a\n" +
			"case-insensitive substring across folders. Fetch a full definition\n" +
			"with `dashboard get <id>`.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, cancel := cmdContext(s)
			defer cancel()
			client, err := s.newClient()
			if err != nil {
				return err
			}
			summaries, err := client.ListDashboards(ctx, s.org(), apiclient.DashboardListOptions{
				Folder: folder, Title: title,
			})
			if err != nil {
				return err
			}
			return s.emitList(summaries, pageInfo{})
		},
	}
	f := cmd.Flags()
	f.StringVar(&folder, "folder", "", "folder ID to list (the server defaults to its \"default\" folder)")
	f.StringVar(&title, "title", "", "case-insensitive title substring; searches across folders when --folder is unset")
	return cmd
}

func newDashboardGetCmd(s *appState) *cobra.Command {
	var folder string
	cmd := &cobra.Command{
		Use:   "get <id>",
		Short: "Get a dashboard definition and its current hash",
		Long: "Returns the full dashboard JSON (the version-keyed body plus the\n" +
			"concurrent-edit hash) exactly as stored. The output feeds straight\n" +
			"into `dashboard create` / `update` — both unwrap the envelope for you.\n" +
			"Discover IDs with `dashboard list`.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := cmdContext(s)
			defer cancel()
			client, err := s.newClient()
			if err != nil {
				return err
			}
			d, err := client.GetDashboard(ctx, s.org(), args[0], folder)
			if err != nil {
				return err
			}
			return s.emit(d)
		},
	}
	cmd.Flags().StringVar(&folder, "folder", "", "folder ID the dashboard lives in (default: \"default\")")
	return cmd
}

func newDashboardCreateCmd(s *appState) *cobra.Command {
	var file, folder string
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a dashboard from a JSON definition file",
		Long: "The file holds the dashboard definition OpenObserve expects (a JSON\n" +
			"object carrying its schema `version`). A full `dashboard get` response\n" +
			"works too — the version-keyed body is unwrapped and the hash envelope\n" +
			"dropped. Use --file - to read from stdin. Blocked by read-only mode\n" +
			"unless you pass --allow-writes.",
		Example: "  openobserve-cli dashboard create --file dashboard.json\n" +
			"  openobserve-cli dashboard get ops-1 > ops-1.json && openobserve-cli dashboard create --file ops-1.json",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := readDashboardBody(file)
			if err != nil {
				return err
			}
			body, err = normalizeDashboardBody(body)
			if err != nil {
				return err
			}
			if dryRun {
				return s.emit(map[string]any{
					"dry_run": true,
					"request": s.requestPlan(http.MethodPost,
						"/api/"+url.PathEscape(s.org())+"/dashboards",
						dashboardPlanQuery(folder, ""), body),
				})
			}
			ctx, cancel := cmdContext(s)
			defer cancel()
			client, err := s.newClient()
			if err != nil {
				return err
			}
			d, err := client.CreateDashboard(ctx, s.org(), folder, body)
			if err != nil {
				return err
			}
			return s.emit(dashboardWriteSummary("created", d, folder))
		},
	}
	f := cmd.Flags()
	f.StringVar(&file, "file", "", "dashboard definition file (- for stdin); required")
	f.StringVar(&folder, "folder", "", "folder ID to create in (default: \"default\")")
	f.BoolVar(&dryRun, "dry-run", false, "print the request plan without sending anything")
	return cmd
}

func newDashboardUpdateCmd(s *appState) *cobra.Command {
	var file, folder, hash string
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "update <id>",
		Short: "Replace a dashboard's definition from a JSON file",
		Long: "Read-modify-write built in: unless --hash pins a revision, update\n" +
			"first GETs the dashboard to learn its current hash, then PUTs the new\n" +
			"body so the server's concurrent-edit check can run. A --dry-run plan\n" +
			"shows both requests and sends nothing; it never contacts the server.\n" +
			"Blocked by read-only mode unless you pass --allow-writes.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := args[0]
			body, err := readDashboardBody(file)
			if err != nil {
				return err
			}
			body, err = normalizeDashboardBody(body)
			if err != nil {
				return err
			}
			if dryRun {
				return s.emit(map[string]any{
					"dry_run": true,
					"request": s.requestPlan(http.MethodPut,
						"/api/"+url.PathEscape(s.org())+"/dashboards/"+url.PathEscape(id),
						dashboardPlanQuery(folder, hash), body),
					"prefetch": map[string]any{
						"method": http.MethodGet,
						"url":    s.planURL("/api/"+url.PathEscape(s.org())+"/dashboards/"+url.PathEscape(id), dashboardPlanQuery(folder, "")),
						"note":   "at run time (without --hash) the current hash is read from this GET and sent with the PUT",
					},
				})
			}
			ctx, cancel := cmdContext(s)
			defer cancel()
			client, err := s.newClient()
			if err != nil {
				return err
			}
			if hash == "" {
				// Read the current hash (a read, so it also passes through the
				// read-only wrapper) to feed the concurrent-edit check.
				current, err := client.GetDashboard(ctx, s.org(), id, folder)
				if err != nil {
					return err
				}
				hash = current.Hash
			}
			d, err := client.UpdateDashboard(ctx, s.org(), id, folder, hash, body)
			if err != nil {
				return err
			}
			return s.emit(dashboardWriteSummary("updated", d, folder))
		},
	}
	f := cmd.Flags()
	f.StringVar(&file, "file", "", "dashboard definition file (- for stdin); required")
	f.StringVar(&folder, "folder", "", "folder ID the dashboard lives in (default: \"default\")")
	f.StringVar(&hash, "hash", "", "expected current hash (skips the read-modify-write GET)")
	f.BoolVar(&dryRun, "dry-run", false, "print the request plan without sending anything")
	return cmd
}

func newDashboardDeleteCmd(s *appState) *cobra.Command {
	var folder string
	var yes, dryRun bool
	cmd := &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete a dashboard permanently (requires --yes)",
		Long: "Deletion cannot be undone, so --yes is mandatory. --dry-run prints\n" +
			"the request plan without deleting (and does not need --yes). Blocked\n" +
			"by read-only mode unless you pass --allow-writes.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := args[0]
			if !yes && !dryRun {
				return cerrors.Newf(cerrors.CategoryUsage, "CONFIRM_REQUIRED",
					"refusing to delete dashboard %q without --yes", id).
					WithHint("Deletion is permanent; confirm it explicitly.").
					WithNextSteps(fmt.Sprintf("openobserve-cli dashboard delete %s --yes", id),
						"Back up first: openobserve-cli dashboard get "+id)
			}
			if dryRun {
				return s.emit(map[string]any{
					"dry_run": true,
					"request": map[string]any{
						"method": http.MethodDelete,
						"url":    s.planURL("/api/"+url.PathEscape(s.org())+"/dashboards/"+url.PathEscape(id), dashboardPlanQuery(folder, "")),
					},
				})
			}
			ctx, cancel := cmdContext(s)
			defer cancel()
			client, err := s.newClient()
			if err != nil {
				return err
			}
			if err := client.DeleteDashboard(ctx, s.org(), id, folder); err != nil {
				return err
			}
			return s.emit(map[string]any{"dashboard_id": id, "folder": folderOrDefault(folder), "deleted": true})
		},
	}
	f := cmd.Flags()
	f.StringVar(&folder, "folder", "", "folder ID the dashboard lives in (default: \"default\")")
	f.BoolVar(&yes, "yes", false, "confirm the deletion (mandatory)")
	f.BoolVar(&dryRun, "dry-run", false, "print the request plan without sending anything")
	return cmd
}

// readDashboardBody loads the --file value: "-" reads stdin, a leading "@" is
// tolerated for consistency with --sql @file, anything else is a path.
func readDashboardBody(file string) (json.RawMessage, error) {
	if file == "" {
		return nil, cerrors.New(cerrors.CategoryUsage, "BODY_FILE_REQUIRED",
			"a dashboard definition file is required").
			WithNextSteps("--file dashboard.json", "--file -  (read from stdin)")
	}
	var data []byte
	var err error
	if file == "-" {
		data, err = io.ReadAll(os.Stdin)
	} else {
		name := file
		if len(name) > 1 && name[0] == '@' {
			name = name[1:]
		}
		data, err = os.ReadFile(name)
	}
	if err != nil {
		return nil, cerrors.Wrap(err, cerrors.CategoryUsage, "FILE_READ",
			"could not read "+file)
	}
	if len(data) == 0 {
		return nil, cerrors.New(cerrors.CategoryUsage, "EMPTY_BODY",
			"the dashboard definition file is empty")
	}
	return json.RawMessage(data), nil
}

// normalizeDashboardBody validates that body is a JSON object and unwraps a
// `dashboard get` envelope ({v<N>: {...}, version, hash, ...}) down to the
// definition OpenObserve expects for create/update.
func normalizeDashboardBody(body json.RawMessage) (json.RawMessage, error) {
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, cerrors.Wrap(err, cerrors.CategoryUsage, "BAD_BODY",
			"the dashboard definition is not valid JSON")
	}
	if m == nil {
		return nil, cerrors.New(cerrors.CategoryUsage, "BAD_BODY",
			"the dashboard definition must be a JSON object")
	}
	if _, hasHash := m["hash"]; hasHash {
		for i := 1; i <= 8; i++ {
			if inner, ok := m[fmt.Sprintf("v%d", i)].(map[string]any); ok {
				return json.Marshal(inner)
			}
		}
	}
	return json.Marshal(m)
}

// dashboardWriteSummary is the compact, stable output of create/update:
// identifiers and the new hash, not the (potentially huge) definition.
func dashboardWriteSummary(verb string, d *apiclient.Dashboard, folder string) map[string]any {
	out := map[string]any{
		"dashboard_id": d.DashboardID,
		"folder":       folderOrDefault(folder),
		verb:           true,
	}
	if d.Title != "" {
		out["title"] = d.Title
	}
	if d.Version != 0 {
		out["version"] = d.Version
	}
	if d.Hash != "" {
		out["hash"] = d.Hash
	}
	return out
}

// dashboardPlanQuery mirrors the query parameters the API client sends, so a
// --dry-run plan shows the real URL.
func dashboardPlanQuery(folder, hash string) url.Values {
	q := url.Values{}
	q.Set("folder", folderOrDefault(folder))
	if hash != "" {
		q.Set("hash", hash)
	}
	return q
}

// planURL renders the absolute URL a request would go to.
func (s *appState) planURL(path string, q url.Values) string {
	base, err := apiclient.NormalizeBaseURL(s.cfg().BaseURL)
	if err != nil {
		return path
	}
	if enc := q.Encode(); enc != "" {
		return base + path + "?" + enc
	}
	return base + path
}

// requestPlan builds the plan object shared by create/update --dry-run.
func (s *appState) requestPlan(method, path string, q url.Values, body json.RawMessage) map[string]any {
	var parsed any
	_ = json.Unmarshal(body, &parsed)
	return map[string]any{
		"method": method,
		"url":    s.planURL(path, q),
		"body":   parsed,
	}
}

func folderOrDefault(folder string) string {
	if folder == "" {
		return "default"
	}
	return folder
}
