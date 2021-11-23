#!/usr/bin/env bash
# Package tree used for installing netdata on distribution:
# << CentOS >>
# supported versions: 7,8

set -e

function os_version {
  if [[ -f /etc/os-release ]]; then
    cat /etc/os-release | grep VERSION_ID | cut -d'=' -f2 | cut -d'"' -f2
  else
    echo "Erorr: Cannot determine OS version!"
    exit 1
  fi
}

declare -a package_tree=(
  gcc
  gcc-c++
  make
  autoconf
  autoconf-archive
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
  python3
  elfutils-libelf-devel
  git
  tar
  curl
  gzip
)

if [[ $(os_version) -eq 8 ]]; then
  package_manager=dnf
else
  package_manager=yum
  package_tree+=('Judy-devel' 'file')
fi

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
  ${package_manager} install -y ${packages_to_install[@]} || true
fi

