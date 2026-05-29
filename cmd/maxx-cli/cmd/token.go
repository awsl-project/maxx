package cmd

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/awsl-project/maxx/cmd/maxx-cli/internal/api"
	"github.com/awsl-project/maxx/cmd/maxx-cli/internal/output"
)

func newTokenCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "token",
		Aliases: []string{"tokens", "api-token", "api-tokens"},
		Short:   "Manage API tokens",
	}
	cmd.AddCommand(
		newTokenListCmd(),
		newTokenGetCmd(),
		newTokenCreateCmd(),
		newTokenUpdateCmd(),
		newTokenDeleteCmd(),
	)
	return cmd
}

func newTokenListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List API tokens",
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, _, err := authedClient()
			if err != nil {
				return err
			}
			tokens, err := client.ListAPITokens()
			if err != nil {
				return handleAPIError(err)
			}
			return output.PrintOrJSON(cmd.OutOrStdout(), outputFormat(), tokens, func() output.Table {
				t := output.Table{Headers: []string{"ID", "NAME", "PREFIX", "PROJECT", "ENABLED", "DEV", "USES", "EXPIRES"}}
				for _, tk := range tokens {
					t.Rows = append(t.Rows, []string{
						strconv.FormatUint(tk.ID, 10),
						output.Truncate(tk.Name, 30),
						tk.TokenPrefix,
						strconv.FormatUint(tk.ProjectID, 10),
						output.FormatBool(tk.IsEnabled),
						output.FormatBool(tk.DevMode),
						strconv.FormatUint(tk.UseCount, 10),
						output.FormatTimePtr(tk.ExpiresAt),
					})
				}
				return t
			})
		},
	}
}

func newTokenGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get ID",
		Short: "Get one API token",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseUint(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid id %q", args[0])
			}
			client, _, err := authedClient()
			if err != nil {
				return err
			}
			tk, err := client.GetAPIToken(id)
			if err != nil {
				return handleAPIError(err)
			}
			return output.JSON(cmd.OutOrStdout(), tk)
		},
	}
}

func newTokenCreateCmd() *cobra.Command {
	var (
		name        string
		description string
		projectID   uint64
		expiresAt   string
	)
	cmd := &cobra.Command{
		Use:   "create --name NAME [--description ...] [--project-id N] [--expires-at RFC3339]",
		Short: "Create an API token (plaintext token returned once)",
		Long: `Create an API token. The plaintext token is in the "token" field of
the response and IS RETURNED ONLY ONCE — save it immediately. The
"apiToken" field contains the persisted metadata (id, prefix, etc.).`,
		Example: `  # Plain create, no expiry:
  maxx-cli -o json token create --name "prod-cli"

  # Bound to a project, expiring in 30 days:
  maxx-cli token create --name release-bot --project-id 4 --expires-at 2026-06-29T00:00:00Z

  # Just the plaintext token, for piping:
  maxx-cli -o json token create --name scratch | jq -r .token`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if name == "" {
				return fmt.Errorf("--name is required")
			}
			req := api.CreateAPITokenRequest{
				Name:        name,
				Description: description,
				ProjectID:   projectID,
			}
			if expiresAt != "" {
				req.ExpiresAt = &expiresAt
			}
			if flagDryRun {
				return previewJSON(cmd.OutOrStdout(), "POST /api/admin/api-tokens", req)
			}
			client, _, err := authedClient()
			if err != nil {
				return err
			}
			result, err := client.CreateAPIToken(req)
			if err != nil {
				return handleAPIError(err)
			}
			return output.JSON(cmd.OutOrStdout(), result)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "display name (required)")
	cmd.Flags().StringVar(&description, "description", "", "free-form description")
	cmd.Flags().Uint64Var(&projectID, "project-id", 0, "associated project ID (0 = global)")
	cmd.Flags().StringVar(&expiresAt, "expires-at", "", "RFC3339 expiry time (optional)")
	return cmd
}

func newTokenUpdateCmd() *cobra.Command {
	var (
		name        string
		description string
		projectID   int64 // signed so we can detect "unset"
		isEnabled   string
		devMode     string
		expiresAt   string
	)
	cmd := &cobra.Command{
		Use:   "update ID [flags]",
		Short: "Update fields of an API token (only flags you pass are sent)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseUint(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid id %q", args[0])
			}
			var req api.UpdateAPITokenRequest
			if cmd.Flags().Changed("name") {
				req.Name = &name
			}
			if cmd.Flags().Changed("description") {
				req.Description = &description
			}
			if cmd.Flags().Changed("project-id") {
				if projectID < 0 {
					return fmt.Errorf("--project-id must be >= 0")
				}
				pid := uint64(projectID)
				req.ProjectID = &pid
			}
			if cmd.Flags().Changed("enabled") {
				b, err := strconv.ParseBool(isEnabled)
				if err != nil {
					return fmt.Errorf("--enabled: %w", err)
				}
				req.IsEnabled = &b
			}
			if cmd.Flags().Changed("dev-mode") {
				b, err := strconv.ParseBool(devMode)
				if err != nil {
					return fmt.Errorf("--dev-mode: %w", err)
				}
				req.DevMode = &b
			}
			if cmd.Flags().Changed("expires-at") {
				req.ExpiresAt = &expiresAt
			}
			if flagDryRun {
				return previewJSON(cmd.OutOrStdout(), fmt.Sprintf("PUT /api/admin/api-tokens/%d", id), req)
			}
			client, _, err := authedClient()
			if err != nil {
				return err
			}
			tk, err := client.UpdateAPIToken(id, req)
			if err != nil {
				return handleAPIError(err)
			}
			return output.JSON(cmd.OutOrStdout(), tk)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "")
	cmd.Flags().StringVar(&description, "description", "", "")
	cmd.Flags().Int64Var(&projectID, "project-id", 0, "")
	cmd.Flags().StringVar(&isEnabled, "enabled", "", "true|false")
	cmd.Flags().StringVar(&devMode, "dev-mode", "", "true|false")
	cmd.Flags().StringVar(&expiresAt, "expires-at", "", "RFC3339, or empty string to clear")
	return cmd
}

func newTokenDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete ID",
		Short: "Revoke (delete) an API token",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseUint(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid id %q", args[0])
			}
			if flagDryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "[dry-run] DELETE /api/admin/api-tokens/%d\n", id)
				return nil
			}
			if !confirm(fmt.Sprintf("Revoke API token %d?", id)) {
				return fmt.Errorf("aborted")
			}
			client, _, err := authedClient()
			if err != nil {
				return err
			}
			if err := client.DeleteAPIToken(id); err != nil {
				return handleAPIError(err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Revoked token %d.\n", id)
			return nil
		},
	}
}
