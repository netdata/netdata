#!/usr/bin/env bash

set -eu -o pipefail

cargo build -q --profile profiling --package ng-index

TARGET=/home/vk/repos/nd/sjr-tree8/src/crates/target

$TARGET/profiling/ng-index \
    --in ~/repos/tmp/ng/ec94ba22c03d4e27bc6c1709269525cb-7f7ecdcbbde0486ca91e2db86060a68a-00000-0000000001-0000000000000000.wal
