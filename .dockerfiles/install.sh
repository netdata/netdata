#!/bin/sh

install_deb() {
	apt-get update
	apt-get install -y --no-install-recommends \
		curl=7.64.0-4 \
		netcat=1.10-41.1 \
		jq=1.5+dfsg-2+b1
	apt-get install -y "/artifacts/netdata_${VERSION}_${ARCH}.deb"
	apt-get clean
	rm -rf /var/lib/apt/lists/*
}

case "${DISTRO}" in
debian | ubuntu)
	install_deb
	;;
*)
	printf "unspported distro: %s_%s" "${DISTRO}" "${DISTRO_VERSION}"
	exit 1
	;;
esac
