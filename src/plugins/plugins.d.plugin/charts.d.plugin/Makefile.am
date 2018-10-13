# SPDX-License-Identifier: GPL-3.0-or-later

MAINTAINERCLEANFILES = Makefile.in
CLEANFILES = \
	charts.d.plugin \
	$(NULL)

charts.d.plugin: charts.d.plugin.in
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
	charts.d.dryrun-helper.sh \
	charts.d.plugin \
	loopsleepms.sh.inc \
	$(NULL)

dist_noinst_DATA = \
	charts.d.plugin.in \
	$(NULL)
