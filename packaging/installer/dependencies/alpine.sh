#!/bin/sh
# Package tree used for installing netdata on distribution:
# << Alpine: [3.12] [3.13] [3.14] [3.15] [edge] >>

set -e

NON_INTERACTIVE=0
DONT_WAIT=0

package_tree="
  alpine-sdk
  bison
  cmake
  coreutils
  curl
  elfutils-dev
  flex
  g++
  gcc
  git
  gzip
  json-c-dev
  libatomic
  libmnl-dev
  libuv-dev
  lz4-dev
  make
  openssl-dev
  pkgconfig
  python3
  tar
  util-linux-dev
  yaml-dev
  zlib-dev
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
    printf "Press ENTER to run it > " 
    read -r || exit 1
  fi
}

# shellcheck disable=2068
check_flags ${@}

packages_to_install=

handle_old_alpine() {
  version="$(grep VERSION_ID /etc/os-release | cut -f 2 -d '=')"
  major="$(echo "${version}" | cut -f 1 -d '.')"
  minor="$(echo "${version}" | cut -f 2 -d '.')"

  if [ "${major}" -le 3 ] && [ "${minor}" -le 16 ]; then
    package_tree="$(echo "${package_tree}" | sed 's/musl-fts-dev/fts-dev/')"
  fi
}

for package in $package_tree; do
  if apk -e info "$package" > /dev/null 2>&1 ; then
    echo "Package '${package}' is installed"
  else
    echo "Package '${package}' is NOT installed"
    packages_to_install="$packages_to_install $package"
  fi
done

if [ -z "${packages_to_install}" ]; then
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
