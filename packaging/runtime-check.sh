#!/bin/sh

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
      return 2
    fi
    i="$((i + 1))"
  done
  printf "OK\n"
}

wait_for localhost 19999 netdata

case $? in
    1) exit 2 ;;
    2) exit 3 ;;
esac

curl -sfS http://127.0.0.1:19999/api/v1/info > ./response || exit 1

cat ./response

jq '.version' ./response || exit 1

curl -sfS http://127.0.0.1:19999/index.html || exit 1
curl -sfS http://127.0.0.1:19999/v0/index.html || exit 1
curl -sfS http://127.0.0.1:19999/v1/index.html || exit 1
curl -sfS http://127.0.0.1:19999/v2/index.html || exit 1
