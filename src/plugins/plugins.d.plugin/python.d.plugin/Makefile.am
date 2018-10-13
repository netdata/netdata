# SPDX-License-Identifier: GPL-3.0-or-later

MAINTAINERCLEANFILES = Makefile.in
CLEANFILES = \
	python.d.plugin \
	$(NULL)

python.d.plugin: python.d.plugin.in
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
	python.d.plugin \
	$(NULL)

dist_noinst_DATA = \
	python.d.plugin.in \
	$(NULL)
