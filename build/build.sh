#!/bin/bash

if [ ! -f .gitignore ]; then
	echo "Run as ./travis/$(basename "$0") from top level directory of git repository"
	exit 1
fi

if [ "$IS_CONTAINER" != "" ]; then
  autoreconf -ivf
  ./configure --enable-maintainer-mode
  make dist
  rm -rf autom4te.cache
else
  docker run --rm -it \
    --env IS_CONTAINER=TRUE \
    --volume "${PWD}:/project:Z" \
    --workdir "/project" \
    netdata/builder:gcc \
    ./build/build.sh
fi
