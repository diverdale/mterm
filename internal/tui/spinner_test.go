package tui

import "testing"

func TestSpinnerGlyphCycles(t *testing.T) {
	// Frame advances every 3 ticks; 10 glyphs total.
	first := spinnerGlyph(0)
	if first == "" {
		t.Fatal("spinnerGlyph(0) is empty")
	}
	// Same glyph for ticks 0,1,2; advances at tick 3.
	if spinnerGlyph(1) != first || spinnerGlyph(2) != first {
		t.Fatal("spinner must hold a glyph for 3 ticks")
	}
	if spinnerGlyph(3) == first {
		t.Fatal("spinner must advance on the 3rd tick")
	}
	// Wraps after 10 glyphs (30 ticks).
	if spinnerGlyph(30) != first {
		t.Fatalf("spinner must wrap after 10 glyphs: glyph(30)=%q first=%q", spinnerGlyph(30), first)
	}
}

func TestSpinnerHandlesNegative(t *testing.T) {
	// Must not panic on a zero/negative counter.
	_ = spinnerGlyph(-1)
}
