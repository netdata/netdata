#!/bin/sh
# shellcheck disable=SC1090

. "$(dirname "$0")/functions.sh"

_main() {
  until nc -z "$1" "$2"; do
    sleep 0.1
  done
}

if [ -n "$0" ] && [ x"$0" != x"-bash" ]; then
  _main "$@"
fi
