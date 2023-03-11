#!/usr/bin/env bash
# Package tree used for installing netdata on distribution:
# << Fedora: [24->35] >>

set -e

NON_INTERACTIVE=0
DONT_WAIT=0

os_version() {
  if [[ -f /etc/os-release ]]; then
    # shellcheck disable=SC2002
    cat /etc/os-release | grep VERSION_ID | cut -d'=' -f2
  else
    echo "Error: Cannot determine OS version!"
    exit 1
  fi
}

if [[ $(os_version) -gt 24 ]]; then
  ulogd_pkg=
else
  ulogd_pkg=ulogd
fi

declare -a package_tree=(
  findutils
  gcc
  gcc-c++
  make
  autoconf
  autoconf-archive
  autogen
  automake
  libatomic
  libtool
  cmake
  zlib-devel
  libuuid-devel
  libmnl-devel
  json-c-devel
  libyaml-devel
  libuv-devel
  lz4-devel
  openssl-devel
  elfutils-libelf-devel
  git
  pkgconfig
  tar
  curl
  gzip
  python3
  "${ulogd_pkg}"
)

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

# shellcheck disable=SC2068
for package in ${package_tree[@]}; do
  if rpm -q "$package" &> /dev/null; then
    echo "Package '${package}' is installed"
  else
    echo "Package '$package' is NOT installed"
    packages_to_install="$packages_to_install $package"
  fi
done

if [[ -z $packages_to_install ]]; then
  echo "All required packages are already installed. Skipping .."
else
  echo "packages_to_install:" "${packages_to_install[@]}"
  opts=
  if [ "${NON_INTERACTIVE}" -eq 1 ]; then
    echo >&2 "Running in non-interactive mode"
    opts="-y"
  fi
  # shellcheck disable=SC2068
  dnf install ${opts} ${packages_to_install[@]}
fi
