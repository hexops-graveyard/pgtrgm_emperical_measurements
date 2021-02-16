#!/bin/bash
set -e

query () {
    date '+TIME:%H:%M:%S'
    echo "query: $1"
    set +e
    PRINTQUERY=false DATABASE=postgres://postgres@127.0.0.1:5432/postgres PARALLEL=32 go run ./cmd/tablesplitgen/main.go query "$1" 100 200
    set -e
}

echo "limit 100: 'error'"
for i in {1..10}; do
    query 'error'
done

echo "limit 100: 'fmt\.Error'"
for i in {1..10}; do
    query 'fmt\.Error'
done

echo "limit 100: 'error'"
for i in {1..10}; do
    query 'error'
done

echo "limit 100: 'fmt\.Println'"
for i in {1..10}; do
    query 'fmt\.Println'
done

echo "limit 100: 'fmt\.Print.*'"
for i in {1..10}; do
    query 'fmt\.Print.*'
done

echo "limit 100: 'var'"
for i in {1..10}; do
    query 'var'
done

echo "limit 100: '123456789'"
for i in {1..10}; do
    query '123456789'
done

echo "limit 100: 'bytes.Buffer'"
for i in {1..10}; do
    query 'bytes.Buffer'
done

echo "limit 100: 'ac8ac5d63b66b83b90ce41a2d4061635'"
for i in {1..10}; do
    query 'ac8ac5d63b66b83b90ce41a2d4061635'
done

echo "limit 100: 'd97f1d3ff91543[e-f]49.8b07517548877'"
for i in {1..10}; do
    query 'd97f1d3ff91543[e-f]49.8b07517548877'
done
