#!/usr/bin/env bash
# Package tree used for installing netdata on distribution:
# << Debian >>
# supported versions: 9, 10, 11

set -e

package_tree="
  git
  gcc
  g++
  make
  automake
  cmake
  autoconf
  autoconf-archive
  autogen
  libtool
  pkg-config
  tar
  curl
  gzip
  netcat
  zlib1g-dev
  uuid-dev
  libmnl-dev
  libjson-c-dev
  libuv1-dev
  liblz4-dev
  libssl-dev
  libjudy-dev
  libelf-dev
  python
  python3
  "

packages_to_install=

for package in $package_tree; do
  if dpkg -s $package &> /dev/null; then
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
  apt-get install -y $packages_to_install
fi
