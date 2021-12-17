#!/usr/bin/env bash
# Package tree used for installing netdata on distribution:
# << Oracle Linux >>
# supported versions: 8

source ./functions.sh

set -e

NON_INTERACTIVE=0
DONT_WAIT=0

check_flags ${@}

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
  tar
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
  curl
  gzip
)

function os_version {
  if [[ -f /etc/os-release ]]; then
    cat /etc/os-release | grep VERSION_ID | cut -d'=' -f2 | cut -d'"' -f2
  else
    echo "Erorr: Cannot determine OS version!"
    exit 1
  fi
}

function enable_repo {
  if ! dnf repolist | grep -q codeready; then
    if prompt "CodeReady Builder not found, shall I install it?"; then	  
      cat > /etc/yum.repos.d/ol8_codeready.repo <<-EOF
      [ol8_codeready_builder]
      name=Oracle Linux \$releasever CodeReady Builder (\$basearch)
      baseurl=http://yum.oracle.com/repo/OracleLinux/OL8/codeready/builder/\$basearch
      gpgkey=file:///etc/pki/rpm-gpg/RPM-GPG-KEY-oracle
      gpgcheck=1
      enabled=1
EOF
    fi
  else 
    echo "Something went wrong!"
    exit 1 
  fi
}


validate_tree_ol() {

  opts=
  if [ "${NON_INTERACTIVE}" -eq 1 ]; then
    echo >&2 "Running in non-interactive mode"
    opts="-y"
  fi


  echo >&2 " > Checking for config-manager ..."
  if ! dnf config-manager; then
    if prompt "config-manager not found, shall I install it?"; then
      dnf ${opts} install 'dnf-command(config-manager)'
    fi
  fi

  echo " > Checking for CodeReady Builder ..."
  if ! dnf repolist | grep ol8_codeready_builder; then
    if prompt "CodeReadyBuilder not found, shall I install it?"; then
      dnf ${opts} config-manager --set-enabled ol8_codeready_builder || enable_repo
    fi
  fi

  dnf makecache --refresh
}

validate_tree_ol

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
  opts=
  if [ "${NON_INTERACTIVE}" -eq 1 ]; then
    echo >&2 "Running in non-interactive mode"
    opts="-y"
  fi
  echo "packages_to_install: ${packages_to_install[@]}"
  dnf install ${opts} ${packages_to_install[@]}
fi

