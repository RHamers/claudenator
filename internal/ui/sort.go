package ui

import (
	"sort"

	"github.com/RHamers/claudenator/internal/session"
)

type SortColumn int

const (
	SortByLastActive SortColumn = iota
	SortByDir
	SortByBranch
)

func (c SortColumn) Next() SortColumn {
	return (c + 1) % 3
}

func (c SortColumn) String() string {
	switch c {
	case SortByDir:
		return "Dir"
	case SortByBranch:
		return "Branch"
	default:
		return "Last edited"
	}
}

func sortSessions(sessions []session.Session, col SortColumn) {
	sort.SliceStable(sessions, func(i, j int) bool {
		switch col {
		case SortByDir:
			return sessions[i].Dir < sessions[j].Dir
		case SortByBranch:
			return sessions[i].Branch < sessions[j].Branch
		default:
			return sessions[i].LastActive.After(sessions[j].LastActive)
		}
	})
}
