#!/bin/sh

set -e

case "$1" in
  install)
    if ! getent group netdata > /dev/null; then
      addgroup --quiet --system netdata
    fi
    ;;
esac
