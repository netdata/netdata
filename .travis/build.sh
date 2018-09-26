#!/bin/bash

#!/bin/sh
if [ "$IS_CONTAINER" != "" ]; then
  autoreconf -ivf
  ./configure --enable-maintainer-mode
  make dist
else
  docker run --rm -it \
    --env IS_CONTAINER=TRUE \
    --volume "${PWD}:/project:Z" \
    --workdir "/project" \
    netdata:gcc \
    ./.travis/build.sh
#    ./.travis/build.sh "{@}"
fi
