#!/usr/bin/env bash
# Package tree used for installing netdata on distribution:
# << Oracle Linux: [8, 9] >>

set -e

NON_INTERACTIVE=0
DONT_WAIT=0

declare -a package_tree=(
  bison
  cmake
  curl
  elfutils-libelf-devel
  flex
  gcc
  gcc-c++
  git
  gzip
  json-c-devel
  libatomic
  libmnl-devel
  libuuid-devel
  libuv-devel
  libyaml-devel
  lz4-devel
  make
  openssl-devel
  pkgconfig
  python3
  systemd-devel
  tar
  zlib-devel
)

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

validate_tree_ol() {
  opts=
  if [ "${NON_INTERACTIVE}" -eq 1 ]; then
    echo >&2 "Running in non-interactive mode"
    opts="-y"
  fi

  # shellcheck disable=SC1091
  source /etc/os-release

  # shellcheck disable=SC2153
  version="$(echo "${VERSION}" | cut -f 1 -d '.')"

  echo >&2 " > Checking for config-manager ..."
  if ! dnf config-manager &> /dev/null; then
    if prompt "config-manager not found, shall I install it?"; then
      dnf ${opts} install 'dnf-command(config-manager)'
    fi
  fi

  echo " > Checking for CodeReady Builder ..."
  if [[ "${version}" =~ ^8(\..*)?$ ]]; then
    if ! dnf repolist enabled | grep ol8_codeready_builder; then
      if prompt "CodeReadyBuilder not found, shall I install it?"; then
        dnf ${opts} config-manager --set-enabled ol8_codeready_builder || enable_repo
      fi
    fi
  elif [[ "${version}" =~ ^9(\..*)?$ ]]; then
    if ! dnf repolist enabled | grep ol9_codeready_builder; then
      if prompt "CodeReadyBuilder not found, shall I install it?"; then
        dnf ${opts} config-manager --set-enabled ol9_codeready_builder || enable_repo
      fi
    fi
  fi

  dnf makecache --refresh
}

# shellcheck disable=SC2068
check_flags ${@}
validate_tree_ol

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
  opts=
  if [ "${NON_INTERACTIVE}" -eq 1 ]; then
    echo >&2 "Running in non-interactive mode"
    opts="-y"
  fi
  echo "packages_to_install:" "${packages_to_install[@]}"
  # shellcheck disable=SC2068
  dnf install ${opts} ${packages_to_install[@]}
fi
