#!/bin/bash
set -e

query () {
    date '+TIME:%H:%M:%S'
    echo "query: $1"
    set +e
    docker exec -it postgres psql -U postgres -P pager=off -c "\timing" -c "$1"
    set -e
}

echo "unlimited: 'error'"
for i in {1..2}; do
    query "EXPLAIN ANALYZE select count(id) from (select id from files where contents ~ 'error') as e;"
done

echo "unlimited: 'fmt\.Error'"
for i in {1..2}; do
    query "EXPLAIN ANALYZE select count(id) from (select id from files where contents ~ 'fmt\.Error') as e;"
done

echo "unlimited: 'error'"
for i in {1..2}; do
    query "EXPLAIN ANALYZE select count(id) from (select id from files where contents ~ 'error') as e;"
done

echo "unlimited: 'fmt\.Println'"
for i in {1..2}; do
    query "EXPLAIN ANALYZE select count(id) from (select id from files where contents ~ 'fmt\.Println') as e;"
done

echo "unlimited: 'fmt\.Print.*'"
for i in {1..2}; do
    query "EXPLAIN ANALYZE select count(id) from (select id from files where contents ~ 'fmt\.Print.*') as e;"
done

echo "unlimited: 'var'"
for i in {1..2}; do
    query "EXPLAIN ANALYZE select count(id) from (select id from files where contents ~ 'var') as e;"
done

echo "unlimited: '123456789'"
for i in {1..2}; do
    query "EXPLAIN ANALYZE select count(id) from (select id from files where contents ~ '123456789') as e;"
done

echo "unlimited: 'bytes.Buffer'"
for i in {1..2}; do
    query "EXPLAIN ANALYZE select count(id) from (select id from files where contents ~ 'bytes\.Buffer') as e;"
done

echo "unlimited: 'ac8ac5d63b66b83b90ce41a2d4061635'"
for i in {1..2}; do
    query "EXPLAIN ANALYZE select count(id) from (select id from files where contents ~ 'ac8ac5d63b66b83b90ce41a2d4061635') as e;"
done

echo "unlimited: 'd97f1d3ff91543[e-f]49.8b07517548877'"
for i in {1..2}; do
    query "EXPLAIN ANALYZE select count(id) from (select id from files where contents ~ 'd97f1d3ff91543[e-f]49.8b07517548877') as e;"
done

