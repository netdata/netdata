#!/bin/bash

set -e

if [ ! -f .gitignore ]
then
  echo "Run as ./travis/$(basename "$0") from top level directory of git repository"
  exit 1
fi

trap "docker rm -f netdata-test" EXIT

# Start long running container
docker run -d -v "${PWD}:/code:rw" -w /code --name netdata-test "netdata/os-test:$1" sleep 1h

# Test installation
docker exec -it netdata-test ./netdata-installer.sh --dont-wait --dont-start-it --install /tmp

# Test update
docker exec -it netdata-test /etc/cron.daily/netdata-updater -f

# Test deinstalation
docker exec -it netdata-test ./netdata-uninstaller.sh --force
