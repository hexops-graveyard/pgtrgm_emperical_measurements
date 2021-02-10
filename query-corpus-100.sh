#!/bin/bash
set -e

query () {
    date '+TIME:%H:%M:%S'
    echo "query: $1"
    set +e
    docker exec -it postgres psql -U postgres -P pager=off -c "\timing" -c "$1"
    set -e
}

echo "limit 100: 'error'"
for i in {1..100}; do
    query "EXPLAIN ANALYZE select count(id) from (select id from files where contents ~ 'error' limit 100) as e;"
done

echo "limit 100: 'fmt\.Error'"
for i in {1..100}; do
    query "EXPLAIN ANALYZE select count(id) from (select id from files where contents ~ 'fmt\.Error' limit 100) as e;"
done

echo "limit 100: 'error'"
for i in {1..100}; do
    query "EXPLAIN ANALYZE select count(id) from (select id from files where contents ~ 'error' limit 100) as e;"
done

echo "limit 100: 'fmt\.Println'"
for i in {1..100}; do
    query "EXPLAIN ANALYZE select count(id) from (select id from files where contents ~ 'fmt\.Println' limit 100) as e;"
done

echo "limit 100: 'fmt\.Print.*'"
for i in {1..100}; do
    query "EXPLAIN ANALYZE select count(id) from (select id from files where contents ~ 'fmt\.Print.*' limit 100) as e;"
done

echo "limit 100: 'var'"
for i in {1..100}; do
    query "EXPLAIN ANALYZE select count(id) from (select id from files where contents ~ 'var' limit 100) as e;"
done

echo "limit 100: '123456789'"
for i in {1..100}; do
    query "EXPLAIN ANALYZE select count(id) from (select id from files where contents ~ '123456789' limit 100) as e;"
done

echo "limit 100: 'bytes.Buffer'"
for i in {1..4}; do
    query "EXPLAIN ANALYZE select count(id) from (select id from files where contents ~ 'bytes\.Buffer' limit 100) as e;"
done

echo "limit 100: 'ac8ac5d63b66b83b90ce41a2d4061635'"
for i in {1..2}; do
    query "EXPLAIN ANALYZE select count(id) from (select id from files where contents ~ 'ac8ac5d63b66b83b90ce41a2d4061635' limit 100) as e;"
done

echo "limit 100: 'd97f1d3ff91543[e-f]49.8b07517548877'"
for i in {1..2}; do
    query "EXPLAIN ANALYZE select count(id) from (select id from files where contents ~ 'd97f1d3ff91543[e-f]49.8b07517548877' limit 100) as e;"
done

