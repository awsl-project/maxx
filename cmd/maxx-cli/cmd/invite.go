package cmd

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/awsl-project/maxx/cmd/maxx-cli/internal/api"
	"github.com/awsl-project/maxx/cmd/maxx-cli/internal/output"
)

func newInviteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "invite",
		Aliases: []string{"invites", "invite-code", "invite-codes"},
		Short:   "Manage invite codes",
	}
	cmd.AddCommand(
		newInviteListCmd(),
		newInviteGetCmd(),
		newInviteCreateCmd(),
		newInviteUpdateCmd(),
		newInviteDeleteCmd(),
		newInviteUsagesCmd(),
	)
	return cmd
}

func newInviteListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List invite codes",
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, _, err := authedClient()
			if err != nil {
				return err
			}
			codes, err := client.ListInviteCodes()
			if err != nil {
				return handleAPIError(err)
			}
			return output.PrintOrJSON(cmd.OutOrStdout(), outputFormat(), codes, func() output.Table {
				t := output.Table{Headers: []string{"ID", "PREFIX", "STATUS", "USED/MAX", "EXPIRES", "NOTE"}}
				for _, c := range codes {
					used := fmt.Sprintf("%d/%d", c.UsedCount, c.MaxUses)
					t.Rows = append(t.Rows, []string{
						strconv.FormatUint(c.ID, 10),
						c.CodePrefix,
						string(c.Status),
						used,
						output.FormatTimePtr(c.ExpiresAt),
						output.Truncate(c.Note, 40),
					})
				}
				return t
			})
		},
	}
}

func newInviteGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get ID",
		Short: "Get one invite code",
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
			c, err := client.GetInviteCode(id)
			if err != nil {
				return handleAPIError(err)
			}
			return output.JSON(cmd.OutOrStdout(), c)
		},
	}
}

func newInviteCreateCmd() *cobra.Command {
	var (
		count     int
		maxUses   uint64
		expiresAt string
		note      string
	)
	cmd := &cobra.Command{
		Use:   "create [--count N] [--max-uses N] [--expires-at RFC3339] [--note ...]",
		Short: "Generate one or more invite codes (plain codes returned once)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if count <= 0 {
				count = 1
			}
			req := api.CreateInviteCodesRequest{
				Count: count,
				Note:  note,
			}
			if cmd.Flags().Changed("max-uses") {
				req.MaxUses = &maxUses
			}
			if expiresAt != "" {
				req.ExpiresAt = &expiresAt
			}
			if flagDryRun {
				return previewJSON(cmd.OutOrStdout(), "POST /api/admin/invite-codes", req)
			}
			client, _, err := authedClient()
			if err != nil {
				return err
			}
			result, err := client.CreateInviteCodes(req)
			if err != nil {
				return handleAPIError(err)
			}
			return output.JSON(cmd.OutOrStdout(), result)
		},
	}
	cmd.Flags().IntVar(&count, "count", 1, "")
	cmd.Flags().Uint64Var(&maxUses, "max-uses", 1, "")
	cmd.Flags().StringVar(&expiresAt, "expires-at", "", "RFC3339")
	cmd.Flags().StringVar(&note, "note", "", "")
	return cmd
}

func newInviteUpdateCmd() *cobra.Command {
	var (
		status    string
		maxUses   uint64
		expiresAt string
		note      string
	)
	cmd := &cobra.Command{
		Use:   "update ID [flags]",
		Short: "Update an invite code (only flags you pass are sent)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseUint(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid id %q", args[0])
			}
			var req api.UpdateInviteCodeRequest
			if cmd.Flags().Changed("status") {
				req.Status = &status
			}
			if cmd.Flags().Changed("max-uses") {
				req.MaxUses = &maxUses
			}
			if cmd.Flags().Changed("expires-at") {
				req.ExpiresAt = &expiresAt
			}
			if cmd.Flags().Changed("note") {
				req.Note = &note
			}
			if flagDryRun {
				return previewJSON(cmd.OutOrStdout(), fmt.Sprintf("PUT /api/admin/invite-codes/%d", id), req)
			}
			client, _, err := authedClient()
			if err != nil {
				return err
			}
			c, err := client.UpdateInviteCode(id, req)
			if err != nil {
				return handleAPIError(err)
			}
			return output.JSON(cmd.OutOrStdout(), c)
		},
	}
	cmd.Flags().StringVar(&status, "status", "", "active|disabled")
	cmd.Flags().Uint64Var(&maxUses, "max-uses", 0, "")
	cmd.Flags().StringVar(&expiresAt, "expires-at", "", "RFC3339, or empty to clear")
	cmd.Flags().StringVar(&note, "note", "", "")
	return cmd
}

func newInviteDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete ID",
		Short: "Delete an invite code",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseUint(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid id %q", args[0])
			}
			if flagDryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "[dry-run] DELETE /api/admin/invite-codes/%d\n", id)
				return nil
			}
			if !confirm(fmt.Sprintf("Delete invite code %d?", id)) {
				return fmt.Errorf("aborted")
			}
			client, _, err := authedClient()
			if err != nil {
				return err
			}
			if err := client.DeleteInviteCode(id); err != nil {
				return handleAPIError(err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Deleted invite code %d.\n", id)
			return nil
		},
	}
}

func newInviteUsagesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "usages ID",
		Short: "List usages of an invite code",
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
			usages, err := client.ListInviteCodeUsages(id)
			if err != nil {
				return handleAPIError(err)
			}
			return output.PrintOrJSON(cmd.OutOrStdout(), outputFormat(), usages, func() output.Table {
				t := output.Table{Headers: []string{"ID", "USED AT", "USERNAME", "IP", "RESULT", "REASON"}}
				for _, u := range usages {
					t.Rows = append(t.Rows, []string{
						strconv.FormatUint(u.ID, 10),
						output.FormatTime(u.UsedAt),
						u.Username,
						u.IP,
						u.Result,
						output.Truncate(u.Reason, 30),
					})
				}
				return t
			})
		},
	}
}
