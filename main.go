package main

import (
	"bufio"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	orderedmap "github.com/wk8/go-ordered-map/v2"
)

func main() {
	hours := true
	bulk := true

	file, err := os.Open("demoData.csv")

	if err != nil {
		log.Fatal("Error while reading the file", err)
	}

	defer file.Close()

	if err != nil {
		log.Fatal("Error reading reports", err)
	}

	var toDateTime = func(d string) time.Time {
		u, _ := strconv.ParseFloat(d, 64)

		return time.Unix(int64(u), 0)
	}

	var truncToHour = func(dt time.Time) time.Time {
		return time.Date(dt.Year(), dt.Month(), dt.Day(), dt.Hour(), 0, 0, 0, time.UTC)
	}

	var toEpoch = func(dt time.Time) string {
		return strconv.Itoa(int(dt.Unix()))
	}

	fileScanner := bufio.NewScanner(file)

	reports := orderedmap.New[string, []string]()
	bulkReports := orderedmap.New[string, string]()

	for fileScanner.Scan() {
		s := strings.SplitN(fileScanner.Text(), ",", 2)
		reports.Set(s[0], strings.Split(s[1], ","))
		bulkReports.Set(s[0], s[1])
	}

	reportsHour := orderedmap.New[string, []string]()
	// reportsHour := orderedmap.New[string, *orderedmap.OrderedMap[string, []string]]()

	for pair := reports.Oldest(); pair != nil; pair = pair.Next() {
		groupKey := toEpoch(truncToHour(toDateTime(pair.Key)))
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
		groupKey := toEpoch(truncToHour(toDateTime(pair.Key)))
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
	if hours {
		listing = reportsHour
		bulkLlisting = reportsHourBulk
	}

	for pair := listing.Oldest(); pair != nil; pair = pair.Next() {
		var dt = toDateTime(pair.Key)

		log.Println(pair.Key + " sending report " + dt.Format("2006-01-02 15:04:05"))

		if bulk {
			value, _ := bulkLlisting.Get(pair.Key)
			log.Println(value)
		} else {
			values, _ := listing.Get(pair.Key)

			for value := range values {
				log.Println(value)
			}
		}

		time.Sleep(2 * time.Second)
	}
}
