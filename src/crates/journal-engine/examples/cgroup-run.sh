#!/usr/bin/env bash

set -exu -o pipefail

cargo build --example index

sudo sync
echo 3 | sudo tee /proc/sys/vm/drop_caches
sleep 1

sudo cgexec -g io:/slow-io ../../target/debug/examples/index
