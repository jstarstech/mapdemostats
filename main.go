package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
	"github.com/redis/go-redis/v9/logging"

	orderedmap "github.com/wk8/go-ordered-map/v2"
)

type redisClient interface {
	Publish(ctx context.Context, channel string, message interface{}) *redis.IntCmd
	Ping(ctx context.Context) *redis.StatusCmd
}

type config struct {
	DataFile                  string
	RedisURL                  string
	RedisChannel              string
	RedisConnectRetry         bool
	RedisConnectRetryInterval time.Duration
	GroupByHour               bool
	BulkPublish               bool
	PublishInterval           time.Duration
}

func loadConfig() (config, error) {
	groupByHour, err := parseBoolEnv("GROUP_BY_HOUR", true)
	if err != nil {
		return config{}, err
	}

	bulkPublish, err := parseBoolEnv("BULK_PUBLISH", true)
	if err != nil {
		return config{}, err
	}

	redisConnectRetry, err := parseBoolEnv("REDIS_CONNECT_RETRY", true)
	if err != nil {
		return config{}, err
	}

	redisConnectRetryInterval, err := parseDurationEnv("REDIS_CONNECT_RETRY_INTERVAL", 2*time.Second)
	if err != nil {
		return config{}, err
	}

	publishInterval, err := parseDurationEnv("PUBLISH_INTERVAL", 2*time.Second)
	if err != nil {
		return config{}, err
	}

	return config{
		DataFile:                  getEnvDefault("DATA_FILE", "data/demoData.csv"),
		RedisURL:                  getEnvDefault("REDIS_URL", "redis://localhost:6379"),
		RedisChannel:              getEnvDefault("REDIS_CHANNEL", "hub-counts"),
		RedisConnectRetry:         redisConnectRetry,
		RedisConnectRetryInterval: redisConnectRetryInterval,
		GroupByHour:               groupByHour,
		BulkPublish:               bulkPublish,
		PublishInterval:           publishInterval,
	}, nil
}

func getEnvDefault(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}

	return value
}

func parseBoolEnv(key string, fallback bool) (bool, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback, nil
	}

	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("parse %s: %w", key, err)
	}

	return parsed, nil
}

func parseDurationEnv(key string, fallback time.Duration) (time.Duration, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback, nil
	}

	parsed, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", key, err)
	}

	if parsed <= 0 {
		return 0, fmt.Errorf("parse %s: duration must be positive", key)
	}

	return parsed, nil
}

func toDateTime(d string) (time.Time, error) {
	u, err := strconv.ParseFloat(d, 64)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse timestamp %q: %w", d, err)
	}

	return time.Unix(int64(u), 0).UTC(), nil
}

func truncToHour(dt time.Time) time.Time {
	return time.Date(dt.Year(), dt.Month(), dt.Day(), dt.Hour(), 0, 0, 0, time.UTC)
}

func toEpoch(dt time.Time) string {
	return strconv.Itoa(int(dt.Unix()))
}

func loadEnvFile(path string) error {
	if err := godotenv.Load(path); err != nil && !os.IsNotExist(err) {
		return err
	}

	return nil
}

func loadReports(scanner *bufio.Scanner) (*orderedmap.OrderedMap[string, []string], *orderedmap.OrderedMap[string, string], error) {
	reports := orderedmap.New[string, []string]()
	bulkReports := orderedmap.New[string, string]()

	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}

		key, rawValues, ok := strings.Cut(line, ",")
		if !ok || key == "" || rawValues == "" {
			return nil, nil, fmt.Errorf("invalid report line: %q", line)
		}

		reports.Set(key, strings.Split(rawValues, ","))
		bulkReports.Set(key, line)
	}

	if err := scanner.Err(); err != nil {
		return nil, nil, err
	}

	return reports, bulkReports, nil
}

func selectReports(
	reports *orderedmap.OrderedMap[string, []string],
	bulkReports *orderedmap.OrderedMap[string, string],
	groupByHour bool,
) (*orderedmap.OrderedMap[string, []string], *orderedmap.OrderedMap[string, string], error) {
	if !groupByHour {
		return reports, bulkReports, nil
	}

	reportsHour := orderedmap.New[string, []string]()
	for pair := reports.Oldest(); pair != nil; pair = pair.Next() {
		dt, err := toDateTime(pair.Key)
		if err != nil {
			return nil, nil, fmt.Errorf("parse report timestamp: %w", err)
		}

		groupKey := toEpoch(truncToHour(dt))
		if _, exists := reportsHour.Get(groupKey); !exists {
			reportsHour.Set(groupKey, pair.Value)
		}
	}

	reportsHourBulk := orderedmap.New[string, string]()
	for pair := bulkReports.Oldest(); pair != nil; pair = pair.Next() {
		dt, err := toDateTime(pair.Key)
		if err != nil {
			return nil, nil, fmt.Errorf("parse report timestamp: %w", err)
		}

		groupKey := toEpoch(truncToHour(dt))
		if _, exists := reportsHourBulk.Get(groupKey); !exists {
			reportsHourBulk.Set(groupKey, pair.Value)
		}
	}

	return reportsHour, reportsHourBulk, nil
}

func runPublisher(
	ctx context.Context,
	client redisClient,
	config config,
	listing *orderedmap.OrderedMap[string, []string],
	bulkListing *orderedmap.OrderedMap[string, string],
) error {
	for {
		for pair := listing.Oldest(); pair != nil; pair = pair.Next() {
			if err := ctx.Err(); err != nil {
				return err
			}

			dt, err := toDateTime(pair.Key)
			if err != nil {
				return fmt.Errorf("parse report timestamp: %w", err)
			}

			log.Println(pair.Key + " sending report " + dt.Format("2006-01-02 15:04:05"))

			if config.BulkPublish {
				value, _ := bulkListing.Get(pair.Key)
				if err := publishWithRedisRecovery(ctx, client, config, value); err != nil {
					return err
				}
			} else {
				values, _ := listing.Get(pair.Key)
				for _, value := range values {
					if err := ctx.Err(); err != nil {
						return err
					}

					if err := publishWithRedisRecovery(ctx, client, config, value); err != nil {
						return err
					}
				}
			}

			timer := time.NewTimer(config.PublishInterval)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
		}
	}
}

func publishWithRedisRecovery(ctx context.Context, client redisClient, config config, value string) error {
	for {
		if err := client.Publish(ctx, config.RedisChannel, value).Err(); err == nil {
			return nil
		} else if !config.RedisConnectRetry {
			return err
		} else {
			log.Printf("Redis publish failed, waiting for recovery: %v", err)
		}

		if err := waitForRedis(ctx, client, config); err != nil {
			return err
		}
	}
}

func waitForRedis(ctx context.Context, client redisClient, config config) error {
	for {
		if err := client.Ping(ctx).Err(); err == nil {
			return nil
		} else if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		} else if !config.RedisConnectRetry {
			return fmt.Errorf("ping redis: %w", err)
		} else {
			log.Printf("Redis ping failed, retrying in %s: %v", config.RedisConnectRetryInterval, err)
		}

		timer := time.NewTimer(config.RedisConnectRetryInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func main() {
	redis.SetLogger(logging.NewBlacklistLogger([]string{
		"connection pool: failed to dial",
	}))

	if err := loadEnvFile(".env"); err != nil {
		log.Fatal("Error loading env file: ", err)
	}

	config, err := loadConfig()
	if err != nil {
		log.Fatal("Error loading config: ", err)
	}

	opts, err := redis.ParseURL(config.RedisURL)
	if err != nil {
		panic(err)
	}

	rdb := redis.NewClient(opts)
	defer rdb.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := waitForRedis(ctx, rdb, config); err != nil {
		if errors.Is(err, context.Canceled) {
			return
		}

		log.Fatal("Error connecting to Redis: ", err)
	}

	file, err := os.Open(config.DataFile)

	if err != nil {
		log.Fatal("Error while reading the file", err)
	}

	defer file.Close()

	fileScanner := bufio.NewScanner(file)
	reports, bulkReports, err := loadReports(fileScanner)
	if err != nil {
		log.Fatal("Error reading reports: ", err)
	}

	listing, bulkListing, err := selectReports(reports, bulkReports, config.GroupByHour)
	if err != nil {
		log.Fatal("Error grouping reports: ", err)
	}

	if err := runPublisher(ctx, rdb, config, listing, bulkListing); err != nil && !errors.Is(err, context.Canceled) {
		log.Fatal("Error publishing reports: ", err)
	}
}
