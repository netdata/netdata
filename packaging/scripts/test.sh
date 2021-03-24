#!/bin/sh

dump_log() {
  cat ./netdata.log
}

trap dump_log EXIT

wait_for() {
  host="${1}"
  port="${2}"
  name="${3}"
  timeout="30"

  printf "Waiting for %s on %s:%s ... " "${name}" "${host}" "${port}"

  sleep 30

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

/usr/sbin/netdata -D > ./netdata.log 2>&1 &

wait_for localhost 19999 netdata || exit 1

curl -sS http://127.0.0.1:19999/api/v1/info > ./response || exit 1

cat ./response

jq '.version' ./response || exit 1

trap - EXIT

cp -a /packages/* /artifacts
