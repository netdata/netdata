#!/usr/bin/env sh
# Package tree used for installing netdata on distribution:
# << Alpine >>
# supported versions: 3.12, 3.13, 3.14, 3.15, edge

set -e

package_tree="
  alpine-sdk
  git
  gcc
  g++
  automake
  autoconf
  cmake
  make
  libtool
  pkgconfig
  tar
  curl
  gzip
  netcat-openbsd
  libuv-dev
  lz4-dev
  openssl-dev
  elfutils-dev
  python3
  zlib-dev
  util-linux-dev
  libmnl-dev
  json-c-dev
  "

packages_to_install=

for package in $package_tree; do
  if apk -e info $package &> /dev/null; then
    echo "Package '${package}' is installed"
  else
    echo "Package '${package}' is NOT installed"
    packages_to_install="$packages_to_install $package"
  fi
done

if [[ -z "$packages_to_install" ]]; then
  echo "All required packages are already installed. Skipping .."
else
  echo "packages_to_install: $packages_to_install"
  apk add $packages_to_install
fi
