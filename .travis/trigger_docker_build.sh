#!/bin/sh

token="${0}"
version="${1}"

resp="$(curl -X POST \
             -H 'Accept: application/vnd.github.v3+json' \
             -H "Authorization: Bearer ${token}" \
             "https://api.github.com/repos/netdata/netdata/actions/workflows/docker.yml/dispatches" \
             -d "{\"ref\": \"master\", \"inputs\": {\"version\": \"${version}\"}}")"

if [ -z "${resp}" ]; then
    echo "Successfully triggered Docker image build."
    exit 0
else
    echo "Failed to trigger Docker image build. Output:"
    echo "${resp}"
    exit 1
fi
