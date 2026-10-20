#!/bin/bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Synthetic lifecycle example: critical, failed collection, then recovery.
set -eu
source "$(dirname "${BASH_SOURCE[0]}")/../../lib/native.sh"
[[ ${1:-} == serve ]] || exit 2
nd_ready
count=0
while nd_next; do
    count=$((count+1))
    if [[ $count == 2 ]]; then
        nd_fail
        continue
    fi
    state=ok
    [[ $count != 1 ]] || state=critical
    nd_begin
    nd_metric processed_total "$count" queue mail
    nd_check backlog "$state" queue mail
    nd_end
done
