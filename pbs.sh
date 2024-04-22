#!/usr/bin/env bash

set -exu -o pipefail

pbs=(
    "ubuntu20.04" "ubuntu22.04" "ubuntu23.10" "ubuntu24.04"
    "debian10" "debian11" "debian12"
)

for pb in "${pbs[@]}"; do
    git clean -xfd .

    docker run \
        --security-opt seccomp=unconfined \
        -e DISABLE_TELEMETRY=1 \
        -e VERSION=1.1.1 \
        --platform=linux/amd64 \
        -v "$PWD":/netdata \
        "netdata/package-builders:$pb"

    if [[ -d "artifacts" ]]; then
        mv artifacts "/home/vk/artifacts/$pb"
    else
        echo "Could not find artifacts/ directory for package builder >>>${pb}<<<."
        exit 1
    fi
done

