package main

import (
	"encoding/json"
	"flag"
	"io/ioutil"
	"log"
	"os"
	"strconv"
	"strings"
)

var (
	maxPlanningTimeMs  = flag.Float64("max-planning-time-ms", 0, "limit PlanningTimeMs to this max value")
	maxExecutionTimeMs = flag.Float64("max-execution-time-ms", 0, "limit ExecutionTimeMs to this max value")
)

type result struct {
	Time            float64 // hour.min timestamp
	Timeout         bool
	PlanningTimeMs  float64
	ExecutionTimeMs float64
	Limit           float64
	IndexRechecks   float64
	Query           string
	Rows            float64
}

func main() {
	flag.Parse()
	data, err := ioutil.ReadAll(os.Stdin)
	if err != nil {
		log.Fatal(err)
	}

	var (
		lines   = strings.Split(string(data), "\n")
		results []result
		current result
		limit   float64
	)
	for _, line := range lines {
		current.Limit = limit
		switch {
		case strings.HasPrefix(line, "TIME"):
			split := strings.Split(line, ":") // TIME:14:11:11
			hour, _ := strconv.ParseInt(split[1], 10, 64)
			min, _ := strconv.ParseInt(split[2], 10, 64)
			s, _ := strconv.ParseInt(split[3], 10, 64)
			current.Time = float64(hour) + (float64(min) / 60)
			_ = s
		case strings.HasPrefix(line, "BEGIN"):
			s := strings.TrimPrefix(line, "BEGIN ./query-corpus-")
			s = strings.TrimSuffix(s, "\r")
			s = strings.TrimSuffix(s, ".sh")
		case strings.HasPrefix(line, "limit ") || strings.HasPrefix(line, "unlimited"):
			current.Query = strings.Split(line, ": '")[1]
			current.Query = current.Query[:len(current.Query)-1]
			if strings.HasPrefix(line, "unlimited") {
				limit = -1
			} else {
				fields := strings.Fields(line) // ["limit", "10:", "'error'"]
				limitStr := strings.TrimSuffix(fields[1], ":")
				limit, err = strconv.ParseFloat(limitStr, 64)
				if err != nil {
					panic(err)
				}
			}
		case strings.Contains(line, "Rows Removed by Index Recheck: "):
			s := strings.TrimSpace(line)
			s = strings.TrimPrefix(s, "Rows Removed by Index Recheck: ")
			s = strings.TrimSuffix(s, "\r")
			current.IndexRechecks, _ = strconv.ParseFloat(s, 64)
		case strings.Contains(line, "Planning Time"):
			s := strings.Split(line, ":")[1]
			s = strings.TrimPrefix(s, " ")
			s = strings.TrimSuffix(s, " ms\r")
			current.PlanningTimeMs, _ = strconv.ParseFloat(s, 64)
			if *maxPlanningTimeMs != 0 {
				if current.PlanningTimeMs > *maxPlanningTimeMs {
					current.PlanningTimeMs = *maxPlanningTimeMs
				}
			}
		case strings.Contains(line, "Execution Time"):
			s := strings.Split(line, ":")[1]
			s = strings.TrimPrefix(s, " ")
			s = strings.TrimSuffix(s, " ms\r")
			current.ExecutionTimeMs, _ = strconv.ParseFloat(s, 64)
			if *maxExecutionTimeMs != 0 {
				if current.ExecutionTimeMs > *maxExecutionTimeMs {
					current.ExecutionTimeMs = *maxExecutionTimeMs
				}
			}
		case strings.Contains(line, " rows)"):
			s := strings.TrimPrefix(line, "(")
			s = strings.Split(s, " ")[0]
			current.Rows, _ = strconv.ParseFloat(s, 64)
			results = append(results, current)
		case strings.Contains(line, " results in "):
			fields := strings.Fields(line) // ["10", "results", "in", "170ms"]
			current.Rows, err = strconv.ParseFloat(fields[0], 64)
			if err != nil {
				panic(err)
			}
			current.ExecutionTimeMs, err = strconv.ParseFloat(strings.TrimSuffix(fields[3], "ms"), 64)
			if err != nil {
				panic(err)
			}
			results = append(results, current)
		case strings.HasPrefix(line, "ERROR:"):
			if strings.Contains(line, "canceling statement due to statement timeout") {
				current.Timeout = true
				results = append(results, current)
			} else {
				panic(line)
			}
		default:
		}
	}
	json.NewEncoder(os.Stdout).Encode(results)
}
