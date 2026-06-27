#!/usr/bin/env bash

set -eu -o pipefail

cargo build -q --profile profiling --package ng-index

TARGET=/home/vk/repos/nd/sjr-tree8/src/crates/target
NG_INDEX=$TARGET/profiling/ng-index

PROTOBUF_WAL=~/repos/tmp/ng/ec94ba22c03d4e27bc6c1709269525cb-7f7ecdcbbde0486ca91e2db86060a68a-00000-0000000001-0000000000000000.wal
FLAT_DIR=~/repos/tmp/ng/flat

# Phase 1 (convert once): flatten the protobuf WAL into a flattened WAL. Re-run
# only when the source WAL changes; comment out to iterate on phase 2 alone.
mkdir -p "$FLAT_DIR"
$NG_INDEX --convert --in "$PROTOBUF_WAL" --flat "$FLAT_DIR"

# Phase 2 (the measured step): merge the flattened WAL into one global tree.
time $NG_INDEX --flat "$FLAT_DIR"
