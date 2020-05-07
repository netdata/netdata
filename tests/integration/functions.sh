#!/bin/sh

SSH_CMD="ssh -q -o UserKnownHostsFile=/dev/null -o StrictHostKeyChecking=no -p 2222 root@localhost"
EXEC_CMD="docker exec $CONTAINER_NAME"
CURL_CMD="curl -q -s -S --connect-to $HOST_IP:$CONTAINER_PORT"
export SSH_CMD EXEC_CMD CURL_CMD

color() {
  fg="$1"
  bg="${2}"
  ft="${3:-0}"

  printf "\33[%s;%s;%s" "$ft" "$fg" "$bg"
}

color_reset() {
  printf "\033[0m"
}

ok() {
  if [ -t 1 ]; then
    printf "%s[ OK ]%s\n" "$(color 37 42m 1)" "$(color_reset)"
  else
    printf "%s\n" "[ OK ]"
  fi
}

err() {
  if [ -t 1 ]; then
    printf "%s[ ERR ]%s\n" "$(color 37 41m 1)" "$(color_reset)"
  else
    printf "%s\n" "[ ERR ]"
  fi
}

run() {
  retval=0
  logfile="$(mktemp -t "run-XXXXXX")"
  if "$@" 2> "$logfile"; then
    ok
  else
    retval=$?
    err
    cat "$logfile" || true
  fi
  rm -rf "$logfile"
  return $retval
}

progress() {
  printf "%-40s" "$(printf "%s ... " "$1")"
}

log() {
  printf "%s\n" "$1"
}

error() {
  log "ERROR: ${1}"
}

fail() {
  log "FATAL: ${1}"
  exit 1
}
