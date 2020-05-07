#!/bin/sh

get_host_ip() {
  if [ "$(uname)" = "Linux" ]; then
    hostname -i
  elif [ "$(uname)" = "Darwin" ]; then
    /sbin/ifconfig | grep -Eo 'inet (addr:)?([0-9]*\.){3}[0-9]*' | grep -Eo '([0-9]*\.){3}[0-9]*' | grep -v -E '(127\.0|169\.254)'
  else
    printf >&2 "FATAL: Unsupported OS %s !" "$(unaem)"
    exit 255
  fi
}

HOST_IP="${HOST_IP:-"$(get_host_ip)"}"
HOST_PORT="${HOST_PORT:-29999}"
export HOST_IP HOST_PORT

CONTAINER_NAME="${CONTAINER_NAME:-netdata}"
CONTAINER_PORT="${CONTAINER_PORT:-19999}"
CONTAINER_WAIT_TIMEOUT="${CONTAINER_WAIT_TIMEOUT:-10s}"
export CONTAINER_NAME CONTAINER_PORT CONTAINER_WAIT_TIMEOUT

BASEURL="http://$HOST_IP:$HOST_PORT"
export BASEURL
