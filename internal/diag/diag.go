// Package diag holds small runtime-diagnostic helpers shared across the app:
// goroutine stack dumps, panic capture, etc. Kept separate from the TUI and
// from main so it has no dependency on either.
package diag

import (
	"fmt"
	"os"
	"runtime"
	"time"
)

// DumpGoroutines writes every goroutine's stack trace to a file under /tmp
// named "<appname>-stacks-<pid>-<unix-timestamp>.txt" and returns the path.
// Best-effort: returns ("", err) on filesystem failure.
func DumpGoroutines(appname string) (string, error) {
	buf := make([]byte, 1<<20) // 1 MiB; truncates only for very large dumps.
	n := runtime.Stack(buf, true)
	path := fmt.Sprintf("/tmp/%s-stacks-%d-%d.txt", appname, os.Getpid(), time.Now().Unix())
	if err := os.WriteFile(path, buf[:n], 0o600); err != nil {
		return "", err
	}
	return path, nil
}
