#
# Copyright (C) 2015 Alon Bar-Lev <alon.barlev@gmail.com>
# SPDX-License-Identifier: GPL-3.0-or-later
#
AUTOMAKE_OPTIONS = subdir-objects
MAINTAINERCLEANFILES = $(srcdir)/Makefile.in
CLEANFILES = \
    version.txt \
    $(NULL)

SUBDIRS = \
    dashboard \
    $(NULL)

DASHBOARD_JS_FILES = \
    src/dashboard.js/prologue.js.inc \
    src/dashboard.js/utils.js \
    src/dashboard.js/server-detection.js \
    src/dashboard.js/dependencies.js \
    src/dashboard.js/error-handling.js \
    src/dashboard.js/compatibility.js \
    src/dashboard.js/xss.js \
    src/dashboard.js/colors.js \
    src/dashboard.js/units-conversion.js \
    src/dashboard.js/options.js \
    src/dashboard.js/localstorage.js \
    src/dashboard.js/timeout.js \
    src/dashboard.js/themes.js \
    src/dashboard.js/charting/dygraph.js \
    src/dashboard.js/charting/sparkline.js \
    src/dashboard.js/charting/google-charts.js \
    src/dashboard.js/charting/gauge.js \
    src/dashboard.js/charting/easy-pie-chart.js \
    src/dashboard.js/charting/d3pie.js \
    src/dashboard.js/charting/d3.js \
    src/dashboard.js/charting/peity.js \
    src/dashboard.js/charting/textonly.js \
    src/dashboard.js/charting.js \
    src/dashboard.js/chart-registry.js \
    src/dashboard.js/common.js \
    src/dashboard.js/main.js \
    src/dashboard.js/alarms.js \
    src/dashboard.js/registry.js \
    src/dashboard.js/boot.js \
    src/dashboard.js/epilogue.js.inc \
    $(NULL)

dist_noinst_DATA = \
    README.md \
    $(DASHBOARD_JS_FILES) \
    $(NULL)

dist_web_DATA = \
    dashboard.js \
    dashboard_info.js \
    dashboard_info_custom_example.js \
    main.css \
    main.js \
    version.txt \
    $(NULL)

webolddir=$(webdir)/old
dist_webold_DATA = \
    old/index.html \
    $(NULL)

webstaticdir=$(webdir)/static/img
dist_webstatic_DATA = \
    static/img/netdata-logomark.svg \
    $(NULL)

webcssdir=$(webdir)/css
dist_webcss_DATA = \
    css/morris-0.5.1.css \
    css/c3-0.4.18.min.css \
    $(NULL)

dashboard.js: $(DASHBOARD_JS_FILES)
	if test -f $@; then rm -f $@; fi
	cat $(DASHBOARD_JS_FILES) > $@.tmp && mv $@.tmp $@

webwellknowndir=$(webdir)/.well-known
dist_webwellknown_DATA = \
    $(NULL)

webdntdir=$(webdir)/.well-known/dnt
dist_webdnt_DATA = \
    .well-known/dnt/cookies \
    $(NULL)

version.txt:
	if test -d "$(top_srcdir)/.git"; then \
		git --git-dir="$(top_srcdir)/.git" log -n 1 --format=%H; \
	fi > $@.tmp
	test -s $@.tmp || echo 0 > $@.tmp
	mv $@.tmp $@

# regenerate these files, even if they are up to date
.PHONY: version.txt dashboard.js
