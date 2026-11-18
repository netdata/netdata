#!/bin/sh

unset CDPATH
case "${0}" in
  */*) dir="${0%/*}" ;;
  *) dir=. ;;
esac
dir="$(cd -- "${dir}" && pwd -P)"
export NETDATA_BASH_LOADABLES="DISABLE"
export PATH="${dir}:${PATH}"

exec "${dir}/srv/netdata" "${@}"
