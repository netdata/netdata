#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later

# shellcheck source=./packaging/makeself/functions.sh
. "${NETDATA_MAKESELF_PATH}"/functions.sh "${@}" || exit 1

dump_log() {
  cat ./netdata.log
}

wait_for() {
  host="${1}"
  port="${2}"
  name="${3}"
  timeout="30"

  if command -v nc > /dev/null ; then
    netcat="nc"
  elif command -v netcat > /dev/null ; then
    netcat="netcat"
  else
    printf "Unable to find a usable netcat command.\n"
    return 1
  fi

  printf "Waiting for %s on %s:%s ... " "${name}" "${host}" "${port}"

  sleep 30

  i=0
  while ! ${netcat} -z "${host}" "${port}"; do
    sleep 1
    if [ "$i" -gt "$timeout" ]; then
      printf "Timed out!\n"
      return 1
    fi
    i="$((i + 1))"
  done
  printf "OK\n"
}

trap dump_log EXIT

"${NETDATA_INSTALL_PATH}/bin/netdata" -D > ./netdata.log 2>&1 &

wait_for localhost 19999 netdata || exit 1

curl -sS http://127.0.0.1:19999/api/v1/info > ./response || exit 1

cat ./response

jq '.version' ./response || exit 1

trap - EXIT
