#!/bin/sh

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
  apt-get install -y $(find /netdata/artifacts -type f -name 'netdata*.deb' ! -name '*dbgsym*' ! -name '*cups*' ! -name '*freeipmi*') || exit 3

  # Install testing tools
  apt-get install -y --no-install-recommends curl "${netcat}" jq || exit 1
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
  "$PKGMGR" install -y /netdata/artifacts/netdata*.rpm || exit 1

  # Install testing tools
  "$PKGMGR" install -y curl nc jq || exit 1
}

install_centos() {
  # Using a glob pattern here because I can't reliably determine what the
  # resulting package name will be (TODO: There must be a better way!)

  PKGMGR="$( (command -v dnf > /dev/null && echo "dnf") || echo "yum")"

  if [ "${PKGMGR}" = "dnf" ]; then
    opts="--allowerasing"
  fi

  # Install EPEL (needed for `jq`
  "$PKGMGR" install -y epel-release || exit 1

  # Install Netdata
  # Strange quoting is required here so that glob matching works.
  "$PKGMGR" install -y /netdata/artifacts/netdata*.rpm || exit 1

  # Install testing tools
  # shellcheck disable=SC2086
  "$PKGMGR" install -y ${opts} curl nc jq || exit 1
}

install_amazon_linux() {
  PKGMGR="$( (command -v dnf > /dev/null && echo "dnf") || echo "yum")"

  if [ "${PKGMGR}" = "dnf" ]; then
    opts="--allowerasing"
  fi

  # Install Netdata
  # Strange quoting is required here so that glob matching works.
  "$PKGMGR" install -y /netdata/artifacts/netdata*.rpm || exit 1

  # Install testing tools
  # shellcheck disable=SC2086
  "$PKGMGR" install -y ${opts} curl nc jq || exit 1
}

install_suse_like() {
  # Using a glob pattern here because I can't reliably determine what the
  # resulting package name will be (TODO: There must be a better way!)

  # Install Netdata
  # Strange quoting is required here so that glob matching works.
  zypper install -y --allow-downgrade --allow-unsigned-rpm /netdata/artifacts/netdata*.rpm || exit 1

  # Install testing tools
  zypper install -y --allow-downgrade --no-recommends curl netcat-openbsd jq || exit 1
}

dump_log() {
  cat ./netdata.log
}

wait_for() {
  host="${1}"
  port="${2}"
  name="${3}"
  timeout="30"

  if command -v nc > /dev/null ; then
    netcat="nc"
  elif command -v netcat > /dev/null ; then
    netcat="netcat"
  else
    printf "Unable to find a usable netcat command.\n"
    return 1
  fi

  printf "Waiting for %s on %s:%s ... " "${name}" "${host}" "${port}"

  sleep 30

  i=0
  while ! ${netcat} -z "${host}" "${port}"; do
    sleep 1
    if [ "$i" -gt "$timeout" ]; then
      printf "Timed out!\n"
      return 1
    fi
    i="$((i + 1))"
  done
  printf "OK\n"
}

case "${DISTRO}" in
  debian | ubuntu)
    install_debian_like
    ;;
  fedora | oraclelinux)
    install_fedora_like
    ;;
  centos | rockylinux | almalinux)
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

/usr/sbin/netdata -D > ./netdata.log 2>&1 &

wait_for localhost 19999 netdata || exit 1

curl -sS http://127.0.0.1:19999/api/v1/info > ./response || exit 1

cat ./response

jq '.version' ./response || exit 1

trap - EXIT
