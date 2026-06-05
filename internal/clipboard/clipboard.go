// Package clipboard puts text on the system clipboard via two
// complementary paths so something works regardless of where mterm is
// running.
//
//  1. OSC 52 — emits ESC]52;c;<base64>ESC\ to the supplied writer
//     (typically os.Stdout). The user's terminal emulator parses it
//     and sets the system clipboard. Works on iTerm2, kitty, alacritty,
//     wezterm, foot, recent gnome-terminal, recent Terminal.app, recent
//     xterm. Survives SSH — the sequence streams back through the same
//     PTY the user is looking at.
//  2. Native shell-out — pbcopy (darwin), wl-copy (Wayland), or
//     xclip / xsel (X11). Works when the host running mterm is the
//     same machine the user is on AND the helper is installed.
//
// Either success counts. Failure returned only when both paths fail.
package clipboard

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
)

// Copy puts text on the system clipboard via OSC 52 to w plus a native
// shell-out. Returns nil if either path succeeds.
//
// w receives the OSC 52 escape sequence (callers pass os.Stdout when
// running under Bubble Tea — the renderer treats the bytes as
// out-of-band and the terminal consumes them without rendering).
func Copy(w io.Writer, text string) error {
	osc52Err := emitOSC52(w, text)
	nativeErr := writeNative(text)

	// Either path succeeding is enough. Native is the stronger signal
	// (we can detect the helper's exit code) so prefer it for the
	// "did anything actually land?" question.
	if nativeErr == nil {
		return nil
	}
	if osc52Err == nil {
		return nil
	}
	return fmt.Errorf("clipboard: OSC 52: %v; native: %v", osc52Err, nativeErr)
}

// OSC52Sequence returns the raw bytes a terminal needs to see to set
// its system clipboard to text. Exported for callers that prefer to
// embed the sequence in their own output stream (e.g. a Bubble Tea
// View string) rather than write directly to os.Stdout.
func OSC52Sequence(text string) string {
	encoded := base64.StdEncoding.EncodeToString([]byte(text))
	// ESC ] 52 ; c ; <base64> ESC \
	// 'c' selects the system clipboard. ESC \ (ST) is the canonical
	// string terminator; some terminals also accept BEL (\a) but ST
	// is the spec form.
	return "\x1b]52;c;" + encoded + "\x1b\\"
}

func emitOSC52(w io.Writer, text string) error {
	if w == nil {
		return errors.New("nil writer")
	}
	_, err := io.WriteString(w, OSC52Sequence(text))
	return err
}

// NativeCommand returns the argv slice that, when stdin-piped with
// text, writes text to the system clipboard. Returns nil if no
// supported helper is detectable for the current platform / env.
//
// Order of detection on Linux:
//
//	$WAYLAND_DISPLAY present + wl-copy on PATH → wl-copy
//	$DISPLAY present + xclip on PATH → xclip -selection clipboard
//	$DISPLAY present + xsel on PATH → xsel --clipboard --input
func NativeCommand() []string {
	switch runtime.GOOS {
	case "darwin":
		if _, err := exec.LookPath("pbcopy"); err == nil {
			return []string{"pbcopy"}
		}
	case "linux":
		if os.Getenv("WAYLAND_DISPLAY") != "" {
			if _, err := exec.LookPath("wl-copy"); err == nil {
				return []string{"wl-copy"}
			}
		}
		if os.Getenv("DISPLAY") != "" {
			if _, err := exec.LookPath("xclip"); err == nil {
				return []string{"xclip", "-selection", "clipboard"}
			}
			if _, err := exec.LookPath("xsel"); err == nil {
				return []string{"xsel", "--clipboard", "--input"}
			}
		}
	}
	return nil
}

func writeNative(text string) error {
	argv := NativeCommand()
	if argv == nil {
		return errors.New("no native clipboard helper detected")
	}
	cmd := exec.Command(argv[0], argv[1:]...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("%s: stdin: %w", argv[0], err)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("%s: start: %w", argv[0], err)
	}
	if _, err := io.WriteString(stdin, text); err != nil {
		_ = stdin.Close()
		_ = cmd.Wait()
		return fmt.Errorf("%s: write: %w", argv[0], err)
	}
	if err := stdin.Close(); err != nil {
		_ = cmd.Wait()
		return fmt.Errorf("%s: close: %w", argv[0], err)
	}
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("%s: wait: %w", argv[0], err)
	}
	return nil
}
