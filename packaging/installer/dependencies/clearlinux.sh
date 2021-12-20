#!/usr/bin/env bash
# Package tree used for installing netdata on distribution:
# << ClearLinux >>
# supported versions: base

source ./functions.sh

set -e

NON_INTERACTIVE=0
DONT_WAIT=0

check_flags ${@}

declare -a package_tree=(
  c-basic
  make
  sysadmin-basic
  devpkg-zlib
  devpkg-util-linux
  devpkg-libmnl
  devpkg-json-c
  devpkg-libuv
  devpkg-lz4
  devpkg-openssl
  devpkg-elfutils
  git
  findutils
  curl
  gzip
  python3-basic
)

packages_to_install=

for package in ${package_tree[@]}; do
  if [[ "$(swupd bundle-info $package | grep Status | cut -d':' -f2)" == " Not installed" ]]; then
    echo "Package '$package' is NOT installed"
    packages_to_install="$packages_to_install $package"
  else
    echo "Package '$package' is installed"
  fi
done

if [[ -z $packages_to_install ]]; then
  echo "All required packages are already installed. Skipping .."
else
  echo "packages_to_install: ${packages_to_install[@]}"
  swupd bundle-add ${packages_to_install[@]}
fi

