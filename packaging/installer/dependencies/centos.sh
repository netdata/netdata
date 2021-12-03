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

function enable_powertools_repo {
  if ! dnf repolist | grep -q powertools; then
    cat > /etc/yum.repos.d/powertools.repo <<-EOF
    [powertools]
    name=CentOS Linux $releasever - PowerTools
    mirrorlist=http://mirrorlist.centos.org/?release=$releasever&arch=$basearch&repo=PowerTools&infra=$infra
    #baseurl=http://mirror.centos.org/$contentdir/$releasever/PowerTools/$basearch/os/
    gpgcheck=1
    enabled=1
    gpgkey=file:///etc/pki/rpm-gpg/RPM-GPG-KEY-centosofficial
EOF
  else
    echo "Something went wrong!"
    exit 1
  fi
}

function enable_epel_release {
  if ! rpm -qa | grep epel-release > /dev/null; then
    echo "EPEL not found, installing it.."
    yum install -y epel-release
  else
    echo "EPEL is installed"
  fi
}

function check_plugins_core {
  if rpm -q dnf-plugins-core; then
    echo "Package dnf-plugins-core is INSTALLED"
  else
    echo "Installing 'dnf-command(config-manager)' ..."
    dnf install -y 'dnf-command(config-manager)'
  fi
}

if [[ $(os_version) -eq 8 ]]; then
  package_manager=dnf
  check_plugins_core
  dnf config-manager --set-enabled powertools || enable_repo
  dnf makecache --refresh
else
 package_manager=yum
 enable_epel_release
 yum makecache

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
  ${package_manager} install -y ${packages_to_install[@]}
fi

