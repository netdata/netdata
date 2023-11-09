#!/bin/bash

version=$1

if [[ $version =~ v([0-9]+)\.([0-9]+)\.([0-9]+)-([0-9]+)-nightly ]]; then
    msb=${BASH_REMATCH[1]}}
    folder="packaging/agent-metadata/nightly"
    filename="latest_v${msb}"
else
    if [[ $version =~ v([0-9]+)\.([0-9]+)\.([0-9]+) ]]; then
        msb=${BASH_REMATCH[1]}
        folder="packaging/agent-metadata/stable"
        filename="latest_v${msb}"
    else
        echo "Invalid version format."
        exit 1
    fi
fi

mkdir -p $folder
echo $version > "$folder/$filename"
