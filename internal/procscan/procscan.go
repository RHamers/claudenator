// Package procscan detects a running `claude --resume <id>` process by
// reading /proc/*/cmdline, so the UI can warn before double-attaching to
// a session that's already open elsewhere.
package procscan

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// FindResumeProcess scans procRoot (normally "/proc") for a process whose
// argv contains both "--resume" and sessionID. It returns found=false,
// rather than an error, if procRoot can't be read — the guard is a
// best-effort safety check, not a hard dependency.
func FindResumeProcess(procRoot, sessionID string) (pid int, found bool) {
	entries, err := os.ReadDir(procRoot)
	if err != nil {
		return 0, false
	}
	for _, e := range entries {
		candidatePID, err := strconv.Atoi(e.Name())
		if err != nil {
			continue // not a PID directory
		}
		data, err := os.ReadFile(filepath.Join(procRoot, e.Name(), "cmdline"))
		if err != nil {
			continue
		}
		args := strings.Split(strings.TrimRight(string(data), "\x00"), "\x00")
		hasResume := false
		hasSessionID := false
		for _, a := range args {
			if a == "--resume" {
				hasResume = true
			}
			if a == sessionID {
				hasSessionID = true
			}
		}
		if hasResume && hasSessionID {
			return candidatePID, true
		}
	}
	return 0, false
}
