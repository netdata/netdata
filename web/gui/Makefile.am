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
    $(srcdir)/src/dashboard.js/prologue.js.inc \
    $(srcdir)/src/dashboard.js/utils.js \
    $(srcdir)/src/dashboard.js/server-detection.js \
    $(srcdir)/src/dashboard.js/dependencies.js \
    $(srcdir)/src/dashboard.js/error-handling.js \
    $(srcdir)/src/dashboard.js/compatibility.js \
    $(srcdir)/src/dashboard.js/xss.js \
    $(srcdir)/src/dashboard.js/colors.js \
    $(srcdir)/src/dashboard.js/units-conversion.js \
    $(srcdir)/src/dashboard.js/options.js \
    $(srcdir)/src/dashboard.js/localstorage.js \
    $(srcdir)/src/dashboard.js/timeout.js \
    $(srcdir)/src/dashboard.js/themes.js \
    $(srcdir)/src/dashboard.js/charting/dygraph.js \
    $(srcdir)/src/dashboard.js/charting/sparkline.js \
    $(srcdir)/src/dashboard.js/charting/google-charts.js \
    $(srcdir)/src/dashboard.js/charting/gauge.js \
    $(srcdir)/src/dashboard.js/charting/easy-pie-chart.js \
    $(srcdir)/src/dashboard.js/charting/d3pie.js \
    $(srcdir)/src/dashboard.js/charting/d3.js \
    $(srcdir)/src/dashboard.js/charting/peity.js \
    $(srcdir)/src/dashboard.js/charting/textonly.js \
    $(srcdir)/src/dashboard.js/charting.js \
    $(srcdir)/src/dashboard.js/chart-registry.js \
    $(srcdir)/src/dashboard.js/common.js \
    $(srcdir)/src/dashboard.js/main.js \
    $(srcdir)/src/dashboard.js/alarms.js \
    $(srcdir)/src/dashboard.js/registry.js \
    $(srcdir)/src/dashboard.js/boot.js \
    $(srcdir)/src/dashboard.js/epilogue.js.inc \
    $(NULL)

dist_noinst_DATA = \
    $(srcdir)/README.md \
    $(DASHBOARD_JS_FILES) \
    $(NULL)

dist_web_DATA = \
    dashboard.js \
    $(srcdir)/dashboard_info.js \
    $(srcdir)/dashboard_info_custom_example.js \
    $(srcdir)/main.css \
    $(srcdir)/main.js \
    version.txt \
    $(NULL)

webolddir=$(webdir)/old
dist_webold_DATA = \
    $(srcdir)/old/index.html \
    $(NULL)

webstaticdir=$(webdir)/static/img
dist_webstatic_DATA = \
    $(srcdir)/static/img/netdata-logomark.svg \
    $(NULL)

webcssdir=$(webdir)/css
dist_webcss_DATA = \
    $(srcdir)/css/morris-0.5.1.css \
    $(srcdir)/css/c3-0.4.18.min.css \
    $(NULL)

dashboard.js: $(DASHBOARD_JS_FILES)
	if test -f $@; then rm -f $@; fi
	cat $(DASHBOARD_JS_FILES) > $@.tmp && mv $@.tmp $@

webwellknowndir=$(webdir)/.well-known
dist_webwellknown_DATA = \
    $(NULL)

webdntdir=$(webdir)/.well-known/dnt
dist_webdnt_DATA = \
    $(srcdir)/.well-known/dnt/cookies \
    $(NULL)

version.txt:
	if test -d "$(top_srcdir)/.git"; then \
		git --git-dir="$(top_srcdir)/.git" log -n 1 --format=%H; \
	fi > $@.tmp
	test -s $@.tmp || echo 0 > $@.tmp
	mv $@.tmp $@

# regenerate these files, even if they are up to date
.PHONY: version.txt dashboard.js
