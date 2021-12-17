#!/usr/bin/env bash
# Package tree used for installing netdata on distribution:
# << CentOS >>
# supported versions: 7,8

source ./functions.sh

set -e

NON_INTERACTIVE=0
DONT_WAIT=0

check_flags ${@}

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

validate_tree_centos() {
  local opts=
  if [ "${NON_INTERACTIVE}" -eq 1 ]; then
    echo >&2 "Running in non-interactive mode"
    opts="-y"
  fi

  echo >&2 " > CentOS Version: $(os_version) ..."

  if [[ $(os_version) =~ ^8(\..*)?$ ]]; then
    package_manager=dnf
    echo >&2 " > Checking for config-manager ..."
    if ! dnf config-manager; then
      if prompt "config-manager not found, shall I install it?"; then
        dnf ${opts} install 'dnf-command(config-manager)'
      fi
    fi

    echo >&2 " > Checking for PowerTools ..."
    if ! dnf repolist | grep PowerTools; then
      if prompt "PowerTools not found, shall I install it?"; then
        dnf ${opts} config-manager --set-enabled powertools || enable_powertools_repo
      fi
    fi

    echo >&2 " > Updating libarchive ..."
    dnf ${opts} install libarchive

    echo >&2 " > Installing Judy-devel directly ..."
    dnf ${opts} install http://mirror.centos.org/centos/8/PowerTools/x86_64/os/Packages/Judy-devel-1.0.5-18.module_el8.3.0+757+d382997d.x86_64.rpm
    dnf makecache --refresh

  elif [[ $(os_version) =~ ^7(\..*)?$ ]]; then
    package_manager=yum
    echo >&2 " > Checking for EPEL ..."
    if ! rpm -qa | grep epel-release > /dev/null; then
      if prompt "EPEL not found, shall I install it?"; then
        yum ${opts} install epel-release
      fi
    fi
    yum makecache
  fi
}

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

validate_tree_centos

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
  opts=
  if [ "${NON_INTERACTIVE}" -eq 1 ]; then
    echo >&2 "Running in non-interactive mode"
    opts="-y"
  fi
  ${package_manager} install ${opts} ${packages_to_install[@]}
fi

