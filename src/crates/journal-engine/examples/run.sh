#!/usr/bin/env bash

set -eu -o pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
WORKSPACE="${SCRIPT_DIR}/../.."
BINARY="${WORKSPACE}/target/release/examples/index"

RUSTFLAGS="-A warnings" cargo build --release -p journal-engine --example index \
    --features allocative \
    --manifest-path "${WORKSPACE}/Cargo.toml"

exec "$BINARY" "$@"
