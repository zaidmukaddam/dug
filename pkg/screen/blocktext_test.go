package screen

import "testing"

// One instance of every exported *Props type in props.go. blockText's type
// switch has no default case that renders anything, so a props type left
// out of the switch silently prints nothing for curl. This table is the
// guard: add a props type, add it here, or the assertion catches the gap.
//
// GanttProps is the one documented exception, marked wantEmpty with a
// comment next to its case in text.go: starts and ends are fractions of the
// chart's own span, not dates, so text could only repeat the labels.
func TestBlockText(t *testing.T) {
	for _, test := range []struct {
		name      string
		block     Block
		wantEmpty bool
	}{
		{
			name:  "SpecProps",
			block: Block{Component: "GraphSpec", Props: SpecProps{Title: "spec", Rows: []SpecRow{{Label: "asn", Value: "AS15169"}}}},
		},
		{
			name:  "CheckProps",
			block: Block{Component: "GraphCheck", Props: CheckProps{Title: "checks", Items: []CheckItem{{Label: "mx", Done: true}}}},
		},
		{
			name:  "TableProps",
			block: Block{Component: "GraphTable", Props: TableProps{Title: "table", Headers: []string{"h"}, Rows: [][]string{{"a"}}}},
		},
		{
			name: "SheetProps",
			block: Block{Component: "GraphSheet", Props: SheetProps{Title: "sheet", Sections: []SheetSection{
				{Title: "section", Rows: [][]string{{"a", "b"}}},
			}}},
		},
		{
			name:  "StatProps",
			block: Block{Component: "GraphStat", Props: StatProps{Title: "stat", Items: []StatItem{{Value: "1", Label: "one"}}}},
		},
		{
			name:  "KpiProps",
			block: Block{Component: "GraphKpi", Props: KpiProps{Title: "kpi", Value: "1", Label: "one"}},
		},
		{
			name:  "RankProps",
			block: Block{Component: "GraphRank", Props: RankProps{Title: "rank", Items: []RankItem{{Label: "a", Value: 1}}}},
		},
		{
			name:  "BulletProps",
			block: Block{Component: "GraphBullet", Props: BulletProps{Title: "bullet", Items: []BulletItem{{Label: "a", Value: 1}}}},
		},
		{
			name:  "TreeProps",
			block: Block{Component: "GraphTree", Props: TreeProps{Title: "tree", Nodes: []TreeNode{{Label: "root"}}}},
		},
		{
			name: "FlowProps",
			block: Block{Component: "GraphFlow", Props: FlowProps{Title: "flow", Rows: []FlowRow{
				{Nodes: []FlowNode{{Label: "start"}}},
			}}},
		},
		{
			name:  "TimelineProps",
			block: Block{Component: "GraphTimeline", Props: TimelineProps{Title: "timeline", Events: []TimelineEvent{{Date: "2024-01-01", Label: "created"}}}},
		},
		{
			// Documented exception (text.go, near blockText's GanttProps case):
			// starts and ends are fractions of the chart's own span, not dates,
			// so text could only repeat the labels.
			name:      "GanttProps",
			block:     Block{Component: "GraphGantt", Props: GanttProps{Title: "gantt", Items: []GanttItem{{Label: "issue", Start: 0, End: 1}}}},
			wantEmpty: true,
		},
		{
			name:  "UptimeProps",
			block: Block{Component: "GraphUptime", Props: UptimeProps{Title: "uptime", Days: []string{"up"}}},
		},
		{
			name: "HeatmapProps",
			block: Block{Component: "GraphHeatmap", Props: HeatmapProps{Title: "heatmap", Columns: []string{"mon"}, Rows: []HeatRow{
				{Label: "week1", Values: []int{1}},
			}}},
		},
		{
			name: "MatrixProps",
			block: Block{Component: "GraphMatrix", Props: MatrixProps{Title: "matrix", Columns: []string{"col"}, Rows: []MatrixRow{
				{Label: "row", Values: []any{1}},
			}}},
		},
		{
			name:  "DiffProps",
			block: Block{Component: "GraphDiff", Props: DiffProps{Title: "diff", Rows: []DiffRow{{Label: "a", Value: "1", Sign: "add"}}}},
		},
		{
			name: "CompareProps",
			block: Block{Component: "GraphCompare", Props: CompareProps{Title: "compare", Columns: []string{"col"}, Rows: []CompareRow{
				{Label: "row", Values: []any{1}},
			}}},
		},
		{
			name: "SlopeProps",
			block: Block{Component: "GraphSlope", Props: SlopeProps{Title: "slope", FromLabel: "before", ToLabel: "after", Items: []SlopeItem{
				{Label: "a", From: 1, To: 2},
			}}},
		},
		{
			name:  "FunnelProps",
			block: Block{Component: "GraphFunnel", Props: FunnelProps{Title: "funnel", Steps: []FunnelStep{{Label: "step", Value: 1}}}},
		},
		{
			name:  "WaterfallProps",
			block: Block{Component: "GraphWaterfall", Props: WaterfallProps{Title: "waterfall", Items: []WaterfallItem{{Label: "item", Value: 1}}}},
		},
		{
			name:  "PlotProps",
			block: Block{Component: "GraphPlot", Props: PlotProps{Title: "plot", Data: []int{1, 2, 3}}},
		},
		{
			name:  "SparkProps",
			block: Block{Component: "GraphSpark", Props: SparkProps{Title: "spark", Data: []int{1, 2, 3}, Caption: "trend"}},
		},
		{
			name: "BarsProps",
			block: Block{Component: "GraphBars", Props: BarsProps{Title: "bars",
				From: BarSeries{Label: "from", Values: []int{1}},
				To:   BarSeries{Label: "to", Values: []int{2}},
			}},
		},
		{
			name:  "MeterProps",
			block: Block{Component: "GraphMeter", Props: MeterProps{Title: "meter", Value: 0.5, Caption: "half"}},
		},
		{
			name:  "WaffleProps",
			block: Block{Component: "GraphWaffle", Props: WaffleProps{Title: "waffle", Value: 0.5, Caption: "half"}},
		},
		{
			name: "CellsProps",
			block: Block{Component: "GraphCells", Props: CellsProps{Title: "cells", Items: []CellGrid{
				{Label: "grid", Cells: [][]int{{1}}},
			}}},
		},
		{
			// TimerProps renders only its caption; an empty caption is itself an
			// explicit empty case in blockText, so the field under test here is
			// Caption, not Kind or At.
			name:  "TimerProps",
			block: Block{Component: "GraphTimer", Props: TimerProps{Title: "timer", Caption: "elapsed"}},
		},
		{
			name:  "CountdownProps",
			block: Block{Component: "GraphCountdown", Props: CountdownProps{Title: "countdown", To: "2025-01-01"}},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := blockText(test.block)
			if test.wantEmpty {
				if got != "" {
					t.Errorf("blockText() = %q, want empty", got)
				}
				return
			}
			if got == "" {
				t.Errorf("blockText() is empty, want a non-empty rendering for %s", test.name)
			}
		})
	}
}
