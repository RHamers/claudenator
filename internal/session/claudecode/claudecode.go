package claudecode

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/RHamers/claudenator/internal/session"
	"github.com/RHamers/claudenator/internal/trash"
)

const ProviderName = "claude-code"

// Provider discovers Claude Code sessions under a ~/.claude/projects-style
// directory tree: <projectsRoot>/<escaped-cwd>/<session-id>.jsonl.
type Provider struct {
	ProjectsRoot string
	Trash        trash.Store
}

func New(projectsRoot string, t trash.Store) *Provider {
	return &Provider{ProjectsRoot: projectsRoot, Trash: t}
}

type rawLine struct {
	CWD       string      `json:"cwd"`
	Timestamp string      `json:"timestamp"`
	GitBranch string      `json:"gitBranch"`
	SessionID string      `json:"sessionId"`
	Message   *rawMessage `json:"message"`
}

type rawMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type rawContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func extractText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		return asString
	}
	var blocks []rawContentBlock
	if err := json.Unmarshal(raw, &blocks); err == nil {
		var parts []string
		for _, b := range blocks {
			if b.Text != "" {
				parts = append(parts, b.Text)
			}
		}
		return strings.Join(parts, " ")
	}
	return ""
}

func parseFile(path string) (session.Session, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return session.Session{}, err
	}
	trimmed := strings.TrimRight(string(data), "\n")
	if trimmed == "" {
		return session.Session{}, fmt.Errorf("claudecode: %s is empty", path)
	}

	var (
		dir, branch, lastMsg, sessionID string
		lastActive                      time.Time
		sawCWD                          bool
	)

	for _, line := range strings.Split(trimmed, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var raw rawLine
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			continue // skip corrupt/truncated line, keep scanning
		}
		if raw.SessionID != "" {
			sessionID = raw.SessionID
		}
		if raw.CWD != "" {
			dir = raw.CWD
			sawCWD = true
		}
		if raw.GitBranch != "" {
			branch = raw.GitBranch
		}
		if raw.Timestamp != "" {
			if t, err := time.Parse(time.RFC3339, raw.Timestamp); err == nil {
				lastActive = t
			}
		}
		if raw.Message != nil && raw.Message.Role == "user" {
			if text := extractText(raw.Message.Content); text != "" {
				lastMsg = text
			}
		}
	}

	if !sawCWD {
		return session.Session{}, fmt.Errorf("claudecode: no cwd found in %s", path)
	}
	if sessionID == "" {
		sessionID = strings.TrimSuffix(filepath.Base(path), ".jsonl")
	}

	return session.Session{
		ID:          sessionID,
		Provider:    ProviderName,
		Dir:         dir,
		Branch:      branch,
		LastActive:  lastActive,
		LastMessage: lastMsg,
		FilePath:    path,
	}, nil
}

func (p *Provider) List() ([]session.Session, error) {
	matches, err := filepath.Glob(filepath.Join(p.ProjectsRoot, "*", "*.jsonl"))
	if err != nil {
		return nil, err
	}
	var sessions []session.Session
	for _, path := range matches {
		s, err := parseFile(path)
		if err != nil {
			continue // unreadable, empty, or missing cwd — not listable
		}
		sessions = append(sessions, s)
	}
	return sessions, nil
}

func (p *Provider) ResumeArgv(s session.Session) []string {
	return []string{"claude", "--resume", s.ID}
}

func (p *Provider) Delete(s session.Session) error {
	return p.Trash.Move(s)
}
