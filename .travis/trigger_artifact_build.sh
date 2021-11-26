#!/bin/sh

token="${1}"
version="${2}"
type="${3}"

resp="$(curl -X POST \
             -H 'Accept: application/vnd.github.v3+json' \
             -H "Authorization: Bearer ${token}" \
             "https://api.github.com/repos/netdata/netdata/actions/workflows/build.yml/dispatches" \
             -d "{\"ref\": \"master\", \"inputs\": {\"version\": \"${version}\", \"type\": \"${type}\"}}")"

if [ -z "${resp}" ]; then
    echo "Successfully triggered release artifact build."
    exit 0
else
    echo "Failed to trigger release artifact build. Output:"
    echo "${resp}"
    exit 1
fi
