package output

import (
	"fmt"
	"strings"

	"github.com/pterm/pterm"
)

// TableResult renders tabular data for list-style commands.
type TableResult struct {
	Headers []string   // column headers
	Rows    [][]string // row data
	Data    any        // typed slice for JSON serialization
}

func (t TableResult) HumanReadable(o OutputOptions) string {
	if len(t.Rows) == 0 {
		return "No results found.\n"
	}

	tableData := pterm.TableData{t.Headers}
	for _, row := range t.Rows {
		tableData = append(tableData, row)
	}

	if o.mode == ModePlain {
		return renderPlainTable(t.Headers, t.Rows)
	}

	rendered, err := pterm.DefaultTable.
		WithHasHeader(true).
		WithData(tableData).
		WithSeparator("  ").
		Srender()
	if err != nil {
		return renderPlainTable(t.Headers, t.Rows)
	}
	return rendered + "\n"
}

func (t TableResult) MachineReadable() any {
	return t.Data
}

// renderPlainTable produces a simple aligned text table without ANSI codes.
func renderPlainTable(headers []string, rows [][]string) string {
	colWidths := make([]int, len(headers))
	for i, h := range headers {
		colWidths[i] = len(h)
	}
	for _, row := range rows {
		for i, cell := range row {
			if i < len(colWidths) && len(cell) > colWidths[i] {
				colWidths[i] = len(cell)
			}
		}
	}

	var sb strings.Builder
	writePaddedRow(&sb, headers, colWidths)
	for _, row := range rows {
		writePaddedRow(&sb, row, colWidths)
	}
	return sb.String()
}

func writePaddedRow(sb *strings.Builder, cells []string, widths []int) {
	for i, cell := range cells {
		if i > 0 {
			sb.WriteString("  ")
		}
		if i < len(widths) {
			fmt.Fprintf(sb, "%-*s", widths[i], cell)
		} else {
			sb.WriteString(cell)
		}
	}
	sb.WriteByte('\n')
}
