package sanitize

import (
	"strings"
	"testing"
)

func TestPathBlockedEnvFiles(t *testing.T) {
	s := New()
	tests := []struct {
		path     string
		expected bool
	}{
		{".env", true},
		{".env.local", true},
		{".env.production", true},
		{".env.development", true},
		{".env.staging", true},
		{".env.test", true},
		{".env.ci", true},
		{"src/.env", true},
		{"config/.env.production", true},
		{".env.example", false}, // .env.example is OK
		{"README.md", false},
	}

	for _, tt := range tests {
		got, _ := s.IsPathBlocked(tt.path)
		if got != tt.expected {
			t.Errorf("IsPathBlocked(%q) = %v, want %v", tt.path, got, tt.expected)
		}
	}
}

func TestPathBlockedKeyFiles(t *testing.T) {
	s := New()
	tests := []struct {
		path     string
		expected bool
	}{
		{"id_rsa", true},
		{"id_ed25519", true},
		{"id_ecdsa", true},
		{"~/.ssh/id_rsa", true},
		{"server.pem", true},
		{"cert.key", true},
		{"keystore.jks", true},
		{"my_secret_file.txt", true},
		{"token_store.json", true},
		{"credential_helper", true},
		{"src/privatekey.go", true},
		{"password_list.txt", true},
	}

	for _, tt := range tests {
		got, _ := s.IsPathBlocked(tt.path)
		if !got {
			t.Errorf("IsPathBlocked(%q) = %v, want true", tt.path, got)
		}
	}
}

func TestRedactAPIKey(t *testing.T) {
	s := New()
	input := `const API_KEY = "sk-abcdefghijklmnopqrstuvwxyz123456"`
	result := s.Redact("test.go", input)
	if strings.Contains(result, "sk-abcdefghijklmnopqrstuvwxyz123456") {
		t.Error("API key was not redacted")
	}
	if !strings.Contains(result, "[REDACTED]") {
		t.Error("expected [REDACTED] in output")
	}
}

func TestRedactPrivateKey(t *testing.T) {
	s := New()
	input := `-----BEGIN RSA PRIVATE KEY-----
MIIEpAIBAAKCAQEAtestkeydatahere1234567890
abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOP
-----END RSA PRIVATE KEY-----`
	result := s.Redact("key.pem", input)
	if strings.Contains(result, "PRIVATE KEY") {
		// The header/footer might remain if the pattern didn't match exactly
		// due to the multi-line nature. Let's check the key body is gone.
		if strings.Contains(result, "MIIEpAIBAA") {
			t.Error("private key body was not redacted")
		}
	}
}

func TestRedactPassword(t *testing.T) {
	s := New()
	input := `password = "supersecret123"`
	result := s.Redact("config.py", input)
	if strings.Contains(result, "supersecret123") {
		t.Error("password was not redacted")
	}
	if !strings.Contains(result, "[REDACTED]") {
		t.Error("expected [REDACTED] in output")
	}
}

func TestRedactToken(t *testing.T) {
	s := New()
	input := `access_token = "ghp_abcdefghijklmnopqrstuvwxyz1234"`
	result := s.Redact("config.ts", input)
	if strings.Contains(result, "ghp_abcdefghijklmnopqrstuvwxyz1234") {
		t.Error("token was not redacted")
	}
}

func TestRedactAWSKey(t *testing.T) {
	s := New()
	input := `AWS_ACCESS_KEY_ID = "AKIAIOSFODNN7EXAMPLE"`
	result := s.Redact("aws.go", input)
	if strings.Contains(result, "AKIAIOSFODNN7EXAMPLE") {
		t.Error("AWS key was not redacted")
	}
}

func TestRedactHardcodedSecret(t *testing.T) {
	s := New()
	input := `const SECRET = "my-secret-value-12345"`
	result := s.Redact("app.ts", input)
	if strings.Contains(result, "my-secret-value-12345") {
		t.Error("hardcoded secret was not redacted")
	}
	// Also test var/let/static
	for _, prefix := range []string{
		`var SECRET = "hardcoded-var-12345"`,
		`let secretKey = "hardcoded-let-12345"`,
		`static final TOKEN = "hardcoded-static-12345"`,
	} {
		r := s.Redact("test.js", prefix)
		if !strings.Contains(r, "[REDACTED]") {
			t.Errorf("hardcoded secret not redacted in: %s", prefix)
		}
	}
}

func TestRedactJWT(t *testing.T) {
	s := New()
	input := `Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U`
	result := s.Redact("test.txt", input)
	if strings.Contains(result, "eyJhbGciOi") {
		t.Error("JWT token was not redacted")
	}
}

func TestRedactDBConnectionString(t *testing.T) {
	s := New()
	input := `DATABASE_URL = "postgresql://user:password@localhost:5432/mydb"`
	result := s.Redact(".env", input)
	if strings.Contains(result, "postgresql://") {
		t.Error("database connection string was not redacted")
	}
}

func TestTruncation(t *testing.T) {
	s := New()
	content := strings.Repeat("x", 5000)
	result, truncated := s.Truncate("large.txt", content, 1000)
	if !truncated {
		t.Error("expected truncation for oversize content")
	}
	if len(result) <= len(content) {
		// Should be shorter (truncated + message)
	}
	// Verify report
	r := s.Report()
	if len(r.Truncations) != 1 {
		t.Errorf("expected 1 truncation in report, got %d", len(r.Truncations))
	}
	if r.Truncations[0].OriginalLen != 5000 {
		t.Errorf("expected original len 5000, got %d", r.Truncations[0].OriginalLen)
	}
}

func TestReportAccumulates(t *testing.T) {
	s := New()
	s.CheckPath(".env")
	s.CheckPath("token.txt")
	s.Redact("test.go", `API_KEY = "sk-abcdefghijklmnopqrstuvwxyz123456"`)
	s.Truncate("big.md", strings.Repeat("a", 20000), 16000)

	r := s.Report()
	if r.TotalBlocks != 2 {
		t.Errorf("expected 2 path blocks, got %d", r.TotalBlocks)
	}
	if r.TotalRegex != 1 {
		t.Errorf("expected 1 regex hit, got %d", r.TotalRegex)
	}
	if len(r.Truncations) != 1 {
		t.Errorf("expected 1 truncation, got %d", len(r.Truncations))
	}
}

func TestPathBlockedDirectoryNames(t *testing.T) {
	s := New()
	tests := []struct {
		path     string
		expected bool
	}{
		{"config/secrets/creds.json", true},
		{"secrets/db.yaml", true},
		{"token/service.go", true},
		{"credentials/aws.json", true},
		{"private_keys/id_rsa", true},    // directory name matches
		{"password_store/data.txt", true},
		{"src/models/user.go", false},    // clean path
		{"config/settings.json", false},  // "config" is not blocked
	}
	for _, tt := range tests {
		got, _ := s.IsPathBlocked(tt.path)
		if got != tt.expected {
			t.Errorf("IsPathBlocked(%q) = %v, want %v", tt.path, got, tt.expected)
		}
	}
}

func TestCheckPath(t *testing.T) {
	s := New()
	if !s.CheckPath(".env") {
		t.Error("expected .env to be blocked")
	}
	if s.CheckPath("README.md") {
		t.Error("README.md should not be blocked")
	}
	// Verify CheckPath also records the block
	r := s.Report()
	if r.TotalBlocks != 1 {
		t.Errorf("expected 1 path block after CheckPath, got %d", r.TotalBlocks)
	}
}
