package screen

// Plain text rendering of a payload, for curl and for agents.
//
// The same Payload backs all three representations, so a text answer can never
// drift from the JSON or the screen: it is the same blocks, rendered with
// characters instead of components. Anything that cannot be said in text is
// omitted rather than approximated, because a wrong number is worse than an
// absent one.

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

const indent = "  "

// Text renders the payload as plain text. No ANSI escapes: output is piped into
// files and into agent context as often as it is read in a terminal.
func (p Payload) Text() string {
	var b strings.Builder

	fmt.Fprintf(&b, "%s %s\n\n", p.Command, p.Target)

	glyph := map[string]string{"ok": "[x]", "warn": "[!]", "none": "[ ]"}[p.Verdict.State]
	if glyph == "" {
		glyph = "[ ]"
	}
	fmt.Fprintf(&b, "%s%s %s\n", indent, glyph, p.Verdict.Headline)
	if p.Verdict.Detail != "" {
		fmt.Fprintf(&b, "%s    %s\n", indent, p.Verdict.Detail)
	}

	// A refusal read over curl should carry the same code the JSON does, or the
	// text form is the one representation you cannot act on programmatically.
	// It also replaces the freshness line, which would be nonsense here: a
	// refusal held nothing and looked nothing up.
	if p.Error != nil {
		fmt.Fprintf(&b, "\n%serror %s\n", indent, p.Error.Code)
		if p.Error.Hint != "" {
			fmt.Fprintf(&b, "%s%s\n", indent, p.Error.Hint)
		}
	} else {
		fmt.Fprintf(&b, "\n%slive, held %ds · %dms · %d lookups\n",
			indent, p.TTL, p.ElapsedMS, p.Upstream)

		for _, block := range p.Blocks {
			body := blockText(block)
			if body == "" {
				continue
			}
			fmt.Fprintf(&b, "\n[ %s ]\n\n%s", strings.ToUpper(blockTitle(block)), body)
		}

		if len(p.Notes) > 0 {
			b.WriteString("\n[ NOTES ]\n\n")
			for _, note := range p.Notes {
				fmt.Fprintf(&b, "%s%s\n", indent, note)
			}
		}
	}

	return printable(b.String())
}

// printable drops every control character except the two the layout uses.
// Titles, descriptions and txt records are someone else's bytes, and the text
// form is printed to terminals and handed to models: an escape sequence in a
// page title must not rewrite either.
func printable(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' {
			return r
		}
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
}

// blockTitle reads the title off whichever prop struct this is. Every prop type
// has one, so this goes through JSON rather than repeating the type switch.
func blockTitle(block Block) string {
	var probe struct {
		Title string `json:"title"`
	}
	if raw, err := json.Marshal(block.Props); err == nil {
		_ = json.Unmarshal(raw, &probe)
	}
	if probe.Title == "" {
		return block.Component
	}
	return probe.Title
}

func blockText(block Block) string {
	switch p := block.Props.(type) {
	case SpecProps:
		pairs := make([][2]string, 0, len(p.Rows))
		for _, row := range p.Rows {
			pairs = append(pairs, [2]string{row.Label, row.Value})
		}
		return pairText(pairs)

	case CheckProps:
		var b strings.Builder
		for _, item := range p.Items {
			mark := "[ ]"
			if item.Done {
				mark = "[x]"
			}
			fmt.Fprintf(&b, "%s%s %s", indent, mark, item.Label)
			if item.Note != "" {
				fmt.Fprintf(&b, " · %s", item.Note)
			}
			b.WriteString("\n")
		}
		return b.String()

	case TableProps:
		return tableText(p.Headers, p.Rows)

	case SheetProps:
		var b strings.Builder
		for i, section := range p.Sections {
			if i > 0 {
				b.WriteString("\n")
			}
			fmt.Fprintf(&b, "%s%s\n", indent, section.Title)
			pairs := make([][2]string, 0, len(section.Rows))
			for _, row := range section.Rows {
				if len(row) >= 2 {
					pairs = append(pairs, [2]string{row[0], row[1]})
				}
			}
			b.WriteString(indentBy(pairText(pairs), indent))
		}
		return b.String()

	case StatProps:
		pairs := make([][2]string, 0, len(p.Items))
		for _, item := range p.Items {
			value := item.Value
			if item.Hint != "" {
				value += " · " + item.Hint
			}
			pairs = append(pairs, [2]string{item.Label, value})
		}
		return pairText(pairs)

	case KpiProps:
		value := p.Value
		if p.Hint != "" {
			value += " · " + p.Hint
		}
		return pairText([][2]string{{p.Label, value}})

	case RankProps:
		pairs := make([][2]string, 0, len(p.Items))
		for _, item := range p.Items {
			pairs = append(pairs, [2]string{item.Label, display(item.Display, item.Value)})
		}
		return pairText(pairs)

	case BulletProps:
		pairs := make([][2]string, 0, len(p.Items))
		for _, item := range p.Items {
			pairs = append(pairs, [2]string{item.Label, display(item.Display, item.Value)})
		}
		return pairText(pairs)

	case FunnelProps:
		pairs := make([][2]string, 0, len(p.Steps))
		for _, step := range p.Steps {
			pairs = append(pairs, [2]string{step.Label, display(step.Display, step.Value)})
		}
		return pairText(pairs)

	case WaterfallProps:
		pairs := make([][2]string, 0, len(p.Items))
		for _, item := range p.Items {
			pairs = append(pairs, [2]string{item.Label, display(item.Display, item.Value)})
		}
		return pairText(pairs)

	case PlotProps:
		return tableText([]string{}, seriesRows(p.Labels, p.Data))

	case SparkProps:
		var b strings.Builder
		b.WriteString(tableText([]string{}, seriesRows(nil, p.Data)))
		if p.Caption != "" {
			fmt.Fprintf(&b, "%s%s\n", indent, p.Caption)
		}
		return b.String()

	case BarsProps:
		// One row per series, not per bar: an individual bar carries no label
		// of its own (BarSeries is a title plus a slice of values in list
		// order), so the series label is the only real label there is.
		rows := [][]string{
			{p.From.Label, joinInts(p.From.Values)},
			{p.To.Label, joinInts(p.To.Values)},
		}
		var b strings.Builder
		b.WriteString(tableText([]string{}, rows))
		if p.Processor != "" {
			fmt.Fprintf(&b, "%s%s\n", indent, p.Processor)
		}
		return b.String()

	case SlopeProps:
		rows := make([][]string, 0, len(p.Items))
		for _, item := range p.Items {
			rows = append(rows, []string{item.Label, fmt.Sprint(item.From), fmt.Sprint(item.To)})
		}
		return tableText([]string{"", p.FromLabel, p.ToLabel}, rows)

	case TreeProps:
		var b strings.Builder
		writeTree(&b, p.Nodes, indent)
		return b.String()

	case FlowProps:
		var b strings.Builder
		for _, row := range p.Rows {
			labels := make([]string, 0, len(row.Nodes))
			for _, node := range row.Nodes {
				labels = append(labels, node.Label)
			}
			fmt.Fprintf(&b, "%s%s\n", indent, strings.Join(labels, " -> "))
		}
		return b.String()

	case TimelineProps:
		pairs := make([][2]string, 0, len(p.Events))
		for _, event := range p.Events {
			label := event.Label
			if event.State != "" {
				label += " · " + event.State
			}
			pairs = append(pairs, [2]string{event.Date, label})
		}
		return pairText(pairs)

	case DiffProps:
		var b strings.Builder
		rows := p.Rows
		if p.Footer != nil {
			rows = append(append([]DiffRow(nil), rows...), *p.Footer)
		}
		for _, row := range rows {
			sign := map[string]string{"add": "+", "remove": "-", "keep": " "}[row.Sign]
			if sign == "" {
				sign = " "
			}
			fmt.Fprintf(&b, "%s%s %s · %s\n", indent, sign, row.Label, row.Value)
		}
		return b.String()

	case MatrixProps:
		rows := make([][]string, 0, len(p.Rows))
		for _, row := range p.Rows {
			rows = append(rows, append([]string{row.Label}, cellStrings(row.Values)...))
		}
		return tableText(append([]string{""}, p.Columns...), rows)

	case CompareProps:
		rows := make([][]string, 0, len(p.Rows))
		for _, row := range p.Rows {
			rows = append(rows, append([]string{row.Label}, cellStrings(row.Values)...))
		}
		return tableText(append([]string{""}, p.Columns...), rows)

	case HeatmapProps:
		rows := make([][]string, 0, len(p.Rows))
		for _, row := range p.Rows {
			cells := make([]string, 0, len(row.Values))
			for _, value := range row.Values {
				cells = append(cells, fmt.Sprint(value))
			}
			rows = append(rows, append([]string{row.Label}, cells...))
		}
		return tableText(append([]string{""}, p.Columns...), rows)

	case UptimeProps:
		var b strings.Builder
		fmt.Fprintf(&b, "%s%s\n", indent, strings.Join(p.Days, " "))
		if p.From != "" || p.To != "" {
			fmt.Fprintf(&b, "%s%s .. %s\n", indent, p.From, p.To)
		}
		return b.String()

	case MeterProps:
		return percentText(p.Value, p.Caption)

	case WaffleProps:
		return percentText(p.Value, p.Caption)

	case CellsProps:
		// CellGrid carries one label for the whole grid, not a label per row
		// or per column, so the table gets a row label (repeated only on a
		// grid's first row) and a glyph string; no header row, since there
		// are no column labels to put in one.
		var rows [][]string
		for _, item := range p.Items {
			for i, cellRow := range item.Cells {
				label := ""
				if i == 0 {
					label = item.Label
				}
				glyphs := make([]string, len(cellRow))
				for j, v := range cellRow {
					glyphs[j] = cellGlyph(v)
				}
				rows = append(rows, []string{label, strings.Join(glyphs, "")})
			}
		}
		return tableText([]string{}, rows)

	case CountdownProps:
		line := p.To
		if p.Caption != "" {
			line += " · " + p.Caption
		}
		return fmt.Sprintf("%s%s\n", indent, line)

	case GanttProps:
		// Omitted. Starts and ends are fractions of the chart's own span, not
		// dates, so text could only repeat the labels. Its one caller is TLS,
		// where the chain sheet carries the same spans as real dates.
		return ""
	}

	return ""
}

func writeTree(b *strings.Builder, nodes []TreeNode, prefix string) {
	for _, node := range nodes {
		fmt.Fprintf(b, "%s%s", prefix, node.Label)
		if node.Meta != "" {
			fmt.Fprintf(b, " · %s", node.Meta)
		}
		b.WriteString("\n")
		writeTree(b, node.Children, prefix+"  ")
	}
}

// seriesRows lays a plot or spark series out as x/y rows, x taken from labels
// where one exists at that index and the point's index otherwise. A spark is
// a shape, not a table, so past 24 points the middle is replaced by a count
// rather than printed in full: this is the one place in blockText where a
// count stands in for content instead of rendering all of it.
func seriesRows(labels []string, data []int) [][]string {
	rows := make([][]string, len(data))
	for i, v := range data {
		x := strconv.Itoa(i)
		if i < len(labels) {
			x = labels[i]
		}
		rows[i] = []string{x, strconv.Itoa(v)}
	}
	if len(rows) <= 24 {
		return rows
	}
	out := make([][]string, 0, 25)
	out = append(out, rows[:12]...)
	out = append(out, []string{"…", fmt.Sprintf("(%d points)", len(rows))})
	out = append(out, rows[len(rows)-12:]...)
	return out
}

func joinInts(values []int) string {
	parts := make([]string, len(values))
	for i, v := range values {
		parts[i] = strconv.Itoa(v)
	}
	return strings.Join(parts, " ")
}

// cellGlyph mirrors the frontend's cells grid, which draws a cell only on
// exactly 1 and leaves every other value blank.
func cellGlyph(v int) string {
	switch v {
	case 1:
		return "x"
	case 0:
		return "."
	default:
		return strconv.Itoa(v)
	}
}

func cellStrings(values []any) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		switch v := value.(type) {
		case bool:
			if v {
				out = append(out, "yes")
			} else {
				out = append(out, "no")
			}
		case nil:
			out = append(out, "-")
		default:
			out = append(out, fmt.Sprint(v))
		}
	}
	return out
}

func display(text string, value int) string {
	if text != "" {
		return text
	}
	return fmt.Sprint(value)
}

func percentText(value float64, caption string) string {
	line := fmt.Sprintf("%.0f%%", value*100)
	if caption != "" {
		line += " · " + caption
	}
	return fmt.Sprintf("%s%s\n", indent, line)
}

// pad right-pads to a column measured in runes. %-*s pads by bytes, which put
// every idn and every · one column out.
func pad(s string, columns int) string {
	if extra := columns - width(s); extra > 0 {
		return s + strings.Repeat(" ", extra)
	}
	return s
}

// pairText lays label and value out as two aligned columns.
func pairText(pairs [][2]string) string {
	widest := 0
	for _, pair := range pairs {
		widest = max(widest, width(pair[0]))
	}

	var b strings.Builder
	for _, pair := range pairs {
		fmt.Fprintf(&b, "%s%s  %s\n", indent, pad(pair[0], widest), pair[1])
	}
	return b.String()
}

// tableText aligns every column to its widest cell. Nothing is truncated, for
// the same reason the frontend widens a block rather than cutting a value.
func tableText(headers []string, rows [][]string) string {
	columns := len(headers)
	for _, row := range rows {
		columns = max(columns, len(row))
	}
	if columns == 0 {
		return ""
	}

	widths := make([]int, columns)
	for i, header := range headers {
		widths[i] = width(header)
	}
	for _, row := range rows {
		for i, cell := range row {
			widths[i] = max(widths[i], width(cell))
		}
	}

	var b strings.Builder
	writeRow := func(cells []string) {
		b.WriteString(indent)
		parts := make([]string, 0, columns)
		for i := 0; i < columns; i++ {
			cell := ""
			if i < len(cells) {
				cell = cells[i]
			}
			if i == columns-1 {
				parts = append(parts, cell)
				continue
			}
			parts = append(parts, pad(cell, widths[i]))
		}
		b.WriteString(strings.TrimRight(strings.Join(parts, "  "), " "))
		b.WriteString("\n")
	}

	if strings.Join(headers, "") != "" {
		writeRow(headers)
	}
	for _, row := range rows {
		writeRow(row)
	}
	return b.String()
}

func indentBy(text, prefix string) string {
	if text == "" {
		return ""
	}
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	for i, line := range lines {
		lines[i] = prefix + line
	}
	return strings.Join(lines, "\n") + "\n"
}
