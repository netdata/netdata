#!/usr/bin/env bash

set -exu -o pipefail

cargo build --example index

sudo find /mnt/slow-disk/foyer-cache -type f -delete

sudo cgexec -g io:/slow-io ../../target/debug/examples/index
