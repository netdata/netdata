# Copyright 1999-2016 Gentoo Foundation
# Distributed under the terms of the GNU General Public License v2
# $Id$

EAPI=6

inherit linux-info systemd user

if [[ ${PV} == "9999" ]] ; then
	EGIT_REPO_URI="git://github.com/firehol/${PN}.git"
	EGIT_BRANCH="master"
	inherit git-r3 autotools
	SRC_URI=""
	KEYWORDS=""
else
	SRC_URI="http://firehol.org/download/${PN}/releases/v${PV}/${P}.tar.xz"
fi


DESCRIPTION="Linux real time system monitoring, over the web!"
HOMEPAGE="https://github.com/firehol/netdata"

LICENSE="GPL-3+"
SLOT="0"
KEYWORDS="~amd64 ~x86"
IUSE="+zlib nfacct nodejs"

# most unconditional dependencies are for plugins.d/charts.d.plugin:
RDEPEND=">=app-shells/bash-4
	sys-apps/sed
	sys-apps/grep
	virtual/awk
	net-misc/curl
	net-misc/wget
	
	zlib? ( sys-libs/zlib )
	
	nfacct? (
		net-firewall/nfacct
		net-libs/libmnl
	)

	nodejs? (
		net-libs/nodejs
	)"

DEPEND="${RDEPEND}
	virtual/pkgconfig"

# check for Kernel-Samepage-Merging (CONFIG_KSM)
CONFIG_CHECK="
	~KSM
"

pkg_setup() {
	linux-info_pkg_setup

	enewgroup ${PN}
	enewuser ${PN} -1 -1 / ${PN}
}

src_unpack() {
	if [[ ${PV} == "9999" ]] ; then
		git-r3_src_unpack
	fi
	default
}

src_prepare() {
	if [[ ${PV} == "9999" ]] ; then
		eautoreconf
	fi
	default
}

src_configure() {
	econf \
		--localstatedir=/var \
		$(use_enable nfacct plugin-nfacct) \
		$(use_with zlib) \
		--with-user=${PN} \
		|| die "configure failed"
}

src_install() {
	default

	fowners ${PN} /var/log/netdata

	newinitd system/netdata-openrc ${PN}
	systemd_dounit system/netdata.service
}
