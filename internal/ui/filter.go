package ui

import (
	"github.com/sahilm/fuzzy"

	"github.com/RHamers/claudenator/internal/session"
)

func applyFilter(all []session.Session, query string) []session.Session {
	if query == "" {
		return append([]session.Session(nil), all...)
	}
	sources := make([]string, len(all))
	for i, s := range all {
		sources[i] = s.Dir + " " + s.Branch + " " + s.LastMessage
	}
	matches := fuzzy.Find(query, sources)
	result := make([]session.Session, len(matches))
	for i, match := range matches {
		result[i] = all[match.Index]
	}
	return result
}
