#!/usr/bin/env bash
# Package tree used for installing netdata on distribution:
# << FreeBSD  >>

set -e

NON_INTERACTIVE=0
DONT_WAIT=0

usage() {
  cat << EOF
OPTIONS:
[--dont-wait] [--non-interactive] [ ]
EOF
}

if [ ! -z "${1}" ]; then 	
  case "${1}" in
      dont-wait | --dont-wait | -n)
        DONT_WAIT=1
        ;;

      non-interactive | --non-interactive | -y)
        NON_INTERACTIVE=1
        ;;

      help | -h | --help)
        usage
        exit 1
        ;;
      
      *)
        echo >&2 "ERROR: Cannot understand option '${1}'"
        echo >&2
        usage
        exit 1
        ;;
  esac
else
  echo "Run script without flags"
fi

packages_to_install=
package_tree="
  git
  gcc
  autoconf
  autoconf-archive
  autogen
  automake
  libtool
  pkgconf
  cmake
  curl
  gzip
  netcat
  lzlib
  e2fsprogs-libuuid
  json-c
  libuv
  liblz4
  openssl
  Judy
  python3
  "

function os_version {
  if [[ -f /etc/os-release ]]; then
    cat /etc/os-release | grep VERSION_ID | cut -d'=' -f2
  else
    echo "Erorr: Cannot determine OS version!"
    exit 1
  fi
}

prompt() {
  if [ "${NON_INTERACTIVE}" -eq 1 ]; then
    echo >&2 "Running in non-interactive mode, assuming yes (y)"
    echo >&2 " > Would have prompted for ${1} ..."
    return 0
  fi

  while true; do
    read -r -p "${1} [y/n] " yn
    case $yn in
      [Yy]*) return 0 ;;
      [Nn]*) return 1 ;;
      *) echo >&2 "Please answer with yes (y) or no (n)." ;;
    esac
  done
}

version=$(os_version)

validate_tree_freebsd() {
  opts=
  if [ "${NON_INTERACTIVE}" -eq 1 ]; then
    echo >&2 "Running in non-interactive mode"
    opts="-y"
  fi

  echo >&2 " > FreeBSD Version: ${version} ..."

  echo >&2 " > Checking for gmake ..."
  if ! pkg query %n-%v | grep -q gmake; then
    if prompt "gmake is required to build on FreeBSD and is not installed. Shall I install it?"; then
      pkg install ${opts} gmake
    fi
  fi
}

validate_tree_freebsd

for package in $package_tree; do
  if pkg info -Ix $package &> /dev/null; then
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
  opts=
  if [ "${NON_INTERACTIVE}" -eq 1 ]; then
    echo >&2 "Running in non-interactive mode"
    opts="-y"
  fi
  pkg install ${opts} $packages_to_install
fi

