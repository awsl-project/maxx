package cmd

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"

	"github.com/awsl-project/maxx/cmd/maxx-cli/internal/output"
)

func newSettingsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "settings",
		Aliases: []string{"setting"},
		Short:   "Manage system settings (key/value)",
	}
	cmd.AddCommand(
		newSettingsListCmd(),
		newSettingsGetCmd(),
		newSettingsSetCmd(),
		newSettingsDeleteCmd(),
	)
	return cmd
}

func newSettingsListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all settings",
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, _, err := authedClient()
			if err != nil {
				return err
			}
			settings, err := client.ListSettings()
			if err != nil {
				return handleAPIError(err)
			}
			return output.PrintOrJSON(cmd.OutOrStdout(), outputFormat(), settings, func() output.Table {
				t := output.Table{Headers: []string{"KEY", "VALUE"}}
				keys := make([]string, 0, len(settings))
				for k := range settings {
					keys = append(keys, k)
				}
				sort.Strings(keys)
				for _, k := range keys {
					t.Rows = append(t.Rows, []string{k, output.Truncate(settings[k], 60)})
				}
				return t
			})
		},
	}
}

func newSettingsGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get KEY",
		Short: "Get a single setting",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, _, err := authedClient()
			if err != nil {
				return err
			}
			s, err := client.GetSetting(args[0])
			if err != nil {
				return handleAPIError(err)
			}
			if outputFormat() == output.FormatJSON {
				return output.JSON(cmd.OutOrStdout(), s)
			}
			fmt.Fprintln(cmd.OutOrStdout(), s.Value)
			return nil
		},
	}
}

func newSettingsSetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "set KEY VALUE",
		Short: "Create or update a setting",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if flagDryRun {
				return previewJSON(cmd.OutOrStdout(),
					fmt.Sprintf("PUT /api/admin/settings/%s", args[0]),
					map[string]string{"value": args[1]})
			}
			client, _, err := authedClient()
			if err != nil {
				return err
			}
			s, err := client.SetSetting(args[0], args[1])
			if err != nil {
				return handleAPIError(err)
			}
			return output.JSON(cmd.OutOrStdout(), s)
		},
	}
}

func newSettingsDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete KEY",
		Short: "Delete a setting",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if flagDryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "[dry-run] DELETE /api/admin/settings/%s\n", args[0])
				return nil
			}
			if !confirm(fmt.Sprintf("Delete setting %q?", args[0])) {
				return fmt.Errorf("aborted")
			}
			client, _, err := authedClient()
			if err != nil {
				return err
			}
			if err := client.DeleteSetting(args[0]); err != nil {
				return handleAPIError(err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Deleted setting %q.\n", args[0])
			return nil
		},
	}
}
