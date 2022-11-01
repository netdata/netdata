#!/bin/sh

token="${1}"
version="${2}"
pkgtype="${3}"

resp="$(curl -X POST \
             -H 'Accept: application/vnd.github.v3+json' \
             -H "Authorization: Bearer ${token}" \
             "https://api.github.com/repos/netdata/netdata/actions/workflows/packaging.yml/dispatches" \
             -d "{\"ref\": \"master\", \"inputs\": {\"version\": \"${version}\", \"type\": \"${pkgtype}\"}}")"

if [ -z "${resp}" ]; then
    echo "Successfully triggered binary package builds."
    exit 0
else
    echo "Failed to trigger binary package builds. Output:"
    echo "${resp}"
    exit 1
fi
