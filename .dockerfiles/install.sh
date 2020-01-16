#!/bin/sh

install_deb() {
	# This is needed to ensure package installs don't prompt for any user input.
	ENV DEBIAN_FRONTEND=noninteractive

	apt-get update

	# Install NetData
	apt-get install -y "/artifacts/netdata_${VERSION}_${ARCH}.deb"

	# Install testing tools
	apt-get install -y --no-install-recommends \
		curl netcat jq
}

install_rpm() {
	# Using a glob pattern here because I can't reliably determine what the
	# resulting package name will be (TODO: There must be a better way!)

	# Install NetData
	dnf install -y /artifacts/netdata-"${VERSION}"-*.rpm

	# Install testing tools
	dnf install -y curl nc jq
}

case "${DISTRO}" in
debian | ubuntu)
	install_deb
	;;
fedora | centos)
	install_rpm
	;;
*)
	printf "ERROR: unspported distro: %s_%s\n" "${DISTRO}" "${DISTRO_VERSION}"
	exit 1
	;;
esac
