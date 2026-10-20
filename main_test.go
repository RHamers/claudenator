package main

import (
	"testing"

	"github.com/RHamers/claudenator/internal/session"
)

type fakeAggregateProvider struct {
	sessions []session.Session
	err      error
}

func (p fakeAggregateProvider) List() ([]session.Session, error) { return p.sessions, p.err }
func (p fakeAggregateProvider) ResumeArgv(s session.Session) []string {
	return []string{"claude", "--resume", s.ID}
}
func (p fakeAggregateProvider) Delete(s session.Session) error { return nil }

func TestAggregateSessionsMergesProviders(t *testing.T) {
	providers := map[string]session.Provider{
		"a": fakeAggregateProvider{sessions: []session.Session{{ID: "1"}}},
		"b": fakeAggregateProvider{sessions: []session.Session{{ID: "2"}}},
	}
	sessions, err := aggregateSessions(providers)
	if err != nil {
		t.Fatalf("aggregateSessions: %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(sessions))
	}
}

func TestAggregateSessionsReturnsPartialResultsOnProviderError(t *testing.T) {
	providers := map[string]session.Provider{
		"a": fakeAggregateProvider{sessions: []session.Session{{ID: "1"}}},
		"b": fakeAggregateProvider{err: errTestProvider},
	}
	sessions, err := aggregateSessions(providers)
	if err == nil {
		t.Fatalf("expected an error from the failing provider")
	}
	if len(sessions) != 1 {
		t.Fatalf("expected the successful provider's session to still be returned, got %d", len(sessions))
	}
}

func TestBuildProvidersRegistersClaudeCode(t *testing.T) {
	providers, _ := buildProviders(t.TempDir())
	if _, ok := providers["claude-code"]; !ok {
		t.Fatalf("expected claude-code provider registered")
	}
}

func TestVersionRequestedRecognizesFlag(t *testing.T) {
	if !versionRequested([]string{"--version"}) {
		t.Fatalf("expected --version to be recognized")
	}
	if !versionRequested([]string{"-v"}) {
		t.Fatalf("expected -v to be recognized")
	}
}

func TestVersionRequestedFalseForOtherArgs(t *testing.T) {
	if versionRequested(nil) {
		t.Fatalf("expected no args to not request the version")
	}
	if versionRequested([]string{"--help"}) {
		t.Fatalf("expected an unrelated flag to not request the version")
	}
}

var errTestProvider = fmtErrorf("boom")

func fmtErrorf(msg string) error {
	return &testErr{msg}
}

type testErr struct{ msg string }

func (e *testErr) Error() string { return e.msg }
