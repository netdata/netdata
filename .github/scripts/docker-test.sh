#!/bin/sh

SCRIPT_DIR="$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd -P)"

export DEBIAN_FRONTEND=noninteractive

if [ -z "$(command -v nc 2>/dev/null)" ] && [ -z "$(command -v netcat 2>/dev/null)" ]; then
    sudo apt-get update && sudo apt-get upgrade -y && sudo apt-get install -y netcat
fi

echo "::group::Netdata buildinfo"
docker run -i --rm netdata/netdata:test -W buildinfo
echo "::endgroup::"

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
           -e DISABLE_TELEMETRY=1 \
           netdata/netdata:test

if ! "${SCRIPT_DIR}/../../packaging/runtime-check.sh"; then
  docker ps -a
  echo "::group::Netdata container logs"
  docker logs netdata 2>&1
  echo "::endgroup::"
  exit 1
fi
