#!/usr/bin/env bash
# shellcheck disable=SC2034
# We use lots of computed variable names in here, so we need to disable shellcheck 2034

export PATH="${PATH}:/bin:/sbin:/usr/bin:/usr/sbin:/usr/local/bin:/usr/local/sbin"
export LC_ALL=C

# Be nice on production environments
renice 19 $$ > /dev/null 2> /dev/null

ME="${0}"

if [ "${BASH_VERSINFO[0]}" -lt "4" ]; then
  echo >&2 "Sorry! This script needs BASH version 4+, but you have BASH version ${BASH_VERSION}"
  exit 1
fi

# These options control which packages we are going to install
# They can be pre-set, but also can be controlled with command line options
PACKAGES_NETDATA=${PACKAGES_NETDATA-1}
PACKAGES_NETDATA_NODEJS=${PACKAGES_NETDATA_NODEJS-0}
PACKAGES_NETDATA_PYTHON=${PACKAGES_NETDATA_PYTHON-0}
PACKAGES_NETDATA_PYTHON3=${PACKAGES_NETDATA_PYTHON3-1}
PACKAGES_NETDATA_PYTHON_MYSQL=${PACKAGES_NETDATA_PYTHON_MYSQL-0}
PACKAGES_NETDATA_PYTHON_POSTGRES=${PACKAGES_NETDATA_PYTHON_POSTGRES-0}
PACKAGES_NETDATA_PYTHON_MONGO=${PACKAGES_NETDATA_PYTHON_MONGO-0}
PACKAGES_DEBUG=${PACKAGES_DEBUG-0}
PACKAGES_IPRANGE=${PACKAGES_IPRANGE-0}
PACKAGES_FIREHOL=${PACKAGES_FIREHOL-0}
PACKAGES_FIREQOS=${PACKAGES_FIREQOS-0}
PACKAGES_UPDATE_IPSETS=${PACKAGES_UPDATE_IPSETS-0}
PACKAGES_NETDATA_DEMO_SITE=${PACKAGES_NETDATA_DEMO_SITE-0}
PACKAGES_NETDATA_SENSORS=${PACKAGES_NETDATA_SENSORS-0}
PACKAGES_NETDATA_DATABASE=${PACKAGES_NETDATA_DATABASE-1}
PACKAGES_NETDATA_EBPF=${PACKAGES_NETDATA_EBPF-1}

# needed commands
lsb_release=$(command -v lsb_release 2> /dev/null)

# Check which package managers are available
apk=$(command -v apk 2> /dev/null)
apt_get=$(command -v apt-get 2> /dev/null)
brew=$(command -v brew 2> /dev/null)
pkg=$(command -v pkg 2> /dev/null)
dnf=$(command -v dnf 2> /dev/null)
emerge=$(command -v emerge 2> /dev/null)
equo=$(command -v equo 2> /dev/null)
pacman=$(command -v pacman 2> /dev/null)
swupd=$(command -v swupd 2> /dev/null)
yum=$(command -v yum 2> /dev/null)
zypper=$(command -v zypper 2> /dev/null)

distribution=
release=
version=
codename=
package_installer=
tree=
detection=
NAME=
ID=
ID_LIKE=
VERSION=
VERSION_ID=

usage() {
  cat << EOF
OPTIONS:
${ME} [--dont-wait] [--non-interactive] \\
  [distribution DD [version VV] [codename CN]] [installer IN] [packages]
Supported distributions (DD):
    - arch           (all Arch Linux derivatives)
    - centos         (all CentOS derivatives)
    - gentoo         (all Gentoo Linux derivatives)
    - sabayon        (all Sabayon Linux derivatives)
    - debian, ubuntu (all Debian and Ubuntu derivatives)
    - redhat, fedora (all Red Hat and Fedora derivatives)
    - suse, opensuse (all SUSE and openSUSE derivatives)
    - clearlinux     (all Clear Linux derivatives)
    - macos          (Apple's macOS)
Supported installers (IN):
    - apt-get        all Debian / Ubuntu Linux derivatives
    - dnf            newer Red Hat / Fedora Linux
    - emerge         all Gentoo Linux derivatives
    - equo           all Sabayon Linux derivatives
    - pacman         all Arch Linux derivatives
    - yum            all Red Hat / Fedora / CentOS Linux derivatives
    - zypper         all SUSE Linux derivatives
    - apk            all Alpine derivatives
    - swupd          all Clear Linux derivatives
    - brew           macOS Homebrew
    - pkg            FreeBSD Ports
Supported packages (you can append many of them):
    - netdata-all    all packages required to install netdata
                     including mysql client, postgres client,
                     node.js, python, sensors, etc
    - netdata        minimum packages required to install netdata
                     (no mysql client, no nodejs, includes python)
    - nodejs         install nodejs
                     (required for monitoring named and SNMP)
    - python         install python
    - python3        install python3
    - python-mysql   install MySQLdb
                     (for monitoring mysql, will install python3 version
                     if python3 is enabled or detected)
    - python-postgres install psycopg2
                     (for monitoring postgres, will install python3 version
                     if python3 is enabled or detected)
    - python-pymongo install python-pymongo (or python3-pymongo for python3)
    - sensors        install lm_sensors for monitoring h/w sensors
    - firehol-all    packages required for FireHOL, FireQoS, update-ipsets
    - firehol        packages required for FireHOL
    - fireqos        packages required for FireQoS
    - update-ipsets  packages required for update-ipsets
    - demo           packages required for running a netdata demo site
                     (includes nginx and various debugging tools)
If you don't supply the --dont-wait option, the program
will ask you before touching your system.
EOF
}

release2lsb_release() {
  # loads the given /etc/x-release file
  # this file is normally a single line containing something like
  #
  # X Linux release 1.2.3 (release-name)
  #
  # It attempts to parse it
  # If it succeeds, it returns 0
  # otherwise it returns 1

  local file="${1}" x DISTRIB_ID="" DISTRIB_RELEASE="" DISTRIB_CODENAME=""
  echo >&2 "Loading ${file} ..."

  x="$(grep -v "^$" "${file}" | head -n 1)"

  if [[ "${x}" =~ ^.*[[:space:]]+Linux[[:space:]]+release[[:space:]]+.*[[:space:]]+(.*)[[:space:]]*$ ]]; then
    eval "$(echo "${x}" | sed "s|^\(.*\)[[:space:]]\+Linux[[:space:]]\+release[[:space:]]\+\(.*\)[[:space:]]\+(\(.*\))[[:space:]]*$|DISTRIB_ID=\"\1\"\nDISTRIB_RELEASE=\"\2\"\nDISTRIB_CODENAME=\"\3\"|g" | grep "^DISTRIB")"
  elif [[ "${x}" =~ ^.*[[:space:]]+Linux[[:space:]]+release[[:space:]]+.*[[:space:]]+$ ]]; then
    eval "$(echo "${x}" | sed "s|^\(.*\)[[:space:]]\+Linux[[:space:]]\+release[[:space:]]\+\(.*\)[[:space:]]*$|DISTRIB_ID=\"\1\"\nDISTRIB_RELEASE=\"\2\"|g" | grep "^DISTRIB")"
  elif [[ "${x}" =~ ^.*[[:space:]]+release[[:space:]]+.*[[:space:]]+(.*)[[:space:]]*$ ]]; then
    eval "$(echo "${x}" | sed "s|^\(.*\)[[:space:]]\+release[[:space:]]\+\(.*\)[[:space:]]\+(\(.*\))[[:space:]]*$|DISTRIB_ID=\"\1\"\nDISTRIB_RELEASE=\"\2\"\nDISTRIB_CODENAME=\"\3\"|g" | grep "^DISTRIB")"
  elif [[ "${x}" =~ ^.*[[:space:]]+release[[:space:]]+.*[[:space:]]+$ ]]; then
    eval "$(echo "${x}" | sed "s|^\(.*\)[[:space:]]\+release[[:space:]]\+\(.*\)[[:space:]]*$|DISTRIB_ID=\"\1\"\nDISTRIB_RELEASE=\"\2\"|g" | grep "^DISTRIB")"
  fi

  distribution="${DISTRIB_ID}"
  version="${DISTRIB_RELEASE}"
  codename="${DISTRIB_CODENAME}"

  [ -z "${distribution}" ] && echo >&2 "Cannot parse this lsb-release: ${x}" && return 1
  detection="${file}"
  return 0
}

get_os_release() {
  # Loads the /etc/os-release or /usr/lib/os-release file(s)
  # Only the required fields are loaded
  #
  # If it manages to load a valid os-release, it returns 0
  # otherwise it returns 1
  #
  # It searches the ID_LIKE field for a compatible distribution

  os_release_file=
  if [ -s "/etc/os-release" ]; then
    os_release_file="/etc/os-release"
  elif [ -s "/usr/lib/os-release" ]; then
    os_release_file="/usr/lib/os-release"
  else
    echo >&2 "Cannot find an os-release file ..."
    return 1
  fi

  local x
  echo >&2 "Loading ${os_release_file} ..."

  eval "$(grep -E "^(NAME|ID|ID_LIKE|VERSION|VERSION_ID)=" "${os_release_file}")"
  for x in "${ID}" ${ID_LIKE}; do
    case "${x,,}" in
      alpine | arch | centos | clear-linux-os | debian | fedora | gentoo | manjaro | opensuse-leap | ol | rhel | sabayon | sles | suse | ubuntu)
        distribution="${x}"
        version="${VERSION_ID}"
        codename="${VERSION}"
        detection="${os_release_file}"
        break
        ;;
      *)
        echo >&2 "Unknown distribution ID: ${x}"
        ;;
    esac
  done
  [ -z "${distribution}" ] && echo >&2 "Cannot find valid distribution in: ${ID} ${ID_LIKE}" && return 1

  [ -z "${distribution}" ] && return 1
  return 0
}

get_lsb_release() {
  # Loads the /etc/lsb-release file
  # If it fails, it attempts to run the command: lsb_release -a
  # and parse its output
  #
  # If it manages to find the lsb-release, it returns 0
  # otherwise it returns 1

  if [ -f "/etc/lsb-release" ]; then
    echo >&2 "Loading /etc/lsb-release ..."
    local DISTRIB_ID="" DISTRIB_RELEASE="" DISTRIB_CODENAME=""
    eval "$(grep -E "^(DISTRIB_ID|DISTRIB_RELEASE|DISTRIB_CODENAME)=" /etc/lsb-release)"
    distribution="${DISTRIB_ID}"
    version="${DISTRIB_RELEASE}"
    codename="${DISTRIB_CODENAME}"
    detection="/etc/lsb-release"
  fi

  if [ -z "${distribution}" ] && [ -n "${lsb_release}" ]; then
    echo >&2 "Cannot find distribution with /etc/lsb-release"
    echo >&2 "Running command: lsb_release ..."
    eval "declare -A release=( $(lsb_release -a 2> /dev/null | sed -e "s|^\(.*\):[[:space:]]*\(.*\)$|[\1]=\"\2\"|g") )"
    distribution="${release["Distributor ID"]}"
    version="${release[Release]}"
    codename="${release[Codename]}"
    detection="lsb_release"
  fi

  [ -z "${distribution}" ] && echo >&2 "Cannot find valid distribution with lsb-release" && return 1
  return 0
}

find_etc_any_release() {
  # Check for any of the known /etc/x-release files
  # If it finds one, it loads it and returns 0
  # otherwise it returns 1

  if [ -f "/etc/arch-release" ]; then
    release2lsb_release "/etc/arch-release" && return 0
  fi

  if [ -f "/etc/centos-release" ]; then
    release2lsb_release "/etc/centos-release" && return 0
  fi

  if [ -f "/etc/redhat-release" ]; then
    release2lsb_release "/etc/redhat-release" && return 0
  fi

  if [ -f "/etc/SuSe-release" ]; then
    release2lsb_release "/etc/SuSe-release" && return 0
  fi

  return 1
}

autodetect_distribution() {
  # autodetection of distribution/OS
  case "$(uname -s)" in
    "Linux")
      get_os_release || get_lsb_release || find_etc_any_release
      ;;
    "FreeBSD")
      distribution="freebsd"
      version="$(uname -r)"
      detection="uname"
      ;;
    "Darwin")
      distribution="macos"
      version="$(uname -r)"
      detection="uname"

      if [ ${EUID} -eq 0 ]; then
        echo >&2 "This script does not support running as EUID 0 on macOS. Please run it as a regular user."
        exit 1
      fi
      ;;
    *)
      return 1
      ;;
  esac
}

user_picks_distribution() {
  # let the user pick a distribution

  echo >&2
  echo >&2 "I NEED YOUR HELP"
  echo >&2 "It seems I cannot detect your system automatically."

  if [ "${NON_INTERACTIVE}" -eq 1 ]; then
    echo >&2 "Running in non-interactive mode"
    echo >&2 " > Bailing out..."
    exit 1
  fi

  if [ -z "${equo}" ] && [ -z "${emerge}" ] && [ -z "${apt_get}" ] && [ -z "${yum}" ] && [ -z "${dnf}" ] && [ -z "${pacman}" ] && [ -z "${apk}" ] && [ -z "${swupd}" ]; then
    echo >&2 "And it seems I cannot find a known package manager in this system."
    echo >&2 "Please open a github issue to help us support your system too."
    exit 1
  fi

  local opts=
  echo >&2 "I found though that the following installers are available:"
  echo >&2
  [ -n "${apt_get}" ] && echo >&2 " - Debian/Ubuntu based (installer is: apt-get)" && opts="apt-get ${opts}"
  [ -n "${yum}" ] && echo >&2 " - Red Hat/Fedora/CentOS based (installer is: yum)" && opts="yum ${opts}"
  [ -n "${dnf}" ] && echo >&2 " - Red Hat/Fedora/CentOS based (installer is: dnf)" && opts="dnf ${opts}"
  [ -n "${zypper}" ] && echo >&2 " - SuSe based (installer is: zypper)" && opts="zypper ${opts}"
  [ -n "${pacman}" ] && echo >&2 " - Arch Linux based (installer is: pacman)" && opts="pacman ${opts}"
  [ -n "${emerge}" ] && echo >&2 " - Gentoo based (installer is: emerge)" && opts="emerge ${opts}"
  [ -n "${equo}" ] && echo >&2 " - Sabayon based (installer is: equo)" && opts="equo ${opts}"
  [ -n "${apk}" ] && echo >&2 " - Alpine Linux based (installer is: apk)" && opts="apk ${opts}"
  [ -n "${swupd}" ] && echo >&2 " - Clear Linux based (installer is: swupd)" && opts="swupd ${opts}"
  [ -n "${brew}" ] && echo >&2 " - macOS based (installer is: brew)" && opts="brew ${opts}"
  # XXX: This is being removed in another PR.
  echo >&2

  REPLY=
  while [ -z "${REPLY}" ]; do
    echo "To proceed please write one of these:"
    echo "${opts// /, }"
    if ! read -r -p ">" REPLY; then
      continue
    fi

    if [ "${REPLY}" = "yum" ] && [ -z "${distribution}" ]; then
      REPLY=
      while [ -z "${REPLY}" ]; do
        if ! read -r -p "yum in centos, rhel, ol or fedora? > "; then
          continue
        fi

        case "${REPLY,,}" in
          fedora | rhel)
            distribution="rhel"
            ;;
          ol)
            distribution="ol"
            ;;
          centos)
            distribution="centos"
            ;;
          *)
            echo >&2 "Please enter 'centos', 'fedora', 'ol' or 'rhel'."
            REPLY=
            ;;
        esac
      done
      REPLY="yum"
    fi
    check_package_manager "${REPLY}" || REPLY=
  done
}

detect_package_manager_from_distribution() {
  case "${1,,}" in
    arch* | manjaro*)
      package_installer="install_pacman"
      tree="arch"
      if [ "${IGNORE_INSTALLED}" -eq 0 ] && [ -z "${pacman}" ]; then
        echo >&2 "command 'pacman' is required to install packages on a '${distribution} ${version}' system."
        exit 1
      fi
      ;;

    sabayon*)
      package_installer="install_equo"
      tree="sabayon"
      if [ "${IGNORE_INSTALLED}" -eq 0 ] && [ -z "${equo}" ]; then
        echo >&2 "command 'equo' is required to install packages on a '${distribution} ${version}' system."
        # Maybe offer to fall back on emerge? Both installers exist in Sabayon...
        exit 1
      fi
      ;;

    alpine*)
      package_installer="install_apk"
      tree="alpine"
      if [ "${IGNORE_INSTALLED}" -eq 0 ] && [ -z "${apk}" ]; then
        echo >&2 "command 'apk' is required to install packages on a '${distribution} ${version}' system."
        exit 1
      fi
      ;;

    gentoo*)
      package_installer="install_emerge"
      tree="gentoo"
      if [ "${IGNORE_INSTALLED}" -eq 0 ] && [ -z "${emerge}" ]; then
        echo >&2 "command 'emerge' is required to install packages on a '${distribution} ${version}' system."
        exit 1
      fi
      ;;

    debian* | ubuntu*)
      package_installer="install_apt_get"
      tree="debian"
      if [ "${IGNORE_INSTALLED}" -eq 0 ] && [ -z "${apt_get}" ]; then
        echo >&2 "command 'apt-get' is required to install packages on a '${distribution} ${version}' system."
        exit 1
      fi
      ;;

    centos* | clearos*)
      package_installer=""
      tree="centos"
      [ -n "${dnf}" ] && package_installer="install_dnf"
      [ -n "${yum}" ] && package_installer="install_yum"
      if [ "${IGNORE_INSTALLED}" -eq 0 ] && [ -z "${yum}" ]; then
        echo >&2 "command 'yum' or 'dnf' is required to install packages on a '${distribution} ${version}' system."
        exit 1
      fi
      ;;

    fedora* | redhat* | red\ hat* | rhel*)
      package_installer=
      tree="rhel"
      [ -n "${dnf}" ] && package_installer="install_dnf"
      [ -n "${yum}" ] && package_installer="install_yum"
      if [ "${IGNORE_INSTALLED}" -eq 0 ] && [ -z "${package_installer}" ]; then
        echo >&2 "command 'yum' or 'dnf' is required to install packages on a '${distribution} ${version}' system."
        exit 1
      fi
      ;;

    ol*)
      package_installer=
      tree="ol"
      [ -n "${dnf}" ] && package_installer="install_dnf"
      [ -n "${yum}" ] && package_installer="install_yum"
      if [ "${IGNORE_INSTALLED}" -eq 0 ] && [ -z "${package_installer}" ]; then
        echo >&2 "command 'yum' or 'dnf' is required to install packages on a '${distribution} ${version}' system."
        exit 1
      fi
      ;;

    suse* | opensuse* | sles*)
      package_installer="install_zypper"
      tree="suse"
      if [ "${IGNORE_INSTALLED}" -eq 0 ] && [ -z "${zypper}" ]; then
        echo >&2 "command 'zypper' is required to install packages on a '${distribution} ${version}' system."
        exit 1
      fi
      ;;

    clear-linux* | clearlinux*)
      package_installer="install_swupd"
      tree="clearlinux"
      if [ "${IGNORE_INSTALLED}" -eq 0 ] && [ -z "${swupd}" ]; then
        echo >&2 "command 'swupd' is required to install packages on a '${distribution} ${version}' system."
        exit 1
      fi
      ;;

    freebsd)
      package_installer="install_pkg"
      tree="freebsd"
      if [ "${IGNORE_INSTALLED}" -eq 0 ] && [ -z "${pkg}" ]; then
        echo >&2 "command 'pkg' is required to install packages on a '${distribution} ${version}' system."
        exit 1
      fi
      ;;
    macos)
      package_installer="install_brew"
      tree="macos"
      if [ "${IGNORE_INSTALLED}" -eq 0 ] && [ -z "${brew}" ]; then
        echo >&2 "command 'brew' is required to install packages on a '${distribution} ${version}' system."
        exit 1
      fi
      ;;

    *)
      # oops! unknown system
      user_picks_distribution
      ;;
  esac
}

# XXX: This is being removed in another PR.
check_package_manager() {
  # This is called only when the user is selecting a package manager
  # It is used to verify the user selection is right

  echo >&2 "Checking package manager: ${1}"

  case "${1}" in
    apt-get)
      [ "${IGNORE_INSTALLED}" -eq 0 ] && [ -z "${apt_get}" ] && echo >&2 "${1} is not available." && return 1
      package_installer="install_apt_get"
      tree="debian"
      detection="user-input"
      return 0
      ;;

    dnf)
      [ "${IGNORE_INSTALLED}" -eq 0 ] && [ -z "${dnf}" ] && echo >&2 "${1} is not available." && return 1
      package_installer="install_dnf"
      if [ "${distribution}" = "centos" ]; then
        tree="centos"
      elif [ "${distribution}" = "ol" ]; then
        tree="ol"
      else
        tree="rhel"
      fi
      detection="user-input"
      return 0
      ;;

    apk)
      [ "${IGNORE_INSTALLED}" -eq 0 ] && [ -z "${apk}" ] && echo >&2 "${1} is not available." && return 1
      package_installer="install_apk"
      tree="alpine"
      detection="user-input"
      return 0
      ;;

    equo)
      [ "${IGNORE_INSTALLED}" -eq 0 ] && [ -z "${equo}" ] && echo >&2 "${1} is not available." && return 1
      package_installer="install_equo"
      tree="sabayon"
      detection="user-input"
      return 0
      ;;

    emerge)
      [ "${IGNORE_INSTALLED}" -eq 0 ] && [ -z "${emerge}" ] && echo >&2 "${1} is not available." && return 1
      package_installer="install_emerge"
      tree="gentoo"
      detection="user-input"
      return 0
      ;;

    pacman)
      [ "${IGNORE_INSTALLED}" -eq 0 ] && [ -z "${pacman}" ] && echo >&2 "${1} is not available." && return 1
      package_installer="install_pacman"
      tree="arch"
      detection="user-input"

      return 0
      ;;

    zypper)
      [ "${IGNORE_INSTALLED}" -eq 0 ] && [ -z "${zypper}" ] && echo >&2 "${1} is not available." && return 1
      package_installer="install_zypper"
      tree="suse"
      detection="user-input"
      return 0
      ;;

    yum)
      [ "${IGNORE_INSTALLED}" -eq 0 ] && [ -z "${yum}" ] && echo >&2 "${1} is not available." && return 1
      package_installer="install_yum"
      if [ "${distribution}" = "centos" ]; then
        tree="centos"
      elif [ "${distribution}" = "ol" ]; then
        tree="ol"
      else
        tree="rhel"
      fi
      detection="user-input"
      return 0
      ;;

    swupd)
      [ "${IGNORE_INSTALLED}" -eq 0 ] && [ -z "${swupd}" ] && echo >&2 "${1} is not available." && return 1
      package_installer="install_swupd"
      tree="clear-linux"
      detection="user-input"
      return 0
      ;;

    brew)
      [ "${IGNORE_INSTALLED}" -eq 0 ] && [ -z "${brew}" ] && echo >&2 "${1} is not available." && return 1
      package_installer="install_brew"
      tree="macos"
      detection="user-input"

      return 0
      ;;

    *)
      echo >&2 "Invalid package manager: '${1}'."
      return 1
      ;;
  esac
}

require_cmd() {
  # check if any of the commands given as argument
  # are present on this system
  # If any of them is available, it returns 0
  # otherwise 1

  [ "${IGNORE_INSTALLED}" -eq 1 ] && return 1

  local wanted found
  for wanted in "${@}"; do
    if command -v "${wanted}" > /dev/null 2>&1; then
      found="$(command -v "$wanted" 2> /dev/null)"
    fi
    [ -n "${found}" ] && [ -x "${found}" ] && return 0
  done
  return 1
}


validate_package_trees() {
  if type -t validate_tree_${tree} > /dev/null; then
    validate_tree_${tree}
  fi
}

validate_installed_package() {
  validate_${package_installer} "${p}"
}

DRYRUN=0
run() {

  printf >&2 "%q " "${@}"
  printf >&2 "\n"

  if [ ! "${DRYRUN}" -eq 1 ]; then
    "${@}"
    return $?
  fi
  return 0
}

sudo=
if [ ${UID} -ne 0 ]; then
  sudo="sudo"
fi

# -----------------------------------------------------------------------------
# debian / ubuntu

validate_install_apt_get() {
  echo >&2 " > Checking if package '${*}' is installed..."
  [ "$(dpkg-query -W --showformat='${Status}\n' "${*}")" = "install ok installed" ] || echo "${*}"
}

install_apt_get() {
  local opts=""
  if [ "${NON_INTERACTIVE}" -eq 1 ]; then
    echo >&2 "Running in non-interactive mode"
    # http://serverfault.com/questions/227190/how-do-i-ask-apt-get-to-skip-any-interactive-post-install-configuration-steps
    export DEBIAN_FRONTEND="noninteractive"
    opts="${opts} -yq"
  fi

  read -r -a apt_opts <<< "$opts"

  # update apt repository caches

  echo >&2 "NOTE: Running apt-get update and updating your APT caches ..."
  if [ "${version}" = 8 ]; then
    echo >&2 "WARNING: You seem to be on Debian 8 (jessie) which is old enough we have to disable Check-Valid-Until checks"
    if ! cat /etc/apt/sources.list /etc/apt/sources.list.d/* 2> /dev/null | grep -q jessie-backports; then
      echo >&2 "We also have to enable the jessie-backports repository"
      if prompt "Is this okay?"; then
        ${sudo} /bin/sh -c 'echo "deb http://archive.debian.org/debian/ jessie-backports main contrib non-free" >> /etc/apt/sources.list.d/99-archived.list'
      fi
    fi
    run ${sudo} apt-get "${apt_opts[@]}" -o Acquire::Check-Valid-Until=false update
  else
    run ${sudo} apt-get "${apt_opts[@]}" update
  fi

  # install the required packages
  run ${sudo} apt-get "${apt_opts[@]}" install "${@}"
}


validate_install_yum() {
  echo >&2 " > Checking if package '${*}' is installed..."
  yum list installed "${*}" > /dev/null 2>&1 || echo "${*}"
}

install_yum() {
  # download the latest package info
  if [ "${DRYRUN}" -eq 1 ]; then
    echo >&2 " >> IMPORTANT << "
    echo >&2 "    Please make sure your system is up to date"
    echo >&2 "    by running:  ${sudo} yum update  "
    echo >&2
  fi

  local opts=
  if [ "${NON_INTERACTIVE}" -eq 1 ]; then
    echo >&2 "Running in non-interactive mode"
    # http://unix.stackexchange.com/questions/87822/does-yum-have-an-equivalent-to-apt-aptitudes-debian-frontend-noninteractive
    opts="-y"
  fi

  read -r -a yum_opts <<< "${opts}"

  # install the required packages
  run ${sudo} yum "${yum_opts[@]}" install "${@}"
}

# -----------------------------------------------------------------------------
# fedora

validate_install_dnf() {
  echo >&2 " > Checking if package '${*}' is installed..."
  dnf list installed "${*}" > /dev/null 2>&1 || echo "${*}"
}

install_dnf() {
  # download the latest package info
  if [ "${DRYRUN}" -eq 1 ]; then
    echo >&2 " >> IMPORTANT << "
    echo >&2 "    Please make sure your system is up to date"
    echo >&2 "    by running:  ${sudo} dnf update  "
    echo >&2
  fi

  local opts=
  if [ "${NON_INTERACTIVE}" -eq 1 ]; then
    echo >&2 "Running in non-interactive mode"
    # man dnf
    opts="-y"
  fi

  # install the required packages
  # --setopt=strict=0 allows dnf to proceed
  # installing whatever is available
  # even if a package is not found
  opts="$opts --setopt=strict=0"
  read -r -a dnf_opts <<< "$opts"
  run ${sudo} dnf "${dnf_opts[@]}" install "${@}"
}

# -----------------------------------------------------------------------------
# gentoo

validate_install_emerge() {
  echo "${*}"
}

install_emerge() {
  # download the latest package info
  # we don't do this for emerge - it is very slow
  # and most users are expected to do this daily
  # emerge --sync
  if [ "${DRYRUN}" -eq 1 ]; then
    echo >&2 " >> IMPORTANT << "
    echo >&2 "    Please make sure your system is up to date"
    echo >&2 "    by running:  ${sudo} emerge --sync  or  ${sudo} eix-sync  "
    echo >&2
  fi

  local opts="--ask"
  if [ "${NON_INTERACTIVE}" -eq 1 ]; then
    echo >&2 "Running in non-interactive mode"
    opts=""
  fi

  read -r -a emerge_opts <<< "$opts"

  # install the required packages
  run ${sudo} emerge "${emerge_opts[@]}" -v --noreplace "${@}"
}

# -----------------------------------------------------------------------------
# alpine

validate_install_apk() {
  echo "${*}"
}

install_apk() {
  # download the latest package info
  if [ "${DRYRUN}" -eq 1 ]; then
    echo >&2 " >> IMPORTANT << "
    echo >&2 "    Please make sure your system is up to date"
    echo >&2 "    by running:  ${sudo} apk update  "
    echo >&2
  fi

  local opts="--force-broken-world"
  if [ "${NON_INTERACTIVE}" -eq 1 ]; then
    echo >&2 "Running in non-interactive mode"
  else
    opts="${opts} -i"
  fi

  read -r -a apk_opts <<< "$opts"

  # install the required packages
  run ${sudo} apk add "${apk_opts[@]}" "${@}"
}

# -----------------------------------------------------------------------------
# sabayon

validate_install_equo() {
  echo >&2 " > Checking if package '${*}' is installed..."
  equo s --installed "${*}" > /dev/null 2>&1 || echo "${*}"
}

install_equo() {
  # download the latest package info
  if [ "${DRYRUN}" -eq 1 ]; then
    echo >&2 " >> IMPORTANT << "
    echo >&2 "    Please make sure your system is up to date"
    echo >&2 "    by running:  ${sudo} equo up  "
    echo >&2
  fi

  local opts="-av"
  if [ "${NON_INTERACTIVE}" -eq 1 ]; then
    echo >&2 "Running in non-interactive mode"
    opts="-v"
  fi

  read -r -a equo_opts <<< "$opts"

  # install the required packages
  run ${sudo} equo i "${equo_opts[@]}" "${@}"
}

# -----------------------------------------------------------------------------
# arch

PACMAN_DB_SYNCED=0
validate_install_pacman() {

  if [ ${PACMAN_DB_SYNCED} -eq 0 ]; then
    echo >&2 " > Running pacman -Sy to sync the database"
    local x
    x=$(pacman -Sy)
    [ -z "${x}" ] && echo "${*}"
    PACMAN_DB_SYNCED=1
  fi
  echo >&2 " > Checking if package '${*}' is installed..."

  # In pacman, you can utilize alternative flags to exactly match package names,
  # but is highly likely we require pattern matching too in this so we keep -s and match
  # the exceptional cases like so
  local x=""
  case "${package}" in
    "gcc")
      # Temporary workaround: In archlinux, default installation includes runtime libs under package "gcc"
      # These are not sufficient for netdata install, so we need to make sure that the appropriate libraries are there
      # by ensuring devel libs are available
      x=$(pacman -Qs "${*}" | grep "base-devel")
      ;;
    "tar")
      x=$(pacman -Qs "${*}" | grep "local/tar")
      ;;
    "make")
      x=$(pacman -Qs "${*}" | grep "local/make ")
      ;;
    *)
      x=$(pacman -Qs "${*}")
      ;;
  esac

  [ -z "${x}" ] && echo "${*}"
}

install_pacman() {
  # download the latest package info
  if [ "${DRYRUN}" -eq 1 ]; then
    echo >&2 " >> IMPORTANT << "
    echo >&2 "    Please make sure your system is up to date"
    echo >&2 "    by running:  ${sudo} pacman -Syu  "
    echo >&2
  fi

  # install the required packages
  if [ "${NON_INTERACTIVE}" -eq 1 ]; then
    echo >&2 "Running in non-interactive mode"
    # http://unix.stackexchange.com/questions/52277/pacman-option-to-assume-yes-to-every-question/52278
    # Try the noconfirm option, if that fails, go with the legacy way for non-interactive
    run ${sudo} pacman --noconfirm --needed -S "${@}" || yes | run ${sudo} pacman --needed -S "${@}"
  else
    run ${sudo} pacman --needed -S "${@}"
  fi
}

# -----------------------------------------------------------------------------
# suse / opensuse

validate_install_zypper() {
  rpm -q "${*}" > /dev/null 2>&1 || echo "${*}"
}

install_zypper() {
  # download the latest package info
  if [ "${DRYRUN}" -eq 1 ]; then
    echo >&2 " >> IMPORTANT << "
    echo >&2 "    Please make sure your system is up to date"
    echo >&2 "    by running:  ${sudo} zypper update  "
    echo >&2
  fi

  local opts="--ignore-unknown"
  if [ "${NON_INTERACTIVE}" -eq 1 ]; then
    echo >&2 "Running in non-interactive mode"
    # http://unix.stackexchange.com/questions/82016/how-to-use-zypper-in-bash-scripts-for-someone-coming-from-apt-get
    opts="${opts} --non-interactive"
  fi

  read -r -a zypper_opts <<< "$opts"

  # install the required packages
  run ${sudo} zypper "${zypper_opts[@]}" install "${@}"
}

# -----------------------------------------------------------------------------
# clearlinux

validate_install_swupd() {
  swupd bundle-list | grep -q "${*}" || echo "${*}"
}

install_swupd() {
  # download the latest package info
  if [ "${DRYRUN}" -eq 1 ]; then
    echo >&2 " >> IMPORTANT << "
    echo >&2 "    Please make sure your system is up to date"
    echo >&2 "    by running:  ${sudo} swupd update  "
    echo >&2
  fi

  run ${sudo} swupd bundle-add "${@}"
}

# -----------------------------------------------------------------------------
# macOS

validate_install_pkg() {
  pkg query %n-%v | grep -q "${*}" || echo "${*}"
}

validate_install_brew() {
  brew list | grep -q "${*}" || echo "${*}"
}

install_pkg() {
  # download the latest package info
  if [ "${DRYRUN}" -eq 1 ]; then
    echo >&2 " >> IMPORTANT << "
    echo >&2 "    Please make sure your system is up to date"
    echo >&2 "    by running:  pkg update "
    echo >&2
  fi

  local opts=
  if [ "${NON_INTERACTIVE}" -eq 1 ]; then
    echo >&2 "Running in non-interactive mode"
    opts="-y"
  fi

  read -r -a pkg_opts <<< "${opts}"

  run ${sudo} pkg install "${pkg_opts[@]}" "${@}"
}

install_brew() {
  # download the latest package info
  if [ "${DRYRUN}" -eq 1 ]; then
    echo >&2 " >> IMPORTANT << "
    echo >&2 "    Please make sure your system is up to date"
    echo >&2 "    by running:  brew upgrade "
    echo >&2
  fi

  run brew install "${@}"
}

# -----------------------------------------------------------------------------

install_failed() {
  local ret="${1}"
  cat << EOF
We are very sorry!
Installation of required packages failed.
What to do now:
  1. Make sure your system is updated.
     Most of the times, updating your system will resolve the issue.
  2. If the error message is about a specific package, try removing
     that package from the command and run it again.
     Depending on the broken package, you may be able to continue.
  3. Let us know. We may be able to help.
     Open a github issue with the above log, at:
           https://github.com/netdata/netdata/issues
EOF
  remote_log "FAILED" "${ret}"
  exit 1
}

remote_log() {
  # log success or failure on our system
  # to help us solve installation issues
  curl > /dev/null 2>&1 -Ss --max-time 3 "https://registry.my-netdata.io/log/installer?status=${1}&error=${2}&distribution=${distribution}&version=${version}&installer=${package_installer}&tree=${tree}&detection=${detection}&netdata=${PACKAGES_NETDATA}&nodejs=${PACKAGES_NETDATA_NODEJS}&python=${PACKAGES_NETDATA_PYTHON}&python3=${PACKAGES_NETDATA_PYTHON3}&mysql=${PACKAGES_NETDATA_PYTHON_MYSQL}&postgres=${PACKAGES_NETDATA_PYTHON_POSTGRES}&pymongo=${PACKAGES_NETDATA_PYTHON_MONGO}&sensors=${PACKAGES_NETDATA_SENSORS}&database=${PACKAGES_NETDATA_DATABASE}&ebpf=${PACKAGES_NETDATA_EBPF}&firehol=${PACKAGES_FIREHOL}&fireqos=${PACKAGES_FIREQOS}&iprange=${PACKAGES_IPRANGE}&update_ipsets=${PACKAGES_UPDATE_IPSETS}&demo=${PACKAGES_NETDATA_DEMO_SITE}"
}

if [ -z "${1}" ]; then
  usage
  exit 1
fi

pv=$(python --version 2>&1)
if [ "${tree}" = macos ]; then
  pv=3
elif [[ "${pv}" =~ ^Python\ 2.* ]]; then
  pv=2
elif [[ "${pv}" =~ ^Python\ 3.* ]]; then
  pv=3
elif [[ "${tree}" == "centos" ]] && [ "${version}" -lt 8 ]; then
  pv=2
else
  pv=3
fi

# parse command line arguments
DONT_WAIT=0
NON_INTERACTIVE=0
IGNORE_INSTALLED=0
while [ -n "${1}" ]; do
  case "${1}" in
    distribution)
      distribution="${2}"
      shift
      ;;

    version)
      version="${2}"
      shift
      ;;

    codename)
      codename="${2}"
      shift
      ;;

    installer)
      check_package_manager "${2}" || exit 1
      shift
      ;;

    dont-wait | --dont-wait | -n)
      DONT_WAIT=1
      ;;

    non-interactive | --non-interactive | -y)
      NON_INTERACTIVE=1
      ;;

    ignore-installed | --ignore-installed | -i)
      IGNORE_INSTALLED=1
      ;;

    netdata-all)
      PACKAGES_NETDATA=1
      PACKAGES_NETDATA_NODEJS=1
      if [ "${pv}" -eq 2 ]; then
        PACKAGES_NETDATA_PYTHON=1
        PACKAGES_NETDATA_PYTHON_MYSQL=1
        PACKAGES_NETDATA_PYTHON_POSTGRES=1
        PACKAGES_NETDATA_PYTHON_MONGO=1
      else
        PACKAGES_NETDATA_PYTHON3=1
        PACKAGES_NETDATA_PYTHON3_MYSQL=1
        PACKAGES_NETDATA_PYTHON3_POSTGRES=1
        PACKAGES_NETDATA_PYTHON3_MONGO=1
      fi
      PACKAGES_NETDATA_SENSORS=1
      PACKAGES_NETDATA_DATABASE=1
      PACKAGES_NETDATA_EBPF=1
      ;;

    netdata)
      PACKAGES_NETDATA=1
      PACKAGES_NETDATA_PYTHON3=1
      PACKAGES_NETDATA_DATABASE=1
      PACKAGES_NETDATA_EBPF=1
      ;;

    python | netdata-python)
      PACKAGES_NETDATA_PYTHON=1
      ;;

    python3 | netdata-python3)
      PACKAGES_NETDATA_PYTHON3=1
      ;;

    python-mysql | mysql-python | mysqldb | netdata-mysql)
      if [ "${pv}" -eq 2 ]; then
        PACKAGES_NETDATA_PYTHON=1
        PACKAGES_NETDATA_PYTHON_MYSQL=1
      else
        PACKAGES_NETDATA_PYTHON3=1
        PACKAGES_NETDATA_PYTHON3_MYSQL=1
      fi
      ;;

    python-postgres | postgres-python | psycopg2 | netdata-postgres)
      if [ "${pv}" -eq 2 ]; then
        PACKAGES_NETDATA_PYTHON=1
        PACKAGES_NETDATA_PYTHON_POSTGRES=1
      else
        PACKAGES_NETDATA_PYTHON3=1
        PACKAGES_NETDATA_PYTHON3_POSTGRES=1
      fi
      ;;

    python-pymongo)
      if [ "${pv}" -eq 2 ]; then
        PACKAGES_NETDATA_PYTHON=1
        PACKAGES_NETDATA_PYTHON_MONGO=1
      else
        PACKAGES_NETDATA_PYTHON3=1
        PACKAGES_NETDATA_PYTHON3_MONGO=1
      fi
      ;;

    nodejs | netdata-nodejs)
      PACKAGES_NETDATA=1
      PACKAGES_NETDATA_NODEJS=1
      PACKAGES_NETDATA_DATABASE=1
      ;;

    sensors | netdata-sensors)
      PACKAGES_NETDATA=1
      PACKAGES_NETDATA_PYTHON3=1
      PACKAGES_NETDATA_SENSORS=1
      PACKAGES_NETDATA_DATABASE=1
      ;;

    firehol | update-ipsets | firehol-all | fireqos)
      PACKAGES_IPRANGE=1
      PACKAGES_FIREHOL=1
      PACKAGES_FIREQOS=1
      PACKAGES_IPRANGE=1
      PACKAGES_UPDATE_IPSETS=1
      ;;

    demo | all)
      PACKAGES_NETDATA=1
      PACKAGES_NETDATA_NODEJS=1
      if [ "${pv}" -eq 2 ]; then
        PACKAGES_NETDATA_PYTHON=1
        PACKAGES_NETDATA_PYTHON_MYSQL=1
        PACKAGES_NETDATA_PYTHON_POSTGRES=1
        PACKAGES_NETDATA_PYTHON_MONGO=1
      else
        PACKAGES_NETDATA_PYTHON3=1
        PACKAGES_NETDATA_PYTHON3_MYSQL=1
        PACKAGES_NETDATA_PYTHON3_POSTGRES=1
        PACKAGES_NETDATA_PYTHON3_MONGO=1
      fi
      PACKAGES_DEBUG=1
      PACKAGES_IPRANGE=1
      PACKAGES_FIREHOL=1
      PACKAGES_FIREQOS=1
      PACKAGES_UPDATE_IPSETS=1
      PACKAGES_NETDATA_DEMO_SITE=1
      PACKAGES_NETDATA_DATABASE=1
      PACKAGES_NETDATA_EBPF=1
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

# Check for missing core commands like grep, warn the user to install it and bail out cleanly
if ! command -v grep > /dev/null 2>&1; then
  echo >&2
  echo >&2 "ERROR: 'grep' is required for the install to run correctly and was not found on the system."
  echo >&2 "Please install grep and run the installer again."
  echo >&2
  exit 1
fi

if [ -z "${package_installer}" ] || [ -z "${tree}" ]; then
  if [ -z "${distribution}" ]; then
    # we dont know the distribution
    autodetect_distribution || user_picks_distribution
  fi

  # When no package installer is detected, try again from distro info if any
  if [ -z "${package_installer}" ]; then
    detect_package_manager_from_distribution "${distribution}"
  fi

  # Validate package manager trees
  validate_package_trees
fi

[ "${detection}" = "/etc/os-release" ] && cat << EOF
/etc/os-release information:
NAME            : ${NAME}
VERSION         : ${VERSION}
ID              : ${ID}
ID_LIKE         : ${ID_LIKE}
VERSION_ID      : ${VERSION_ID}
EOF

cat << EOF
We detected these:
Distribution    : ${distribution}
Version         : ${version}
Codename        : ${codename}
Package Manager : ${package_installer}
Packages Tree   : ${tree}
Detection Method: ${detection}
Default Python v: ${pv} $([ ${pv} -eq 2 ] && [ "${PACKAGES_NETDATA_PYTHON3}" -eq 1 ] && echo "(will install python3 too)")
EOF

#mapfile -t PACKAGES_TO_INSTALL < <(packages | sort -u)

echo "distribution: ${distribution}"

echo "before our script"
echo "FIRST PARAMETER ${1}"
dependencies/${distribution}.sh ${1} aa bb 

remote_log "OK"

exit 0
© 2021 GitHub, Inc.
Terms
Privacy
Security
Status
Docs
Contact GitHub
Pricing
API
Training
Blog
About
Loading complete
