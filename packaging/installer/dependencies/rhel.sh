#!/usr/bin/env bash
# Package tree used for installing netdata on distribution:
# << Fedora >>
# supported versions: 22->35

declare -a package_tree_list1=(
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
)

declare -a package_tree_list=(
 "findutils"
 "autoconf"
 "autoconf-archive"
 "autogen"
 "automake"
 "libtool"
 "cmake"
 "json-c-devel"
 "bridge-utils"
 "chorny"
 "curl"
 "gzip"
 "tar"
 "git"
 "gcc"
 "gcc-c++"
 "gdb"
 "iotop"
 "iproute"
 "ipset"
 "jq"
 "iptables"
 "zlib-devel"
 "libuuid-devel"
 "libmnl-devel"
 "lm_sensors"
 "logwatch"
 "lxc"
 "mailutils"
 "make"
 "nmap-ncat"
 "nginx"
 "nodejs"
 "pkgconfig"
 "python"
 "MySQL-python"
 "python-psycopg2"
 "python-pip"
 "python-pymongo"
 "python3-pymongo"
 "python-requests"
 "lz4-devel"
 "libuv-devel"
 "openssl-devel"
 "Judy-devel"
 "python3"
 "screen"
 "sudo"
 "sysstat"
 "tcpdump"
 "traceroute"
 "valgrind"
 "ulogd2"
 "unzip"
 "zip"
 "elfutils-libelf-devel"
)

dnf -y install ${package_tree_list1[@]}

#for x in ${package_tree_list[@]};
#do
#  dnf install $x -y
#done
