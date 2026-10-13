package ui

import (
	"github.com/RHamers/claudenator/internal/resume"
	"github.com/RHamers/claudenator/internal/session"
)

type confirmKind int

const (
	confirmDelete confirmKind = iota
	confirmResumeConflict
)

type confirmState struct {
	kind   confirmKind
	target session.Session
	pid    int
	plan   *resume.Plan
}
