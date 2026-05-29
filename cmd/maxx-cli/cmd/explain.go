package cmd

import (
	"io"

	"github.com/spf13/cobra"
)

// explainText is the single-shot, agent-friendly briefing. Keep it under
// ~250 lines so an LLM can ingest it in one turn.
const explainText = `# maxx-cli — agent briefing

maxx-cli is a command-line client for the maxx server's admin HTTP API.
Use it to script everything the web UI does: providers, API tokens,
routes (with weights), routing strategies (with sticky session affinity),
users, invite codes, and system settings.

This briefing is intentionally exhaustive — read it once, then act.
For an interactive human reading, prefer "maxx-cli <command> --help".


## 1. One-time setup

    maxx-cli login --server URL --username NAME [--password PASSWORD]

  - URL is the maxx HTTP base, e.g. http://localhost:9880.
  - If --password is omitted, the CLI prompts (terminal) or reads stdin
    when piped. "-" forces stdin read.
  - A JWT is stored at $XDG_CONFIG_HOME/maxx-cli/config.yaml (mode 0600)
    under context "default". JWT lifetime is 7 days; the CLI warns when
    <24h remain and tells you to re-login on 401.

Multiple servers? Pass --context NAME to login, then
"maxx-cli context use NAME" to switch the default.


## 2. Global flags (work on every subcommand)

  --context NAME      operate on a non-default context
  --server URL        override the server URL for one invocation
  -o, --output FMT    "table" (default) or "json"
  -y, --yes           skip y/N confirmation on destructive verbs
  --dry-run           print what would be sent, do not contact the server

JSON is the format an agent should use:

    maxx-cli -o json provider list


## 3. Command tree (one line per leaf)

  login                            exchange username+password for a JWT
  logout                           clear the saved JWT
  context list|current|use|delete  manage saved server contexts

  provider list                    list providers
  provider get ID                  one provider (JSON)
  provider create -f FILE|-        create from JSON
  provider update ID -f FILE|-     replace from JSON
  provider delete ID               delete (confirms)
  provider export [--out FILE]     dump all providers as JSON
  provider import -f FILE|-        bulk import (accepts a single object
                                   OR an array — normalised internally)

  token list                       list API tokens
  token get ID
  token create --name N            create; plaintext token returned ONCE
  token update ID [flags]          partial update; only flags you pass are sent
  token delete ID                  revoke (confirms)

  route list                       list routes
  route get ID
  route create -f FILE|-           full create
  route update ID [flags]          partial PUT; flags: --enabled, --native,
                                   --project-id, --client-type, --provider-id,
                                   --position, --weight, --retry-config-id
  route set-weight ID WEIGHT       shortcut for --weight WEIGHT (>=1)
  route delete ID                  confirms

  strategy list                    list routing strategies
  strategy get ID
  strategy create --type T         T = weighted_random | priority
  strategy update ID -f FILE|-     replace from JSON
  strategy sticky ID on|off        toggle session-affinity sticky
                                   on: --scope token|conversation, --ttl SECS
  strategy delete ID               confirms

  user list|get|update|delete      user CRUD
  user create --username U         password prompts if omitted
  user password ID                 reset password
  user approve ID                  approve a pending registration

  invite list|get|delete           invite-code admin
  invite create --count N          codes returned ONCE; --max-uses, --note,
                                   --expires-at (RFC3339)
  invite update ID [flags]         --status active|disabled, --max-uses,
                                   --expires-at, --note
  invite usages ID                 list redemptions

  settings list|get KEY|delete KEY system settings
  settings set KEY VALUE           upsert one


## 4. Output, errors, exit codes

  - Default (-o table) is for humans; columns are 2-space padded ASCII.
  - With -o json, list endpoints return JSON arrays; "get" returns one
    object; create/update echo the persisted resource.
  - Errors print to stderr: "Error: <message>"; exit code is non-zero.
  - 401 specifically: "server returned 401 unauthorized; run maxx-cli
    login to refresh your token". Treat that as "re-auth required".
  - --dry-run prints "[dry-run] METHOD /path" and the JSON body that
    WOULD be sent. Safe to use for inspection.


## 5. Worked examples (paste these to learn the shape)

Log in and verify:

    maxx-cli login --server http://localhost:9880 --username admin --password secret
    maxx-cli context current
    maxx-cli -o json provider list

Create a provider from stdin and capture its ID:

    cat <<'JSON' | maxx-cli -o json provider create -f -
    {"type":"custom","name":"Anthropic-Prod","config":{"baseUrl":"https://api.anthropic.com","apiKey":"sk-..."},"supportedClientTypes":["claude"]}
    JSON
    # Parse the "id" field of the returned JSON.

Issue an API token (the plaintext is in "token" — show ONCE):

    maxx-cli -o json token create --name prod-cli

Bump a route's weight (for weighted_random load balancing):

    maxx-cli route set-weight 7 5

Enable sticky session affinity on a routing strategy:

    maxx-cli strategy sticky 1 on --scope conversation --ttl 1800

Preview a destructive change without sending it:

    maxx-cli --dry-run provider delete 42
    maxx-cli --dry-run token update 3 --enabled false

Bulk export then re-import providers:

    maxx-cli provider export --out providers.json
    cat providers.json | maxx-cli provider import -f -


## 6. Tips when scripting

  - Always pass -o json from scripts; never parse the table form.
  - Pass -y on delete verbs so they do not block on confirmation.
  - Use --dry-run as a "what would this do" probe before destructive
    runs (it prints the exact request body).
  - Sticky is a flag of RoutingStrategy.Config, not its own resource;
    use "strategy sticky" rather than editing JSON by hand.
  - For partial updates (route update, token update, invite update,
    user update) only the flags you actually pass are included in
    the request. Unset flags mean "leave alone".
  - Plaintext API tokens and invite codes are returned ONLY on
    creation. Save them when you create them.
`

func newExplainCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "explain",
		Short: "Print an agent-friendly briefing of the whole CLI",
		Long: `Print a single-shot reference covering auth, the full command tree,
output conventions, error semantics, and worked examples. Intended for
LLM agents that should learn maxx-cli without crawling --help.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := io.WriteString(cmd.OutOrStdout(), explainText)
			return err
		},
	}
}

