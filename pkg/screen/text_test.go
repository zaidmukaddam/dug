package screen

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// A non-ASCII label used to misalign its column: fmt's %-*s pads to a byte
// count, and "·é" is 4 bytes but 2 runes, so the value column landed 2 bytes
// short of where the ASCII rows put it.
func TestTextPadding(t *testing.T) {
	valueColumn := func(t *testing.T, line string) int {
		t.Helper()
		// The label/value gap is two spaces (see pairText/tableText), so the
		// value starts right after the first run of 2+ spaces past the indent.
		trimmed := strings.TrimPrefix(line, indent)
		idx := strings.Index(trimmed, "  ")
		if idx < 0 {
			t.Fatalf("line %q has no column gap", line)
		}
		rest := trimmed[idx:]
		return utf8.RuneCountInString(trimmed[:idx]) + (len(rest) - len(strings.TrimLeft(rest, " ")))
	}

	t.Run("pairText", func(t *testing.T) {
		out := pairText([][2]string{{"ab", "x"}, {"·é", "y"}})
		lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
		if len(lines) != 2 {
			t.Fatalf("got %d lines, want 2: %q", len(lines), out)
		}

		// Byte length of "·é" (4) differs from its rune count (2), which is
		// exactly the gap the old %-*s formatting got wrong.
		if got, want := len("·é"), 4; got != want {
			t.Fatalf("test fixture assumption broken: len(·é) = %d, want %d", got, want)
		}

		col0 := valueColumn(t, lines[0])
		col1 := valueColumn(t, lines[1])
		if col0 != col1 {
			t.Errorf("pairText value columns not aligned: row0 at %d, row1 at %d\n%s", col0, col1, out)
		}
	})

	t.Run("tableText", func(t *testing.T) {
		out := tableText([]string{"h1", "h2"}, [][]string{{"ab", "x"}, {"·é", "y"}})
		lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
		if len(lines) != 3 {
			t.Fatalf("got %d lines, want 3: %q", len(lines), out)
		}

		col0 := valueColumn(t, lines[0])
		col1 := valueColumn(t, lines[1])
		col2 := valueColumn(t, lines[2])
		if col0 != col1 || col1 != col2 {
			t.Errorf("tableText second column not aligned: header %d, row0 %d, row1 %d\n%s", col0, col1, col2, out)
		}
	})
}
