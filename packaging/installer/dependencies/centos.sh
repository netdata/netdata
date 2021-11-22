#!/usr/bin/env bash
# Package tree used for installing netdata on distribution:
# << CentOS >>
# supported versions: 6->8

set -e

function os_version {
  if [[ -f /etc/os-release ]]; then
    cat /etc/os-release | grep VERSION_ID | cut -d'=' -f2 | cut -d'"' -f2
  else
    echo "Erorr: Cannot determine OS version!"
    exit 1
  fi
}

if [[ $(os_version) -gt 24 ]]; then
  ulogd_pkg=
else
  ulogd_pkg=ulogd
fi

declare -a package_tree=(
  gcc
  gcc-c++
  make
  autoconf
  autoconf_archive
  autogen
  automake
  libtool
  pkgconfig
  cmake
  nmap-ncat
  zlib-devel
  libuuid-devel
  libmnl-devel
  json-c-devel
  libuv-devel
  lz4-devel
  openssl-devel
  elfutils-libelf-devel
  python3
  git
  tar
  curl
  gzip
)

packages_to_install=

for package in ${package_tree[@]}; do
  if rpm -q $package &> /dev/null; then
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
  dnf -y install ${packages_to_install[@]}
fi
