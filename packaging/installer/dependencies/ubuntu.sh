#!/usr/bin/env bash
# Package tree used for installing netdata on distribution:
# << Ubuntu: [18.04] [20.04] [20.10] [21.04] [21.10] >> | << Linux Mint >>

set -e

NON_INTERACTIVE=0
DONT_WAIT=0

package_tree="
  git
  gcc
  g++
  make
  automake
  cmake
  autoconf
  autoconf-archive
  autogen
  libatomic1
  libtool
  pkg-config
  tar
  curl
  gzip
  zlib1g-dev
  uuid-dev
  libmnl-dev
  libjson-c-dev
  libyaml-dev
  libuv1-dev
  liblz4-dev
  libssl-dev
  libelf-dev
  python3
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

# shellcheck disable=SC2068
check_flags ${@}

packages_to_install=

for package in $package_tree; do
  if dpkg -s "$package" &> /dev/null; then
    echo "Package '${package}' is installed"
  else
    echo "Package '${package}' is NOT installed"
    packages_to_install="$packages_to_install $package"
  fi
done

if [[ -z "$packages_to_install" ]]; then
  echo "All required packages are already installed. Skipping .."
else
  echo "packages_to_install:" "$packages_to_install"
  opts=
  if [ "${NON_INTERACTIVE}" -eq 1 ]; then
    echo >&2 "Running in non-interactive mode"
    export DEBIAN_FRONTEND="noninteractive"
    opts="${opts} -yq"
  fi
  echo "Running apt-get update and updating your APT caches ..."
  apt-get update
  # shellcheck disable=SC2086
  apt-get install ${opts} $packages_to_install
fi
