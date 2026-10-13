#!/bin/bash
# SPDX-License-Identifier: GPL-3.0-or-later
# No manifest or describe operation. Configure command with /bin/bash and this file.
set -eu
source "$(dirname "${BASH_SOURCE[0]}")/../../lib/native.sh"
collect_snapshot() {
    nd_begin
    nd_metric temperature gauge Celsius
    nd_sample "$ND_FAMILY" 21.5
    nd_end
}
case ${1:-} in
    collect) collect_snapshot ;;
    serve) nd_ready; while nd_next; do collect_snapshot; done ;;
    *) exit 2 ;;
esac
