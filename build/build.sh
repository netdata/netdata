#!/bin/bash

if [ -f build.sh ]; then
    cd ../ || exit 1
fi

if [ "$IS_CONTAINER" != "" ]; then
  autoreconf -ivf
  ./configure --enable-maintainer-mode
  make dist
  rm -rf autom4te.cache
else
  if [[ "$(docker images -q netdata-builder:latest 2> /dev/null)" == "" ]]; then
      docker build -t netdata-builder:latest -f build/Dockerfile .
  fi
  docker run --rm -it \
    --env IS_CONTAINER=TRUE \
    --volume "${PWD}:/project:Z" \
    --workdir "/project" \
    netdata-builder:latest \
    ./build/build.sh
fi
