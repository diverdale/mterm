package tui

// spinnerFrames are the braille spinner glyphs, in animation order.
var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// spinnerGlyph returns the spinner glyph for the given render-tick counter. The
// counter increments once per render tick (~33ms); the glyph advances every 3
// ticks (~100ms) so the animation is legible rather than a blur.
func spinnerGlyph(counter int) string {
	if counter < 0 {
		counter = 0
	}
	return spinnerFrames[(counter/3)%len(spinnerFrames)]
}
