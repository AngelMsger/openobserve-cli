package app

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/angelmsger/openobserve-cli/pkg/apiclient"
	cerrors "github.com/angelmsger/openobserve-cli/pkg/errors"
	"github.com/spf13/cobra"
)

// newAlertCmd wires the alert group — the first write surface of the CLI.
// Reads (`list`, `get`) pass through the read-only wrapper like any other
// command; writes (`create`, `update`, `delete`, `enable`, `disable`) are
// blocked by it with a READONLY_BLOCKED error unless the invocation lifted the
// posture. All writes support --dry-run (a request plan, nothing sent), and
// the destructive `delete` requires --yes.
func newAlertCmd(s *appState) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "alert",
		Short: "List, inspect and manage OpenObserve alerts",
		Long: "Alerts live in OpenObserve's meta store, so the query commands\n" +
			"cannot see them. `alert list` discovers IDs, names, streams and folders;\n" +
			"`get` fetches a definition; `create`, `update`, `delete`, `enable` and\n" +
			"`disable` manage it. Writes support --dry-run, `delete` requires --yes,\n" +
			"and all are blocked by read-only mode (defaults.read_only /\n" +
			"OPENOBSERVE_CLI_READ_ONLY) unless you pass the root --allow-writes flag.",
	}
	cmd.AddCommand(
		newAlertListCmd(s),
		newAlertGetCmd(s),
		newAlertCreateCmd(s),
		newAlertUpdateCmd(s),
		newAlertDeleteCmd(s),
		newAlertSetEnabledCmd(s, true),
		newAlertSetEnabledCmd(s, false),
	)
	return cmd
}

func newAlertListCmd(s *appState) *cobra.Command {
	var folder string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List alerts in the organization",
		Long: "Lists alert summaries: alert_id, name, stream, folder, owner and\n" +
			"whether the alert is enabled. Without filters the server lists the\n" +
			"\"default\" folder; --folder picks another. Fetch the full definition\n" +
			"with `alert get <id>`.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, cancel := cmdContext(s)
			defer cancel()
			client, err := s.newClient()
			if err != nil {
				return err
			}
			summaries, err := client.ListAlerts(ctx, s.org(), apiclient.AlertListOptions{Folder: folder})
			if err != nil {
				return err
			}
			return s.emitList(summaries, pageInfo{})
		},
	}
	cmd.Flags().StringVar(&folder, "folder", "", "folder ID to list (the server defaults to its \"default\" folder)")
	return cmd
}

func newAlertGetCmd(s *appState) *cobra.Command {
	var folder string
	cmd := &cobra.Command{
		Use:   "get <id>",
		Short: "Get an alert definition",
		Long: "Returns the full alert JSON (name, stream, query condition, trigger\n" +
			"condition, destinations, ...) exactly as stored. The output feeds\n" +
			"straight into `alert create` / `update` — both strip the server-managed\n" +
			"fields for you. Discover IDs with `alert list`.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := cmdContext(s)
			defer cancel()
			client, err := s.newClient()
			if err != nil {
				return err
			}
			a, err := client.GetAlert(ctx, s.org(), args[0], folder)
			if err != nil {
				return err
			}
			return s.emit(a)
		},
	}
	cmd.Flags().StringVar(&folder, "folder", "", "folder ID the alert lives in (default: \"default\")")
	return cmd
}

func newAlertCreateCmd(s *appState) *cobra.Command {
	var file, folder string
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create an alert from a JSON definition file",
		Long: "The file holds the alert definition OpenObserve expects (name,\n" +
			"stream_type, stream_name, query_condition, trigger_condition). A full\n" +
			"`alert get` response works too — server-managed fields (id, owner,\n" +
			"timestamps) are stripped. Stream names come from `stream list`. Use\n" +
			"--file - to read from stdin. Blocked by read-only mode unless you\n" +
			"pass --allow-writes.",
		Example: "  openobserve-cli alert create --file alert.json\n" +
			"  openobserve-cli alert get 02tzjqPXzvOx8NBvgBWB2rinBDa > alert.json && openobserve-cli alert create --file alert.json",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := readAlertBody(file)
			if err != nil {
				return err
			}
			body, err = normalizeAlertBody(body)
			if err != nil {
				return err
			}
			if dryRun {
				return s.emit(map[string]any{
					"dry_run": true,
					"request": s.requestPlan(http.MethodPost,
						"/api/"+url.PathEscape(s.org())+"/alerts",
						alertPlanQuery(folder, ""), body),
				})
			}
			ctx, cancel := cmdContext(s)
			defer cancel()
			client, err := s.newClient()
			if err != nil {
				return err
			}
			a, err := client.CreateAlert(ctx, s.org(), folder, body)
			if err != nil {
				return err
			}
			return s.emit(alertWriteSummary("created", a, folder))
		},
	}
	f := cmd.Flags()
	f.StringVar(&file, "file", "", "alert definition file (- for stdin); required")
	f.StringVar(&folder, "folder", "", "folder ID to create in (default: \"default\")")
	f.BoolVar(&dryRun, "dry-run", false, "print the request plan without sending anything")
	return cmd
}

func newAlertUpdateCmd(s *appState) *cobra.Command {
	var file, folder string
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "update <id>",
		Short: "Replace an alert's definition from a JSON file",
		Long: "Reads as little as possible: the file is the new definition, sent\n" +
			"verbatim. Server-managed fields from an `alert get` backup are\n" +
			"stripped first, so a copy-edit-restore round trip needs no `jq`\n" +
			"surgery. A --dry-run plan shows the request and sends nothing.\n" +
			"Blocked by read-only mode unless you pass --allow-writes.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := args[0]
			body, err := readAlertBody(file)
			if err != nil {
				return err
			}
			body, err = normalizeAlertBody(body)
			if err != nil {
				return err
			}
			if dryRun {
				return s.emit(map[string]any{
					"dry_run": true,
					"request": s.requestPlan(http.MethodPut,
						"/api/"+url.PathEscape(s.org())+"/alerts/"+url.PathEscape(id),
						alertPlanQuery(folder, ""), body),
				})
			}
			ctx, cancel := cmdContext(s)
			defer cancel()
			client, err := s.newClient()
			if err != nil {
				return err
			}
			a, err := client.UpdateAlert(ctx, s.org(), id, folder, body)
			if err != nil {
				return err
			}
			return s.emit(alertWriteSummary("updated", a, folder))
		},
	}
	f := cmd.Flags()
	f.StringVar(&file, "file", "", "alert definition file (- for stdin); required")
	f.StringVar(&folder, "folder", "", "folder ID the alert lives in (default: \"default\")")
	f.BoolVar(&dryRun, "dry-run", false, "print the request plan without sending anything")
	return cmd
}

func newAlertDeleteCmd(s *appState) *cobra.Command {
	var folder string
	var yes, dryRun bool
	cmd := &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete an alert permanently (requires --yes)",
		Long: "Deletion cannot be undone, so --yes is mandatory. --dry-run prints\n" +
			"the request plan without deleting (and does not need --yes). Blocked\n" +
			"by read-only mode unless you pass --allow-writes.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := args[0]
			if !yes && !dryRun {
				return cerrors.Newf(cerrors.CategoryUsage, "CONFIRM_REQUIRED",
					"refusing to delete alert %q without --yes", id).
					WithHint("Deletion is permanent; confirm it explicitly.").
					WithNextSteps(fmt.Sprintf("openobserve-cli alert delete %s --yes", id),
						"Back up first: openobserve-cli alert get "+id)
			}
			if dryRun {
				return s.emit(map[string]any{
					"dry_run": true,
					"request": map[string]any{
						"method": http.MethodDelete,
						"url":    s.planURL("/api/"+url.PathEscape(s.org())+"/alerts/"+url.PathEscape(id), alertPlanQuery(folder, "")),
					},
				})
			}
			ctx, cancel := cmdContext(s)
			defer cancel()
			client, err := s.newClient()
			if err != nil {
				return err
			}
			if err := client.DeleteAlert(ctx, s.org(), id, folder); err != nil {
				return err
			}
			return s.emit(map[string]any{"alert_id": id, "folder": folderOrDefault(folder), "deleted": true})
		},
	}
	f := cmd.Flags()
	f.StringVar(&folder, "folder", "", "folder ID the alert lives in (default: \"default\")")
	f.BoolVar(&yes, "yes", false, "confirm the deletion (mandatory)")
	f.BoolVar(&dryRun, "dry-run", false, "print the request plan without sending anything")
	return cmd
}

// newAlertSetEnabledCmd wires `alert enable` / `alert disable`: the cheap
// toggle — unlike `update`, it touches nothing but the alert's enabled flag
// and never needs a definition file.
func newAlertSetEnabledCmd(s *appState, enable bool) *cobra.Command {
	var folder string
	var dryRun bool
	verb := "enable"
	if !enable {
		verb = "disable"
	}
	cmd := &cobra.Command{
		Use: verb + " <id>",
		Short: fmt.Sprintf("%s an alert without editing its definition",
			strings.ToUpper(verb[:1])+verb[1:]),
		Long: "Whether an alert fires is a single server-side flag, so it gets its own\n" +
			"command: no definition file, no read-modify-write. Removing the alert\n" +
			"entirely needs `alert delete`; editing its destinations permanently\n" +
			"needs `alert update`. Blocked by read-only mode unless you pass\n" +
			"--allow-writes.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := args[0]
			q := alertPlanQuery(folder, "")
			if !enable {
				q.Set("value", "false")
			} else {
				q.Set("value", "true")
			}
			if dryRun {
				return s.emit(map[string]any{
					"dry_run": true,
					"request": map[string]any{
						"method": http.MethodPatch,
						"url":    s.planURL("/api/"+url.PathEscape(s.org())+"/alerts/"+url.PathEscape(id)+"/enabled", q),
					},
				})
			}
			ctx, cancel := cmdContext(s)
			defer cancel()
			client, err := s.newClient()
			if err != nil {
				return err
			}
			if err := client.SetAlertEnabled(ctx, s.org(), id, folder, enable); err != nil {
				return err
			}
			if enable {
				return s.emit(map[string]any{"alert_id": id, "folder": folderOrDefault(folder), "enabled": true})
			}
			return s.emit(map[string]any{"alert_id": id, "folder": folderOrDefault(folder), "disabled": true})
		},
	}
	f := cmd.Flags()
	f.StringVar(&folder, "folder", "", "folder ID the alert lives in (default: \"default\")")
	f.BoolVar(&dryRun, "dry-run", false, "print the request plan without sending anything")
	return cmd
}

// readAlertBody loads the --file value: "-" reads stdin, a leading "@" is
// tolerated for consistency with --sql @file, anything else is a path.
func readAlertBody(file string) (json.RawMessage, error) {
	if file == "" {
		return nil, cerrors.New(cerrors.CategoryUsage, "BODY_FILE_REQUIRED",
			"an alert definition file is required").
			WithNextSteps("--file alert.json", "--file -  (read from stdin)")
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
			"the alert definition file is empty")
	}
	return json.RawMessage(data), nil
}

// alertServerManagedFields are the fields the server owns; `alert create` and
// `update` strip them from a definition so an `alert get` backup feeds back in
// unchanged.
var alertServerManagedFields = []string{
	"id", "owner", "created_at", "updated_at",
	"last_triggered_at", "last_satisfied_at", "folder_id", "folder_name",
}

// normalizeAlertBody validates that body is a JSON object and strips the
// server-managed fields down to the definition OpenObserve expects for
// create/update.
func normalizeAlertBody(body json.RawMessage) (json.RawMessage, error) {
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, cerrors.Wrap(err, cerrors.CategoryUsage, "BAD_BODY",
			"the alert definition is not valid JSON")
	}
	if m == nil {
		return nil, cerrors.New(cerrors.CategoryUsage, "BAD_BODY",
			"the alert definition must be a JSON object")
	}
	for _, k := range alertServerManagedFields {
		delete(m, k)
	}
	return json.Marshal(m)
}

// alertWriteSummary is the compact, stable output of create/update:
// identifiers, not the (potentially huge) definition.
func alertWriteSummary(verb string, a *apiclient.Alert, folder string) map[string]any {
	out := map[string]any{
		"alert_id": a.AlertID,
		"folder":   folderOrDefault(folder),
		verb:       true,
	}
	if a.Name != "" {
		out["name"] = a.Name
	}
	if a.StreamName != "" {
		out["stream_name"] = a.StreamName
	}
	return out
}

// alertPlanQuery mirrors the query parameters the API client sends, so a
// --dry-run plan shows the real URL.
func alertPlanQuery(folder, value string) url.Values {
	q := url.Values{}
	q.Set("folder", folderOrDefault(folder))
	if value != "" {
		q.Set("value", value)
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
