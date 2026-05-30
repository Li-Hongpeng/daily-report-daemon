package scanner

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ProjectMetadata captures the structure and key files of a project.
type ProjectMetadata struct {
	Root           string        `json:"root"`
	Structure      []DirEntry    `json:"structure"`
	KeyFiles       []KeyFile     `json:"key_files"`
	Languages      []LangStat    `json:"languages"`
	BuildCommands  []CommandHint `json:"build_commands"`
	TestCommands   []CommandHint `json:"test_commands"`
	TotalFiles     int           `json:"total_files"`
	TotalTextFiles int           `json:"total_text_files"`
}

// DirEntry is a single file or directory in the structure listing.
type DirEntry struct {
	Name string `json:"name"`
	Type string `json:"type"` // "file" or "dir"
	Size int64  `json:"size,omitempty"`
}

// KeyFile represents a recognized key file with its (truncated) content.
type KeyFile struct {
	Path      string `json:"path"`
	Name      string `json:"name"`
	Content   string `json:"content"`
	Truncated bool   `json:"truncated,omitempty"`
}

// LangStat counts files by extension.
type LangStat struct {
	Language string `json:"language"`
	Files    int    `json:"files"`
}

// CommandHint is a build/test command extracted from config files.
type CommandHint struct {
	Command string `json:"command"`
	Source  string `json:"source"` // e.g. "Makefile", "package.json"
}

// Scanner walks a workspace and collects project metadata.
type Scanner struct {
	root string
}

// NewScanner creates a Scanner for the given root directory.
func NewScanner(root string) *Scanner {
	return &Scanner{root: root}
}

// Scan collects project metadata.
func (s *Scanner) Scan() (*ProjectMetadata, error) {
	meta := &ProjectMetadata{Root: s.root}

	// Top-level structure
	meta.Structure = s.collectStructure()

	// Walk for key files and language stats
	s.walk(meta)

	// Sort languages by file count
	sort.Slice(meta.Languages, func(i, j int) bool {
		return meta.Languages[i].Files > meta.Languages[j].Files
	})

	// Extract commands
	meta.BuildCommands, meta.TestCommands = s.extractCommands(meta.KeyFiles)

	return meta, nil
}

func (s *Scanner) collectStructure() []DirEntry {
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return nil
	}
	var result []DirEntry
	for i, e := range entries {
		if i >= MaxDirEntries {
			break
		}
		name := e.Name()
		if ShouldSkipDir(name) && e.IsDir() {
			continue
		}
		if e.IsDir() {
			result = append(result, DirEntry{Name: name, Type: "dir"})
		} else {
			info, _ := e.Info()
			size := int64(0)
			if info != nil {
				size = info.Size()
			}
			if ShouldSkipFile(name) {
				continue
			}
			result = append(result, DirEntry{Name: name, Type: "file", Size: size})
		}
	}
	return result
}

func (s *Scanner) walk(meta *ProjectMetadata) {
	filepath.WalkDir(s.root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip inaccessible entries
		}

		// Skip ignored dirs
		if d.IsDir() {
			if ShouldSkipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}

		meta.TotalFiles++

		// Skip files by name
		if ShouldSkipFile(d.Name()) {
			return nil
		}

		// Determine relative path
		rel, _ := filepath.Rel(s.root, path)

		ext := strings.ToLower(filepath.Ext(d.Name()))
		isBinary := IsBinaryExt(ext)

		if !isBinary {
			meta.TotalTextFiles++
			meta.addLangStat(ext)
		}

		// Read key files
		if IsKeyFile(d.Name()) {
			kf := KeyFile{
				Path: rel,
				Name: d.Name(),
			}
			if !isBinary {
				data, err := os.ReadFile(path)
				if err == nil {
					kf.Content = string(data)
					if len(kf.Content) > MaxKeyFileBytes {
						kf.Content = kf.Content[:MaxKeyFileBytes] +
							fmt.Sprintf("\n\n... [truncated, original: %d bytes]", len(kf.Content))
						kf.Truncated = true
					}
				}
			}
			meta.KeyFiles = append(meta.KeyFiles, kf)
		}

		return nil
	})
}

func (meta *ProjectMetadata) addLangStat(ext string) {
	if ext == "" {
		return
	}
	lang := langName(ext)
	for i := range meta.Languages {
		if meta.Languages[i].Language == lang {
			meta.Languages[i].Files++
			return
		}
	}
	meta.Languages = append(meta.Languages, LangStat{Language: lang, Files: 1})
}

func langName(ext string) string {
	switch ext {
	case ".go":
		return "Go"
	case ".ts", ".tsx":
		return "TypeScript"
	case ".js", ".jsx":
		return "JavaScript"
	case ".py":
		return "Python"
	case ".rs":
		return "Rust"
	case ".java":
		return "Java"
	case ".rb":
		return "Ruby"
	case ".c":
		return "C"
	case ".cpp", ".cc", ".cxx":
		return "C++"
	case ".h", ".hpp":
		return "C/C++ Header"
	case ".swift":
		return "Swift"
	case ".kt":
		return "Kotlin"
	case ".scala":
		return "Scala"
	case ".php":
		return "PHP"
	case ".cs":
		return "C#"
	case ".fs", ".fsx":
		return "F#"
	case ".sh", ".bash":
		return "Shell"
	case ".sql":
		return "SQL"
	case ".md":
		return "Markdown"
	case ".yaml", ".yml":
		return "YAML"
	case ".json":
		return "JSON"
	case ".toml":
		return "TOML"
	case ".xml":
		return "XML"
	case ".html", ".htm":
		return "HTML"
	case ".css":
		return "CSS"
	case ".scss", ".sass":
		return "SCSS"
	case ".proto":
		return "Protobuf"
	case ".graphql":
		return "GraphQL"
	case ".tf":
		return "Terraform"
	case ".dockerfile":
		return "Dockerfile"
	case ".vue":
		return "Vue"
	case ".svelte":
		return "Svelte"
	default:
		return strings.TrimPrefix(ext, ".")
	}
}

func (s *Scanner) extractCommands(kfs []KeyFile) (build, test []CommandHint) {
	for _, kf := range kfs {
		switch kf.Name {
		case "Makefile", "makefile":
			build = append(build, extractMakefileCommands("build", kf.Content)...)
			build = append(build, extractMakefileCommands("install", kf.Content)...)
			test = append(test, extractMakefileCommands("test", kf.Content)...)
		case "package.json":
			b, t := extractPackageJSONCommands(kf.Content)
			build = append(build, b...)
			test = append(test, t...)
		case "pyproject.toml":
			test = append(test, extractPyProjectCommands(kf.Content)...)
		}
	}
	return build, test
}

func extractMakefileCommands(target, content string) []CommandHint {
	var hints []CommandHint
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, target+":") {
			hints = append(hints, CommandHint{
				Command: fmt.Sprintf("make %s", target),
				Source:  "Makefile",
			})
			break
		}
	}
	return hints
}

func extractPackageJSONCommands(content string) (build, test []CommandHint) {
	// Lightweight JSON extraction: look for "scripts" section
	scripts := extractJSONSection(content, "scripts")
	if scripts == "" {
		return nil, nil
	}
	for _, key := range []string{"build", "start", "dev", "serve", "compile"} {
		if containsJSONKey(scripts, key) {
			build = append(build, CommandHint{
				Command: fmt.Sprintf("npm run %s", key),
				Source:  "package.json",
			})
		}
	}
	for _, key := range []string{"test", "lint", "check", "typecheck"} {
		if containsJSONKey(scripts, key) {
			test = append(test, CommandHint{
				Command: fmt.Sprintf("npm run %s", key),
				Source:  "package.json",
			})
		}
	}
	return build, test
}

func extractPyProjectCommands(content string) []CommandHint {
	var hints []CommandHint
	// Look for pytest in the file
	if strings.Contains(content, "pytest") {
		hints = append(hints, CommandHint{Command: "pytest", Source: "pyproject.toml"})
	}
	if strings.Contains(content, "[tool.pytest") {
		hints = append(hints, CommandHint{Command: "pytest", Source: "pyproject.toml"})
	}
	return hints
}

// extractJSONSection naively extracts the value of a top-level JSON key.
func extractJSONSection(content, key string) string {
	search := fmt.Sprintf("%q:", key)
	idx := strings.Index(content, search)
	if idx < 0 {
		return ""
	}
	return content[idx:]
}

func containsJSONKey(section, key string) bool {
	search := fmt.Sprintf("%q:", key)
	return strings.Contains(section, search)
}

// SaveMetadata writes ProjectMetadata as JSON to the given path.
func SaveMetadata(meta *ProjectMetadata, path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal metadata: %w", err)
	}
	return os.WriteFile(path, data, 0644)
}

// LoadMetadata reads ProjectMetadata from a previously saved JSON file.
func LoadMetadata(path string) (*ProjectMetadata, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read metadata: %w", err)
	}
	var meta ProjectMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("parse metadata: %w", err)
	}
	return &meta, nil
}
