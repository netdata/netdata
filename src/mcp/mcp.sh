#!/bin/bash
# SPDX-License-Identifier: GPL-3.0-or-later

if [ -z "$1" ] || [ "$1" = "help" ] || [ "$1" = "hello" ]; then
  cat README.md
  exit 0
fi

scope_nodes=""
scope_contexts=""
scope_after=""
scope_before=""

nd_with_scope() {
  local url="http://localhost:19999${1}?dummy"
  if [ ! -z "$scope_nodes" ]; then
    url="${url}&scope_nodes=${scope_nodes}"
  fi
  if [ ! -z "$scope_contexts" ]; then
    url="${url}&scope_contexts=${scope_contexts}"
  fi
  if [ ! -z "$scope_after" ]; then
    url="${url}&after=${scope_after}"
  fi
  if [ ! -z "$scope_before" ]; then
    url="${url}&before=${scope_before}"
  fi

  while [ ! -z "${1}" ]; do
    url="${url}&${1}"
    shift
  done

  curl -sS "${url}"
}

context_categories() {
  nd_with_scope /api/v3/contexts | jq '
    .contexts |
    keys_unsorted |
    map(
      rindex(".") as $ix |
      if $ix then .[:$ix] else . end
    ) |
    group_by(.) |
    map({
      "category": .[0],
      "contexts": length
    })
  '
}

alerts_active() {
  nd_with_scope /api/v2/alerts options=summary,instances,values,minify status=raised |
    sed -e 's/"ni":/"node_index":/g' \
        -e 's/"mg":/"machine_guid":/g' \
        -e 's/"nd":/"mode_uuid":/g' \
        -e 's/"nm":/"name":/g' \
        -e 's/"fami":/"family":/g' \
        -e 's/"sum":/"summary":/g' \
        -e 's/"ctx":/"context":/g' \
        -e 's/"st":/"status":/g' \
        -e 's/"src":/"source":/g' \
        -e 's/"to":/"recipient":/g' \
        -e 's/"tr_i":/"last_status_change_id":/g' \
        -e 's/"tr_v":/"last_status_change_value":/g' \
        -e 's/"tr_t":/"last_status_change_timestamp":/g' \
        -e 's/"v":/"latest_value":/g' \
        -e 's/"t":/"latest_timestamp":/g' \
        -e 's/"cr":/"critical_count":/g' \
        -e 's/"wr":/"warning_count":/g' \
        -e 's/"cl":/"clear_count":/g' \
        -e 's/"er":/"error_count":/g' \
        -e 's/"tp":/"type":/g' \
        -e 's/"cm":/"component":/g' \
        -e 's/"cl":/"classification":/g' \
        -e 's/"gi":/"global_unique_id":/g' \
        -e 's/"ch":/"instance_id":/g' \
        -e 's/"ch_n":/"instance_name":/g' \
        -e 's/"ati":/"alert_index":/g' \
      | jq .
}

nodes_info() {
  nd_with_scope /api/v3/nodes |
    sed -e 's/"ni":/"node_index":/g' \
        -e 's/"mg":/"machine_guid":/g' \
        -e 's/"nd":/"mode_uuid":/g' \
        -e 's/"nm":/"name":/g' \
        -e 's/"v":/"version":/g' \
      | jq .
}

endpoint="${1}"
shift

while [ ! -z "${1}" ]; do
  case "$1" in
    scope-nodes|scope_nodes)
      scope_nodes="$2"
      shift 2
      ;;
    scope-nodes=*|scope_nodes=*)
      scope_nodes="$(echo "$1" | cut -d'=' -f2)"
      shift 1
      ;;
    scope-contexts|scope_contexts)
      scope_contexts="$2"
      shift 2
      ;;
    scope-contexts=*|scope_contexts=*)
      scope_contexts="$(echo "$1" | cut -d'=' -f2)"
      shift 1
      ;;
    after)
      scope_after="$2"
      shift 2
      ;;
    after=*)
      scope_after="$(echo "$1" | cut -d'=' -f2)"
      shift 1
      ;;
    before)
      scope_before="$2"
      shift 2
      ;;
    before=*)
      scope_before="$(echo "$1" | cut -d'=' -f2)"
      shift 1
      ;;
    *)
      echo "Unknown option: $1"
      exit 1
  esac
done

case "${endpoint}" in
  context-categories)
    context_categories
    ;;
  alerts-active)
    alerts_active
    ;;
  nodes-info)
    nodes_info
    ;;
  *)
    echo "Unknown endpoint: $endpoint"
    exit 1
esac
