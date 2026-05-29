package git

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// Runner wraps git CLI calls.
type Runner struct {
	RepoRoot string
}

// NewRunner creates a Runner rooted at repoRoot.
func NewRunner(repoRoot string) *Runner {
	return &Runner{RepoRoot: repoRoot}
}

// Run executes a git command in the repo root and returns stdout as a string.
func (r *Runner) Run(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = r.RepoRoot
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %w\nstderr: %s", strings.Join(args, " "), err, stderr.String())
	}
	return stdout.String(), nil
}

// RunOK is like Run but returns an empty string on error (best-effort).
func (r *Runner) RunOK(args ...string) string {
	out, err := r.Run(args...)
	if err != nil {
		return ""
	}
	return out
}
