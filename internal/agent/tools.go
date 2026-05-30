package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	"github.com/daily-report-daemon/internal/evidence"
	"github.com/daily-report-daemon/internal/sanitize"
	"github.com/daily-report-daemon/internal/scanner"
	"github.com/daily-report-daemon/internal/store"
)

const maxToolOutput = 8192

// MemoryStore is the SQLite-backed memory surface used by agent tools.
type MemoryStore interface {
	GetAgentMemory(key string) (*store.AgentMemoryRow, error)
	SetAgentMemory(id, key, value string) error
}

// ToolSet exposes the read-only workspace tools and scoped memory tools.
type ToolSet struct {
	RepoRoot          string
	Workspace         WorkspaceRef
	Evidence          []evidence.Item
	Metadata          *scanner.ProjectMetadata
	Store             MemoryStore
	AllowedNamespaces []string
	ToolTimeout       time.Duration

	san          *sanitize.Sanitizer
	mu           sync.Mutex
	observations []ToolObservation
}

func NewWorkspaceToolSet(repoRoot string, ws WorkspaceRef, meta *scanner.ProjectMetadata, items []evidence.Item, st MemoryStore, timeout time.Duration) *ToolSet {
	prefix := "workspace:" + ws.ID + ":"
	return &ToolSet{
		RepoRoot:    repoRoot,
		Workspace:   ws,
		Evidence:    items,
		Metadata:    meta,
		Store:       st,
		ToolTimeout: timeout,
		AllowedNamespaces: []string{
			prefix + "facts",
			prefix + "risks",
			prefix + "unfinished",
			prefix + "context_rules",
		},
		san: sanitize.New(),
	}
}

func NewSupervisorToolSet(repoRoot string, ws WorkspaceRef, st MemoryStore, timeout time.Duration) *ToolSet {
	return &ToolSet{
		RepoRoot:    repoRoot,
		Workspace:   ws,
		Store:       st,
		ToolTimeout: timeout,
		AllowedNamespaces: []string{
			"global:daily_themes",
			"global:weekly_themes",
			"global:cross_workspace_risks",
		},
		san: sanitize.New(),
	}
}

func (ts *ToolSet) Tools() []einotool.BaseTool {
	return []einotool.BaseTool{
		ts.tool("read_evidence_index", "Read the sanitized evidence index for this workspace. Use it before drawing conclusions.", map[string]*schema.ParameterInfo{
			"limit": {Type: schema.Integer, Desc: "Maximum evidence items to return.", Required: false},
		}, ts.readEvidenceIndex),
		ts.tool("git_diff_detail", "Read sanitized git diff details for one relative file path.", map[string]*schema.ParameterInfo{
			"path": {Type: schema.String, Desc: "Relative file path inside the workspace.", Required: true},
		}, ts.gitDiffDetail),
		ts.tool("git_log_explore", "Explore recent git history for one relative file path or the workspace root.", map[string]*schema.ParameterInfo{
			"path": {Type: schema.String, Desc: "Relative file path. Use . for repository-level history.", Required: false},
		}, ts.gitLogExplore),
		ts.tool("read_file", "Read a sanitized project file inside this workspace. Never reads daemon data or sensitive paths.", map[string]*schema.ParameterInfo{
			"path": {Type: schema.String, Desc: "Relative file path inside the workspace.", Required: true},
		}, ts.readFile),
		ts.tool("search_pattern", "Search text files for a pattern inside the workspace. Defaults to TODO/FIXME/HACK/XXX.", map[string]*schema.ParameterInfo{
			"path":        {Type: schema.String, Desc: "Relative directory path to search.", Required: false},
			"pattern":     {Type: schema.String, Desc: "Regular expression. Defaults to TODO|FIXME|HACK|XXX.", Required: false},
			"max_matches": {Type: schema.Integer, Desc: "Maximum matches to return.", Required: false},
		}, ts.searchPattern),
		ts.tool("list_directory", "List a workspace directory without entering daemon or sensitive paths.", map[string]*schema.ParameterInfo{
			"path": {Type: schema.String, Desc: "Relative directory path inside the workspace.", Required: false},
		}, ts.listDirectory),
		ts.tool("read_agent_memory", "Read scoped agent memory. Workspace agents may only read their own workspace namespaces; Supervisor may only read global namespaces.", map[string]*schema.ParameterInfo{
			"namespace": {Type: schema.String, Desc: "Allowed memory namespace.", Required: true},
			"key":       {Type: schema.String, Desc: "Memory key.", Required: true},
		}, ts.readAgentMemory),
		ts.tool("write_agent_memory", "Write scoped agent memory as structured JSON or concise text. This is the only write-capable tool.", map[string]*schema.ParameterInfo{
			"namespace": {Type: schema.String, Desc: "Allowed memory namespace.", Required: true},
			"key":       {Type: schema.String, Desc: "Memory key.", Required: true},
			"value":     {Type: schema.String, Desc: "Memory value. Do not include secrets.", Required: true},
		}, ts.writeAgentMemory),
		ts.tool("summarize_workspace_state", "Summarize workspace structure, languages, commands, and evidence counts.", nil, ts.summarizeWorkspaceState),
	}
}

func (ts *ToolSet) Observations() []ToolObservation {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	return append([]ToolObservation(nil), ts.observations...)
}

func (ts *ToolSet) InvokeForFallback(ctx context.Context, name, args string) ToolObservation {
	for _, t := range ts.Tools() {
		info, err := t.Info(ctx)
		if err != nil || info.Name != name {
			continue
		}
		invokable, ok := t.(einotool.InvokableTool)
		if !ok {
			break
		}
		out, err := invokable.InvokableRun(ctx, args)
		obs := ToolObservation{Tool: name, Args: args, Output: truncate(out, maxToolOutput)}
		if err != nil {
			obs.Error = err.Error()
			obs.Output = ""
		}
		return obs
	}
	return ToolObservation{Tool: name, Args: args, Error: "tool not found"}
}

type simpleTool struct {
	info *schema.ToolInfo
	run  func(context.Context, string) (string, error)
}

func (t *simpleTool) Info(context.Context) (*schema.ToolInfo, error) {
	return t.info, nil
}

func (t *simpleTool) InvokableRun(ctx context.Context, argumentsInJSON string, opts ...einotool.Option) (string, error) {
	return t.run(ctx, argumentsInJSON)
}

func (ts *ToolSet) tool(name, desc string, params map[string]*schema.ParameterInfo, run func(context.Context, string) (string, error)) einotool.BaseTool {
	info := &schema.ToolInfo{Name: name, Desc: desc}
	if params != nil {
		info.ParamsOneOf = schema.NewParamsOneOfByParams(params)
	}
	return &simpleTool{
		info: info,
		run: func(ctx context.Context, args string) (string, error) {
			if ts.ToolTimeout > 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, ts.ToolTimeout)
				defer cancel()
			}
			out, err := run(ctx, args)
			ts.record(name, args, out, err)
			return out, err
		},
	}
}

func (ts *ToolSet) readEvidenceIndex(ctx context.Context, args string) (string, error) {
	var in struct {
		Limit int `json:"limit"`
	}
	_ = json.Unmarshal([]byte(args), &in)
	if in.Limit <= 0 || in.Limit > 50 {
		in.Limit = 50
	}
	type item struct {
		ID      string `json:"id"`
		Type    string `json:"type"`
		Path    string `json:"path,omitempty"`
		Summary string `json:"summary"`
		Source  string `json:"source,omitempty"`
	}
	out := make([]item, 0, minInt(len(ts.Evidence), in.Limit))
	for _, ev := range ts.Evidence {
		if len(out) >= in.Limit {
			break
		}
		if ts.san.CheckPath(ev.Path) || blockedRelPath(ev.Path) {
			continue
		}
		out = append(out, item{
			ID:      ev.ID,
			Type:    string(ev.Type),
			Path:    ev.Path,
			Summary: ts.sanitizeText(ev.Path, ev.Summary),
			Source:  ev.Source,
		})
	}
	return jsonString(out)
}

func (ts *ToolSet) gitDiffDetail(ctx context.Context, args string) (string, error) {
	var in struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(args), &in); err != nil {
		return "", fmt.Errorf("parse args: %w", err)
	}
	_, rel, err := ts.safePath(in.Path)
	if err != nil {
		return "", err
	}
	out, err := ts.git(ctx, "diff", "HEAD", "--", rel)
	if err != nil || strings.TrimSpace(out) == "" {
		out, err = ts.git(ctx, "diff", "--", rel)
	}
	if err != nil {
		return "", err
	}
	return truncate(ts.sanitizeText(rel, out), maxToolOutput), nil
}

func (ts *ToolSet) gitLogExplore(ctx context.Context, args string) (string, error) {
	var in struct {
		Path string `json:"path"`
	}
	_ = json.Unmarshal([]byte(args), &in)
	if strings.TrimSpace(in.Path) == "" {
		in.Path = "."
	}
	_, rel, err := ts.safePath(in.Path)
	if err != nil {
		return "", err
	}
	out, err := ts.git(ctx, "log", "--oneline", "--decorate", "--follow", "-n", "12", "--", rel)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(out) == "" {
		out = "(no recent commits)"
	}
	return truncate(ts.sanitizeText(rel, out), maxToolOutput), nil
}

func (ts *ToolSet) readFile(ctx context.Context, args string) (string, error) {
	var in struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(args), &in); err != nil {
		return "", fmt.Errorf("parse args: %w", err)
	}
	abs, rel, err := ts.safePath(in.Path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", fmt.Errorf("%s is a directory", rel)
	}
	if info.Size() > 512*1024 {
		return "", fmt.Errorf("%s is too large to read through agent tool", rel)
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return "", err
	}
	if looksBinary(data) {
		return "", fmt.Errorf("%s appears to be binary", rel)
	}
	return truncate(ts.sanitizeText(rel, string(data)), maxToolOutput), nil
}

func (ts *ToolSet) searchPattern(ctx context.Context, args string) (string, error) {
	var in struct {
		Path       string `json:"path"`
		Pattern    string `json:"pattern"`
		MaxMatches int    `json:"max_matches"`
	}
	_ = json.Unmarshal([]byte(args), &in)
	if strings.TrimSpace(in.Path) == "" {
		in.Path = "."
	}
	if strings.TrimSpace(in.Pattern) == "" {
		in.Pattern = `TODO|FIXME|HACK|XXX`
	}
	if len(in.Pattern) > 120 {
		return "", fmt.Errorf("pattern is too long")
	}
	if in.MaxMatches <= 0 || in.MaxMatches > 100 {
		in.MaxMatches = 50
	}
	re, err := regexp.Compile(in.Pattern)
	if err != nil {
		return "", err
	}
	abs, _, err := ts.safePath(in.Path)
	if err != nil {
		return "", err
	}
	var matches []string
	err = filepath.WalkDir(abs, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		rel, relErr := filepath.Rel(ts.RepoRoot, path)
		if relErr != nil {
			return nil
		}
		if d.IsDir() {
			if rel != "." && blockedRelPath(rel) {
				return filepath.SkipDir
			}
			return nil
		}
		if blockedRelPath(rel) || ts.san.CheckPath(rel) || skipToolFile(path, d) {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil || looksBinary(data) {
			return nil
		}
		for i, line := range strings.Split(string(data), "\n") {
			if re.MatchString(line) {
				matches = append(matches, fmt.Sprintf("%s:%d: %s", filepath.ToSlash(rel), i+1, ts.sanitizeText(rel, strings.TrimSpace(line))))
				if len(matches) >= in.MaxMatches {
					return filepath.SkipAll
				}
			}
		}
		return nil
	})
	if err != nil && err != filepath.SkipAll {
		return "", err
	}
	if len(matches) == 0 {
		return "(no matches)", nil
	}
	return truncate(strings.Join(matches, "\n"), maxToolOutput), nil
}

func (ts *ToolSet) listDirectory(ctx context.Context, args string) (string, error) {
	var in struct {
		Path string `json:"path"`
	}
	_ = json.Unmarshal([]byte(args), &in)
	if strings.TrimSpace(in.Path) == "" {
		in.Path = "."
	}
	abs, _, err := ts.safePath(in.Path)
	if err != nil {
		return "", err
	}
	entries, err := os.ReadDir(abs)
	if err != nil {
		return "", err
	}
	var lines []string
	for _, entry := range entries {
		name := entry.Name()
		if blockedRelPath(name) || ts.san.CheckPath(name) {
			continue
		}
		line := name
		if entry.IsDir() {
			line += "/"
		} else if info, err := entry.Info(); err == nil {
			line += fmt.Sprintf(" (%d bytes)", info.Size())
		}
		lines = append(lines, line)
	}
	sort.Strings(lines)
	if len(lines) == 0 {
		return "(empty)", nil
	}
	return truncate(strings.Join(lines, "\n"), maxToolOutput), nil
}

func (ts *ToolSet) readAgentMemory(ctx context.Context, args string) (string, error) {
	if ts.Store == nil {
		return "{}", nil
	}
	var in struct {
		Namespace string `json:"namespace"`
		Key       string `json:"key"`
	}
	if err := json.Unmarshal([]byte(args), &in); err != nil {
		return "", fmt.Errorf("parse args: %w", err)
	}
	key, err := ts.memoryKey(in.Namespace, in.Key)
	if err != nil {
		return "", err
	}
	row, err := ts.Store.GetAgentMemory(key)
	if err != nil {
		return "", err
	}
	if row == nil {
		return "{}", nil
	}
	return row.Value, nil
}

func (ts *ToolSet) writeAgentMemory(ctx context.Context, args string) (string, error) {
	if ts.Store == nil {
		return `{"status":"skipped","reason":"memory store unavailable"}`, nil
	}
	var in struct {
		Namespace string `json:"namespace"`
		Key       string `json:"key"`
		Value     string `json:"value"`
	}
	if err := json.Unmarshal([]byte(args), &in); err != nil {
		return "", fmt.Errorf("parse args: %w", err)
	}
	key, err := ts.memoryKey(in.Namespace, in.Key)
	if err != nil {
		return "", err
	}
	value := ts.sanitizeText("agent-memory", in.Value)
	if len(value) > 4096 {
		value = value[:4096]
	}
	id := fmt.Sprintf("mem:%x", fnv32(key))
	if err := ts.Store.SetAgentMemory(id, key, value); err != nil {
		return "", err
	}
	return `{"status":"ok"}`, nil
}

func (ts *ToolSet) summarizeWorkspaceState(ctx context.Context, args string) (string, error) {
	type summary struct {
		Workspace      WorkspaceRef       `json:"workspace"`
		Languages      []scanner.LangStat `json:"languages,omitempty"`
		BuildCommands  []string           `json:"build_commands,omitempty"`
		TestCommands   []string           `json:"test_commands,omitempty"`
		TopLevel       []string           `json:"top_level,omitempty"`
		EvidenceCounts map[string]int     `json:"evidence_counts,omitempty"`
	}
	out := summary{
		Workspace:      ts.Workspace,
		EvidenceCounts: map[string]int{},
	}
	if ts.Metadata != nil {
		out.Languages = ts.Metadata.Languages
		for _, cmd := range ts.Metadata.BuildCommands {
			out.BuildCommands = append(out.BuildCommands, cmd.Command)
		}
		for _, cmd := range ts.Metadata.TestCommands {
			out.TestCommands = append(out.TestCommands, cmd.Command)
		}
		for _, entry := range ts.Metadata.Structure {
			name := entry.Name
			if entry.Type == "dir" {
				name += "/"
			}
			if !blockedRelPath(name) && !ts.san.CheckPath(name) {
				out.TopLevel = append(out.TopLevel, name)
			}
		}
	}
	for _, ev := range ts.Evidence {
		out.EvidenceCounts[string(ev.Type)]++
	}
	return jsonString(out)
}

func (ts *ToolSet) safePath(p string) (string, string, error) {
	if strings.TrimSpace(p) == "" {
		p = "."
	}
	root, err := filepath.Abs(ts.RepoRoot)
	if err != nil {
		return "", "", err
	}
	var abs string
	if filepath.IsAbs(p) {
		abs = filepath.Clean(p)
	} else {
		abs = filepath.Clean(filepath.Join(root, p))
	}
	abs, err = filepath.Abs(abs)
	if err != nil {
		return "", "", err
	}
	rel, err := filepath.Rel(root, abs)
	if err != nil {
		return "", "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", "", fmt.Errorf("path traversal blocked")
	}
	rel = filepath.ToSlash(rel)
	if blockedRelPath(rel) {
		return "", "", fmt.Errorf("blocked internal path: %s", rel)
	}
	if ts.san.CheckPath(rel) {
		return "", "", fmt.Errorf("blocked sensitive path: %s", rel)
	}
	return abs, rel, nil
}

func (ts *ToolSet) memoryKey(namespace, key string) (string, error) {
	namespace = strings.TrimSpace(namespace)
	key = strings.TrimSpace(key)
	if namespace == "" || key == "" {
		return "", fmt.Errorf("namespace and key are required")
	}
	allowed := false
	for _, ns := range ts.AllowedNamespaces {
		if namespace == ns {
			allowed = true
			break
		}
	}
	if !allowed {
		return "", fmt.Errorf("memory namespace not allowed: %s", namespace)
	}
	if strings.Contains(key, "..") || strings.ContainsAny(key, "/\\") {
		return "", fmt.Errorf("invalid memory key")
	}
	return namespace + ":" + key, nil
}

func (ts *ToolSet) git(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = ts.RepoRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, truncate(string(out), 300))
	}
	return string(out), nil
}

func (ts *ToolSet) record(tool, args, output string, err error) {
	obs := ToolObservation{Tool: tool, Args: truncate(args, 500), Output: truncate(output, maxToolOutput)}
	if err != nil {
		obs.Error = err.Error()
		obs.Output = ""
	}
	ts.mu.Lock()
	defer ts.mu.Unlock()
	if len(ts.observations) < 100 {
		ts.observations = append(ts.observations, obs)
	}
}

func (ts *ToolSet) sanitizeText(path, text string) string {
	if ts.san == nil {
		ts.san = sanitize.New()
	}
	return ts.san.Redact(path, text)
}

func blockedRelPath(path string) bool {
	path = filepath.ToSlash(filepath.Clean(path))
	if path == "." {
		return false
	}
	for _, part := range strings.Split(path, "/") {
		switch strings.ToLower(part) {
		case ".daily-report-daemon", ".git":
			return true
		}
	}
	return false
}

func skipToolFile(path string, d os.DirEntry) bool {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".png", ".jpg", ".jpeg", ".gif", ".pdf", ".gz", ".zip", ".tar", ".mp4", ".mov", ".ico":
		return true
	}
	if info, err := d.Info(); err == nil && info.Size() > 512*1024 {
		return true
	}
	return false
}

func looksBinary(data []byte) bool {
	limit := minInt(len(data), 4096)
	for i := 0; i < limit; i++ {
		if data[i] == 0 {
			return true
		}
	}
	return false
}

func jsonString(v any) (string, error) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + fmt.Sprintf("\n... [truncated at %d bytes]", maxLen)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
