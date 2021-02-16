#!/bin/bash
set -e

echo "BEGIN ./query-split-corpus-10.sh"
time ./query-split-corpus-10.sh

echo "BEGIN ./query-corpus-100.sh"
time ./query-split-corpus-100.sh

echo "BEGIN ./query-split-corpus-1000.sh"
time ./query-split-corpus-1000.sh

echo "BEGIN ./query-split-corpus-unlimited.sh"
time ./query-split-corpus-unlimited.sh
