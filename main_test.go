package main

import (
	"bufio"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	orderedmap "github.com/wk8/go-ordered-map/v2"
)

type publishCall struct {
	channel string
	message interface{}
}

type fakePublisher struct {
	cancel context.CancelFunc
	calls  []publishCall
}

func (p *fakePublisher) Publish(_ context.Context, channel string, message interface{}) *redis.IntCmd {
	p.calls = append(p.calls, publishCall{channel: channel, message: message})
	if p.cancel != nil {
		p.cancel()
	}

	return redis.NewIntResult(1, nil)
}

type errorPublisher struct {
	err error
}

func (p errorPublisher) Publish(_ context.Context, _ string, _ interface{}) *redis.IntCmd {
	return redis.NewIntResult(0, p.err)
}

type fakeRedisPinger struct {
	cancel context.CancelFunc
	errs   []error
	calls  int
}

func (p *fakeRedisPinger) Ping(_ context.Context) *redis.StatusCmd {
	if p.cancel != nil {
		p.cancel()
	}

	defer func() {
		p.calls++
	}()

	if p.calls >= len(p.errs) {
		return redis.NewStatusResult("PONG", nil)
	}

	return redis.NewStatusResult("", p.errs[p.calls])
}

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

	if !config.RedisConnectRetry {
		t.Fatal("expected Redis connect retry to default to true")
	}

	if config.RedisConnectRetryInterval != 2*time.Second {
		t.Fatalf("unexpected Redis connect retry interval: %s", config.RedisConnectRetryInterval)
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
	t.Setenv("REDIS_CONNECT_RETRY", "true")
	t.Setenv("REDIS_CONNECT_RETRY_INTERVAL", "250ms")
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

	if !config.RedisConnectRetry {
		t.Fatal("expected Redis connect retry to be true")
	}

	if config.RedisConnectRetryInterval != 250*time.Millisecond {
		t.Fatalf("unexpected Redis connect retry interval: %s", config.RedisConnectRetryInterval)
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

func TestSelectReportsGroupsByHour(t *testing.T) {
	reports := orderedmap.New[string, []string]()
	reports.Set("1710000000", []string{"a"})
	reports.Set("1710000100", []string{"b"})
	reports.Set("1710003600", []string{"c"})

	bulkReports := orderedmap.New[string, string]()
	bulkReports.Set("1710000000", "1710000000,a")
	bulkReports.Set("1710000100", "1710000100,b")
	bulkReports.Set("1710003600", "1710003600,c")

	groupedReports, groupedBulkReports, err := selectReports(reports, bulkReports, true)
	if err != nil {
		t.Fatalf("selectReports returned error: %v", err)
	}

	if groupedReports.Len() != 2 {
		t.Fatalf("expected two grouped reports, got %d", groupedReports.Len())
	}

	if groupedBulkReports.Len() != 2 {
		t.Fatalf("expected two grouped bulk reports, got %d", groupedBulkReports.Len())
	}

	first := groupedReports.Oldest()
	if first == nil || first.Key != "1710000000" || first.Value[0] != "a" {
		t.Fatalf("unexpected first grouped report: %#v", first)
	}
}

func TestRunPublisherStopsWhenContextCanceledDuringSleep(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	publisher := &fakePublisher{cancel: cancel}
	listing := orderedmap.New[string, []string]()
	listing.Set("1710000000", []string{"a"})
	bulkListing := orderedmap.New[string, string]()
	bulkListing.Set("1710000000", "1710000000,a")

	err := runPublisher(ctx, publisher, config{
		RedisChannel:    "test-channel",
		BulkPublish:     true,
		PublishInterval: time.Hour,
	}, listing, bulkListing)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}

	if len(publisher.calls) != 1 {
		t.Fatalf("expected one publish call, got %d", len(publisher.calls))
	}

	if publisher.calls[0].channel != "test-channel" {
		t.Fatalf("unexpected channel: %q", publisher.calls[0].channel)
	}

	if publisher.calls[0].message != "1710000000,a" {
		t.Fatalf("unexpected message: %q", publisher.calls[0].message)
	}
}

func TestRunPublisherPublishesIndividualValues(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	publisher := &fakePublisher{}
	listing := orderedmap.New[string, []string]()
	listing.Set("1710000000", []string{"a", "b"})
	bulkListing := orderedmap.New[string, string]()

	publisher.cancel = cancel

	err := runPublisher(ctx, publisher, config{
		RedisChannel:    "test-channel",
		BulkPublish:     false,
		PublishInterval: time.Hour,
	}, listing, bulkListing)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}

	if len(publisher.calls) != 1 {
		t.Fatalf("expected one publish before cancellation, got %d", len(publisher.calls))
	}

	if publisher.calls[0].message != "a" {
		t.Fatalf("unexpected message: %q", publisher.calls[0].message)
	}
}

func TestRunPublisherReturnsPublishError(t *testing.T) {
	expectedErr := errors.New("publish failed")
	listing := orderedmap.New[string, []string]()
	listing.Set("1710000000", []string{"a"})
	bulkListing := orderedmap.New[string, string]()
	bulkListing.Set("1710000000", "1710000000,a")

	err := runPublisher(context.Background(), errorPublisher{err: expectedErr}, config{
		RedisChannel:    "test-channel",
		BulkPublish:     true,
		PublishInterval: time.Hour,
	}, listing, bulkListing)

	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected publish error, got %v", err)
	}
}

func TestWaitForRedisReturnsPingErrorWhenRetryDisabled(t *testing.T) {
	expectedErr := errors.New("redis unavailable")
	client := &fakeRedisPinger{errs: []error{expectedErr}}

	err := waitForRedis(context.Background(), client, config{})
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected Redis ping error, got %v", err)
	}

	if client.calls != 1 {
		t.Fatalf("expected one ping attempt, got %d", client.calls)
	}
}

func TestWaitForRedisRetriesUntilPingSucceeds(t *testing.T) {
	client := &fakeRedisPinger{errs: []error{errors.New("redis unavailable")}}

	err := waitForRedis(context.Background(), client, config{
		RedisConnectRetry:         true,
		RedisConnectRetryInterval: time.Nanosecond,
	})
	if err != nil {
		t.Fatalf("waitForRedis returned error: %v", err)
	}

	if client.calls != 2 {
		t.Fatalf("expected two ping attempts, got %d", client.calls)
	}
}

func TestWaitForRedisStopsWhenContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	client := &fakeRedisPinger{
		cancel: cancel,
		errs:   []error{errors.New("redis unavailable")},
	}

	err := waitForRedis(ctx, client, config{
		RedisConnectRetry:         true,
		RedisConnectRetryInterval: time.Hour,
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func clearConfigEnv(t *testing.T) {
	t.Helper()

	for _, key := range []string{
		"DATA_FILE",
		"REDIS_URL",
		"REDIS_CHANNEL",
		"REDIS_CONNECT_RETRY",
		"REDIS_CONNECT_RETRY_INTERVAL",
		"GROUP_BY_HOUR",
		"BULK_PUBLISH",
		"PUBLISH_INTERVAL",
	} {
		t.Setenv(key, "")
	}
}
