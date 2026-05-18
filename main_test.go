package main

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadConfigDefaults(t *testing.T) {
	clearConfigEnv(t)

	config, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig returned error: %v", err)
	}

	if config.DataFile != "data/demoData.csv" {
		t.Fatalf("unexpected data file: %q", config.DataFile)
	}

	if config.RedisURL != "redis://localhost:6379" {
		t.Fatalf("unexpected Redis URL: %q", config.RedisURL)
	}

	if config.RedisChannel != "hub-counts" {
		t.Fatalf("unexpected Redis channel: %q", config.RedisChannel)
	}

	if !config.GroupByHour {
		t.Fatal("expected group by hour to default to true")
	}

	if !config.BulkPublish {
		t.Fatal("expected bulk publish to default to true")
	}

	if config.PublishInterval != 2*time.Second {
		t.Fatalf("unexpected publish interval: %s", config.PublishInterval)
	}
}

func TestLoadConfigFromEnv(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("DATA_FILE", "/tmp/reports.csv")
	t.Setenv("REDIS_URL", "redis://redis:6379")
	t.Setenv("REDIS_CHANNEL", "custom-channel")
	t.Setenv("GROUP_BY_HOUR", "false")
	t.Setenv("BULK_PUBLISH", "false")
	t.Setenv("PUBLISH_INTERVAL", "500ms")

	config, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig returned error: %v", err)
	}

	if config.DataFile != "/tmp/reports.csv" {
		t.Fatalf("unexpected data file: %q", config.DataFile)
	}

	if config.RedisURL != "redis://redis:6379" {
		t.Fatalf("unexpected Redis URL: %q", config.RedisURL)
	}

	if config.RedisChannel != "custom-channel" {
		t.Fatalf("unexpected Redis channel: %q", config.RedisChannel)
	}

	if config.GroupByHour {
		t.Fatal("expected group by hour to be false")
	}

	if config.BulkPublish {
		t.Fatal("expected bulk publish to be false")
	}

	if config.PublishInterval != 500*time.Millisecond {
		t.Fatalf("unexpected publish interval: %s", config.PublishInterval)
	}
}

func TestLoadConfigRejectsInvalidBool(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("GROUP_BY_HOUR", "sometimes")

	if _, err := loadConfig(); err == nil {
		t.Fatal("expected invalid bool to return an error")
	}
}

func TestLoadConfigRejectsInvalidDuration(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("PUBLISH_INTERVAL", "fast")

	if _, err := loadConfig(); err == nil {
		t.Fatal("expected invalid duration to return an error")
	}
}

func TestLoadConfigRejectsNonPositiveDuration(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("PUBLISH_INTERVAL", "0s")

	if _, err := loadConfig(); err == nil {
		t.Fatal("expected non-positive duration to return an error")
	}
}

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

func clearConfigEnv(t *testing.T) {
	t.Helper()

	for _, key := range []string{
		"DATA_FILE",
		"REDIS_URL",
		"REDIS_CHANNEL",
		"GROUP_BY_HOUR",
		"BULK_PUBLISH",
		"PUBLISH_INTERVAL",
	} {
		t.Setenv(key, "")
	}
}
