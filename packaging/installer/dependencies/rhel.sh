#!/usr/bin/env bash
# Package tree used for installing netdata on distribution:
# << RHEL >>
# supported versions: 6/7/8

set -e

function os_version {
  if [[ -f /etc/redhat-release ]]; then
    cat /etc/redhat-release | grep VERSION_ID | cut -d'=' -f2
  else
    echo "Erorr: Cannot determine OS version!"
    exit 1
  fi
}

declare -a package_tree=(
  autoconf
  autoconf-archive
  autogen
  automake
  libtool
  cmake
  json-c-devel
  bridge-utils
  chrony
  curl
  gzip
  tar
  git
  gcc
  gcc-c++
  gdb
  iotop
  iproute
  ipset
  jq
  iptables
  lm_sensors
  logwatch
  lxc
  make
  nginx
  nodejs
  postfix
  python
  python-mysql
  python-psycopg2
  python-pip
  python3-pip
  python-pymongo
  python3-pymongo
  python-requests
  lz4-devel
  libuv-devel
  openssl-devel
  python3
  screen
  sudo
  sysstat
  tcpdump
  traceroute
  valgrind
  nzip
  zip
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

dnf -y install ${package_tree[@]}
