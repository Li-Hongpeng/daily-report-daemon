package sanitize

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// RedactionReport records the sanitization results for a scan.
type RedactionReport struct {
	PathBlocks   []PathBlock    `json:"path_blocks"`
	RegexHits    []RegexHit     `json:"regex_hits"`
	Truncations  []Truncation   `json:"truncations"`
	TotalBlocks  int            `json:"total_blocks"`
	TotalRegex   int            `json:"total_regex"`
}

// PathBlock records a file skipped due to path rules.
type PathBlock struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

// RegexHit records a content redaction.
type RegexHit struct {
	File    string `json:"file"`
	Pattern string `json:"pattern"`
	Count   int    `json:"count"`
}

// Truncation records a content truncation.
type Truncation struct {
	File         string `json:"file"`
	OriginalLen  int    `json:"original_len"`
	MaxAllowed   int    `json:"max_allowed"`
}

// Sanitizer applies path and content filters.
type Sanitizer struct {
	patterns []*regexp.Regexp
	report   RedactionReport
}

// New creates a Sanitizer with default patterns.
func New() *Sanitizer {
	return &Sanitizer{
		patterns: defaultPatterns(),
	}
}

// Report returns the accumulated redaction report.
func (s *Sanitizer) Report() RedactionReport {
	s.report.TotalBlocks = len(s.report.PathBlocks)
	s.report.TotalRegex = len(s.report.RegexHits)
	return s.report
}

// IsPathBlocked checks whether a file path should be entirely skipped.
// Checks both filename and directory components for sensitive patterns.
func (s *Sanitizer) IsPathBlocked(path string) (blocked bool, reason string) {
	// Check full path (not just base name) for sensitive directory or file patterns
	name := filepath.Base(path)
	lower := strings.ToLower(name)

	// --- Directory-level checks (check path components) ---
	dir := filepath.Dir(path)
	dirLower := strings.ToLower(dir)
	for _, part := range strings.Split(dirLower, string(filepath.Separator)) {
		for _, pat := range []string{"secret", "token", "credential", "password", "privatekey", "private_key"} {
			if strings.Contains(part, pat) {
				return true, fmt.Sprintf("blocked directory: %s", part)
			}
		}
	}

	// --- Filename-level checks ---

	// Exact filename matches
	blockedNames := []string{
		".env", ".env.local", ".env.development", ".env.production",
		".env.staging", ".env.test", ".env.ci",
		"id_rsa", "id_ed25519", "id_ecdsa",
	}
	for _, blocked := range blockedNames {
		if name == blocked {
			return true, fmt.Sprintf("blocked filename: %s", name)
		}
	}

	// Extension matches
	blockedExts := []string{".pem", ".key", ".pfx", ".p12", ".jks", ".keystore"}
	for _, ext := range blockedExts {
		if strings.HasSuffix(lower, ext) {
			return true, fmt.Sprintf("blocked extension: %s", ext)
		}
	}

	// Name patterns (contains)
	blockedPatterns := []string{"secret", "token", "credential", "password", "privatekey"}
	for _, pat := range blockedPatterns {
		if strings.Contains(lower, pat) {
			return true, fmt.Sprintf("blocked pattern in filename: %s", pat)
		}
	}

	return false, ""
}

// CheckPath records a path block if the path is sensitive.
// Returns true if blocked.
func (s *Sanitizer) CheckPath(path string) bool {
	blocked, reason := s.IsPathBlocked(path)
	if blocked {
		s.report.PathBlocks = append(s.report.PathBlocks, PathBlock{
			Path:   path,
			Reason: reason,
		})
	}
	return blocked
}

// Redact applies regex redaction to content and returns the redacted version
// along with the number of replacements.
func (s *Sanitizer) Redact(file string, content string) string {
	result := content
	for _, pat := range s.patterns {
		matches := pat.FindAllStringIndex(result, -1)
		if len(matches) > 0 {
			s.report.RegexHits = append(s.report.RegexHits, RegexHit{
				File:    file,
				Pattern: pat.String(),
				Count:   len(matches),
			})
		}
		// Replace from the end to preserve indices
		for i := len(matches) - 1; i >= 0; i-- {
			m := matches[i]
			result = result[:m[0]] + "[REDACTED]" + result[m[1]:]
		}
	}
	return result
}

// Truncate clips content to maxLen, recording the truncation.
func (s *Sanitizer) Truncate(file string, content string, maxLen int) (string, bool) {
	if len(content) <= maxLen {
		return content, false
	}
	s.report.Truncations = append(s.report.Truncations, Truncation{
		File:        file,
		OriginalLen: len(content),
		MaxAllowed:  maxLen,
	})
	cut := content[:maxLen]
	cut += fmt.Sprintf("\n\n... [truncated: %d → %d chars]", len(content), maxLen)
	return cut, true
}

// SaveReport writes the redaction report as JSON.
func (s *Sanitizer) SaveReport(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}
	data, err := json.MarshalIndent(s.Report(), "", "  ")
	if err != nil {
		return fmt.Errorf("marshal report: %w", err)
	}
	return os.WriteFile(path, data, 0644)
}

// defaultPatterns returns compiled regex patterns for secret detection.
func defaultPatterns() []*regexp.Regexp {
	return []*regexp.Regexp{
		// OpenAI API key
		regexp.MustCompile(`sk-[A-Za-z0-9_-]{20,}`),
		// Private key header/footer blocks
		regexp.MustCompile(`-----BEGIN[ A-Z]*PRIVATE KEY-----[a-zA-Z0-9/+\n=]*-----END[ A-Z]*PRIVATE KEY-----`),
		// Generic API key assignment (any case)
		regexp.MustCompile(`(?i)(api[_-]?key|apikey|secret[_-]?key)\s*[:=]\s*["'\x60]?[A-Za-z0-9_\-]{16,}["'\x60]?`),
		// Password assignment
		regexp.MustCompile(`(?i)(password|passwd|pwd)\s*[:=]\s*["'\x60][^"'\x60]{4,}["'\x60]`),
		// Token assignment
		regexp.MustCompile(`(?i)(access[_-]?token|auth[_-]?token|bearer[_-]?token|personal[_-]?token)\s*[:=]\s*["'\x60]?[A-Za-z0-9_\-.]{16,}["'\x60]?`),
		// AWS access key ID
		regexp.MustCompile(`AKIA[0-9A-Z]{16}`),
		// Generic base64-looking secrets (long alphanumeric strings in assignment context)
		regexp.MustCompile(`(?i)(secret|token|key)\s*[:=]\s*["'\x60][A-Za-z0-9+/=]{32,}["'\x60]`),
		// Hardcoded secrets in source code: const/var/let SECRET = "..."
		regexp.MustCompile(`(?i)(const|var|let|static|final)\s+\w*(secret|token|key|password)\w*\s*[:=]\s*["'\x60][^"'\x60]{8,}["'\x60]`),
		// JWT tokens
		regexp.MustCompile(`eyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}`),
		// Database connection strings (stripped of credentials)
		regexp.MustCompile(`(?i)(mongodb|mysql|postgres|postgresql|redis|sqlite)://[^\s"'\x60]{8,}`),
	}
}
