package screen

import "testing"

// A Kpi used to fall through FitSpan's default case, which made it the one
// block never sized by its content. A long AS name then wrapped to four lines
// inside a single column while the frame beside it sat half empty.
func TestFitSpanMeasuresKpi(t *testing.T) {
	for _, test := range []struct {
		name  string
		props KpiProps
		want  int
	}{
		{
			// 51 characters of label, well past a single column.
			name: "long label widens",
			props: KpiProps{
				Value: "AS135239",
				Label: "SONALI-AS-IN - Sonali Internet Services Pvt Ltd, IN",
				Hint:  "prefix 103.140.107.0/24",
			},
			want: 2,
		},
		{
			name: "short label stays at one",
			props: KpiProps{
				Value: "AS15169",
				Label: "GOOGLE",
				Hint:  "prefix 8.8.8.0/24",
			},
			want: 1,
		},
		{
			// The display-sized value costs roughly twice a body character, so
			// a long value alone is enough to need a second column. 20 runes
			// doubled is 40, just past the 38 a column holds.
			name:  "display value is counted at double width",
			props: KpiProps{Value: "AS4294967295 AS42001", Label: "x", Hint: ""},
			want:  2,
		},
		{
			// And one rune fewer is exactly the budget, so it stays put. Pins
			// the boundary rather than leaving it to drift.
			name:  "value that exactly fits stays at one",
			props: KpiProps{Value: "AS4294967295 AS4200", Label: "x", Hint: ""},
			want:  1,
		},
		{
			name:  "absent hint is not counted",
			props: KpiProps{Value: "none", Label: "not published"},
			want:  1,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := FitSpan("GraphKpi", test.props, 1); got != test.want {
				t.Errorf("span = %d, want %d", got, test.want)
			}
		})
	}
}

// FitSpan widens and never narrows, so a block a handler already asked to be
// wide must not be shrunk by being measured.
func TestFitSpanNeverNarrowsKpi(t *testing.T) {
	props := KpiProps{Value: "AS1", Label: "short", Hint: "short"}
	if got := FitSpan("GraphKpi", props, 3); got != 3 {
		t.Errorf("span = %d, want the requested 3 kept", got)
	}
}

// The table-like components floor at two columns, not three: they wrap inside
// their frame, so a third column just packed the grid less densely. A floor
// still never narrows a span already wider than it.
func TestFitSpanTableFloor(t *testing.T) {
	if got := FitSpan("GraphTable", TableProps{}, 1); got != 2 {
		t.Errorf("span = %d, want the floor of 2", got)
	}
	if got := FitSpan("GraphBars", BarsProps{}, 3); got != 3 {
		t.Errorf("span = %d, want the requested 3 kept", got)
	}
}
