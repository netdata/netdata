#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Run on the completed package tree, never on an installed system.
set -euo pipefail
inventory_failed() {
  printf 'Directory inventory failed: cwd=%q operation=%q status=%s\n' "$PWD" "$2" "$1" >&2
  exit "$1"
}
trap 'inventory_failed "$?" "$BASH_COMMAND"' ERR
printf 'Creating package directory inventory in %q\n' "${1:?package tree required}" >&2
cd "$1"
# Service roots and their contents have explicit initialization/migration policies.
find . \
  \( -path ./var/lib/netdata -o -path ./var/cache/netdata -o -path ./var/log/netdata \) -prune -o \
  -type d -print | LC_ALL=C sort > system/package-directories
