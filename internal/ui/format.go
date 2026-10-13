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
