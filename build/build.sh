#!/bin/bash

BUILDER_IMAGE="netdata-builder:v0.1"
DIR=$(pwd)/pack
VERSION="1.10_git"
ITERATION="$(git rev-parse HEAD | cut -c1-8)"

if [ -f build.sh ]; then
    exit 1
fi

if [ "$IS_CONTAINER" != "" ]; then
  case "$1" in
    "prepare")
      autoreconf -ivf
      #./configure --enable-maintainer-mode
      #./configure --prefix=/usr --sysconfdir=/etc --localstatedir=/var --with-zlib --with-math --with-user=netdata CFLAGS=-O2
      ./configure --prefix=/usr --sysconfdir=/etc --localstatedir=/var --with-zlib --with-math --with-user=root CFLAGS=-O2
      ;;
    "fpm")
      make
      make install DESTDIR="${DIR}"
      fpm --output-type rpm \
          --input-type dir \
          --name netdata \
          --license "GPL-3.0" \
          --url "https://my-netdata.io" \
          --maintainer "Netdata Team" \
          --version "${VERSION}" \
          --iteration "${ITERATION}" \
          --package ./ \
          --depends libcap \
          --config-files /etc/netdata \
          --chdir "${DIR}"
      fpm --output-type deb \
          --input-type dir \
          --name netdata \
          --license "GPL-3.0" \
          --url "https://my-netdata.io" \
          --maintainer "Netdata Team" \
          --version "${VERSION}" \
          --iteration "${ITERATION}" \
          --package ./ \
          --depends libcap \
          --config-files /etc/netdata \
          --chdir "${DIR}"
      ;;
    "tarball")
      make dist
      ;;
    "clean") 
      rm -rf autom4te.cache
      git clean -X -f
      ;;
  esac

else
  if [[ "$(docker images -q $BUILDER_IMAGE 2> /dev/null)" == "" ]]; then
      docker build -t $BUILDER_IMAGE -f build/Dockerfile .
  fi
  docker run --rm -it \
    --env IS_CONTAINER=TRUE \
    --volume "${PWD}:/project:Z" \
    --workdir "/project" \
    "$BUILDER_IMAGE" \
    ./build/build.sh "${@}"
fi
