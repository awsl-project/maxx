package cmd

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/awsl-project/maxx/cmd/maxx-cli/internal/output"
	"github.com/awsl-project/maxx/internal/domain"
)

func newRouteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "route",
		Aliases: []string{"routes"},
		Short:   "Manage routes",
	}
	cmd.AddCommand(
		newRouteListCmd(),
		newRouteGetCmd(),
		newRouteCreateCmd(),
		newRouteUpdateCmd(),
		newRouteSetWeightCmd(),
		newRouteDeleteCmd(),
	)
	return cmd
}

func newRouteListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List routes",
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, _, err := authedClient()
			if err != nil {
				return err
			}
			routes, err := client.ListRoutes()
			if err != nil {
				return handleAPIError(err)
			}
			return output.PrintOrJSON(cmd.OutOrStdout(), outputFormat(), routes, func() output.Table {
				t := output.Table{Headers: []string{"ID", "ENABLED", "NATIVE", "PROJECT", "CLIENT", "PROVIDER", "POS", "WEIGHT", "RETRY"}}
				for _, r := range routes {
					t.Rows = append(t.Rows, []string{
						strconv.FormatUint(r.ID, 10),
						output.FormatBool(r.IsEnabled),
						output.FormatBool(r.IsNative),
						strconv.FormatUint(r.ProjectID, 10),
						string(r.ClientType),
						strconv.FormatUint(r.ProviderID, 10),
						strconv.Itoa(r.Position),
						strconv.Itoa(r.Weight),
						strconv.FormatUint(r.RetryConfigID, 10),
					})
				}
				return t
			})
		},
	}
}

func newRouteGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get ID",
		Short: "Get one route",
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
			r, err := client.GetRoute(id)
			if err != nil {
				return handleAPIError(err)
			}
			return output.JSON(cmd.OutOrStdout(), r)
		},
	}
}

func newRouteCreateCmd() *cobra.Command {
	var fromFile string
	cmd := &cobra.Command{
		Use:   "create -f route.json",
		Short: "Create a route from a JSON file or stdin",
		RunE: func(cmd *cobra.Command, _ []string) error {
			data, err := readJSONInput(fromFile)
			if err != nil {
				return err
			}
			var r domain.Route
			if err := json.Unmarshal(data, &r); err != nil {
				return fmt.Errorf("parse route JSON: %w", err)
			}
			if flagDryRun {
				return previewJSON(cmd.OutOrStdout(), "POST /api/admin/routes", &r)
			}
			client, _, err := authedClient()
			if err != nil {
				return err
			}
			created, err := client.CreateRoute(&r)
			if err != nil {
				return handleAPIError(err)
			}
			return output.JSON(cmd.OutOrStdout(), created)
		},
	}
	cmd.Flags().StringVarP(&fromFile, "file", "f", "-", "JSON file or '-' for stdin")
	return cmd
}

func newRouteUpdateCmd() *cobra.Command {
	var (
		fromFile      string
		isEnabled     string
		isNative      string
		projectID     int64
		clientType    string
		providerID    int64
		position      int
		weight        int
		retryConfigID int64
	)
	cmd := &cobra.Command{
		Use:   "update ID [flags]",
		Short: "Patch a route — only flags you pass are sent",
		Long: `Update a route via partial PUT. Only the flags you actually pass on the
command line are included in the request. Pass -f to send a full JSON patch
instead, which overrides any individual flags.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseUint(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid id %q", args[0])
			}
			var patch map[string]any
			if fromFile != "" {
				data, err := readJSONInput(fromFile)
				if err != nil {
					return err
				}
				if err := json.Unmarshal(data, &patch); err != nil {
					return fmt.Errorf("parse JSON patch: %w", err)
				}
			} else {
				patch = map[string]any{}
				if cmd.Flags().Changed("enabled") {
					b, err := strconv.ParseBool(isEnabled)
					if err != nil {
						return fmt.Errorf("--enabled: %w", err)
					}
					patch["isEnabled"] = b
				}
				if cmd.Flags().Changed("native") {
					b, err := strconv.ParseBool(isNative)
					if err != nil {
						return fmt.Errorf("--native: %w", err)
					}
					patch["isNative"] = b
				}
				if cmd.Flags().Changed("project-id") {
					if projectID < 0 {
						return fmt.Errorf("--project-id must be >= 0")
					}
					patch["projectID"] = uint64(projectID)
				}
				if cmd.Flags().Changed("client-type") {
					patch["clientType"] = clientType
				}
				if cmd.Flags().Changed("provider-id") {
					if providerID < 0 {
						return fmt.Errorf("--provider-id must be >= 0")
					}
					patch["providerID"] = uint64(providerID)
				}
				if cmd.Flags().Changed("position") {
					patch["position"] = position
				}
				if cmd.Flags().Changed("weight") {
					if weight < 1 {
						return fmt.Errorf("--weight must be >= 1")
					}
					patch["weight"] = weight
				}
				if cmd.Flags().Changed("retry-config-id") {
					if retryConfigID < 0 {
						return fmt.Errorf("--retry-config-id must be >= 0")
					}
					patch["retryConfigID"] = uint64(retryConfigID)
				}
				if len(patch) == 0 {
					return fmt.Errorf("no fields to update; pass one of --enabled/--native/--project-id/--client-type/--provider-id/--position/--weight/--retry-config-id, or use -f")
				}
			}
			if flagDryRun {
				return previewJSON(cmd.OutOrStdout(), fmt.Sprintf("PUT /api/admin/routes/%d", id), patch)
			}
			client, _, err := authedClient()
			if err != nil {
				return err
			}
			updated, err := client.UpdateRoute(id, patch)
			if err != nil {
				return handleAPIError(err)
			}
			return output.JSON(cmd.OutOrStdout(), updated)
		},
	}
	cmd.Flags().StringVarP(&fromFile, "file", "f", "", "JSON patch file (overrides individual flags when set)")
	cmd.Flags().StringVar(&isEnabled, "enabled", "", "true|false")
	cmd.Flags().StringVar(&isNative, "native", "", "true|false")
	cmd.Flags().Int64Var(&projectID, "project-id", 0, "")
	cmd.Flags().StringVar(&clientType, "client-type", "", "")
	cmd.Flags().Int64Var(&providerID, "provider-id", 0, "")
	cmd.Flags().IntVar(&position, "position", 0, "")
	cmd.Flags().IntVar(&weight, "weight", 1, "weighted-random weight (>=1)")
	cmd.Flags().Int64Var(&retryConfigID, "retry-config-id", 0, "")
	return cmd
}

func newRouteSetWeightCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "set-weight ID WEIGHT",
		Short: "Shortcut: update only the weight of a route",
		Long: `Update only the Weight field of a route. Weight is used by the
weighted_random routing strategy; higher value = higher chance of
being picked. Must be >= 1. Equivalent to "route update ID --weight N".`,
		Example: `  # Make route 7 three times as likely as a weight-1 sibling:
  maxx-cli route set-weight 7 3

  # Equal weighting (the default — 1 on every route):
  maxx-cli route set-weight 7 1`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseUint(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid id %q", args[0])
			}
			w, err := strconv.Atoi(args[1])
			if err != nil || w < 1 {
				return fmt.Errorf("weight must be a positive integer, got %q", args[1])
			}
			patch := map[string]any{"weight": w}
			if flagDryRun {
				return previewJSON(cmd.OutOrStdout(), fmt.Sprintf("PUT /api/admin/routes/%d", id), patch)
			}
			client, _, err := authedClient()
			if err != nil {
				return err
			}
			updated, err := client.UpdateRoute(id, patch)
			if err != nil {
				return handleAPIError(err)
			}
			return output.JSON(cmd.OutOrStdout(), updated)
		},
	}
}

func newRouteDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete ID",
		Short: "Delete a route",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseUint(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid id %q", args[0])
			}
			if flagDryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "[dry-run] DELETE /api/admin/routes/%d\n", id)
				return nil
			}
			if !confirm(fmt.Sprintf("Delete route %d?", id)) {
				return fmt.Errorf("aborted")
			}
			client, _, err := authedClient()
			if err != nil {
				return err
			}
			if err := client.DeleteRoute(id); err != nil {
				return handleAPIError(err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Deleted route %d.\n", id)
			return nil
		},
	}
}
