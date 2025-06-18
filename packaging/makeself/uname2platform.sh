#!/bin/sh
#
# SPDX-License-Identifier: GPL-3.0-or-later

set -e

BUILDARCH="${1}"

case "${BUILDARCH}" in
  x86_64) echo "linux/amd64" ;;
  armv6l) echo "linux/arm/v6" ;;
  armv7l) echo "linux/arm/v7" ;;
  aarch64) echo "linux/arm64/v8" ;;
  *)
    echo "Unknown target architecture '${BUILDARCH}'." >&2
    exit 1
    ;;
esac
