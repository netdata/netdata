#!/usr/bin/env bash
# Reproduce the `index` example (index.rs) under slow I/O: wipe its caches so
# the run measures cold I/O, then execute it inside the throttled slow-io
# cgroup over the /mnt/slow-disk mount.
#
# One-time setup — the delayed slow-disk mount and the slow-io cgroup cap — is
# documented in index.rs's header comments. Run this script from its own
# directory (src/crates/journal-engine/examples/): the binary path at the
# bottom is relative to it.

set -exu -o pipefail

# Build the example (debug profile) into ../../target/debug/examples/index.
cargo build --example index

# Drop the foyer disk-tier files so the run re-indexes from scratch (files
# only; the directory stays). This only bites if index.rs's cache path was
# switched to the slow disk per its "Alternative" note — as shipped the
# example uses /tmp/foyer-cache, which this leaves alone.
sudo find /mnt/slow-disk/foyer-cache -type f -delete

# Make journal reads hit the slow device instead of RAM: flush dirty data,
# drop page cache, dentries and inodes (3), then let it settle.
sudo sync
echo 3 | sudo tee /proc/sys/vm/drop_caches
sleep 1

# Run the example — scan dir defaults to /mnt/slow-disk/otel-aws, last 24h —
# inside slow-io, which index.rs step 5 caps at 10 MiB/s read+write.
sudo cgexec -g io:/slow-io ../../target/debug/examples/index
