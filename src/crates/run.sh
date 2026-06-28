#!/usr/bin/env bash

# Build a standard SFST index from an existing flattened-frame WAL.
#
# This reads the WAL only — it never deletes or re-ingests. Produce the WAL
# separately with `ng-ingest` fed by `otel-streams` producers, e.g.:
#
#   ng-ingest --listen 127.0.0.1:4317 --out ~/repos/tmp/ng/flat --count 500000 &
#   jetstream --otel-endpoint http://127.0.0.1:4317 --batch-size 1000 --flush-interval-ms 1000
#   certstream --otel-endpoint http://127.0.0.1:4317 --batch-size 1000 --flush-interval-ms 1000

set -eu -o pipefail

SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)

cargo build -q --profile profiling --package ng-index

NG_INDEX=$SCRIPT_DIR/target/profiling/ng-index
FLAT_DIR=~/repos/tmp/ng/flat
SFST_OUT=~/repos/tmp/ng/out.sfst

time "$NG_INDEX" --flat "$FLAT_DIR" --sfst "$SFST_OUT"
