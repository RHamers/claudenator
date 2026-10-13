package session

// Provider discovers and manages sessions for one coding-agent tool.
// It is the only extension point for supporting tools beyond Claude Code.
type Provider interface {
	// List returns every session this provider can find.
	List() ([]Session, error)
	// ResumeArgv returns the argv that resumes this session as a new process.
	ResumeArgv(s Session) []string
	// Delete removes the session's on-disk file (expected to move it to a
	// trash location, not destroy it outright).
	Delete(s Session) error
}
