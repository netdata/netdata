# Copyright 1999-2015 Gentoo Foundation
# Distributed under the terms of the GNU General Public License v2
# $Id$

EAPI=5

inherit autotools git-2 user

DESCRIPTION="Linux real time system monitoring, over the web!"
HOMEPAGE="https://github.com/firehol/netdata"
EGIT_REPO_URI="https://github.com/firehol/netdata"

LICENSE="GPL-2+"
SLOT="0"
KEYWORDS=""
IUSE="nfacct +zlib"

RDEPEND="zlib? ( sys-libs/zlib )
	nfacct? (
		net-firewall/nfacct
		net-libs/libmnl
	)"
DEPEND="${RDEPEND}
	virtual/pkgconfig"

pkg_setup() {
	enewgroup netdata
	enewuser netdata -1 -1 / netdata
}

src_prepare() {
	eautoreconf
}

src_configure() {
	econf \
		--localstatedir="${EROOT}var" \
		$(use_enable nfacct plugin-nfacct) \
		$(use_with zlib) \
		--with-user=netdata
}

src_install() {
	default
	fowners netdata /var/log/netdata
	rm -fr "${ED}/var/cache/netdata"
}
