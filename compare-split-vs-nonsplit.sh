#!/bin/bash
set -e

compare () {
    before_total=$(cat ./query_logs/query-run-2.log ./query_logs/query-run-3.log | go run ./cmd/visualize-query-log/main.go | jq ".[] | select (.Limit == $1)" | jq -c -s '.[]' | wc -l)
    before=$(cat ./query_logs/query-run-2.log ./query_logs/query-run-3.log | go run ./cmd/visualize-query-log/main.go | jq ".[] | select (.Limit == $1) | select (.ExecutionTimeMs < $2)" | jq -c -s '.[]' | wc -l)
    before_percentage=$(echo "scale=2 ; ($before / $before_total) * 100.0" | bc)

    after_total=$(cat ./query_logs/query-run-split-index-1.log | go run ./cmd/visualize-query-log/main.go | jq ".[] | select (.Limit == $1)" | jq -c -s '.[]' | wc -l)
    after=$(cat ./query_logs/query-run-split-index-1.log | go run ./cmd/visualize-query-log/main.go | jq ".[] | select (.Limit == $1) | select (.ExecutionTimeMs < $2)" | jq -c -s '.[]' | wc -l)
    after_percentage=$(echo "scale=2 ; ($after / $after_total) * 100.0" | bc)

    echo "| $2ms | $before_percentage% ($before of $before_total) | $after_percentage% ($after of $after_total) |"
}

test () {
    echo "| Time bucket | Percentage of queries (before) | Percentage of queries (after splitting) |"
    echo "|-------------|--------------------------------|-----------------------------------------|"
    compare $1 50
    compare $1 100
    compare $1 250
    compare $1 500
    compare $1 1000
    compare $1 2500
    compare $1 5000
    compare $1 10000
    compare $1 20000
    compare $1 30000
    compare $1 40000
    compare $1 50000
    compare $1 60000
}

test $1
