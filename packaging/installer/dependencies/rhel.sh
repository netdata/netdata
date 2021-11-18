#!/usr/bin/env bash
# Package tree used for installing netdata on distribution:
# << Fedora >> versions: 22->35
# << CentOS >> versions: 6/7/8
# << RHEL >>   versions: 6/7/8

os_name=$1

if [[ -z $os_name ]]; then
  echo "ERROR: os name must be passed as parameter to $0!"
  exit 1
fi

function os_version {
  if [[ -f /etc/os-release ]]; then
    cat /etc/os-release | grep VERSION_ID | cut -d'=' -f2
  fi
}

if [[ $os_name == "fedora" ]] && [[ $(os_version) -gt 24 ]]; then
  ulogd_pkg=
else
  ulogd_pkg=ulogd
fi

source default.sh

declare -a package_tree_rhel=(
  autoconf-archive
  zlib-devel
  libuuid-devel
  libmnl-devel
  json-c-devel
  libuv
  lz4-devel
  openssl-devel
  Judy-devel
  elfutils-libelf-devel
  mailx
  nmap-ncat
  pkgconfig
  ${ulogd_pkg}
)

dnf -y install ${package_tree_default[@]}
dnf -y install ${package_tree_rhel[@]}
