// Package output formats command results as a human-readable table (default)
// or machine-readable JSON (-o json).
package output

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"
)

// Format is the user-selectable output format.
type Format string

const (
	FormatTable Format = "table"
	FormatJSON  Format = "json"
)

// Parse turns a -o flag value into a Format, falling back to table.
func Parse(s string) Format {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "json":
		return FormatJSON
	case "", "table", "wide":
		return FormatTable
	default:
		return Format(s)
	}
}

// JSON pretty-prints v to w.
func JSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// Table holds rows for tabular output.
type Table struct {
	Headers []string
	Rows    [][]string
}

// Print writes the table to w as fixed-width columns separated by 2 spaces.
// Empty tables produce a single "(no items)" line written to w.
func (t Table) Print(w io.Writer) {
	if len(t.Rows) == 0 {
		fmt.Fprintln(w, "(no items)")
		return
	}
	widths := make([]int, len(t.Headers))
	for i, h := range t.Headers {
		widths[i] = len(h)
	}
	for _, row := range t.Rows {
		for i, cell := range row {
			if i >= len(widths) {
				continue
			}
			if l := visualLen(cell); l > widths[i] {
				widths[i] = l
			}
		}
	}
	writeRow(w, t.Headers, widths)
	for _, row := range t.Rows {
		writeRow(w, row, widths)
	}
}

func writeRow(w io.Writer, cells []string, widths []int) {
	for i, cell := range cells {
		fmt.Fprint(w, cell)
		if i < len(cells)-1 {
			fmt.Fprint(w, strings.Repeat(" ", widths[i]-visualLen(cell)+2))
		}
	}
	fmt.Fprintln(w)
}

// visualLen approximates display width — only ASCII-aware. Adequate for the
// fields we render (numeric IDs, ASCII names).
func visualLen(s string) int {
	return len(s)
}

// FormatTime renders a time in a short, sortable form, or "-" if zero.
func FormatTime(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.Format("2006-01-02 15:04:05")
}

// FormatTimePtr renders a *time.Time, treating nil as "-".
func FormatTimePtr(t *time.Time) string {
	if t == nil {
		return "-"
	}
	return FormatTime(*t)
}

// FormatBool returns Y/N for a bool.
func FormatBool(b bool) string {
	if b {
		return "Y"
	}
	return "N"
}

// Truncate clips s to n runes, appending "…" if shortened. Operates on runes
// so multi-byte UTF-8 sequences are never split in the middle.
func Truncate(s string, n int) string {
	if n <= 0 {
		return s
	}
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n]) + "…"
}

// PrintOrJSON dispatches based on format: JSON serialises raw, table calls
// build() to assemble the Table.
func PrintOrJSON(w io.Writer, format Format, raw any, build func() Table) error {
	switch format {
	case FormatJSON:
		return JSON(w, raw)
	default:
		build().Print(w)
		return nil
	}
}
