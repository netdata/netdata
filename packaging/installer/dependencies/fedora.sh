#!/usr/bin/env bash
# Package tree used for installing netdata on distribution:
# << Fedora >>
# supported versions: 22->35

set -e

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
  ${ulogd_pkg}
)


for val in ${package_tree[@]}; do
  if rpm -q $val; then
    echo "Package ${val} is installed"
  else
    uninstalled_list+=${val}
    uninstalled_list+=" " 
  fi
done

if [[ ${#uninstalled_list[@]} -eq 0 ]]; then
  echo "Everything is up to date"
  exit 2
fi

dnf -y install ${uninstalled_list}
