package server

import (
	"io"
	"log/slog"
)

// discardLogger returns a *slog.Logger that throws all output away. Useful for
// tests that exercise handlers without polluting the test runner output.
func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
