#!/usr/bin/env bash
# Package tree used for installing netdata on distribution:
# << CentOS: [7] [8] [9] >>

set -e

declare -a package_tree=(
  bison
  cmake
  curl
  elfutils-libelf-devel
  findutils
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

os_version() {
  if [[ -f /etc/os-release ]]; then
    # shellcheck disable=SC2002
    cat /etc/os-release | grep VERSION_ID | cut -d'=' -f2 | cut -d'"' -f2
  else
    echo "Error: Cannot determine OS version!"
    exit 1
  fi
}

prompt() {
  if [[ "${NON_INTERACTIVE}" == "1" ]]; then
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

  if [[ "${DONT_WAIT}" == "0" ]] && [[ "${NON_INTERACTIVE}" == "0" ]]; then
    read -r -p "Press ENTER to run it > " || exit 1
  fi
}

validate_tree_centos() {
  local opts=
  package_manager=
  if [[ "${NON_INTERACTIVE}" == "1" ]]; then
    echo >&2 "Running in non-interactive mode"
    opts="-y"
  fi

  echo >&2 " > CentOS Version: $(os_version) ..."

  if [[ $(os_version) =~ ^9(\..*)?$ ]]; then
    package_manager=dnf
    echo >&2 " > Checking for config-manager ..."
    if ! dnf config-manager --help &> /dev/null; then
      if prompt "config-manager not found, shall I install it?"; then
        dnf ${opts} install 'dnf-command(config-manager)'
      fi
    fi

    echo >&2 " > Checking for CRB ..."
    if ! dnf repolist | grep CRB; then
      if prompt "CRB not found, shall I install it?"; then
        dnf ${opts} config-manager --set-enabled crb
      fi
    fi
  elif [[ $(os_version) =~ ^8(\..*)?$ ]]; then
    package_manager=dnf
    echo >&2 " > Checking for config-manager ..."
    if ! dnf config-manager --help &> /dev/null; then
      if prompt "config-manager not found, shall I install it?"; then
        dnf ${opts} install 'dnf-command(config-manager)'
      fi
    fi

    echo >&2 " > Checking for PowerTools ..."
    if ! dnf repolist | grep PowerTools; then
      if prompt "PowerTools not found, shall I install it?"; then
        dnf ${opts} config-manager --set-enabled powertools || enable_powertools_repo
      fi
    fi

    echo >&2 " > Updating libarchive ..."
    dnf ${opts} install libarchive

  elif [[ $(os_version) =~ ^7(\..*)?$ ]]; then
    package_manager=yum
    echo >&2 " > Checking for EPEL ..."
    if ! rpm -qa | grep epel-release > /dev/null; then
      if prompt "EPEL not found, shall I install it?"; then
        yum ${opts} install epel-release
      fi
    fi
    yum makecache
  fi
}

enable_powertools_repo() {
  if ! dnf repolist | grep -q powertools; then
    cat > /etc/yum.repos.d/powertools.repo <<-EOF
    [powertools]
    name=CentOS Linux \$releasever - PowerTools
    mirrorlist=http://mirrorlist.centos.org/?release=\$releasever&arch=\$basearch&repo=PowerTools&infra=\$infra
    #baseurl=http://mirror.centos.org/\$contentdir/\$releasever/PowerTools/\$basearch/os/
    gpgcheck=1
    enabled=1
    gpgkey=file:///etc/pki/rpm-gpg/RPM-GPG-KEY-centosofficial
EOF
  else
    echo "Something went wrong!"
    exit 1
  fi
}

# shellcheck disable=SC2068
check_flags ${@}
validate_tree_centos

packages_to_install=

for package in "${package_tree[@]}"; do
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
  if [[ "${NON_INTERACTIVE}" == "1" ]]; then
    echo >&2 "Running in non-interactive mode"
    opts="-y"
  fi
  # shellcheck disable=SC2068
  ${package_manager} install ${opts} ${packages_to_install[@]}
fi
