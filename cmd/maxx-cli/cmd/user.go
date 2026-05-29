package cmd

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/awsl-project/maxx/cmd/maxx-cli/internal/api"
	"github.com/awsl-project/maxx/cmd/maxx-cli/internal/output"
)

func newUserCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "user",
		Aliases: []string{"users"},
		Short:   "Manage users",
	}
	cmd.AddCommand(
		newUserListCmd(),
		newUserGetCmd(),
		newUserCreateCmd(),
		newUserUpdateCmd(),
		newUserPasswordCmd(),
		newUserApproveCmd(),
		newUserDeleteCmd(),
	)
	return cmd
}

func newUserListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List users",
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, _, err := authedClient()
			if err != nil {
				return err
			}
			users, err := client.ListUsers()
			if err != nil {
				return handleAPIError(err)
			}
			return output.PrintOrJSON(cmd.OutOrStdout(), outputFormat(), users, func() output.Table {
				t := output.Table{Headers: []string{"ID", "USERNAME", "ROLE", "STATUS", "LAST LOGIN"}}
				for _, u := range users {
					t.Rows = append(t.Rows, []string{
						strconv.FormatUint(u.ID, 10),
						u.Username,
						string(u.Role),
						string(u.Status),
						output.FormatTimePtr(u.LastLoginAt),
					})
				}
				return t
			})
		},
	}
}

func newUserGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get ID",
		Short: "Get one user",
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
			u, err := client.GetUser(id)
			if err != nil {
				return handleAPIError(err)
			}
			return output.JSON(cmd.OutOrStdout(), u)
		},
	}
}

func newUserCreateCmd() *cobra.Command {
	var (
		username string
		password string
		role     string
	)
	cmd := &cobra.Command{
		Use:   "create --username U [--password P] [--role admin|member]",
		Short: "Create a user (password read interactively if --password is omitted)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if username == "" {
				return fmt.Errorf("--username is required")
			}
			pw, err := resolvePassword(password)
			if err != nil {
				return err
			}
			req := api.CreateUserRequest{
				Username: username,
				Password: pw,
				Role:     role,
			}
			if flagDryRun {
				safe := req
				safe.Password = "<redacted>"
				return previewJSON(cmd.OutOrStdout(), "POST /api/admin/users", safe)
			}
			client, _, err := authedClient()
			if err != nil {
				return err
			}
			u, err := client.CreateUser(req)
			if err != nil {
				return handleAPIError(err)
			}
			return output.JSON(cmd.OutOrStdout(), u)
		},
	}
	cmd.Flags().StringVar(&username, "username", "", "")
	cmd.Flags().StringVar(&password, "password", "", "password ('-' to read stdin; prompt if omitted)")
	cmd.Flags().StringVar(&role, "role", "", "admin|member (server default: member)")
	return cmd
}

func newUserUpdateCmd() *cobra.Command {
	var (
		username string
		role     string
		status   string
	)
	cmd := &cobra.Command{
		Use:   "update ID [--username U] [--role admin|member] [--status pending|active]",
		Short: "Update fields of a user",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseUint(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid id %q", args[0])
			}
			req := api.UpdateUserRequest{
				Username: username,
				Role:     role,
				Status:   status,
			}
			if flagDryRun {
				return previewJSON(cmd.OutOrStdout(), fmt.Sprintf("PUT /api/admin/users/%d", id), req)
			}
			client, _, err := authedClient()
			if err != nil {
				return err
			}
			u, err := client.UpdateUser(id, req)
			if err != nil {
				return handleAPIError(err)
			}
			return output.JSON(cmd.OutOrStdout(), u)
		},
	}
	cmd.Flags().StringVar(&username, "username", "", "")
	cmd.Flags().StringVar(&role, "role", "", "admin|member")
	cmd.Flags().StringVar(&status, "status", "", "pending|active")
	return cmd
}

func newUserPasswordCmd() *cobra.Command {
	var password string
	cmd := &cobra.Command{
		Use:   "password ID [--password P]",
		Short: "Reset a user's password (prompts if --password is omitted)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseUint(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid id %q", args[0])
			}
			pw, err := resolvePassword(password)
			if err != nil {
				return err
			}
			if flagDryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "[dry-run] PUT /api/admin/users/%d/password (<redacted>)\n", id)
				return nil
			}
			client, _, err := authedClient()
			if err != nil {
				return err
			}
			if err := client.UpdateUserPassword(id, pw); err != nil {
				return handleAPIError(err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Password updated for user %d.\n", id)
			return nil
		},
	}
	cmd.Flags().StringVar(&password, "password", "", "new password ('-' to read stdin; prompt if omitted)")
	return cmd
}

func newUserApproveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "approve ID",
		Short: "Approve a pending user",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseUint(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid id %q", args[0])
			}
			if flagDryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "[dry-run] PUT /api/admin/users/%d/approve\n", id)
				return nil
			}
			client, _, err := authedClient()
			if err != nil {
				return err
			}
			u, err := client.ApproveUser(id)
			if err != nil {
				return handleAPIError(err)
			}
			return output.JSON(cmd.OutOrStdout(), u)
		},
	}
}

func newUserDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete ID",
		Short: "Delete a user",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseUint(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid id %q", args[0])
			}
			if flagDryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "[dry-run] DELETE /api/admin/users/%d\n", id)
				return nil
			}
			if !confirm(fmt.Sprintf("Delete user %d?", id)) {
				return fmt.Errorf("aborted")
			}
			client, _, err := authedClient()
			if err != nil {
				return err
			}
			if err := client.DeleteUser(id); err != nil {
				return handleAPIError(err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Deleted user %d.\n", id)
			return nil
		},
	}
}
