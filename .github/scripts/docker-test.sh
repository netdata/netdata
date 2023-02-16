#!/bin/sh

export DEBIAN_FRONTEND=noninteractive

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
      docker ps -a
      echo "::group::Netdata container logs"
      docker logs netdata 2>&1
      echo "::endgroup::"
      return 1
    fi
    i="$((i + 1))"
  done
  printf "OK\n"
}

if [ -z "$(command -v nc 2>/dev/null)" ] && [ -z "$(command -v netcat 2>/dev/null)" ]; then
    sudo apt-get update && sudo apt-get upgrade -y && sudo apt-get install -y netcat
fi

docker run -d --name=netdata \
           -p 19999:19999 \
           -v netdataconfig:/etc/netdata \
           -v netdatalib:/var/lib/netdata \
           -v netdatacache:/var/cache/netdata \
           -v /etc/passwd:/host/etc/passwd:ro \
           -v /etc/group:/host/etc/group:ro \
           -v /proc:/host/proc:ro \
           -v /sys:/host/sys:ro \
           -v /etc/os-release:/host/etc/os-release:ro \
           --cap-add SYS_PTRACE \
           --security-opt apparmor=unconfined \
           netdata/netdata:test

wait_for localhost 19999 netdata || exit 1

curl -sS http://127.0.0.1:19999/api/v1/info > ./response || exit 1

cat ./response

jq '.version' ./response || exit 1
