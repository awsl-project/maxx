package cmd

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
)

// Help topic IDs (cobra Group IDs).
const (
	groupResources = "resources"
	groupAuth      = "auth"
	groupTopics    = "topics"
)

// topic builds a help-topic command in the gh style: it has no real RunE,
// printing the topic body as both help output and command output so both
// `maxx-cli TOPIC` and `maxx-cli help TOPIC` work.
//
// Topics are intentionally NOT hidden — they show up under the "Help topics"
// group in `--help`, which is how an AI agent discovers `help reference`.
func topic(name, short, body string) *cobra.Command {
	c := &cobra.Command{
		Use:     name,
		Short:   short,
		Long:    body,
		GroupID: groupTopics,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := io.WriteString(cmd.OutOrStdout(), body)
			return err
		},
	}
	return c
}

// newReferenceCmd builds the "reference" topic by walking the cobra tree.
// The output never drifts from the live command set: if you add a command,
// it shows up here automatically.
//
// The body is materialised once at command-tree build time and stored on
// Long, so `maxx-cli help reference` (which renders Long) and
// `maxx-cli reference` (which writes Long verbatim) both produce the same
// output.
func newReferenceCmd(root *cobra.Command) *cobra.Command {
	body := buildReferenceBody(root)
	return topic("reference", "A comprehensive reference of all maxx-cli commands", body)
}

func buildReferenceBody(root *cobra.Command) string {
	var buf strings.Builder
	buf.WriteString("# maxx-cli reference\n\n")
	buf.WriteString("Every command, every flag. Auto-generated from the cobra tree.\n")
	buf.WriteString("For conventions (output format, exit codes, auth), see:\n")
	buf.WriteString("  maxx-cli help formatting\n")
	buf.WriteString("  maxx-cli help auth-config\n\n")
	for _, sub := range root.Commands() {
		if sub.Hidden || sub.Name() == "help" || sub.Name() == "completion" {
			continue
		}
		// Skip the topic commands themselves — they describe themselves
		// already and would just bloat the reference.
		if sub.GroupID == groupTopics {
			continue
		}
		writeRefSection(&buf, sub, 2)
	}
	return buf.String()
}

// writeRefSection recurses into a command, rendering a section per command
// node at the requested header depth.
func writeRefSection(w *strings.Builder, c *cobra.Command, depth int) {
	if c.Hidden {
		return
	}
	header := strings.Repeat("#", depth)
	fmt.Fprintf(w, "%s %s\n\n", header, c.CommandPath())
	if c.Short != "" {
		fmt.Fprintf(w, "%s\n\n", c.Short)
	}
	if usage := c.UseLine(); usage != "" && usage != c.CommandPath() {
		fmt.Fprintf(w, "    %s\n\n", usage)
	}
	// Local (non-inherited) flags. Inherited ones are documented under root.
	if f := c.LocalFlags(); f.HasAvailableFlags() {
		fmt.Fprintln(w, "Flags:")
		fmt.Fprintln(w, indent(f.FlagUsages(), "  "))
		fmt.Fprintln(w)
	}
	for _, sub := range c.Commands() {
		writeRefSection(w, sub, depth+1)
	}
}

func indent(s, prefix string) string {
	s = strings.TrimRight(s, "\n")
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		if line == "" {
			continue
		}
		lines[i] = prefix + line
	}
	return strings.Join(lines, "\n")
}

// formattingTopic documents output format, JSON, exit codes, and --dry-run.
const formattingTopicBody = `# Output, formatting, and exit codes

Output format
  -o, --output table   Human-readable table (default). Padding is whitespace,
                       not tabs. Empty results print "(no items)".
  -o, --output json    Machine-readable JSON. List endpoints return arrays;
                       get / create / update echo a single object.

  Scripts and AI agents should ALWAYS pass -o json — table output is for
  humans and is not a stable contract.

Dry run
  --dry-run            Preview a command without performing any side
                       effects. The CLI never contacts the server, never
                       writes the local config, and never deletes anything.
                       Each command prints either "[dry-run] METHOD /path"
                       followed by the JSON body it would send, or
                       "[dry-run] would <local-action>" for local-only
                       commands (e.g. context delete, logout).

  Useful for inspecting destructive verbs (delete, password reset, sticky
  on/off) before running them for real, and for sanity-checking the
  request shape against an API spec.

Confirmation prompts
  -y, --yes            Skip the y/N prompt on destructive verbs (delete
                       provider/token/route/strategy/user/invite/context,
                       and settings delete).

  Without -y, the CLI prompts on stderr and aborts with exit code 1 if the
  answer is not "y" or "yes". In CI, pass -y or pipe "y" via stdin.

Exit codes
  0   command succeeded
  1   command failed (server error, validation error, aborted confirm)

  All errors are printed to stderr as "Error: <message>". 401 specifically
  reads "server returned 401 unauthorized; run maxx-cli login to refresh
  your token" — treat that as "re-auth required" and re-run login.

JSON tips
  Pipe through jq for extraction:

      maxx-cli -o json token create --name bot | jq -r .token

  Field names match the server's Go struct json tags exactly. The wire
  format is stable; the table format is not. Reference the source at
  internal/domain/model.go if you need a schema check.

Plaintext-only-once values
  Two values are returned ONLY at creation time and cannot be retrieved
  later:
    - API token plaintext   (token create   → "token" field)
    - Invite code plaintext (invite create  → "items[].code" field)
  Capture them when you create them.
`

// authTopicBody documents login, contexts, JWT, and 401 handling.
const authTopicBody = `# Auth, contexts, and JWT handling

Login
  maxx-cli login --server URL --username NAME [--password PW]

  If --password is omitted, the CLI prompts (with echo disabled) when stdin
  is a terminal, or reads stdin when piped. "-" forces stdin read.

  The server enforces a 7-day JWT lifetime. The CLI parses the exp claim
  locally and warns on stderr when <24h remain. There is no refresh
  endpoint — on 401, run login again.

Config file
  Location is resolved in this order:
    1. MAXX_CLI_CONFIG env var (full path)
    2. $XDG_CONFIG_HOME/maxx-cli/config.yaml
    3. $HOME/.config/maxx-cli/config.yaml

  The file is written with mode 0600. The directory is created 0700.
  Tokens are stored in cleartext (it is your local file); never check
  this file into version control.

  On first run, if the legacy path $HOME/.config/maxx/cli.yaml exists it
  is migrated once to the new location.

Contexts (multi-server)
  A context is a (name, server URL, token, username) tuple. One context is
  the "current" one. Use --context NAME on any command to operate on a
  non-current context for that invocation.

  maxx-cli context list
  maxx-cli context current
  maxx-cli context use NAME
  maxx-cli context delete NAME    # prompts; --yes to skip
  maxx-cli login --context staging --server https://maxx.staging --username admin

Per-invocation overrides
  --server URL     pretend the current context points at this URL instead
                   (does not persist; useful for one-off curls)
  --context NAME   pick a different saved context for one invocation

401 handling
  When the server returns 401, the CLI prints:
    Error: server returned 401 unauthorized; run maxx-cli login to refresh your token

  This is your signal to re-run login. The CLI never auto-refreshes (it
  does not store your password).
`

// rootExample is the EXAMPLES block printed in `maxx-cli --help`.
const rootExample = `  # Log in and inspect the current context
  maxx-cli login --server http://localhost:9880 --username admin
  maxx-cli context current

  # JSON output, suitable for agents and scripts
  maxx-cli -o json provider list

  # Create a route and bump its weight for weighted_random
  maxx-cli route create -f route.json
  maxx-cli route set-weight 7 5

  # Enable sticky session affinity on a routing strategy
  maxx-cli strategy sticky 1 on --scope conversation --ttl 1800

  # Preview a destructive change without sending it
  maxx-cli --dry-run provider delete 42`
