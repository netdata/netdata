# SPDX-License-Identifier: GPL-3.0-or-later

MAINTAINERCLEANFILES = Makefile.in

AM_CFLAGS = \
	$(OPTIONAL_MATH_CFLAGS) \
	$(OPTIONAL_ZLIB_CFLAGS) \
	$(OPTIONAL_UUID_CFLAGS) \
	$(NULL)

CLEANFILES = \
	cgroup-name.sh \
	$(NULL)

# these 2 corrupt config.h every second compilation
# include $(top_srcdir)/build/subst.inc
# SUFFIXES = .in

cgroup-name.sh: cgroup-name.sh.in
	if sed \
		-e 's#[@]localstatedir_POST@#$(localstatedir)#g' \
		-e 's#[@]sbindir_POST@#$(sbindir)#g' \
		-e 's#[@]sysconfdir_POST@#$(sysconfdir)#g' \
		-e 's#[@]pythondir_POST@#$(pythondir)#g' \
		-e 's#[@]configdir_POST@#$(configdir)#g' \
		-e 's#[@]libconfigdir_POST@#$(libconfigdir)#g' \
		-e 's#[@]cachedir_POST@#$(cachedir)#g' \
		$< > $@.tmp; then \
		mv "$@.tmp" "$@"; \
	else \
		rm -f "$@.tmp"; \
		false; \
	fi

plugins_PROGRAMS=

noinst_LIBRARIES=libplugin-linux-cgroups.a

libplugin_linux_cgroups_a_SOURCES= \
	sys_fs_cgroup.h \
	sys_fs_cgroup.c \
	$(NULL)

if ENABLE_PLUGIN_CGROUP_NETWORK
plugins_PROGRAMS += cgroup-network

cgroup_network_SOURCES = \
    cgroup-network.c \
	$(NULL)

cgroup_network_LDADD = \
	../../libnetdata/libnetdata.a \
	$(OPTIONAL_MATH_LIBS) \
	$(OPTIONAL_LIBCAP_LIBS) \
	$(NULL)

endif

dist_plugins_SCRIPTS = \
	cgroup-name.sh \
	cgroup-network-helper.sh \
	$(NULL)

dist_noinst_DATA = \
	cgroup-name.sh.in \
	$(NULL)
