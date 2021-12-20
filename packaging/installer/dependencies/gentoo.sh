#!/usr/bin/env bash
# Package tree used for installing netdata on distribution:
# << Gentoo >>

source ./functions.sh

set -e

NON_INTERACTIVE=0
DONT_WAIT=0

check_flags ${@}

package_tree="
  dev-vcs/git
  sys-apps/findutils
  sys-devel/gcc
  sys-devel/make
  sys-devel/autoconf
  sys-devel/autoconf-archive
  sys-devel/autogen
  sys-devel/automake
  virtual/pkgconfig
  dev-util/cmake
  app-arch/tar
  net-misc/curl
  app-arch/gzip
  net-analyzer/netcat
  sys-apps/util-linux
  net-libs/libmnl
  dev-libs/json-c
  dev-libs/libuv
  app-arch/lz4
  dev-libs/openssl
  dev-libs/judy
  virtual/libelf
  dev-lang/python
  dev-libs/libuv
  "

packages_to_install=

for package in $package_tree; do
  if qlist -IRv $package &> /dev/null; then
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
  opts="--ask"
  if [ "${NON_INTERACTIVE}" -eq 1 ]; then
    echo >&2 "Running in non-interactive mode"
    opts=""
  fi
  emerge ${opts} $packages_to_install
fi

