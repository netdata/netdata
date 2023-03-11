#!/usr/bin/env bash
# Package tree used for installing netdata on distribution:
# << Alpine: [3.12] [3.13] [3.14] [3.15] [edge] >>

set -e

NON_INTERACTIVE=0
DONT_WAIT=0

package_tree="
  alpine-sdk
  git
  gcc
  g++
  automake
  autoconf
  cmake
  make
  libatomic
  libtool
  pkgconfig
  tar
  curl
  gzip
  libuv-dev
  lz4-dev
  openssl-dev
  elfutils-dev
  python3
  zlib-dev
  util-linux-dev
  libmnl-dev
  json-c-dev
  yaml-dev
  "

usage() {
  cat << EOF
OPTIONS:
[--dont-wait] [--non-interactive] [ ]
EOF
}

check_flags() {
  while [ -n "${1}" ]; do
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
    shift
  done

  if [ "${DONT_WAIT}" -eq 0 ] && [ "${NON_INTERACTIVE}" -eq 0 ]; then
    read -r -p "Press ENTER to run it > " || exit 1
  fi
}

# shellcheck disable=2068
check_flags ${@}

packages_to_install=

for package in $package_tree; do
  if apk -e info "$package" &> /dev/null; then
    echo "Package '${package}' is installed"
  else
    echo "Package '${package}' is NOT installed"
    packages_to_install="$packages_to_install $package"
  fi
done

if [[ -z $packages_to_install ]]; then
  echo "All required packages are already installed. Skipping .."
else
  echo "packages_to_install:" "$packages_to_install"
  opts="--force-broken-world"
  if [ "${NON_INTERACTIVE}" -eq 1 ]; then
    echo >&2 "Running in non-interactive mode"
  else
    opts="${opts} -i"
  fi
  # shellcheck disable=SC2086
  apk add ${opts} $packages_to_install
fi
