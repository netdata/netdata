#!/bin/sh

wait_for() {
  host="${1}"
  port="${2}"
  name="${3}"
  timeout="${4:-30}"

  printf "Waiting for %s on %s:%s ... " "${name}" "${host}" "${port}"

  i=0
  while ! nc -z "${host}" "${port}"; do
    sleep 1
    if [ "$i" -gt "$timeout" ]; then
      printf "Timed out!\n"
      return 1
    fi
    i="$((i + 1))"
  done
  printf "OK\n"
}

netdata -D > netdata.log 2>&1 &

wait_for localhost 19999 netdata

curl -sS http://127.0.0.1:19999/api/v1/info | jq '.version'
