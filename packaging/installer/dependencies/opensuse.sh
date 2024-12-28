#!/usr/bin/env bash
# Package tree used for installing netdata on distribution:
# << opeSUSE >>
# supported versions: leap/15.3 and tumbleweed
# it may work with SLES as well, although we have not tested with it

set -e

NON_INTERACTIVE=0
DONT_WAIT=0

declare -a package_tree=(
  bison
  cmake
  curl
  flex
  gcc
  gcc-c++
  git
  gzip
  libatomic1
  libelf-devel
  libjson-c-devel
  liblz4-devel
  libmnl-devel
  libopenssl-devel
  libuuid-devel
  libuv-devel
  libyaml-devel
  make
  pkg-config
  python3
  systemd-devel
  tar
  zlib-devel
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
  if zypper search -i "$package" &> /dev/null; then
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
  opts="--ignore-unknown"
  if [ "${NON_INTERACTIVE}" -eq 1 ]; then
    echo >&2 "Running in non-interactive mode"
    opts="--non-interactive"
  fi
  # shellcheck disable=SC2068
  zypper ${opts} install ${packages_to_install[@]}
fi
