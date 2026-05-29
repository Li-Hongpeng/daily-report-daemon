package git

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// MaxDiffChars is the per-file diff character limit to prevent huge diffs from
// bloating model input.
const MaxDiffChars = 8000

// Activity represents the full set of git activity collected for one scan.
type Activity struct {
	RepoRoot string  `json:"repo_root"`
	Branch   string  `json:"branch"`
	Head     string  `json:"head"`
	Remotes  []Remote `json:"remotes"`
	Commits  []Commit `json:"commits"`
	Status   []StatusEntry `json:"status"`
	Diffs    []Diff   `json:"diffs"`
}

// Remote holds a single git remote.
type Remote struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

// Commit holds a single commit from today's log.
type Commit struct {
	Hash    string `json:"hash"`
	Author  string `json:"author"`
	Date    string `json:"date"`
	Subject string `json:"subject"`
}

// StatusEntry mirrors a line from git status --porcelain.
type StatusEntry struct {
	XY   string `json:"xy"`   // the two-character status code
	Path string `json:"path"` // file path
}

// Diff describes a single file's diff.
type Diff struct {
	Scope      string `json:"scope"`       // "staged", "unstaged"
	File       string `json:"file"`
	ChangeType string `json:"change_type"` // "added", "modified", "deleted", "renamed"
	Additions  int    `json:"additions"`
	Deletions  int    `json:"deletions"`
	Patch      string `json:"patch"`
	Truncated  bool   `json:"truncated"`
}

// Analyzer collects git activity from a repo.
type Analyzer struct {
	runner *Runner
}

// NewAnalyzer creates an Analyzer for the given repo root.
func NewAnalyzer(repoRoot string) *Analyzer {
	return &Analyzer{runner: NewRunner(repoRoot)}
}

// FindRepoRoot discovers the git repository root for a given path.
// Returns an empty string if the path is not inside a git repo.
func FindRepoRoot(path string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	cmd.Dir = path
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("not a git repository (or git not installed): %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// Collect gathers all git activity for today.
func (a *Analyzer) Collect() (*Activity, error) {
	act := &Activity{
		RepoRoot: a.runner.RepoRoot,
	}

	// Branch
	act.Branch = strings.TrimSpace(a.runner.RunOK("branch", "--show-current"))

	// HEAD
	act.Head = strings.TrimSpace(a.runner.RunOK("rev-parse", "HEAD"))

	// Remotes
	act.Remotes = a.collectRemotes()

	// Today's commits
	act.Commits = a.collectCommits()

	// Status
	act.Status = a.collectStatus()

	// Diffs (staged and unstaged)
	act.Diffs = a.collectDiffs()

	return act, nil
}

func (a *Analyzer) collectRemotes() []Remote {
	out := a.runner.RunOK("remote", "-v")
	if out == "" {
		return nil
	}
	var remotes []Remote
	seen := map[string]bool{}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		name := parts[0]
		if seen[name] {
			continue
		}
		seen[name] = true
		remotes = append(remotes, Remote{Name: name, URL: parts[1]})
	}
	return remotes
}

func (a *Analyzer) collectCommits() []Commit {
	out := a.runner.RunOK("log", "--since=today 00:00", "--pretty=format:%H%x09%an%x09%ad%x09%s", "--date=iso")
	if out == "" {
		return nil
	}
	var commits []Commit
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 4)
		c := Commit{}
		if len(parts) > 0 {
			c.Hash = parts[0]
		}
		if len(parts) > 1 {
			c.Author = parts[1]
		}
		if len(parts) > 2 {
			c.Date = parts[2]
		}
		if len(parts) > 3 {
			c.Subject = parts[3]
		}
		commits = append(commits, c)
	}
	return commits
}

func (a *Analyzer) collectStatus() []StatusEntry {
	out := a.runner.RunOK("status", "--porcelain")
	if out == "" {
		return nil
	}
	var entries []StatusEntry
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if len(line) < 3 {
			continue
		}
		entries = append(entries, StatusEntry{
			XY:   line[:2],
			Path: strings.TrimSpace(line[2:]),
		})
	}
	return entries
}

func (a *Analyzer) collectDiffs() []Diff {
	var diffs []Diff
	diffs = append(diffs, a.collectDiffScope("staged", "--cached")...)
	diffs = append(diffs, a.collectDiffScope("unstaged", "")...)
	// Untracked files: read whole file content as "added" patch
	diffs = append(diffs, a.collectUntrackedDiffs()...)
	return diffs
}

func (a *Analyzer) collectUntrackedDiffs() []Diff {
	var diffs []Diff
	for _, se := range a.collectStatus() {
		if se.XY != "??" {
			continue
		}
		// Only include text files
		ext := strings.ToLower(filepath.Ext(se.Path))
		if !isTextExtension(ext) {
			continue
		}
		content, err := os.ReadFile(filepath.Join(a.runner.RepoRoot, se.Path))
		if err != nil {
			continue
		}
		patch := string(content)
		lines := strings.Count(patch, "\n")
		truncated := false
		if len(patch) > MaxDiffChars {
			patch = patch[:MaxDiffChars] + "\n\n... [content truncated, original length: " + fmt.Sprintf("%d", len(patch)) + " chars]"
			truncated = true
		}
		diffs = append(diffs, Diff{
			Scope:      "unstaged",
			File:       se.Path,
			ChangeType: "added",
			Additions:  lines,
			Deletions:  0,
			Patch:      patch,
			Truncated:  truncated,
		})
	}
	return diffs
}

func (a *Analyzer) collectDiffScope(scope string, extraArg string) []Diff {
	args := []string{"diff", "--numstat"}
	if extraArg != "" {
		args = append(args, extraArg)
	}
	numstatOut := a.runner.RunOK(args...)
	if numstatOut == "" {
		return nil
	}

	var diffs []Diff
	for _, line := range strings.Split(numstatOut, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 3 {
			continue
		}
		add, del, file := parts[0], parts[1], parts[2]
		// Binary files show "-" for both sides; skip them for patch collection.
		isBinary := add == "-" && del == "-"
		additions := 0
		deletions := 0
		if !isBinary {
			additions = parseIntOrZero(add)
			deletions = parseIntOrZero(del)
		}

		patch, truncated := a.collectPatch(file, scope, extraArg, isBinary)

		diffs = append(diffs, Diff{
			Scope:      scope,
			File:       file,
			ChangeType: detectChangeType(add, del),
			Additions:  additions,
			Deletions:  deletions,
			Patch:      patch,
			Truncated:  truncated,
		})
	}
	return diffs
}

func (a *Analyzer) collectPatch(file, scope, extraArg string, isBinary bool) (patch string, truncated bool) {
	if isBinary {
		return "[binary file]", false
	}
	args := []string{"diff"}
	if extraArg != "" {
		args = append(args, extraArg)
	}
	args = append(args, "--", file)
	out := a.runner.RunOK(args...)
	if len(out) > MaxDiffChars {
		return out[:MaxDiffChars] + "\n\n... [diff truncated, original length: " + fmt.Sprintf("%d", len(out)) + " chars]", true
	}
	return out, false
}

// detectChangeType derives the change type from numstat values.
func detectChangeType(additions, deletions string) string {
	if additions == "0" && deletions == "0" {
		return "renamed"
	}
	if additions == "0" && deletions != "0" {
		return "deleted"
	}
	if additions != "0" && deletions == "0" {
		return "added"
	}
	return "modified"
}

func parseIntOrZero(s string) int {
	// - means binary; treat as 0
	if s == "-" {
		return 0
	}
	var n int
	fmt.Sscanf(s, "%d", &n)
	return n
}

// IsRepo returns true if the given path is inside a git repository.
func IsRepo(path string) bool {
	_, err := FindRepoRoot(path)
	return err == nil
}

// UntrackedTextFiles returns paths of untracked files that appear to be text
// (based on extension).
func (a *Analyzer) UntrackedTextFiles() []string {
	out := a.runner.RunOK("ls-files", "--others", "--exclude-standard")
	if out == "" {
		return nil
	}
	var files []string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		ext := strings.ToLower(filepath.Ext(line))
		if isTextExtension(ext) {
			files = append(files, line)
		}
	}
	return files
}

func isTextExtension(ext string) bool {
	switch ext {
	case ".go", ".ts", ".tsx", ".js", ".jsx", ".py", ".rs", ".c", ".cpp",
		".h", ".hpp", ".java", ".rb", ".php", ".swift", ".kt", ".scala",
		".md", ".txt", ".yaml", ".yml", ".toml", ".json", ".xml", ".csv",
		".cfg", ".ini", ".env", ".sh", ".bash", ".zsh", ".fish",
		".sql", ".proto", ".graphql", ".css", ".scss", ".less", ".html":
		return true
	}
	return false
}

// SaveActivity writes the Activity as JSON to the given path.
func SaveActivity(act *Activity, path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}
	// Marshal to JSON
	data, err := marshalActivity(act)
	if err != nil {
		return fmt.Errorf("marshal activity: %w", err)
	}
	return os.WriteFile(path, data, 0644)
}
