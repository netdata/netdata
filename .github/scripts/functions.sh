#!/bin/sh

# This file is included by download.sh & build.sh

set -e

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
    tail -n 100 "$logfile" || true
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

debug() {
  log "Dropping into a shell for debugging ..."
  exec /bin/sh
}
