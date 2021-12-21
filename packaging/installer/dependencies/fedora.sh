#!/usr/bin/env bash
# Package tree used for installing netdata on distribution:
# << Fedora >>
# supported versions: 24->35

source ./functions.sh

set -e

NON_INTERACTIVE=0
export DONT_WAIT=0

check_flags "${@}"

function os_version {
  if [[ -f /etc/os-release ]]; then
    cat /etc/os-release | grep VERSION_ID | cut -d'=' -f2
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
  findutils
  gcc
  gcc-c++
  make
  autoconf
  autoconf-archive
  autogen
  automake
  libtool
  cmake
  nmap-ncat
  zlib-devel
  libuuid-devel
  libmnl-devel
  json-c-devel
  libuv-devel
  lz4-devel
  openssl-devel
  Judy-devel
  elfutils-libelf-devel
  git
  pkgconfig
  tar
  curl
  gzip
  python3
  "${ulogd_pkg}"
)

packages_to_install=

for package in "${package_tree[@]}"; do
  if rpm -q "$package" &> /dev/null; then
    echo "Package '${package}' is installed"
  else
    echo "Package '$package' is NOT installed"
    packages_to_install="$packages_to_install $package"
  fi
done

if [[ -z $packages_to_install ]]; then
  echo "All required packages are already installed. Skipping .."
else
  echo "packages_to_install:" "${packages_to_install[@]}"
  opts=
  if [ "${NON_INTERACTIVE}" -eq 1 ]; then
    echo >&2 "Running in non-interactive mode"
    opts="-y"
  fi
  dnf install "${opts}" "${packages_to_install[@]}"
fi
