# SPDX-License-Identifier: GPL-3.0-or-later

AUTOMAKE_OPTIONS = subdir-objects
CLEANFILES = \
	tc-qos-helper.sh \
	$(NULL)

dist_plugins_SCRIPTS = \
	tc-qos-helper.sh \
	$(NULL)

dist_noinst_DATA = \
	tc-qos-helper.sh.in \
	$(NULL)

tc-qos-helper.sh: tc-qos-helper.sh.in
	if sed \
		-e 's#[@]configdir_POST@#$(configdir)#g' \
		-e 's#[@]libconfigdir_POST@#$(libconfigdir)#g' \
		$< > $@.tmp; then \
		mv "$@.tmp" "$@"; \
	else \
		rm -f "$@.tmp"; \
		false; \
	fi

