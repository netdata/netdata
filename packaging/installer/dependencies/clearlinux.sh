#!/usr/bin/env bash
# Package tree used for installing netdata on distribution:
# << ClearLinux >>
#Support versions: base

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
  swupd bundle-add -y ${packages_to_install[@]}
fi

