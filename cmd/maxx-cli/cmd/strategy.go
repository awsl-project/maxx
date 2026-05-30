package cmd

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/awsl-project/maxx/cmd/maxx-cli/internal/output"
	"github.com/awsl-project/maxx/internal/domain"
)

func newStrategyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "strategy",
		Aliases: []string{"strategies", "routing-strategy", "routing-strategies"},
		Short:   "Manage routing strategies",
	}
	cmd.AddCommand(
		newStrategyListCmd(),
		newStrategyGetCmd(),
		newStrategyCreateCmd(),
		newStrategyUpdateCmd(),
		newStrategyStickyCmd(),
		newStrategyDeleteCmd(),
	)
	return cmd
}

func newStrategyListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List routing strategies",
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, _, err := authedClient()
			if err != nil {
				return err
			}
			strategies, err := client.ListRoutingStrategies()
			if err != nil {
				return handleAPIError(err)
			}
			return output.PrintOrJSON(cmd.OutOrStdout(), outputFormat(), strategies, func() output.Table {
				t := output.Table{Headers: []string{"ID", "PROJECT", "TYPE", "STICKY", "SCOPE", "TTL"}}
				for _, s := range strategies {
					sticky := "N"
					scope := "-"
					ttl := "-"
					if s.Config != nil && s.Config.StickyEnabled {
						sticky = "Y"
						scope = string(s.Config.StickyScope)
						if scope == "" {
							scope = "token"
						}
						if s.Config.StickyTTLSeconds > 0 {
							ttl = strconv.FormatInt(s.Config.StickyTTLSeconds, 10) + "s"
						}
					}
					t.Rows = append(t.Rows, []string{
						strconv.FormatUint(s.ID, 10),
						strconv.FormatUint(s.ProjectID, 10),
						string(s.Type),
						sticky,
						scope,
						ttl,
					})
				}
				return t
			})
		},
	}
}

func newStrategyGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get ID",
		Short: "Get one routing strategy",
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
			s, err := client.GetRoutingStrategy(id)
			if err != nil {
				return handleAPIError(err)
			}
			return output.JSON(cmd.OutOrStdout(), s)
		},
	}
}

func newStrategyCreateCmd() *cobra.Command {
	var (
		fromFile  string
		projectID uint64
		stratType string
	)
	cmd := &cobra.Command{
		Use:   "create [--type weighted_random|priority] [--project-id N] [-f file.json]",
		Short: "Create a routing strategy",
		RunE: func(cmd *cobra.Command, _ []string) error {
			var s domain.RoutingStrategy
			if fromFile != "" {
				data, err := readJSONInput(fromFile)
				if err != nil {
					return err
				}
				if err := json.Unmarshal(data, &s); err != nil {
					return fmt.Errorf("parse strategy JSON: %w", err)
				}
			} else {
				if stratType == "" {
					return fmt.Errorf("--type is required (weighted_random or priority) when -f is not used")
				}
				s.Type = domain.RoutingStrategyType(stratType)
				s.ProjectID = projectID
			}
			if flagDryRun {
				return previewJSON(cmd.OutOrStdout(), "POST /api/admin/routing-strategies", &s)
			}
			client, _, err := authedClient()
			if err != nil {
				return err
			}
			created, err := client.CreateRoutingStrategy(&s)
			if err != nil {
				return handleAPIError(err)
			}
			return output.JSON(cmd.OutOrStdout(), created)
		},
	}
	cmd.Flags().StringVarP(&fromFile, "file", "f", "", "JSON file (overrides --type/--project-id)")
	cmd.Flags().Uint64Var(&projectID, "project-id", 0, "project ID (0 = global)")
	cmd.Flags().StringVar(&stratType, "type", "", "weighted_random | priority")
	return cmd
}

func newStrategyUpdateCmd() *cobra.Command {
	var fromFile string
	cmd := &cobra.Command{
		Use:   "update ID -f strategy.json",
		Short: "Replace a routing strategy with the JSON in a file or stdin",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseUint(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid id %q", args[0])
			}
			if fromFile == "" {
				return fmt.Errorf("--file is required; pass a path or '-' to read JSON from stdin")
			}
			data, err := readJSONInput(fromFile)
			if err != nil {
				return err
			}
			var s domain.RoutingStrategy
			if err := json.Unmarshal(data, &s); err != nil {
				return fmt.Errorf("parse strategy JSON: %w", err)
			}
			if flagDryRun {
				return previewJSON(cmd.OutOrStdout(), fmt.Sprintf("PUT /api/admin/routing-strategies/%d", id), &s)
			}
			client, _, err := authedClient()
			if err != nil {
				return err
			}
			updated, err := client.UpdateRoutingStrategy(id, &s)
			if err != nil {
				return handleAPIError(err)
			}
			return output.JSON(cmd.OutOrStdout(), updated)
		},
	}
	// Don't default to "-": that would silently block forever on a TTY if the
	// user forgot the flag. Require the flag explicitly.
	cmd.Flags().StringVarP(&fromFile, "file", "f", "", "JSON file path, or '-' to read from stdin (required)")
	return cmd
}

func newStrategyStickyCmd() *cobra.Command {
	var (
		scope   string
		ttlSecs int64
	)
	cmd := &cobra.Command{
		Use:   "sticky ID on|off",
		Short: "Toggle session-affinity sticky on a weighted_random strategy",
		Long: `Convenience for the common case of flipping sticky on or off. Fetches the
existing strategy, modifies the Config.Sticky* fields, and PUTs it back.

Sticky only applies to the weighted_random strategy (the priority
strategy ignores it). With sticky on, the same (APIToken[, SessionID])
will be routed to the provider that previously served it, while it is
still healthy, to maximize upstream prompt-cache hits.

Scopes:
  token         every request from the same API token sticks (coarse,
                higher hit rate, default)
  conversation  (API token, SessionID) sticks (fine, more sticky keys)

TTL is the lifetime of a sticky binding in seconds (server default 1800).`,
		Example: `  # Turn sticky on with sensible defaults:
  maxx-cli strategy sticky 1 on

  # Per-conversation sticky with a 30-minute binding:
  maxx-cli strategy sticky 1 on --scope conversation --ttl 1800

  # Disable sticky:
  maxx-cli strategy sticky 1 off`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseUint(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid id %q", args[0])
			}
			mode := args[1]
			if mode != "on" && mode != "off" {
				return fmt.Errorf("expected on|off, got %q", mode)
			}
			// Pre-validate flag values so dry-run reports the same shape
			// of error a real run would, and so the dry-run preview below
			// reflects the actual intended state change.
			if mode == "on" && scope != "" && scope != "token" && scope != "conversation" {
				return fmt.Errorf("--scope must be 'token' or 'conversation'")
			}
			if mode == "on" && cmd.Flags().Changed("ttl") && ttlSecs < 0 {
				return fmt.Errorf("--ttl must be >= 0")
			}
			// --dry-run: never contact the server (sticky's real path needs
			// a GET to build the PUT body, but the contract is "no server
			// contact"). Print the intent instead — the user already knows
			// what fields they're toggling.
			if flagDryRun {
				switch mode {
				case "on":
					line := fmt.Sprintf("[dry-run] would toggle sticky=on for routing strategy %d", id)
					if scope != "" {
						line += fmt.Sprintf(" (scope=%s)", scope)
					}
					if cmd.Flags().Changed("ttl") {
						line += fmt.Sprintf(" (ttlSeconds=%d)", ttlSecs)
					}
					fmt.Fprintln(cmd.OutOrStdout(), line)
				case "off":
					fmt.Fprintf(cmd.OutOrStdout(), "[dry-run] would toggle sticky=off for routing strategy %d\n", id)
				}
				return nil
			}
			client, _, err := authedClient()
			if err != nil {
				return err
			}
			s, err := client.GetRoutingStrategy(id)
			if err != nil {
				return handleAPIError(err)
			}
			if s.Config == nil {
				s.Config = &domain.RoutingStrategyConfig{}
			}
			if mode == "on" {
				s.Config.StickyEnabled = true
				if scope != "" {
					s.Config.StickyScope = domain.RoutingStickyScope(scope)
				}
				if cmd.Flags().Changed("ttl") {
					s.Config.StickyTTLSeconds = ttlSecs
				}
			} else {
				s.Config.StickyEnabled = false
			}
			updated, err := client.UpdateRoutingStrategy(id, s)
			if err != nil {
				return handleAPIError(err)
			}
			return output.JSON(cmd.OutOrStdout(), updated)
		},
	}
	cmd.Flags().StringVar(&scope, "scope", "", "token | conversation (default: token)")
	cmd.Flags().Int64Var(&ttlSecs, "ttl", 0, "sticky TTL seconds (default: 1800 server-side)")
	return cmd
}

func newStrategyDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete ID",
		Short: "Delete a routing strategy",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseUint(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid id %q", args[0])
			}
			if flagDryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "[dry-run] DELETE /api/admin/routing-strategies/%d\n", id)
				return nil
			}
			if !confirm(fmt.Sprintf("Delete routing strategy %d?", id)) {
				return fmt.Errorf("aborted")
			}
			client, _, err := authedClient()
			if err != nil {
				return err
			}
			if err := client.DeleteRoutingStrategy(id); err != nil {
				return handleAPIError(err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Deleted routing strategy %d.\n", id)
			return nil
		},
	}
}
