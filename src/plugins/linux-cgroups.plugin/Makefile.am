# SPDX-License-Identifier: GPL-3.0-or-later

AUTOMAKE_OPTIONS = subdir-objects
MAINTAINERCLEANFILES = Makefile.in

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
	$(NULL)
