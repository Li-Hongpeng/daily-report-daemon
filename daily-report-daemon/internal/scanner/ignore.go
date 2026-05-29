package scanner

import (
	"strings"
)

// defaultIgnoreDirs are directory names always skipped during scanning.
var defaultIgnoreDirs = map[string]bool{
	".git":         true,
	"node_modules": true,
	".venv":        true,
	"venv":         true,
	"dist":         true,
	"build":        true,
	"__pycache__":  true,
	".idea":        true,
	".vscode":      true,
	".daily-report-daemon": true,
}

// defaultIgnorePrefixes are file name prefixes to skip.
var defaultIgnorePrefixes = []string{
	".",
}

// binaryExtensions are extensions known to be non-text.
var binaryExtensions = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".bmp": true,
	".ico": true, ".svg": true, ".webp": true,
	".pdf": true, ".doc": true, ".docx": true, ".xls": true, ".xlsx": true,
	".ppt": true, ".pptx": true,
	".zip": true, ".tar": true, ".gz": true, ".bz2": true, ".xz": true,
	".7z": true, ".rar": true,
	".exe": true, ".dll": true, ".so": true, ".dylib": true, ".a": true,
	".o": true, ".obj": true,
	".class": true, ".jar": true, ".war": true,
	".pyc": true, ".pyo": true,
	".wasm": true,
	".mp3": true, ".mp4": true, ".avi": true, ".mov": true, ".wav": true,
	".ttf": true, ".otf": true, ".woff": true, ".woff2": true, ".eot": true,
	".db": true, ".sqlite": true, ".sqlite3": true,
	".lock": true, // package-lock, yarn.lock etc. are large, skip content
}

// keyFileNames are files we always want to read.
var keyFileNames = map[string]bool{
	"README.md":      true,
	"README":         true,
	"README.txt":     true,
	"package.json":   true,
	"go.mod":         true,
	"go.sum":         false, // usually not human-relevant but listed as metadata
	"pyproject.toml": true,
	"setup.py":       true,
	"setup.cfg":      true,
	"Cargo.toml":     true,
	"Makefile":       true,
	"makefile":       true,
	"Taskfile":       true,
	"Taskfile.yml":   true,
	"Taskfile.yaml":  true,
	"Dockerfile":     true,
	"docker-compose.yml":    true,
	"docker-compose.yaml":   true,
	".gitignore":     true,
	".env.example":   true,
	"tsconfig.json":  true,
	"jest.config.ts": true,
	"jest.config.js": true,
	"vitest.config.ts": true,
	"vitest.config.js": true,
	"AGENTS.md":      true,
	"CLAUDE.md":      true,
}

// ShouldSkipDir returns true if the directory should be excluded from scanning.
func ShouldSkipDir(name string) bool {
	return defaultIgnoreDirs[name]
}

// ShouldSkipFile returns true if the file should be excluded.
func ShouldSkipFile(name string) bool {
	if strings.HasPrefix(name, ".") {
		// Allow known key files that start with dot
		if name == ".gitignore" || name == ".env.example" {
			return false
		}
		return true
	}
	return false
}

// IsBinaryExt returns true if the extension suggests binary content.
func IsBinaryExt(ext string) bool {
	return binaryExtensions[strings.ToLower(ext)]
}

// IsKeyFile returns true if the filename is a recognized key file.
func IsKeyFile(name string) bool {
	_, ok := keyFileNames[name]
	return ok
}

// MaxKeyFileBytes is the maximum bytes to read from a key file.
const MaxKeyFileBytes = 16384 // 16KB

// MaxDirEntries is the maximum number of top-level entries to enumerate.
const MaxDirEntries = 200
