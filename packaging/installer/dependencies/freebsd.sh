#!/usr/bin/env bash
# Package tree used for installing netdata on distribution:
# << FreeBSD  >>

set -e

NON_INTERACTIVE=0
DONT_WAIT=0

package_tree="
  bison
  cmake
  curl
  e2fsprogs-libuuid
  flex
  git
  gzip
  json-c
  liblz4
  libuv
  libyaml
  lzlib
  openssl
  pkgconf
  python3
  "

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

validate_tree_freebsd() {
  opts=
  if [ "${NON_INTERACTIVE}" -eq 1 ]; then
    echo >&2 "Running in non-interactive mode"
    opts="-y"
  fi

  echo >&2 " > Checking for gmake ..."
  if ! pkg query %n-%v | grep -q gmake; then
    if prompt "gmake is required to build on FreeBSD and is not installed. Shall I install it?"; then
      pkg install ${opts} gmake
    fi
  fi
}

enable_repo () {
  if ! dnf repolist | grep -q codeready; then
cat >> /etc/yum.repos.d/oracle-linux-ol8.repo <<-EOF

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

# shellcheck disable=SC2068
check_flags ${@}
validate_tree_freebsd

packages_to_install=

for package in $package_tree; do
  if pkg info -Ix "$package" &> /dev/null; then
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
    opts="-y"
  fi
  # shellcheck disable=SC2086
  pkg install ${opts} $packages_to_install
fi
