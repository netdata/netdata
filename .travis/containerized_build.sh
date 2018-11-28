#!/bin/bash

set -e

if [ ! -f .gitignore ]
then
  echo "Run as ./travis/$(basename "$0") from top level directory of git repository"
  exit 1
fi

docker run -it -v "${PWD}:/code:rw" -w /code "netdata/os-test:$1" ./netdata-installer.sh --dont-wait --dont-start-it --install /tmp
