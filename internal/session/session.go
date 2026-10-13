package session

import "time"

// Session represents a single coding-agent session discovered on disk.
type Session struct {
	ID           string    // unique session identifier (provider-specific format)
	Provider     string    // e.g. "claude-code"
	Dir          string    // working directory the session ran in
	Branch       string    // git branch checked out when the session was active
	LastActive   time.Time // timestamp of the most recent event in the session
	LastMessage  string    // preview text of the last user message
	FirstMessage string    // preview text of the first user message (the session's opening prompt)
	Running      bool      // true if a resume process for this session is currently active
	FilePath     string    // absolute path to the session's on-disk file
}
