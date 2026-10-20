package ui

import (
	"fmt"
	"strings"
	"time"
)

func humanize(t time.Time) string {
	if t.IsZero() {
		return "unknown"
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}

// footer returns the keybind hint line for the given mode, shown while no
// confirm prompt is active (the confirm prompt already states its own y/n
// keys).
func footer(mode mode) string {
	switch mode {
	case modeFilter:
		return "enter: confirm filter   esc: cancel"
	default:
		return "j/k: move   gg/G: top/bottom   h/l: focus list/preview   s: sort   /: filter   enter: resume   n: new session   d: delete   u: undo   q: quit"
	}
}

// summarize collapses a multi-line prompt into a single line and truncates
// it to maxLen runes (appending an ellipsis) so it fits a table column.
func summarize(s string, maxLen int) string {
	fields := strings.Fields(s)
	collapsed := strings.Join(fields, " ")
	runes := []rune(collapsed)
	if len(runes) <= maxLen {
		return collapsed
	}
	if maxLen <= 1 {
		return string(runes[:maxLen])
	}
	return string(runes[:maxLen-1]) + "…"
}
