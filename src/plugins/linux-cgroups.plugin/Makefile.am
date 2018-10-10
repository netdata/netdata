# SPDX-License-Identifier: GPL-3.0-or-later

AUTOMAKE_OPTIONS = subdir-objects
MAINTAINERCLEANFILES = Makefile.in

AM_CFLAGS = \
	$(OPTIONAL_MATH_CFLAGS) \
	$(OPTIONAL_ZLIB_CFLAGS) \
	$(OPTIONAL_UUID_CFLAGS) \
	$(NULL)

CLEANFILES = \
	cgroup-name.sh \
	$(NULL)

cgroup-name.sh: cgroup-name.sh.in
	if sed \
		-e 's#[@]configdir_POST@#$(configdir)#g' \
		-e 's#[@]libconfigdir_POST@#$(libconfigdir)#g' \
		$< > $@.tmp; then \
		mv "$@.tmp" "$@"; \
	else \
		rm -f "$@.tmp"; \
		false; \
	fi

dist_plugins_SCRIPTS = \
	cgroup-name.sh \
	cgroup-network-helper.sh \
	$(NULL)

dist_noinst_DATA = \
	cgroup-name.sh.in \
	plugin_sources.am \
	$(NULL)

plugins_PROGRAMS=

include ../../libnetdata/libnetdata_sources.am

cgroup_network_SOURCES = \
    cgroup-network.c \
    $(libnetdata_SOURCES) \
	$(NULL)

if ENABLE_PLUGIN_CGROUP_NETWORK
    plugins_PROGRAMS += cgroup-network

    cgroup_network_LDADD = \
        $(OPTIONAL_MATH_LIBS) \
        $(OPTIONAL_ZLIB_LIBS) \
        $(OPTIONAL_UUID_LIBS) \
        $(NULL)
endif
