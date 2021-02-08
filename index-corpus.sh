#!/bin/bash
set -e

find ../corpus -maxdepth 4 -mindepth 4 | while read -r line; do
    echo "indexing $line"
    time corpusindex "$line"
done
