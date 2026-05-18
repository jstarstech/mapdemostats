package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"

	orderedmap "github.com/wk8/go-ordered-map/v2"
)

var ctx = context.Background()

type config struct {
	DataFile        string
	RedisURL        string
	RedisChannel    string
	GroupByHour     bool
	BulkPublish     bool
	PublishInterval time.Duration
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

	publishInterval, err := parseDurationEnv("PUBLISH_INTERVAL", 2*time.Second)
	if err != nil {
		return config{}, err
	}

	return config{
		DataFile:        getEnvDefault("DATA_FILE", "data/demoData.csv"),
		RedisURL:        getEnvDefault("REDIS_URL", "redis://localhost:6379"),
		RedisChannel:    getEnvDefault("REDIS_CHANNEL", "hub-counts"),
		GroupByHour:     groupByHour,
		BulkPublish:     bulkPublish,
		PublishInterval: publishInterval,
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

	return time.Unix(int64(u), 0), nil
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

func main() {
	if err := loadEnvFile(".env"); err != nil {
		log.Fatal("Error loading env file: ", err)
	}

	config, err := loadConfig()
	if err != nil {
		log.Fatal("Error loading config: ", err)
	}

	file, err := os.Open(config.DataFile)

	if err != nil {
		log.Fatal("Error while reading the file", err)
	}

	defer file.Close()

	opts, err := redis.ParseURL(config.RedisURL)
	if err != nil {
		panic(err)
	}

	rdb := redis.NewClient(opts)
	defer rdb.Close()

	fileScanner := bufio.NewScanner(file)
	reports, bulkReports, err := loadReports(fileScanner)
	if err != nil {
		log.Fatal("Error reading reports: ", err)
	}

	reportsHour := orderedmap.New[string, []string]()
	// reportsHour := orderedmap.New[string, *orderedmap.OrderedMap[string, []string]]()

	for pair := reports.Oldest(); pair != nil; pair = pair.Next() {
		dt, err := toDateTime(pair.Key)
		if err != nil {
			log.Fatal("Error parsing report timestamp: ", err)
		}

		groupKey := toEpoch(truncToHour(dt))
		_, exists := reportsHour.Get(groupKey)

		if exists {
			// reportHour.Set(groupKey, pair.Value)
		} else {
			reportsHour.Set(groupKey, pair.Value)

			// reportsHour.Set(groupKey, orderedmap.New[string, []string](
			// 	orderedmap.WithInitialData(orderedmap.Pair[string, []string]{
			// 		Key:   pair.Key,
			// 		Value: pair.Value,
			// 	})))
		}
	}

	reportsHourBulk := orderedmap.New[string, string]()
	// reportsHourBulk := orderedmap.New[string, *orderedmap.OrderedMap[string, string]]()

	for pair := bulkReports.Oldest(); pair != nil; pair = pair.Next() {
		dt, err := toDateTime(pair.Key)
		if err != nil {
			log.Fatal("Error parsing report timestamp: ", err)
		}

		groupKey := toEpoch(truncToHour(dt))
		_, exists := reportsHourBulk.Get(groupKey)

		if exists {
			// reportHourBulk.Set(groupKey, pair.Value)
		} else {
			reportsHourBulk.Set(groupKey, pair.Value)

			// reportsHourBulk.Set(groupKey, orderedmap.New[string, string](
			// 	orderedmap.WithInitialData(orderedmap.Pair[string, string]{
			// 		Key:   pair.Key,
			// 		Value: pair.Value,
			// 	})))
		}
	}

	listing := reports
	bulkLlisting := bulkReports
	if config.GroupByHour {
		listing = reportsHour
		bulkLlisting = reportsHourBulk
	}

	for {
		for pair := listing.Oldest(); pair != nil; pair = pair.Next() {
			dt, err := toDateTime(pair.Key)
			if err != nil {
				log.Fatal("Error parsing report timestamp: ", err)
			}

			log.Println(pair.Key + " sending report " + dt.Format("2006-01-02 15:04:05"))

			if config.BulkPublish {
				value, _ := bulkLlisting.Get(pair.Key)

				err := rdb.Publish(ctx, config.RedisChannel, value).Err()
				if err != nil {
					panic(err)
				}
			} else {
				values, _ := listing.Get(pair.Key)

				for _, value := range values {
					err := rdb.Publish(ctx, config.RedisChannel, value).Err()
					if err != nil {
						panic(err)
					}
				}
			}

			time.Sleep(config.PublishInterval)
		}

	}
}
