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

debug() {
  log "Dropping into a shell for debugging ..."
  exec /bin/sh
}

config() {
  if grep "CONFIG_$2" .config; then
    sed -i "s|.*CONFIG_$2.*|CONFIG_$2=$1|" .config
  else
    echo "CONFIG_$2=$1" >> .config
  fi
}

save_argv() {
  for i; do
    printf %s\\n "$i" | sed "s/'/'\\\\''/g;1s/^/'/;\$s/\$/' \\\\/"
  done
  echo " "
}

restore_argv() {
  eval "set -- $*"
}

fnmatch() {
  case "$2" in
    $1)
      return 0
      ;;
    *)
      return 1
      ;;
  esac
}

parse_version() {
  r="${1}"
  s="${2}"
  if echo "${r}" | grep -q '^v.*'; then
    # shellcheck disable=SC2001
    # XXX: Need a regex group subsitutation here.
    r="$(echo "${r}" | sed -e 's/^v\(.*\)/\1/')"
  fi

  old="$(save_argv)"
  eval "set -- $(echo "${r}" | tr '-' ' ')"

  v="$1"
  b="$2"

  if [ -z "$b" ] || fnmatch '*[!0-9]*' "$b"; then
    b=
  fi

  eval "set -- $(echo "${v}" | tr '.' ' ')"
  if [ -n "$b" ]; then
    printf "%03d%s%03d%s%03d%s%03d" "$1" "$s" "$2" "$s" "$3" "$s" "$b"
  else
    printf "%03d%s%03d%s%03d" "$1" "$s" "$2" "$s" "$3"
  fi
  restore_argv "$old"
}

bump_version() {
  v="$1"
  n="$2"

  old="$(save_argv)"
  eval "set -- $(parse_version "$v" " ")"

  major="${1#0*}"
  minor="${2#0*}"
  patch="${3#0*}"
  build="${4#0*}"

  restore_argv "$old"

  case "$n" in
    0)
      major=$((major + 1))
      minor=0
      patch=0
      [ -n "$build" ] && build=0
      ;;
    1)
      minor=$((minor + 1))
      patch=0
      [ -n "$build" ] && build=0
      ;;
    2)
      patch=$((patch + 1))
      [ -n "$build" ] && build=0
      ;;
    3)
      if [ -n "$build" ]; then
        build=$((build + 1))
      fi
      ;;
    *)
      patch=$((patch + 1))
      [ -n "$build" ] && build=0
      ;;
  esac

  if [ -n "$build" ]; then
    printf "%d.%d.%d-%d" "$major" "$minor" "$patch" "$build"
  else
    printf "%d.%d.%d" "$major" "$minor" "$patch"
  fi
}
