package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/awsl-project/maxx/cmd/maxx-cli/internal/output"
	"github.com/awsl-project/maxx/internal/domain"
)

func newProviderCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "provider",
		Aliases: []string{"providers"},
		Short:   "Manage providers",
	}
	cmd.AddCommand(
		newProviderListCmd(),
		newProviderGetCmd(),
		newProviderCreateCmd(),
		newProviderUpdateCmd(),
		newProviderDeleteCmd(),
		newProviderExportCmd(),
		newProviderImportCmd(),
	)
	return cmd
}

func newProviderListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List providers",
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, _, err := authedClient()
			if err != nil {
				return err
			}
			providers, err := client.ListProviders()
			if err != nil {
				return handleAPIError(err)
			}
			return output.PrintOrJSON(cmd.OutOrStdout(), outputFormat(), providers, func() output.Table {
				t := output.Table{Headers: []string{"ID", "TYPE", "NAME", "CLIENTS", "MODELS", "EXCLUDE"}}
				for _, p := range providers {
					t.Rows = append(t.Rows, []string{
						strconv.FormatUint(p.ID, 10),
						p.Type,
						output.Truncate(p.Name, 40),
						clientTypesString(p.SupportedClientTypes),
						strconv.Itoa(len(p.SupportModels)),
						output.FormatBool(p.ExcludeFromExport),
					})
				}
				return t
			})
		},
	}
}

func clientTypesString(types []domain.ClientType) string {
	if len(types) == 0 {
		return "-"
	}
	out := make([]byte, 0, len(types)*8)
	for i, t := range types {
		if i > 0 {
			out = append(out, ',')
		}
		out = append(out, t...)
	}
	return string(out)
}

func newProviderGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get ID",
		Short: "Get one provider by ID",
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
			p, err := client.GetProvider(id)
			if err != nil {
				return handleAPIError(err)
			}
			// JSON only here — provider config is too rich for a table.
			return output.JSON(cmd.OutOrStdout(), p)
		},
	}
}

func newProviderCreateCmd() *cobra.Command {
	var fromFile string
	cmd := &cobra.Command{
		Use:   "create -f provider.json",
		Short: "Create a provider from a JSON file or stdin",
		Long: `Create one provider. Input is a JSON object matching the server's
Provider shape; the same shape that "provider export" emits. Required
fields are type and name; everything else is provider-type-specific.`,
		Example: `  # Create from a file:
  maxx-cli provider create -f my-provider.json

  # Create from inline JSON (heredoc):
  cat <<'JSON' | maxx-cli -o json provider create -f -
  {"type":"custom","name":"Anthropic","config":{"baseUrl":"https://api.anthropic.com","apiKey":"sk-..."},"supportedClientTypes":["claude"]}
  JSON

  # Preview the request body, do not send:
  maxx-cli --dry-run provider create -f my-provider.json`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			data, err := readJSONInput(fromFile)
			if err != nil {
				return err
			}
			var p domain.Provider
			if err := json.Unmarshal(data, &p); err != nil {
				return fmt.Errorf("parse provider JSON: %w", err)
			}
			if flagDryRun {
				return previewJSON(cmd.OutOrStdout(), "POST /api/admin/providers", &p)
			}
			client, _, err := authedClient()
			if err != nil {
				return err
			}
			created, err := client.CreateProvider(&p)
			if err != nil {
				return handleAPIError(err)
			}
			return output.JSON(cmd.OutOrStdout(), created)
		},
	}
	cmd.Flags().StringVarP(&fromFile, "file", "f", "-", "JSON file path or '-' for stdin")
	return cmd
}

func newProviderUpdateCmd() *cobra.Command {
	var fromFile string
	cmd := &cobra.Command{
		Use:   "update ID -f provider.json",
		Short: "Replace a provider with the JSON in a file or stdin",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseUint(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid id %q", args[0])
			}
			data, err := readJSONInput(fromFile)
			if err != nil {
				return err
			}
			var p domain.Provider
			if err := json.Unmarshal(data, &p); err != nil {
				return fmt.Errorf("parse provider JSON: %w", err)
			}
			if flagDryRun {
				return previewJSON(cmd.OutOrStdout(), fmt.Sprintf("PUT /api/admin/providers/%d", id), &p)
			}
			client, _, err := authedClient()
			if err != nil {
				return err
			}
			updated, err := client.UpdateProvider(id, &p)
			if err != nil {
				return handleAPIError(err)
			}
			return output.JSON(cmd.OutOrStdout(), updated)
		},
	}
	cmd.Flags().StringVarP(&fromFile, "file", "f", "-", "JSON file path or '-' for stdin")
	return cmd
}

func newProviderDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete ID",
		Short: "Delete a provider",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseUint(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid id %q", args[0])
			}
			if flagDryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "[dry-run] DELETE /api/admin/providers/%d\n", id)
				return nil
			}
			if !confirm(fmt.Sprintf("Delete provider %d?", id)) {
				return fmt.Errorf("aborted")
			}
			client, _, err := authedClient()
			if err != nil {
				return err
			}
			if err := client.DeleteProvider(id); err != nil {
				return handleAPIError(err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Deleted provider %d.\n", id)
			return nil
		},
	}
}

func newProviderExportCmd() *cobra.Command {
	var outFile string
	cmd := &cobra.Command{
		Use:   "export [--out file.json]",
		Short: "Export all providers as a JSON array",
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, _, err := authedClient()
			if err != nil {
				return err
			}
			providers, err := client.ExportProviders()
			if err != nil {
				return handleAPIError(err)
			}
			data, err := json.MarshalIndent(providers, "", "  ")
			if err != nil {
				return err
			}
			data = append(data, '\n')
			if outFile == "" || outFile == "-" {
				_, err := cmd.OutOrStdout().Write(data)
				return err
			}
			if err := os.WriteFile(outFile, data, 0o600); err != nil {
				return fmt.Errorf("write %s: %w", outFile, err)
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "Exported %d providers to %s\n", len(providers), outFile)
			return nil
		},
	}
	// Local -o overrides the persistent -o output flag for this command, since
	// "output to file" is a different concept than "output format".
	cmd.Flags().StringVar(&outFile, "out", "-", "output file path, or '-' for stdout")
	return cmd
}

func newProviderImportCmd() *cobra.Command {
	var fromFile string
	cmd := &cobra.Command{
		Use:   "import -f providers.json",
		Short: "Import providers from a JSON file or stdin (accepts a single object or an array)",
		Long: `Import one or many providers from JSON. Input may be either a single
object or an array of objects; the CLI normalises to the array form the
server expects.`,
		Example: `  # Bulk import from an export:
  maxx-cli provider export --out backup.json
  cat backup.json | maxx-cli provider import -f -

  # Import a single provider (object form, also accepted):
  cat <<'JSON' | maxx-cli provider import -f -
  {"type":"custom","name":"Inline","supportedClientTypes":["claude"]}
  JSON

  # See what would be sent, do not import:
  maxx-cli --dry-run provider import -f backup.json`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			data, err := readJSONInput(fromFile)
			if err != nil {
				return err
			}
			providers, err := normaliseProviderImportInput(data)
			if err != nil {
				return err
			}
			if flagDryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "[dry-run] POST /api/admin/providers/import (%d providers)\n", len(providers))
				return output.JSON(cmd.OutOrStdout(), providers)
			}
			client, _, err := authedClient()
			if err != nil {
				return err
			}
			result, err := client.ImportProviders(providers)
			if err != nil {
				return handleAPIError(err)
			}
			return output.JSON(cmd.OutOrStdout(), result)
		},
	}
	cmd.Flags().StringVarP(&fromFile, "file", "f", "-", "JSON file path or '-' for stdin")
	return cmd
}

// utf8BOM is the byte-order mark some editors (notably Windows Notepad)
// prepend to UTF-8 files. We strip it so those exports import cleanly.
var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// normaliseProviderImportInput accepts either a single Provider object or an
// array, and always returns the array form expected by the server. JSONL
// (one object per line) is NOT supported — pass a JSON array instead.
func normaliseProviderImportInput(data []byte) ([]*domain.Provider, error) {
	data = bytes.TrimPrefix(data, utf8BOM)
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("empty input")
	}
	if trimmed[0] == '[' {
		var arr []*domain.Provider
		if err := json.Unmarshal(trimmed, &arr); err != nil {
			return nil, fmt.Errorf("parse JSON array: %w", err)
		}
		return arr, nil
	}
	if trimmed[0] == '{' {
		var single domain.Provider
		if err := json.Unmarshal(trimmed, &single); err != nil {
			return nil, fmt.Errorf("parse JSON object: %w", err)
		}
		return []*domain.Provider{&single}, nil
	}
	return nil, fmt.Errorf("expected JSON object or array, got %q", string(trimmed[:1]))
}

// readJSONInput reads from a file or stdin (when path is "" or "-").
func readJSONInput(path string) ([]byte, error) {
	if path == "" || path == "-" {
		return io.ReadAll(os.Stdin)
	}
	return os.ReadFile(path)
}

// previewJSON prints a dry-run header and the request body.
func previewJSON(w io.Writer, header string, body any) error {
	fmt.Fprintf(w, "[dry-run] %s\n", header)
	return output.JSON(w, body)
}
