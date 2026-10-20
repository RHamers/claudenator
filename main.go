package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/RHamers/claudenator/internal/procscan"
	"github.com/RHamers/claudenator/internal/session"
	"github.com/RHamers/claudenator/internal/session/claudecode"
	"github.com/RHamers/claudenator/internal/trash"
	"github.com/RHamers/claudenator/internal/ui"
)

// version is stamped at build time via -ldflags "-X main.version=<sha>" for
// release binaries; it stays "dev" for local go build/go install.
var version = "dev"

// versionRequested reports whether args asks to print the version instead
// of launching the TUI.
func versionRequested(args []string) bool {
	return len(args) > 0 && (args[0] == "--version" || args[0] == "-v")
}

func buildProviders(home string) (map[string]session.Provider, trash.Store) {
	trashStore := trash.Store{Dir: filepath.Join(home, ".claudenator", "trash")}
	projectsRoot := filepath.Join(home, ".claude", "projects")
	providers := map[string]session.Provider{
		claudecode.ProviderName: claudecode.New(projectsRoot, trashStore),
	}
	return providers, trashStore
}

func aggregateSessions(providers map[string]session.Provider) ([]session.Session, error) {
	var all []session.Session
	var errs []error
	for _, p := range providers {
		sessions, err := p.List()
		if err != nil {
			errs = append(errs, err)
			continue
		}
		all = append(all, sessions...)
	}
	if len(errs) > 0 {
		return all, errors.Join(errs...)
	}
	return all, nil
}

func main() {
	if versionRequested(os.Args[1:]) {
		fmt.Println(version)
		return
	}

	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	launchDir, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	providers, trashStore := buildProviders(home)

	if err := trashStore.Prune(30*24*time.Hour, time.Now()); err != nil {
		fmt.Fprintln(os.Stderr, "warning: trash prune failed:", err)
	}

	sessions, err := aggregateSessions(providers)
	if err != nil {
		fmt.Fprintln(os.Stderr, "warning: some sessions could not be listed:", err)
	}

	findRunning := func(id string) (int, bool) {
		return procscan.FindResumeProcess("/proc", id)
	}

	for i := range sessions {
		_, running := findRunning(sessions[i].ID)
		sessions[i].Running = running
	}

	m := ui.New(sessions, providers, trashStore, findRunning, launchDir)
	p := tea.NewProgram(m, tea.WithAltScreen())
	finalModel, err := p.Run()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	final, ok := finalModel.(ui.Model)
	if !ok {
		return
	}
	plan := final.PendingResume()
	if plan == nil {
		return
	}

	if err := os.Chdir(plan.Dir); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	bin, err := exec.LookPath(plan.Argv[0])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := syscall.Exec(bin, plan.Argv, os.Environ()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
