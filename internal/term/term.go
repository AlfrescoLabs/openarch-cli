// Package term detects terminals.
package term

import "golang.org/x/term"

type fder interface {
	Fd() uintptr
}

// IsTerminal reports whether v is a file connected to a terminal.
func IsTerminal(v any) bool {
	f, ok := v.(fder)
	return ok && term.IsTerminal(int(f.Fd()))
}
