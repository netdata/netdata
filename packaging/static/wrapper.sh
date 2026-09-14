#!/bin/sh

unset CDPATH
dir="$(cd -- "$(dirname -- "${0}")" && pwd -P)"
export NETDATA_BASH_LOADABLES="DISABLE"
export PATH="${dir}:${PATH}"

exec "${dir}/srv/netdata" "${@}"
