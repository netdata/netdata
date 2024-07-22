#!/bin/sh

SCRIPT_DIR="$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd -P)"

install_debian_like() {
  # This is needed to ensure package installs don't prompt for any user input.
  export DEBIAN_FRONTEND=noninteractive

  if apt-cache show netcat 2>&1 | grep -q "No packages found"; then
    netcat="netcat-traditional"
  else
    netcat="netcat"
  fi

  apt-get update

  # Install Netdata
  # Strange quoting is required here so that glob matching works.
  # shellcheck disable=SC2046
  apt-get install -y $(find /netdata/artifacts -type f -name 'netdata*.deb' \
! -name '*dbgsym*' ! -name '*cups*' ! -name '*freeipmi*') || exit 3

  # Install testing tools
  apt-get install -y --no-install-recommends curl dpkg-dev "${netcat}" jq || exit 1

  dpkg-architecture --equal amd64 || NETDATA_SKIP_EBPF=1
}

install_fedora_like() {
  # Using a glob pattern here because I can't reliably determine what the
  # resulting package name will be (TODO: There must be a better way!)

  PKGMGR="$( (command -v dnf > /dev/null && echo "dnf") || echo "yum")"

  if [ "${PKGMGR}" = "dnf" ]; then
    opts="--allowerasing"
  fi

  # Install Netdata
  # Strange quoting is required here so that glob matching works.
  "${PKGMGR}" install -y /netdata/artifacts/netdata*.rpm || exit 1

  # Install testing tools
  "${PKGMGR}" install -y curl nc jq || exit 1

  [ "$(rpm --eval '%{_build_arch}')" = "x86_64" ] || NETDATA_SKIP_EBPF=1
}

install_centos() {
  # Using a glob pattern here because I can't reliably determine what the
  # resulting package name will be (TODO: There must be a better way!)

  PKGMGR="$( (command -v dnf > /dev/null && echo "dnf") || echo "yum")"

  if [ "${PKGMGR}" = "dnf" ]; then
    opts="--allowerasing"
  fi

  # Install EPEL (needed for `jq`
  "${PKGMGR}" install -y epel-release || exit 1

  # Install Netdata
  # Strange quoting is required here so that glob matching works.
  "${PKGMGR}" install -y /netdata/artifacts/netdata*.rpm || exit 1

  # Install testing tools
  # shellcheck disable=SC2086
  "${PKGMGR}" install -y ${opts} curl nc jq || exit 1

  [ "$(rpm --eval '%{_build_arch}')" = "x86_64" ] || NETDATA_SKIP_EBPF=1
}

install_amazon_linux() {
  PKGMGR="$( (command -v dnf > /dev/null && echo "dnf") || echo "yum")"

  if [ "${PKGMGR}" = "dnf" ]; then
    opts="--allowerasing"
  fi

  # Install Netdata
  # Strange quoting is required here so that glob matching works.
  "${PKGMGR}" install -y /netdata/artifacts/netdata*.rpm || exit 1

  # Install testing tools
  # shellcheck disable=SC2086
  "${PKGMGR}" install -y ${opts} curl nc jq || exit 1

  [ "$(rpm --eval '%{_build_arch}')" = "x86_64" ] || NETDATA_SKIP_EBPF=1
}

install_suse_like() {
  # Using a glob pattern here because I can't reliably determine what the
  # resulting package name will be (TODO: There must be a better way!)

  # Install Netdata
  # Strange quoting is required here so that glob matching works.
  zypper install -y --allow-downgrade --allow-unsigned-rpm /netdata/artifacts/netdata*.rpm || exit 1

  # Install testing tools
  zypper install -y --allow-downgrade --no-recommends curl netcat-openbsd jq || exit 1

  [ "$(rpm --eval '%{_build_arch}')" = "x86_64" ] || NETDATA_SKIP_EBPF=1
}

dump_log() {
  cat ./netdata.log
}

case "${DISTRO}" in
  debian | ubuntu)
    install_debian_like
    ;;
  fedora | oraclelinux)
    install_fedora_like
    ;;
  centos | centos-stream | rockylinux | almalinux)
    install_centos
    ;;
  amazonlinux)
    install_amazon_linux
    ;;
  opensuse)
    install_suse_like
    ;;
  *)
    printf "ERROR: unsupported distro: %s_%s\n" "${DISTRO}" "${DISTRO_VERSION}"
    exit 1
    ;;
esac

trap dump_log EXIT

export NETDATA_LIBEXEC_PREFIX=/usr/libexec/netdata
export NETDATA_SKIP_LIBEXEC_PARTS="freeipmi|xenstat|nfacct|cups"

if [ -n "${NETDATA_SKIP_EBPF}" ]; then
    export NETDATA_SKIP_LIBEXEC_PARTS="${NETDATA_SKIP_LIBEXEC_PARTS}|ebpf"
fi

/usr/sbin/netdata -D > ./netdata.log 2>&1 &

"${SCRIPT_DIR}/../../packaging/runtime-check.sh" || exit 1

trap - EXIT
