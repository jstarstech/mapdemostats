package main

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadEnvFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte("REDIS_URL=redis://env-file:6379\n"), 0o600); err != nil {
		t.Fatalf("write env file: %v", err)
	}

	previousRedisURL, hadRedisURL := os.LookupEnv("REDIS_URL")
	t.Cleanup(func() {
		if hadRedisURL {
			_ = os.Setenv("REDIS_URL", previousRedisURL)
			return
		}

		_ = os.Unsetenv("REDIS_URL")
	})

	if err := os.Unsetenv("REDIS_URL"); err != nil {
		t.Fatalf("unset REDIS_URL: %v", err)
	}

	if err := loadEnvFile(path); err != nil {
		t.Fatalf("loadEnvFile returned error: %v", err)
	}

	if got := os.Getenv("REDIS_URL"); got != "redis://env-file:6379" {
		t.Fatalf("unexpected REDIS_URL: %q", got)
	}
}

func TestLoadEnvFileDoesNotOverrideExistingEnv(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte("REDIS_URL=redis://env-file:6379\n"), 0o600); err != nil {
		t.Fatalf("write env file: %v", err)
	}

	t.Setenv("REDIS_URL", "redis://shell-env:6379")

	if err := loadEnvFile(path); err != nil {
		t.Fatalf("loadEnvFile returned error: %v", err)
	}

	if got := os.Getenv("REDIS_URL"); got != "redis://shell-env:6379" {
		t.Fatalf("expected existing env to win, got %q", got)
	}
}

func TestLoadEnvFileIgnoresMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")

	if err := loadEnvFile(path); err != nil {
		t.Fatalf("loadEnvFile returned error for missing file: %v", err)
	}
}

func TestLoadReports(t *testing.T) {
	scanner := bufio.NewScanner(strings.NewReader("1710000000,a,b,c\n1710003600,d,e\n"))

	reports, bulkReports, err := loadReports(scanner)
	if err != nil {
		t.Fatalf("loadReports returned error: %v", err)
	}

	values, ok := reports.Get("1710000000")
	if !ok {
		t.Fatalf("expected first report to be present")
	}

	if len(values) != 3 || values[0] != "a" || values[1] != "b" || values[2] != "c" {
		t.Fatalf("unexpected parsed values: %#v", values)
	}

	line, ok := bulkReports.Get("1710003600")
	if !ok {
		t.Fatalf("expected second bulk report to be present")
	}

	if line != "1710003600,d,e" {
		t.Fatalf("unexpected bulk report line: %q", line)
	}
}

func TestLoadReportsRejectsMalformedLine(t *testing.T) {
	scanner := bufio.NewScanner(strings.NewReader("not-a-valid-row\n"))

	_, _, err := loadReports(scanner)
	if err == nil {
		t.Fatal("expected malformed line to return an error")
	}
}
