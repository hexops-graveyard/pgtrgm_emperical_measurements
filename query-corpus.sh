#!/bin/bash
set -e

echo "BEGIN ./query-corpus-10.sh"
time ./query-corpus-10.sh

echo "BEGIN ./query-corpus-100.sh"
time ./query-corpus-100.sh

echo "BEGIN ./query-corpus-1000.sh"
time ./query-corpus-1000.sh

echo "BEGIN ./query-corpus-unlimited.sh"
time ./query-corpus-unlimited.sh
