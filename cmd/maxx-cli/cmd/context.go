package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/awsl-project/maxx/cmd/maxx-cli/internal/cfg"
	"github.com/awsl-project/maxx/cmd/maxx-cli/internal/output"
)

func newContextCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "context",
		Aliases: []string{"ctx"},
		Short:   "Manage CLI contexts (server endpoints + tokens)",
	}
	cmd.AddCommand(
		newContextListCmd(),
		newContextCurrentCmd(),
		newContextUseCmd(),
		newContextDeleteCmd(),
	)
	return cmd
}

func newContextListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List configured contexts",
		RunE: func(cmd *cobra.Command, _ []string) error {
			conf, err := cfg.Load()
			if err != nil {
				return err
			}
			return output.PrintOrJSON(cmd.OutOrStdout(), outputFormat(), conf, func() output.Table {
				t := output.Table{Headers: []string{"CURRENT", "NAME", "SERVER", "USERNAME", "EXPIRES"}}
				for _, c := range conf.Contexts {
					marker := ""
					if c.Name == conf.CurrentContext {
						marker = "*"
					}
					t.Rows = append(t.Rows, []string{
						marker, c.Name, c.Server, c.Username, output.FormatTime(c.ExpiresAt),
					})
				}
				return t
			})
		},
	}
}

func newContextCurrentCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "current",
		Short: "Print the current context name",
		RunE: func(cmd *cobra.Command, _ []string) error {
			conf, err := cfg.Load()
			if err != nil {
				return err
			}
			if conf.CurrentContext == "" {
				return cfg.ErrNoContext
			}
			fmt.Fprintln(cmd.OutOrStdout(), conf.CurrentContext)
			return nil
		},
	}
}

func newContextUseCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "use NAME",
		Short: "Set NAME as the current context",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			conf, err := cfg.Load()
			if err != nil {
				return err
			}
			if conf.FindContext(args[0]) == nil {
				return fmt.Errorf("context %q not found", args[0])
			}
			// --dry-run must not write the local config; it only previews
			// the context switch that would be persisted.
			if flagDryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "[dry-run] would switch current context to %q\n", args[0])
				return nil
			}
			conf.CurrentContext = args[0]
			if err := cfg.Save(conf); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Switched to context %q.\n", args[0])
			return nil
		},
	}
}

func newContextDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete NAME",
		Short: "Delete a saved context",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			conf, err := cfg.Load()
			if err != nil {
				return err
			}
			if conf.FindContext(args[0]) == nil {
				return fmt.Errorf("context %q not found", args[0])
			}
			// --dry-run prevents both the confirmation prompt and the
			// write. The contract is "preview, never mutate".
			if flagDryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "[dry-run] would delete context %q from config\n", args[0])
				return nil
			}
			if !confirm(fmt.Sprintf("Delete context %q?", args[0])) {
				return fmt.Errorf("aborted")
			}
			if !conf.Remove(args[0]) {
				return fmt.Errorf("context %q not found", args[0])
			}
			if err := cfg.Save(conf); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Deleted context %q.\n", args[0])
			return nil
		},
	}
}
