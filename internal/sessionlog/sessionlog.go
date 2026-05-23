// Package sessionlog writes the byte stream of an SSH session's output to a
// file. One Logger = one session = one file. Concurrency: a single Logger is
// written to by the session's reader goroutine; the package is not safe for
// concurrent Write calls on the same Logger.
package sessionlog

import (
	"bufio"
	"os"
	"path/filepath"
	"time"
)

// Logger appends bytes to a file. Bytes are buffered (default 4 KiB) and
// flushed by Close or when the buffer fills.
type Logger struct {
	file *os.File
	buf  *bufio.Writer
}

// Open creates path's parent directories if needed and opens the file for
// append-write. Returns the Logger or an error.
func Open(path string) (*Logger, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	return &Logger{file: f, buf: bufio.NewWriter(f)}, nil
}

// Write appends p to the buffer. Errors propagate from the underlying file.
func (l *Logger) Write(p []byte) (int, error) {
	return l.buf.Write(p)
}

// Close flushes any buffered output and closes the file. Safe to call once.
func (l *Logger) Close() error {
	if l == nil || l.file == nil {
		return nil
	}
	flushErr := l.buf.Flush()
	closeErr := l.file.Close()
	l.file = nil
	if flushErr != nil {
		return flushErr
	}
	return closeErr
}

// Path returns the file path the logger is writing to. Empty if closed.
func (l *Logger) Path() string {
	if l == nil || l.file == nil {
		return ""
	}
	return l.file.Name()
}

// FileName returns the conventional log filename for a session starting at t.
// Format: YYYYMMDD-HHMMSS.log (UTC). Callers join this with a host directory.
func FileName(t time.Time) string {
	return t.UTC().Format("20060102-150405") + ".log"
}

// PathFor returns the full log file path for host under root, with timestamp
// t. The host segment uses the host's display name (caller supplies); empty
// hosts default to "_unknown".
func PathFor(root, host string, t time.Time) string {
	if host == "" {
		host = "_unknown"
	}
	return filepath.Join(root, host, FileName(t))
}
