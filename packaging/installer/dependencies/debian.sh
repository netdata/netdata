#!/usr/bin/env bash
# Package tree used for installing netdata on distribution:
# << Debian >>
# supported versions: 8, 9, 10, 11

set -e

function os_version {
  if [[ -f /etc/os-release ]]; then
    cat /etc/os-release | grep VERSION_ID | cut -d'=' -f2 | cut -d'"' -f2
  else
    echo "Erorr: Cannot determine OS version!"
    exit 1
  fi
}

if [[ $(os_version) -gt 8 ]]; then
  libuv=libuv1-dev
else
  libuv=libuv-dev
fi

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
  ${libuv}
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
