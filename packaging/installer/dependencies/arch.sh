#!/usr/bin/env bash
# Package tree used for installing netdata on distribution:
# << ArchLinux >>
# supported versions: base / base-devel

set -e

declare -a package_tree=(
  gcc
  make
  autoconf
  autoconf-archive
  autogen
  automake
  libtool
  cmake
  gnu-netcat
  zlib
  util-linux
  libmnl
  json-c
  libuv
  lz4
  openssl
  judy
  libelf
  git
  pkgconfig
  tar
  curl
  gzip
  python3
  binutils
)

packages_to_install=

for package in ${package_tree[@]}; do
  if pacman -Qn $package &> /dev/null; then
    echo "Package '${package}' is installed"
  else
    echo "Package '$package' is NOT installed"
    packages_to_install="$packages_to_install $package"
  fi
done

if [[ -z $packages_to_install ]]; then
  echo "All required packages are already installed. Skipping .."
else
  echo "packages_to_install: ${packages_to_install[@]}"
  pacman -Sy ${packages_to_install[@]}
fi
