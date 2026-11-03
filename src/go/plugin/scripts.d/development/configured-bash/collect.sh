#!/bin/bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Synthetic configured example; requires jq. Works with either manifest mode.
set -eu
source "$(dirname "${BASH_SOURCE[0]}")/../../lib/native.sh"
nd_read_config
queue=$(printf '%s' "$ND_CONFIG" | jq -er '.config.queue')
depth=$(printf '%s' "$ND_CONFIG" | jq -er '.config.depth')
warning=$(printf '%s' "$ND_CONFIG" | jq -er '.config.warning')
unset ND_CONFIG
collect_snapshot() {
    local state=ok
    if (( depth >= warning )); then state=warning; fi
    nd_begin
    nd_metric depth gauge jobs
    nd_sample "$ND_FAMILY" "$depth" queue "$queue"
    nd_check backlog 'Queue Backlog' queue
    nd_check_sample "$ND_FAMILY" "$state" queue "$queue"
    nd_end
}
case ${1:-} in
    collect) collect_snapshot ;;
    serve) nd_ready; while nd_next; do collect_snapshot; done ;;
    *) exit 2 ;;
esac
