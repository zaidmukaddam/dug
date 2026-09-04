package screen

import (
	"strings"
	"unicode/utf8"
)

// Typed props per graph component. The frontend registry keys on the component
// name and passes these straight through, so a field name here is part of the
// contract with the component.

type SpecRow struct {
	Label  string `json:"label"`
	Value  string `json:"value"`
	Accent bool   `json:"accent,omitempty"`
}

type SpecProps struct {
	Title string    `json:"title"`
	Rows  []SpecRow `json:"rows"`
}

type CheckItem struct {
	Label string `json:"label"`
	Done  bool   `json:"done"`
	Note  string `json:"note,omitempty"`
}

type CheckProps struct {
	Title string      `json:"title"`
	Items []CheckItem `json:"items"`
}

// Checks builds a check list in the order given.
//
// The order is a parameter rather than the map's own, because ranging a map
// walks differently every time and a screen that reorders itself between two
// identical queries reads as unstable.
//
// A failing row keeps its note when it has one. "http 404" says more than
// "absent", and the word is only worth printing where there is nothing better.
func Checks(order []string, done map[string]bool, notes map[string]string) []CheckItem {
	items := make([]CheckItem, 0, len(order))
	for _, label := range order {
		note := notes[label]
		if note == "" && !done[label] {
			note = "absent"
		}
		items = append(items, CheckItem{Label: label, Done: done[label], Note: note})
	}
	return items
}

type TableProps struct {
	Title   string     `json:"title"`
	Headers []string   `json:"headers"`
	Rows    [][]string `json:"rows"`
	Align   []string   `json:"align,omitempty"`
	Footer  []string   `json:"footer,omitempty"`
}

type SheetSection struct {
	Title string     `json:"title"`
	Rows  [][]string `json:"rows"`
}

type SheetProps struct {
	Title    string         `json:"title"`
	Headers  []string       `json:"headers"`
	Sections []SheetSection `json:"sections"`
}

type StatItem struct {
	Value  string `json:"value"`
	Label  string `json:"label"`
	Hint   string `json:"hint,omitempty"`
	Accent bool   `json:"accent,omitempty"`
}

type StatProps struct {
	Title string     `json:"title"`
	Items []StatItem `json:"items"`
}

type KpiProps struct {
	Title string `json:"title"`
	Value string `json:"value"`
	Label string `json:"label"`
	Hint  string `json:"hint,omitempty"`
	Data  []int  `json:"data"`
}

type RankItem struct {
	Label   string `json:"label"`
	Value   int    `json:"value"`
	Display string `json:"display,omitempty"`
}

type RankProps struct {
	Title string     `json:"title"`
	Items []RankItem `json:"items"`
	Max   int        `json:"max,omitempty"`
}

type BulletItem struct {
	Label   string `json:"label"`
	Value   int    `json:"value"`
	Target  int    `json:"target,omitempty"`
	Max     int    `json:"max,omitempty"`
	Display string `json:"display,omitempty"`
}

type BulletProps struct {
	Title string       `json:"title"`
	Items []BulletItem `json:"items"`
}

type TreeNode struct {
	Label    string     `json:"label"`
	Meta     string     `json:"meta,omitempty"`
	Accent   bool       `json:"accent,omitempty"`
	Children []TreeNode `json:"children,omitempty"`
}

type TreeProps struct {
	Title string     `json:"title"`
	Nodes []TreeNode `json:"nodes"`
}

type FlowNode struct {
	Label   string `json:"label"`
	Tone    string `json:"tone,omitempty"`
	Stretch bool   `json:"stretch,omitempty"`
}

type FlowRow struct {
	Nodes []FlowNode `json:"nodes"`
}

type FlowProps struct {
	Title string    `json:"title"`
	Rows  []FlowRow `json:"rows"`
}

type TimelineEvent struct {
	Date  string `json:"date"`
	Label string `json:"label"`
	State string `json:"state,omitempty"`
}

type TimelineProps struct {
	Title  string          `json:"title"`
	Events []TimelineEvent `json:"events"`
}

type GanttItem struct {
	Label  string  `json:"label"`
	Start  float64 `json:"start"`
	End    float64 `json:"end"`
	Accent bool    `json:"accent,omitempty"`
}

type GanttProps struct {
	Title    string      `json:"title"`
	Items    []GanttItem `json:"items"`
	Ticks    []string    `json:"ticks,omitempty"`
	Stage    string      `json:"stage,omitempty"`
	Progress float64     `json:"progress,omitempty"`
}

type UptimeProps struct {
	Title   string   `json:"title"`
	Days    []string `json:"days"`
	From    string   `json:"from,omitempty"`
	To      string   `json:"to,omitempty"`
	Columns int      `json:"columns,omitempty"`
}

type HeatRow struct {
	Label  string `json:"label"`
	Values []int  `json:"values"`
}

type HeatmapProps struct {
	Title   string    `json:"title"`
	Columns []string  `json:"columns"`
	Rows    []HeatRow `json:"rows"`
	Max     int       `json:"max,omitempty"`
	Legend  bool      `json:"legend,omitempty"`
	Caption string    `json:"caption,omitempty"`
}

type MatrixRow struct {
	Label  string `json:"label"`
	Values []any  `json:"values"`
}

type MatrixProps struct {
	Title   string      `json:"title"`
	Columns []string    `json:"columns"`
	Rows    []MatrixRow `json:"rows"`
}

type DiffRow struct {
	Label string `json:"label"`
	Value string `json:"value"`
	Sign  string `json:"sign,omitempty"` // add, remove, keep
}

type DiffProps struct {
	Title  string    `json:"title"`
	Rows   []DiffRow `json:"rows"`
	Footer *DiffRow  `json:"footer,omitempty"`
}

type CompareRow struct {
	Label  string `json:"label"`
	Values []any  `json:"values"`
}

type CompareProps struct {
	Title   string       `json:"title"`
	Columns []string     `json:"columns"`
	Rows    []CompareRow `json:"rows"`
}

type SlopeItem struct {
	Label string `json:"label"`
	From  int    `json:"from"`
	To    int    `json:"to"`
}

type SlopeProps struct {
	Title     string      `json:"title"`
	FromLabel string      `json:"fromLabel"`
	ToLabel   string      `json:"toLabel"`
	Items     []SlopeItem `json:"items"`
}

type FunnelStep struct {
	Label   string `json:"label"`
	Value   int    `json:"value"`
	Display string `json:"display,omitempty"`
}

type FunnelProps struct {
	Title string       `json:"title"`
	Steps []FunnelStep `json:"steps"`
	Stage string       `json:"stage,omitempty"`
}

type WaterfallItem struct {
	Label   string `json:"label"`
	Value   int    `json:"value"`
	Display string `json:"display,omitempty"`
	Kind    string `json:"kind,omitempty"`
}

type WaterfallProps struct {
	Title string          `json:"title"`
	Items []WaterfallItem `json:"items"`
}

type PlotProps struct {
	Title   string   `json:"title"`
	Data    []int    `json:"data"`
	Labels  []string `json:"labels,omitempty"`
	Variant string   `json:"variant,omitempty"`
	Height  int      `json:"height,omitempty"`
}

type SparkProps struct {
	Title   string `json:"title"`
	Data    []int  `json:"data"`
	Caption string `json:"caption,omitempty"`
}

type BarSeries struct {
	Label  string `json:"label"`
	Values []int  `json:"values"`
}

type BarsProps struct {
	Title     string    `json:"title"`
	From      BarSeries `json:"from"`
	To        BarSeries `json:"to"`
	Processor string    `json:"processor,omitempty"`
}

type MeterProps struct {
	Title   string  `json:"title"`
	Value   float64 `json:"value"`
	Caption string  `json:"caption,omitempty"`
}

type WaffleProps struct {
	Title   string  `json:"title"`
	Value   float64 `json:"value"`
	Caption string  `json:"caption,omitempty"`
}

type CellGrid struct {
	Label string  `json:"label"`
	Cells [][]int `json:"cells"`
}

type CellsProps struct {
	Title string     `json:"title"`
	Items []CellGrid `json:"items"`
}

type CountdownProps struct {
	Title   string `json:"title"`
	To      string `json:"to"`
	Done    string `json:"done,omitempty"`
	Caption string `json:"caption,omitempty"`
}

// Characters that fit per grid column at 14px mono. Conservative: guessing low
// widens a block unnecessarily, guessing high lets a value escape its frame.
const (
	colChars    = 38
	twoColChars = 84
)

// FitSpan widens a block until its content fits. Never narrows, never truncates.
//
// Only the text components are measured. Charts scale their glyph tracks to any
// width, and the tables scroll inside their own frame.
//
// Minimum columns each component needs before its content overflows the frame.
// The tables wrap inside their frame, so two columns is enough. The track
// components need ~440px for their fixed label and display columns plus a
// 20 to 24 glyph tick track, which also lands at two.
func FitSpan(component string, props any, span int) int {
	floor := map[string]int{
		"GraphTable": 2, "GraphSheet": 2, "GraphMatrix": 2, "GraphCompare": 2,
		"GraphBullet": 2, "GraphRank": 2, "GraphFunnel": 2, "GraphWaterfall": 2,
		"GraphBars": 2,
	}[component]
	if floor > span {
		span = floor
	}

	widest := 0
	switch p := props.(type) {
	case SpecProps:
		// A Spec row is a label track beside a value track, not one line, so
		// the binding constraint is the longest value against the value column
		// alone. Measured budgets at 14px mono: 15 characters at one column,
		// 47 at two, 106 at three. Scaled onto the shared thresholds below.
		longest := 0
		for _, row := range p.Rows {
			longest = max(longest, width(row.Value))
		}
		switch {
		case longest > 47:
			return 3
		case longest > 15:
			return max(span, 2)
		}
		return span
	case CheckProps:
		for _, item := range p.Items {
			widest = max(widest, width(item.Label)+6, width(item.Note)+6)
		}
	case TimelineProps:
		for _, event := range p.Events {
			widest = max(widest, width(event.Date)+width(event.Label)+6)
		}
	case DiffProps:
		for _, row := range p.Rows {
			widest = max(widest, width(row.Label)+width(row.Value)+6)
		}
	case FlowProps:
		for _, row := range p.Rows {
			line := 0
			for _, node := range row.Nodes {
				line += width(node.Label) + 8
			}
			widest = max(widest, line)
		}
	case TreeProps:
		widest = max(widest, treeWidth(p.Nodes, 0))
	case StatProps:
		for _, item := range p.Items {
			widest = max(widest, width(item.Value)+2)
		}
	case KpiProps:
		// The value is display sized, so it eats about twice the column budget
		// a body character does. Label and hint are stacked in the component
		// and each get the full width, so the constraint is the longest of the
		// three rather than their sum.
		//
		// Without this a Kpi was the one block never measured at all, and an
		// AS name like "SONALI-AS-IN - Sonali Internet Services Pvt Ltd, IN"
		// wrapped to four lines in a single column while the frame beside it
		// sat half empty.
		widest = max(width(p.Value)*2, width(p.Label), width(p.Hint))
	default:
		return span
	}

	switch {
	case widest > twoColChars:
		return 3
	case widest > colChars && span < 2:
		return 2
	}
	return span
}

func treeWidth(nodes []TreeNode, depth int) int {
	widest := 0
	for _, node := range nodes {
		widest = max(widest, depth*3+width(node.Label)+width(node.Meta)+4)
		widest = max(widest, treeWidth(node.Children, depth+1))
	}
	return widest
}

func width(s string) int { return utf8.RuneCountInString(s) }

// Truncate is deliberately absent. Values are never cut; a block whose content
// is wider than its column takes more columns instead. This helper exists only
// to normalise whitespace in upstream error strings.
func Clean(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// OrNone renders absence the way every screen does: checked, and not there.
func OrNone(value string) string {
	if value == "" {
		return "none"
	}
	return value
}
