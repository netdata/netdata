#!/bin/sh

install_deb() {
	# This is needed to ensure package installs don't prompt for any user input.
	ENV DEBIAN_FRONTEND=noninteractive

	apt-get update
	apt-get install -y --no-install-recommends \
		curl=7.64.0-4 \
		netcat=1.10-41.1 \
		jq=1.5+dfsg-2+b1
	apt-get install -y "/artifacts/netdata_${VERSION}_${ARCH}.deb"
}

install_rpm() {
	# Using a glob pattern here because I can't reliably determine what the
	# resulting package name will be (TODO: There must be a better way!)
	dnf install -y curl nc jq
	dnf install -y /artifacts/netdata-"${VERSION}"-*.rpm
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
