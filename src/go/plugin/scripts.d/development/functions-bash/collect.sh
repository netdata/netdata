#!/bin/bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Synthetic mixed collection/Function package. Requires Bash and jq.
set -eu
source "$(dirname "${BASH_SOURCE[0]}")/../../lib/native.sh"
collect_snapshot() {
    nd_begin
    nd_metric depth gauge jobs
    nd_sample "$ND_FAMILY" 17 queue mail
    nd_end
}
answer_request() {
    local id method result
    id=$(printf '%s' "$ND_REQUEST" | jq -er '.id')
    method=$(printf '%s' "$ND_REQUEST" | jq -er '.method')
    case $method in
        collect) result=$(collect_snapshot) ;;
        function)
            # Raw input means this script owns selector defaults and validation.
            # Keep request values on stdin; jq's program is fixed code.
            result=$(printf '%s' "$ND_REQUEST" | jq -ce '
                if .function != "items" then
                    {version:"v1",status:404,message:"Unknown Function"}
                elif .info then {version:"v1",status:200}
                else
                    (if .payload_base64 then (.payload_base64 | @base64d | fromjson) else {} end) as $payload |
                    ([.args[] | select(startswith("queue:") or startswith("queue=")) | .[6:]] +
                     ($payload.selections.queue // []) +
                     [($payload.queue // empty)]) as $values |
                    ($values[0] // "mail") as $queue |
                    if ($values | length) > 1 or ($queue != "mail" and $queue != "batch") then
                        {version:"v1",status:400,message:"Choose one known queue"}
                    else
                        {version:"v1",status:200,
                         columns:{queue:{index:0,name:"Queue",type:"string",unique_key:true},
                                  depth:{index:1,name:"Depth",type:"integer",units:"jobs"}},
                         data:[[$queue,17]],default_sort_column:"depth"}
                    end
                end' 2>/dev/null) || result='{"version":"v1","status":400,"message":"Invalid Function input"}'
            ;;
        *) return 2 ;;
    esac
    nd_reply "$id" "$result"
}
case ${1:-} in
    collect) collect_snapshot ;;
    function) nd_read_request; answer_request ;;
    serve) nd_ready; while nd_read_request; do answer_request; done ;;
    *) exit 2 ;;
esac
