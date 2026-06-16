package app

import (
	"github.com/spf13/cobra"
)

func newStreamCmd(s *appState) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stream",
		Short: "Discover streams and inspect their schema",
		Long: "Streams are the logs / metrics / traces collections you query with\n" +
			"`search`. List them to find names, then inspect a stream's schema to\n" +
			"learn which columns exist before writing SQL.",
	}
	cmd.AddCommand(
		newStreamListCmd(s),
		newStreamGetCmd(s),
		newStreamSchemaCmd(s),
		newStreamStatsCmd(s),
	)
	return cmd
}

func newStreamListCmd(s *appState) *cobra.Command {
	var (
		streamType string
		schema     bool
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List streams in the organization (the discovery map)",
		Long: "Lists streams with their type and storage stats. Schema is omitted by\n" +
			"default to keep the output a compact map; add --schema for full fields,\n" +
			"or use `stream schema <name>` for one stream.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, cancel := cmdContext(s)
			defer cancel()
			client, err := s.newClient()
			if err != nil {
				return err
			}
			streams, err := client.ListStreams(ctx, s.org(), streamType, schema)
			if err != nil {
				return err
			}
			return s.emitList(streams, pageInfo{})
		},
	}
	f := cmd.Flags()
	f.StringVar(&streamType, "type", "", "filter by stream type: logs, metrics or traces")
	f.BoolVar(&schema, "schema", false, "include each stream's field schema")
	enumComplete(cmd, "type", "logs", "metrics", "traces")
	return cmd
}

func newStreamGetCmd(s *appState) *cobra.Command {
	var streamType string
	cmd := &cobra.Command{
		Use:   "get <name>",
		Short: "Get one stream including schema, settings and stats",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := cmdContext(s)
			defer cancel()
			client, err := s.newClient()
			if err != nil {
				return err
			}
			st, err := client.GetStream(ctx, s.org(), args[0], streamType)
			if err != nil {
				return err
			}
			return s.emit(st)
		},
	}
	cmd.Flags().StringVar(&streamType, "type", "", "stream type hint: logs, metrics or traces")
	enumComplete(cmd, "type", "logs", "metrics", "traces")
	return cmd
}

func newStreamSchemaCmd(s *appState) *cobra.Command {
	var streamType string
	cmd := &cobra.Command{
		Use:   "schema <name>",
		Short: "Show just a stream's queryable columns and search settings",
		Long: "Returns the stream's field schema plus its full-text-search and\n" +
			"partition settings — what you need to write a correct SQL query.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := cmdContext(s)
			defer cancel()
			client, err := s.newClient()
			if err != nil {
				return err
			}
			st, err := client.GetStream(ctx, s.org(), args[0], streamType)
			if err != nil {
				return err
			}
			return s.emit(map[string]any{
				"name":        st.Name,
				"stream_type": st.StreamType,
				"schema":      st.Schema,
				"settings":    st.Settings,
			})
		},
	}
	cmd.Flags().StringVar(&streamType, "type", "", "stream type hint: logs, metrics or traces")
	enumComplete(cmd, "type", "logs", "metrics", "traces")
	return cmd
}

func newStreamStatsCmd(s *appState) *cobra.Command {
	var streamType string
	cmd := &cobra.Command{
		Use:   "stats <name>",
		Short: "Show a stream's document count, time range and storage size",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := cmdContext(s)
			defer cancel()
			client, err := s.newClient()
			if err != nil {
				return err
			}
			st, err := client.GetStream(ctx, s.org(), args[0], streamType)
			if err != nil {
				return err
			}
			out := map[string]any{"name": st.Name, "stream_type": st.StreamType}
			if st.Stats != nil {
				out["stats"] = st.Stats
			}
			return s.emit(out)
		},
	}
	cmd.Flags().StringVar(&streamType, "type", "", "stream type hint: logs, metrics or traces")
	enumComplete(cmd, "type", "logs", "metrics", "traces")
	return cmd
}
