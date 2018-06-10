# SPDX-License-Identifier: GPL-3.0+
MAINTAINERCLEANFILES= $(srcdir)/Makefile.in

dist_noinst_DATA = \
	README.md \
	debian/copyright \
	debian/netdata.conf \
	debian/source/format \
	debian/control.wheezy \
	debian/compat \
	debian/netdata.install \
	debian/netdata.lintian-overrides \
	debian/rules \
	debian/netdata.docs \
	debian/netdata.default \
	debian/control \
	debian/netdata.postinst.in \
	debian/netdata.service \
	debian/changelog \
	debian/netdata.postrm \
	rhel/build-netdata-rpm.sh \
	$(NULL)

dist_noinst_SCRIPTS = \
	debian/netdata.init \
	nc-backend.sh \
	$(NULL)

debian/changelog:
	echo "netdata ($(PACKAGE_VERSION)) UNRELEASED; urgency=medium" | \
		tr '_' '~' > $@
	echo "  * Latest release" >> $@
	echo " -- Netdata Team <>  `date -R`" >> $@
