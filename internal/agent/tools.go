package agent

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// Tool represents an agent-callable tool.
type Tool struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// ToolCall is a request to invoke a tool.
type ToolCall struct {
	Tool string `json:"tool"`
	Args string `json:"args"`
}

// ToolResult is the result of a tool invocation.
type ToolResult struct {
	Tool    string `json:"tool"`
	Args    string `json:"args"`
	Output  string `json:"output"`
	Error   string `json:"error,omitempty"`
}

// ToolRegistry manages available tools.
type ToolRegistry struct {
	repoRoot string
}

// NewToolRegistry creates a tool registry for the given repo.
func NewToolRegistry(repoRoot string) *ToolRegistry {
	return &ToolRegistry{repoRoot: repoRoot}
}

// AvailableTools returns the tools the agent can use.
func (tr *ToolRegistry) AvailableTools() []Tool {
	return []Tool{
		{Name: "git_log_explore", Description: "深挖指定文件的 commit 历史。参数：文件路径"},
		{Name: "git_diff_detail", Description: "获取指定文件的完整 diff。参数：文件路径"},
		{Name: "read_file", Description: "读取项目文件内容（裁剪至 8KB）。参数：文件路径"},
		{Name: "search_pattern", Description: "扫描代码中的 TODO/FIXME/HACK/XXX 标记。参数：可选目录路径"},
		{Name: "list_directory", Description: "浏览指定目录结构。参数：相对路径"},
	}
}

// Execute runs a tool call and returns the result.
func (tr *ToolRegistry) Execute(call ToolCall) ToolResult {
	switch call.Tool {
	case "git_log_explore":
		return tr.gitLogExplore(call.Args)
	case "git_diff_detail":
		return tr.gitDiffDetail(call.Args)
	case "read_file":
		return tr.readFile(call.Args)
	case "search_pattern":
		return tr.searchPattern(call.Args)
	case "list_directory":
		return tr.listDirectory(call.Args)
	default:
		return ToolResult{Tool: call.Tool, Args: call.Args, Error: fmt.Sprintf("unknown tool: %s", call.Tool)}
	}
}

func (tr *ToolRegistry) gitLogExplore(file string) ToolResult {
	out := runGit(tr.repoRoot, "log", "--oneline", "--follow", "-n", "10", "--", file)
	return ToolResult{Tool: "git_log_explore", Args: file, Output: truncate(out, MaxToolOutput)}
}

func (tr *ToolRegistry) gitDiffDetail(file string) ToolResult {
	out := runGit(tr.repoRoot, "diff", "HEAD", "--", file)
	if out == "" {
		out = runGit(tr.repoRoot, "diff", "--", file)
	}
	return ToolResult{Tool: "git_diff_detail", Args: file, Output: truncate(out, MaxToolOutput)}
}

func (tr *ToolRegistry) readFile(file string) ToolResult {
	// Prevent path traversal: clean and verify result stays within repo root
	cleanPath := filepath.Clean(filepath.Join(tr.repoRoot, file))
	if !strings.HasPrefix(cleanPath, tr.repoRoot) {
		return ToolResult{Tool: "read_file", Args: file, Error: "path traversal blocked"}
	}
	data, err := os.ReadFile(cleanPath)
	if err != nil {
		return ToolResult{Tool: "read_file", Args: file, Error: err.Error()}
	}
	content := string(data)
	truncated := false
	if len(content) > MaxToolOutput {
		content = content[:MaxToolOutput] + fmt.Sprintf("\n... [truncated at %d bytes]", MaxToolOutput)
		truncated = true
	}
	_ = truncated
	return ToolResult{Tool: "read_file", Args: file, Output: content}
}

var searchPatterns = regexp.MustCompile(`TODO|FIXME|HACK|XXX`)

func (tr *ToolRegistry) searchPattern(dir string) ToolResult {
	targetDir := tr.repoRoot
	if dir != "" {
		targetDir = filepath.Join(tr.repoRoot, dir)
	}

	var matches []string
	filepath.WalkDir(targetDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		// Skip binary and large files
		ext := strings.ToLower(filepath.Ext(path))
		if ext == ".png" || ext == ".jpg" || ext == ".pdf" || ext == ".gz" || ext == ".zip" {
			return nil
		}
		info, _ := d.Info()
		if info != nil && info.Size() > 500000 {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		for i, line := range strings.Split(string(data), "\n") {
			if searchPatterns.MatchString(line) {
				rel, _ := filepath.Rel(tr.repoRoot, path)
				matches = append(matches, fmt.Sprintf("%s:%d: %s", rel, i+1, strings.TrimSpace(line)))
			}
		}
		return nil
	})

	if len(matches) == 0 {
		return ToolResult{Tool: "search_pattern", Args: dir, Output: "(no matches)"}
	}
	return ToolResult{Tool: "search_pattern", Args: dir, Output: truncate(strings.Join(matches, "\n"), MaxToolOutput)}
}

func (tr *ToolRegistry) listDirectory(dir string) ToolResult {
	targetDir := filepath.Join(tr.repoRoot, dir)
	entries, err := os.ReadDir(targetDir)
	if err != nil {
		return ToolResult{Tool: "list_directory", Args: dir, Error: err.Error()}
	}
	var lines []string
	for _, e := range entries {
		if e.IsDir() {
			lines = append(lines, e.Name()+"/")
		} else {
			info, _ := e.Info()
			size := ""
			if info != nil {
				size = fmt.Sprintf(" (%d bytes)", info.Size())
			}
			lines = append(lines, e.Name()+size)
		}
	}
	return ToolResult{Tool: "list_directory", Args: dir, Output: truncate(strings.Join(lines, "\n"), MaxToolOutput)}
}

// MaxToolOutput limits tool result size.
const MaxToolOutput = 8192

func runGit(dir string, args ...string) string {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return fmt.Sprintf("(git error: %v)", err)
	}
	return string(out)
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + fmt.Sprintf("\n... [truncated at %d bytes]", maxLen)
}

// ToLLMToolDefs converts the tool registry to LLM-compatible tool definitions.
func (tr *ToolRegistry) ToLLMToolDefs() []interface{} {
	tools := tr.AvailableTools()
	defs := make([]interface{}, len(tools))
	for i, t := range tools {
		defs[i] = map[string]interface{}{
			"type": "function",
			"function": map[string]interface{}{
				"name":        t.Name,
				"description": t.Description,
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"path": map[string]interface{}{
							"type":        "string",
							"description": "文件或目录的相对路径",
						},
					},
					"required": []string{"path"},
				},
			},
		}
	}
	return defs
}
