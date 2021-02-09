package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strconv"
	"strings"
)

var trimStart = flag.Int("trim-start", 0, "trim N samples from start of stream")
var trimEnd = flag.Int("trim-end", 0, "trim N samples from end of stream")

type result struct {
	Time        float64 // hour.min
	MemUsageMiB float64
	CPUPerc     float64
}

func main() {
	flag.Parse()
	data, err := ioutil.ReadAll(os.Stdin)
	if err != nil {
		log.Fatal(err)
	}

	lines := strings.Split(string(data), "\n")
	results := []result{}
	var current result
	for i, line := range lines {
		if i < *trimStart || i > len(lines)-*trimEnd {
			continue
		}
		if strings.HasPrefix(line, "TIME") {
			split := strings.Split(line, ":") // TIME:14:11:11
			hour, _ := strconv.ParseInt(split[1], 10, 64)
			min, _ := strconv.ParseInt(split[2], 10, 64)
			s, _ := strconv.ParseInt(split[3], 10, 64)
			current.Time = float64(hour) + (float64(min) / 60)
			_ = s
			continue
		}
		var entry struct {
			BlockIO   string
			CPUPerc   string
			Container string
			ID        string
			MemPerc   string
			MemUsage  string
			Name      string
			NetIO     string
			PIDs      string
		}
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			break
		}

		v, err := parseByteSize(strings.Split(entry.MemUsage, " / ")[0])
		if err != nil {
			log.Fatal(err, "\n", line)
		}
		current.MemUsageMiB = v / 1024 / 1024

		current.CPUPerc, _ = strconv.ParseFloat(entry.CPUPerc[:len(entry.CPUPerc)-1], 64)
		results = append(results, current)
	}
	json.NewEncoder(os.Stdout).Encode(results)
}

func parseByteSize(s string) (float64, error) {
	if strings.HasSuffix(s, "KiB") {
		s = strings.TrimSuffix(s, "KiB")
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			fmt.Println("KiB", s)
			return 0, err
		}
		return f * 1024, nil
	}
	if strings.HasSuffix(s, "MiB") {
		s = strings.TrimSuffix(s, "MiB")
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			fmt.Println("MiB", s)
			return 0, err
		}
		return f * 1024 * 1024, nil
	}
	if strings.HasSuffix(s, "GiB") {
		s = strings.TrimSuffix(s, "GiB")
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			fmt.Println("GiB", s)
			return 0, err
		}
		return f * 1024 * 1024 * 1024, nil
	}
	if strings.HasSuffix(s, "B") {
		s = strings.TrimSuffix(s, "B")
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			fmt.Println("B", s)
			return 0, err
		}
		return f, nil
	}
	return 0, nil
}
