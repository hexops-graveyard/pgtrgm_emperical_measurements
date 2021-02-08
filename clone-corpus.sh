#!/bin/bash
set -e

trap 'kill $(jobs -p)' EXIT

clone_subset() {
    filename=$1
    cat "$filename" | jq -r '.[]' | xargs -L1 | while read -r line; do
        repo_name=$(echo $line | cut -c9- | sed 's#/#\\/#g')
        language=$(basename $filename .json)
        dir="../corpus/$language/$repo_name"

        if [ ! -d "$dir" ] 
        then
            echo "cloning $dir"
            mkdir -p $dir
            git clone --quiet --depth 1 "$line" "$dir"
            rm -r "$dir/.git"
            gfind "$dir" -type f -size +1M -exec rm {} +
        else
            rm -rf "$dir/.git"
            gfind "$dir" -type f -size +1M -exec rm {} +
        fi
    done
}

for filename in top_repos/$1*.json; do clone_subset "$filename" & done

wait
