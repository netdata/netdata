#!/usr/bin/env bash
# Package tree used for installing netdata on distribution:
# << ClearLinux: [base] >>

set -e

NON_INTERACTIVE=0
DONT_WAIT=0

declare -a package_tree=(
  bison
  c-basic
  curl
  devpkg-elfutils
  devpkg-json-c
  devpkg-libmnl
  devpkg-libuv
  devpkg-lz4
  devpkg-openssl
  devpkg-util-linux
  devpkg-zlib
  findutils
  flex
  git
  gzip
  make
  python3-basic
  service-os-dev
  sysadmin-basic
  yaml-dev
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
  if [[ "$(swupd bundle-info "$package" | grep Status | cut -d':' -f2)" == " Not installed" ]]; then
    echo "Package '$package' is NOT installed"
    packages_to_install="$packages_to_install $package"
  else
    echo "Package '$package' is installed"
  fi
done

if [[ -z $packages_to_install ]]; then
  echo "All required packages are already installed. Skipping .."
else
  echo "packages_to_install: " "${packages_to_install[@]}"
  # shellcheck disable=SC2068
  swupd bundle-add ${packages_to_install[@]}
fi
