#!/bin/sh

install_debian_like() {
  # This is needed to ensure package installs don't prompt for any user input.
  export DEBIAN_FRONTEND=noninteractive

  apt-get update

  # Install NetData
  apt-get install -y "/packages/netdata_${VERSION}_${ARCH}.deb"

  # Install testing tools
  apt-get install -y --no-install-recommends \
    curl netcat jq
}

install_fedora_like() {
  # Using a glob pattern here because I can't reliably determine what the
  # resulting package name will be (TODO: There must be a better way!)

  PKGMGR="$( (command -v dnf > /dev/null && echo "dnf") || echo "yum")"

  pkg_version="$(echo "${VERSION}" | tr - .)"

  # Install NetData
  "$PKGMGR" install -y /packages/netdata-"${pkg_version}"-*.rpm

  # Install testing tools
  "$PKGMGR" install -y curl nc jq
}

install_centos() {
  # Using a glob pattern here because I can't reliably determine what the
  # resulting package name will be (TODO: There must be a better way!)

  PKGMGR="$( (command -v dnf > /dev/null && echo "dnf") || echo "yum")"

  pkg_version="$(echo "${VERSION}" | tr - .)"

  # Install EPEL (needed for `jq`
  "$PKGMGR" install -y epel-release

  # Install NetData
  "$PKGMGR" install -y /packages/netdata-"${pkg_version}"-*.rpm

  # Install testing tools
  "$PKGMGR" install -y curl nc jq
}

install_suse_like() {
  # Using a glob pattern here because I can't reliably determine what the
  # resulting package name will be (TODO: There must be a better way!)

  pkg_version="$(echo "${VERSION}" | tr - .)"

  # Install NetData
  # FIXME: Allow unsigned packages (for now) #7773
  zypper install -y --allow-unsigned-rpm \
    /packages/netdata-"${pkg_version}"-*.rpm

  # Install testing tools
  zypper install -y --no-recommends \
    curl netcat-openbsd jq
}

case "${DISTRO}" in
  debian | ubuntu)
    install_debian_like
    ;;
  fedora)
    install_fedora_like
    ;;
  centos)
    install_centos
    ;;
  opensuse)
    install_suse_like
    ;;
  *)
    printf "ERROR: unsupported distro: %s_%s\n" "${DISTRO}" "${DISTRO_VERSION}"
    exit 1
    ;;
esac
