# Copyright 1999-2016 Gentoo Foundation
# Distributed under the terms of the GNU General Public License v2
# $Id$

EAPI=6

inherit linux-info systemd user

if [[ ${PV} == "9999" ]] ; then
	EGIT_REPO_URI="git://github.com/firehol/${PN}.git"
	inherit git-r3 autotools
	SRC_URI=""
	KEYWORDS=""
else
	SRC_URI="http://firehol.org/download/${PN}/releases/v${PV}/${P}.tar.xz"
	KEYWORDS="~amd64 ~x86"
fi


DESCRIPTION="Linux real time system monitoring, done right!"
HOMEPAGE="https://github.com/firehol/netdata http://netdata.firehol.org/"

LICENSE="GPL-3+ MIT BSD"
SLOT="0"
IUSE="+compression nfacct nodejs"

# most unconditional dependencies are for plugins.d/charts.d.plugin:
RDEPEND="
	>=app-shells/bash-4:0
	net-misc/curl
	net-misc/wget
	virtual/awk
	compression? ( sys-libs/zlib )
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

: ${NETDATA_USER:=netdata}
: ${NETDATA_GROUP:=netdata}

pkg_setup() {
	linux-info_pkg_setup

	enewgroup ${PN}
	enewuser ${PN} -1 -1 / ${PN}
}

src_prepare() {
	default
	[[ ${PV} == "9999" ]] && eautoreconf
}

src_configure() {
	econf \
		--localstatedir=/var \
		--with-user=${NETDATA_USER} \
		$(use_enable nfacct plugin-nfacct) \
		$(use_with compression zlib)
}

src_install() {
	default

	fowners ${NETDATA_USER}:${NETDATA_GROUP} /var/log/netdata
	fowners ${NETDATA_USER}:${NETDATA_GROUP} /var/cache/netdata

	chown -Rc ${NETDATA_USER}:${NETDATA_GROUP} "${ED}"/usr/share/${PN} || die

	cat >> "${T}"/${PN}-sysctl <<- EOF
	kernel.mm.ksm.run = 1
	kernel.mm.ksm.sleep_millisecs = 1000
	EOF

	dodoc "${T}"/${PN}-sysctl

	newinitd system/netdata-openrc ${PN}
	systemd_dounit system/netdata.service
}

pkg_postinst() {
	if [[ -e "/sys/kernel/mm/ksm/run" ]]; then
		elog "INFORMATION:"
		echo ""
		elog "I see you have kernel memory de-duper (called Kernel Same-page Merging,"
		elog "or KSM) available, but it is not currently enabled."
		echo ""
		elog "To enable it run:"
		echo ""
		elog "echo 1 >/sys/kernel/mm/ksm/run"
		elog "echo 1000 >/sys/kernel/mm/ksm/sleep_millisecs"
		echo  ""
		elog "If you enable it, you will save 20-60% of netdata memory."
	else
		elog "INFORMATION:"
		echo ""
		elog "I see you do not have kernel memory de-duper (called Kernel Same-page"
		elog "Merging, or KSM) available."
		echo ""
		elog "To enable it, you need a kernel built with CONFIG_KSM=y"
		echo ""
		elog "If you can have it, you will save 20-60% of netdata memory."
	fi

}
