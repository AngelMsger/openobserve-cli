package app

import (
	"encoding/json"
	"fmt"

	"github.com/angelmsger/openobserve-cli/pkg/apiclient"
	cerrors "github.com/angelmsger/openobserve-cli/pkg/errors"
	"github.com/spf13/cobra"
)

// newDashboardCmd wires the dashboard group — a write surface of the
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
			body, err = apiclient.NormalizeDashboardBody(body)
			if err != nil {
				return err
			}
			if dryRun {
				return s.emitWritePlan(apiclient.CreateDashboardReq{Org: s.org(), Folder: folder, Body: body})
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
		Long: "Update uses --hash when supplied, otherwise the exported backup's hash.\n" +
			"For a bare definition without a hash, it first GETs the current revision.\n" +
			"A stale backup fails with a conflict; fetch and re-apply your edits.\n" +
			"--dry-run stays offline and includes a prefetch GET when the PUT hash\n" +
			"must be resolved at run time. Blocked by read-only mode unless you\n" +
			"pass --allow-writes.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := args[0]
			body, err := readDashboardBody(file)
			if err != nil {
				return err
			}
			// Validate without dropping the exported revision hash.
			if _, err := apiclient.NormalizeDashboardBody(body); err != nil {
				return err
			}
			if dryRun {
				return s.emitWritePlan(apiclient.UpdateDashboardReq{Org: s.org(), ID: id, Folder: folder, Hash: hash, Body: body})
			}
			ctx, cancel := cmdContext(s)
			defer cancel()
			client, err := s.newClient()
			if err != nil {
				return err
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
	f.StringVar(&hash, "hash", "", "expected revision hash (overrides the backup hash; skips prefetch)")
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
				return s.emitWritePlan(apiclient.DeleteDashboardReq{Org: s.org(), ID: id, Folder: folder})
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

func readDashboardBody(file string) (json.RawMessage, error) {
	return readDefinitionBody(file, "dashboard")
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
