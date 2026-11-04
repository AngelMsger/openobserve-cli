package apiclient

import (
	"encoding/json"

	cerrors "github.com/angelmsger/openobserve-cli/pkg/errors"
)

// A write changes remote state, so a failure after the request was sent can
// still have applied it: a timed-out create may have stored the alert. The
// family's write-outcome rule (cerrors.FromWrite) keeps such a failure from
// reading as retryable: only a definite refusal passes through; anything else
// says the outcome is unknown and points at the read below, so an agent
// reconciles before it creates a second alert or dashboard. Each target names
// what the write changes and a bounded, read-only command that shows it. The
// reads pass --org and --folder, so they look where the write went.

// alertWriteTarget is the target of an alert write op (a *Req from
// alerts_write.go).
func alertWriteTarget(op any) cerrors.WriteTarget {
	var org, id, folder string
	var body json.RawMessage
	switch v := op.(type) {
	case CreateAlertReq:
		org, folder, body = v.Org, v.Folder, v.Body
	case UpdateAlertReq:
		org, id, folder = v.Org, v.ID, v.Folder
	case DeleteAlertReq:
		org, id, folder = v.Org, v.ID, v.Folder
	case SetAlertEnabledReq:
		org, id, folder = v.Org, v.ID, v.Folder
	}
	folder = folderOrDefault(folder)
	scope := " in folder " + folder + " of org " + org
	if id == "" {
		return cerrors.WriteTarget{
			Description: "new alert" + quotedField(body, "name") + scope,
			ReadSteps:   []string{orgCommand(org) + " alert list --folder " + cerrors.ShellArg(folder)},
		}
	}
	return cerrors.WriteTarget{
		Description: "alert " + id + scope,
		ReadSteps: []string{orgCommand(org) + " alert get " + cerrors.ShellArg(id) +
			" --folder " + cerrors.ShellArg(folder)},
	}
}

// dashboardWriteTarget is the target of a dashboard write op (a *Req from
// dashboards_write.go). A new dashboard is found by its title, read from the
// definition that was sent: a backup envelope's title is in its active
// version, not at the top.
func dashboardWriteTarget(op any) cerrors.WriteTarget {
	var org, id, folder string
	var body json.RawMessage
	switch v := op.(type) {
	case CreateDashboardReq:
		org, folder, body = v.Org, v.Folder, v.Body
		if sent, err := NormalizeDashboardBody(v.Body); err == nil {
			body = sent
		}
	case UpdateDashboardReq:
		org, id, folder = v.Org, v.ID, v.Folder
	case DeleteDashboardReq:
		org, id, folder = v.Org, v.ID, v.Folder
	}
	folder = folderOrDefault(folder)
	scope := " in folder " + folder + " of org " + org
	if id == "" {
		read := orgCommand(org) + " dashboard list --folder " + cerrors.ShellArg(folder)
		if title := field(body, "title"); title != "" {
			read += " --title " + cerrors.ShellArg(title)
		}
		return cerrors.WriteTarget{
			Description: "new dashboard" + quotedField(body, "title") + scope,
			ReadSteps:   []string{read},
		}
	}
	return cerrors.WriteTarget{
		Description: "dashboard " + id + scope,
		ReadSteps: []string{orgCommand(org) + " dashboard get " + cerrors.ShellArg(id) +
			" --folder " + cerrors.ShellArg(folder)},
	}
}

func orgCommand(org string) string { return "openobserve-cli --org " + cerrors.ShellArg(org) }

// field reads a top-level string field from a definition body, or "".
func field(body json.RawMessage, key string) string {
	var m map[string]json.RawMessage
	if json.Unmarshal(body, &m) != nil {
		return ""
	}
	var s string
	_ = json.Unmarshal(m[key], &s)
	return s
}

// quotedField renders a body field for a description: ` "name"`, or "".
func quotedField(body json.RawMessage, key string) string {
	if s := field(body, key); s != "" {
		b, _ := json.Marshal(s)
		return " " + string(b)
	}
	return ""
}
