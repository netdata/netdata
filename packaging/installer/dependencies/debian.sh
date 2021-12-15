#!/usr/bin/env bash
# Package tree used for installing netdata on distribution:
# << Debian >>
# supported versions: 9, 10, 11

source "./functions.sh"

set -e

NON_INTERACTIVE=0
DONT_WAIT=0

check_flags ${@}

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
  opts=
  if [ "${NON_INTERACTIVE}" -eq 1 ]; then
    echo >&2 "Running in non-interactive mode"
    export DEBIAN_FRONTEND="noninteractive"
    opts="${opts} -yq"
  fi
  echo "Running apt-get update and updating your APT caches ..."
  apt-get update
  apt-get install ${opts} $packages_to_install 
fi

