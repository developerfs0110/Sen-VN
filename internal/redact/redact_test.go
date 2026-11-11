package redact_test

import (
	"strings"
	"testing"

	"github.com/developerfs0110/Sen-VN/internal/redact"
)

func TestFilterEnvStripsSensitiveAndKeepsSafeVars(t *testing.T) {
	input := []string{
		"PATH=C:\\Program Files\\Go\\bin;C:\\Windows\\System32",
		"GOPATH=C:\\Users\\student\\go",
		"GOCACHE=C:\\Users\\student\\AppData\\Local\\go-build",
		"SystemRoot=C:\\Windows",
		"SEN_API_KEY=sk-secret1234567890abcdef",
		"OPENAI_API_KEY=sk-proj-abcdef1234567890",
		"GITHUB_TOKEN=ghp_1234567890abcdefghijklmnop",
		"AWS_SECRET_ACCESS_KEY=supersecret",
		"DB_PASSWORD=hunter2",
		"ADMIN_PASSWD=rootpass",
		"GOOGLE_APPLICATION_CREDENTIALS=C:\\keys.json",
		"SSH_PRIVATE_VAL=privkey",
		"DATABASE_URL=postgres://user:pass@localhost/db",
		"SENTRY_DSN=https://abc@sentry.io/1",
		"NORMAL_FLAG=enabled",
	}

	got := redact.FilterEnv(input)
	joined := strings.Join(got, "\n")

	mustKeep := []string{"PATH=", "GOPATH=", "GOCACHE=", "SystemRoot=", "NORMAL_FLAG=enabled"}
	for _, k := range mustKeep {
		if !strings.Contains(joined, k) {
			t.Errorf("expected FilterEnv to preserve %q, got:\n%s", k, joined)
		}
	}

	mustDrop := []string{
		"SEN_API_KEY", "OPENAI_API_KEY", "GITHUB_TOKEN", "AWS_SECRET_ACCESS_KEY",
		"DB_PASSWORD", "ADMIN_PASSWD", "GOOGLE_APPLICATION_CREDENTIALS",
		"SSH_PRIVATE_VAL", "DATABASE_URL", "SENTRY_DSN",
	}
	for _, k := range mustDrop {
		if strings.Contains(joined, k) {
			t.Errorf("expected FilterEnv to strip %q, got:\n%s", k, joined)
		}
	}
}

func TestScrubTextRedactsKnownSecretPatterns(t *testing.T) {
	raw := strings.Join([]string{
		"leak1: sk-proj-1234567890abcdefghij",
		"leak2: ghp_abcdefghijklmnopqrstuvwxyz1234",
		"leak3: AIzaSyDaGmWKa4JsXZ-0123456789abcdef",
		"leak4: Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.secret",
		"leak5: SEN_API_KEY=my_ultra_secret_val",
	}, "\n")

	scrubbed := redact.ScrubText(raw)

	forbidden := []string{
		"sk-proj-1234567890abcdefghij",
		"ghp_abcdefghijklmnopqrstuvwxyz1234",
		"AIzaSyDaGmWKa4JsXZ-0123456789abcdef",
		"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.secret",
		"my_ultra_secret_val",
	}
	for _, f := range forbidden {
		if strings.Contains(scrubbed, f) {
			t.Errorf("ScrubText failed to redact %q in:\n%s", f, scrubbed)
		}
	}
	if strings.Count(scrubbed, "[REDACTED]") < 5 {
		t.Errorf("expected at least 5 [REDACTED] markers, got:\n%s", scrubbed)
	}
}

func TestScrubTextRedactsURLBasicAuthAndPEMPrivateKey(t *testing.T) {
	raw := "conn=postgres://admin:MySup3rPass!@db.internal:5432/prod\n" +
		"url=https://alice:hunter2secret@api.example.com/v1\n" +
		"-----BEGIN RSA PRIVATE KEY-----\nMIIEpAIBAAKCAQEAsecretkeybytes\n-----END RSA PRIVATE KEY-----"
	got := redact.ScrubText(raw)
	for _, leak := range []string{"MySup3rPass!", "hunter2secret", "MIIEpAIBAAKCAQEAsecretkeybytes"} {
		if strings.Contains(got, leak) {
			t.Errorf("expected %q scrubbed, got:\n%s", leak, got)
		}
	}
}
