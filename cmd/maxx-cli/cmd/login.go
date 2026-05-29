package cmd

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/awsl-project/maxx/cmd/maxx-cli/internal/api"
	"github.com/awsl-project/maxx/cmd/maxx-cli/internal/cfg"
)

func newLoginCmd() *cobra.Command {
	var (
		serverURL string
		username  string
		password  string
		contextNm string
		insecure  bool
	)
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Log in to a maxx server and save a context",
		Long: `Log in with a username and password. The returned JWT is stored in the CLI
config under the named context (default: "default"). A password may be passed
via --password or piped on stdin; otherwise it is prompted for without echo.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			conf, err := cfg.Load()
			if err != nil {
				return err
			}

			if serverURL == "" {
				return errors.New("--server is required")
			}
			if username == "" {
				return errors.New("--username is required")
			}
			pw, err := resolvePassword(password)
			if err != nil {
				return err
			}

			ctxName := contextNm
			if ctxName == "" {
				ctxName = "default"
			}
			tmpCtx := &cfg.Context{
				Name:               ctxName,
				Server:             serverURL,
				InsecureSkipVerify: insecure,
			}
			client, err := api.NewFromContext(tmpCtx)
			if err != nil {
				return err
			}
			resp, err := client.Login(username, pw)
			if err != nil {
				return err
			}
			if !resp.Success || resp.Token == "" {
				return errors.New("login response missing token")
			}

			tmpCtx.Token = resp.Token
			tmpCtx.Username = username
			tmpCtx.ExpiresAt = api.JWTExpiry(resp.Token)

			conf.Upsert(tmpCtx)
			conf.CurrentContext = ctxName
			if err := cfg.Save(conf); err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(),
				"Logged in to %s as %s (tenant %s) — context %q saved.\n",
				serverURL, resp.User.Username, resp.User.TenantName, ctxName)
			if !tmpCtx.ExpiresAt.IsZero() {
				fmt.Fprintf(cmd.OutOrStdout(), "Token expires at %s.\n", tmpCtx.ExpiresAt.Format("2006-01-02 15:04:05"))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&serverURL, "server", "", "server base URL, e.g. http://localhost:9880")
	cmd.Flags().StringVarP(&username, "username", "u", "", "username")
	cmd.Flags().StringVarP(&password, "password", "p", "", "password (use '-' to read stdin)")
	cmd.Flags().StringVar(&contextNm, "context", "", "context name to save under (default: \"default\")")
	cmd.Flags().BoolVar(&insecure, "insecure-skip-verify", false, "skip TLS verification for this context")
	return cmd
}

func resolvePassword(flagValue string) (string, error) {
	if flagValue == "-" {
		buf := make([]byte, 4096)
		n, err := os.Stdin.Read(buf)
		if err != nil && n == 0 {
			return "", fmt.Errorf("read password from stdin: %w", err)
		}
		return strings.TrimRight(string(buf[:n]), "\r\n"), nil
	}
	if flagValue != "" {
		return flagValue, nil
	}
	if !term.IsTerminal(int(syscall.Stdin)) {
		return "", errors.New("no --password given and stdin is not a terminal")
	}
	fmt.Fprint(os.Stderr, "Password: ")
	pw, err := term.ReadPassword(int(syscall.Stdin))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", fmt.Errorf("read password: %w", err)
	}
	return string(pw), nil
}

func newLogoutCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Remove the saved token from the current (or named) context",
		RunE: func(cmd *cobra.Command, _ []string) error {
			conf, err := cfg.Load()
			if err != nil {
				return err
			}
			name := flagContextName
			if name == "" {
				name = conf.CurrentContext
			}
			if name == "" {
				return cfg.ErrNoContext
			}
			ctx := conf.FindContext(name)
			if ctx == nil {
				return fmt.Errorf("context %q not found", name)
			}
			ctx.Token = ""
			ctx.ExpiresAt = api.JWTExpiry("") // zero time
			if err := cfg.Save(conf); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Removed token from context %q.\n", name)
			return nil
		},
	}
}
