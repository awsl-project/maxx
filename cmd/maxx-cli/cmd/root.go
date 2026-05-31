// Package cmd defines the maxx-cli cobra command tree.
package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/awsl-project/maxx/cmd/maxx-cli/internal/api"
	"github.com/awsl-project/maxx/cmd/maxx-cli/internal/cfg"
	"github.com/awsl-project/maxx/cmd/maxx-cli/internal/output"
	"github.com/awsl-project/maxx/internal/version"
)

// Global flags exposed on the root command.
var (
	flagContextName string
	flagServer      string
	flagOutput      string
	flagYes         bool
	flagDryRun      bool
)

// Execute is the entrypoint called from main.
func Execute() error {
	root := newRootCmd()
	return root.Execute()
}

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "maxx-cli",
		Short: "Configure a maxx server from the command line",
		Long: `Configure a maxx server from the command line — providers, API tokens,
routes, routing strategies, users, invite codes, and settings.`,
		Example:       rootExample,
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       version.Full(),
	}
	root.PersistentFlags().StringVar(&flagContextName, "context", "", "config context to use (default: current context)")
	root.PersistentFlags().StringVar(&flagServer, "server", "", "override server URL for this invocation")
	root.PersistentFlags().StringVarP(&flagOutput, "output", "o", "table", "output format: table | json")
	root.PersistentFlags().BoolVarP(&flagYes, "yes", "y", false, "skip confirmation for destructive operations")
	root.PersistentFlags().BoolVar(&flagDryRun, "dry-run", false, "print what would be sent without contacting the server")

	// Cobra groups (gh-style sectioning in `--help`).
	root.AddGroup(
		&cobra.Group{ID: groupAuth, Title: "Auth & contexts:"},
		&cobra.Group{ID: groupResources, Title: "Resource commands:"},
		&cobra.Group{ID: groupTopics, Title: "Help topics:"},
	)

	auth := []*cobra.Command{
		newLoginCmd(),
		newLogoutCmd(),
		newContextCmd(),
	}
	resources := []*cobra.Command{
		newProviderCmd(),
		newTokenCmd(),
		newRouteCmd(),
		newStrategyCmd(),
		newUserCmd(),
		newInviteCmd(),
		newSettingsCmd(),
	}
	for _, c := range auth {
		c.GroupID = groupAuth
		root.AddCommand(c)
	}
	for _, c := range resources {
		c.GroupID = groupResources
		root.AddCommand(c)
	}

	// Help topics. `reference` walks the now-fully-populated tree.
	root.AddCommand(
		topic("formatting", "Output formats, exit codes, --dry-run, and JSON tips", formattingTopicBody),
		topic("auth-config", "Login, contexts, JWT lifetime, and 401 handling", authTopicBody),
		newReferenceCmd(root),
	)

	return root
}

// outputFormat returns the parsed -o flag.
func outputFormat() output.Format {
	return output.Parse(flagOutput)
}

// resolveContext returns the active CLI context, honouring --context override
// and the persisted CurrentContext.
func resolveContext() (*cfg.Config, *cfg.Context, error) {
	conf, err := cfg.Load()
	if err != nil {
		return nil, nil, err
	}
	name := flagContextName
	if name == "" {
		name = conf.CurrentContext
	}
	if name == "" {
		return conf, nil, cfg.ErrNoContext
	}
	ctx := conf.FindContext(name)
	if ctx == nil {
		return conf, nil, fmt.Errorf("context %q not found", name)
	}
	if flagServer != "" {
		// Don't mutate the persisted context — clone for this invocation.
		clone := *ctx
		clone.Server = flagServer
		ctx = &clone
	}
	return conf, ctx, nil
}

// authedClient returns a Client configured for the current context. It warns
// to stderr when the JWT is within 24h of expiry.
func authedClient() (*api.Client, *cfg.Context, error) {
	_, ctx, err := resolveContext()
	if err != nil {
		return nil, nil, err
	}
	if ctx.Token == "" {
		return nil, ctx, fmt.Errorf("context %q has no token; run `maxx-cli login`", ctx.Name)
	}
	warnIfExpiringSoon(ctx)
	c, err := api.NewFromContext(ctx)
	if err != nil {
		return nil, ctx, err
	}
	return c, ctx, nil
}

func warnIfExpiringSoon(ctx *cfg.Context) {
	exp := ctx.ExpiresAt
	if exp.IsZero() {
		exp = api.JWTExpiry(ctx.Token)
	}
	if exp.IsZero() {
		return
	}
	remaining := time.Until(exp)
	if remaining <= 0 {
		fmt.Fprintf(os.Stderr, "[maxx-cli] token for context %q expired at %s — run `maxx-cli login` again\n",
			ctx.Name, exp.Format(time.RFC3339))
		return
	}
	if remaining < 24*time.Hour {
		fmt.Fprintf(os.Stderr, "[maxx-cli] warning: token for context %q expires in %s\n",
			ctx.Name, remaining.Round(time.Minute))
	}
}

// handleAPIError translates 401 into a friendly hint to re-login.
func handleAPIError(err error) error {
	if api.IsUnauthorized(err) {
		return fmt.Errorf("server returned 401 unauthorized; run `maxx-cli login` to refresh your token")
	}
	return err
}

// confirm prompts y/N unless --yes was given. Returns true if approved.
func confirm(prompt string) bool {
	if flagYes {
		return true
	}
	fmt.Fprintf(os.Stderr, "%s [y/N]: ", prompt)
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return false
	}
	line = strings.ToLower(strings.TrimSpace(line))
	return line == "y" || line == "yes"
}
