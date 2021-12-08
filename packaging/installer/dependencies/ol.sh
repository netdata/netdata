#!/usr/bin/env bash
# Package tree used for installing netdata on distribution:
# << Oracle Linux >>
# supported versions: 8

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

function enable_repo {
  if ! dnf repolist | grep -q codeready; then
    cat > /etc/yum.repos.d/ol8_codeready.repo <<-EOF
    [ol8_codeready_builder]
    name=Oracle Linux \$releasever CodeReady Builder (\$basearch)
    baseurl=http://yum.oracle.com/repo/OracleLinux/OL8/codeready/builder/\$basearch
    gpgkey=file:///etc/pki/rpm-gpg/RPM-GPG-KEY-oracle
    gpgcheck=1
    enabled=1
EOF
  else 
    echo "Something went wrong!"
    exit 1 
  fi
}

if [[ $(os_version) =~ 8* ]]; then
  package_manager=dnf
  echo " > Checking for CodeReady Builder ..."
  dnf config-manager --set-enabled ol8_codeready_builder || enable_repo
else
  package_manager=yum
fi

dnf makecache --refresh

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

