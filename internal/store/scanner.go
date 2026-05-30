package store

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// BuildCandidates walks a directory and returns file candidates for incremental scanning.
// It reads file size, mtime, and computes a SHA256 hash for text files.
// Directories matching skip patterns are excluded.
func BuildCandidates(root string) ([]FileCandidate, error) {
	var candidates []FileCandidate

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip inaccessible
		}
		if d.IsDir() {
			if shouldSkipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return nil
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}

		mtime := info.ModTime().UTC().Format("2006-01-02T15:04:05Z")
		size := info.Size()

		// Compute hash for files under 1MB
		hash := ""
		if size < 1<<20 {
			h, err := fileHash(path)
			if err == nil {
				hash = h
			}
		}

		candidates = append(candidates, FileCandidate{
			Path:  rel,
			Mtime: mtime,
			Hash:  hash,
			Size:  size,
		})
		return nil
	})

	return candidates, err
}

func fileHash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

// Dirs to skip during walk (same as scanner package).
var skipDirs = map[string]bool{
	".git":         true,
	".daily-report-daemon": true,
	"node_modules": true,
	".venv":        true,
	"venv":         true,
	"__pycache__":  true,
	".mypy_cache":  true,
	".pytest_cache": true,
	".tox":         true,
	"dist":         true,
	"build":        true,
	"target":       true,
	".next":        true,
	".nuxt":        true,
	".output":      true,
	"coverage":     true,
	".idea":        true,
	".vscode":      true,
	"vendor":       true,
	"tmp":          true,
	"temp":         true,
}

func shouldSkipDir(name string) bool {
	return skipDirs[name]
}
