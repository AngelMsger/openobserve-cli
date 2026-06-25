package app

import (
	"github.com/angelmsger/openobserve-cli/internal/config"
	cerrors "github.com/angelmsger/openobserve-cli/pkg/errors"
	"github.com/spf13/cobra"
)

func newOrgCmd(s *appState) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "org",
		Short: "List organizations and set the default one",
		Long: "Organizations scope every OpenObserve request. `org list` discovers\n" +
			"the identifiers your credential can use; `org use` records one as the\n" +
			"default so later commands need no --org.",
	}
	cmd.AddCommand(newOrgListCmd(s), newOrgUseCmd(s))
	return cmd
}

func newOrgListCmd(s *appState) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List organizations the credential can access",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, cancel := cmdContext(s)
			defer cancel()
			client, err := s.newClient()
			if err != nil {
				return err
			}
			orgs, err := client.ListOrgs(ctx)
			if err != nil {
				return err
			}
			return s.emitList(orgs, pageInfo{})
		},
	}
}

func newOrgUseCmd(s *appState) *cobra.Command {
	return &cobra.Command{
		Use:   "use <identifier>",
		Short: "Set the default organization in the active context",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			orgID := args[0]
			file, ok, err := config.ReadFile(s.cfgDir)
			if err != nil {
				return cerrors.Wrap(err, cerrors.CategoryConfig, "CONFIG_READ", "failed to read config")
			}
			if !ok || len(file.Contexts) == 0 {
				return cerrors.New(cerrors.CategoryConfig, "NO_CONTEXT",
					"no configured context to set the org on").
					WithNextSteps("openobserve-cli config init")
			}
			target := s.resolved.ActiveContext
			if target == "" {
				target = file.CurrentContext
			}
			if target == "" {
				target = file.Contexts[0].Name
			}
			nc, found := file.Context(target)
			if !found {
				return cerrors.Newf(cerrors.CategoryConfig, "NO_CONTEXT",
					"context %q not found", target).
					WithNextSteps("openobserve-cli config contexts")
			}
			nc.Org = orgID
			file.Upsert(nc)
			if err := config.WriteFile(s.cfgDir, file); err != nil {
				return cerrors.Wrap(err, cerrors.CategoryConfig, "CONFIG_WRITE", "failed to write config")
			}
			return s.emit(map[string]any{"org": orgID, "context": nc.Name, "updated": true})
		},
	}
}
