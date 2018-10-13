# SPDX-License-Identifier: GPL-3.0-or-later

AUTOMAKE_OPTIONS = subdir-objects
MAINTAINERCLEANFILES = Makefile.in

CLEANFILES = \
	fping.plugin \
	$(NULL)

fping.plugin: fping.plugin.in
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
	fping.plugin \
	$(NULL)

dist_noinst_DATA = \
	fping.plugin.in \
	$(NULL)
