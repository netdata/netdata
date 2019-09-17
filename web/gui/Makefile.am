#
# Copyright (C) 2015 Alon Bar-Lev <alon.barlev@gmail.com>
# SPDX-License-Identifier: GPL-3.0-or-later
#
MAINTAINERCLEANFILES= $(srcdir)/Makefile.in
CLEANFILES = \
	version.txt \
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
	demo.html \
	demo2.html \
	demosites.html \
	demosites2.html \
	dashboard.html \
	dashboard.js \
	dashboard_info.js \
	dashboard_info_custom_example.js \
	dashboard.css \
	dashboard.slate.css \
	favicon.ico \
	goto-host-from-alarm.html \
	index.html \
	main.css \
	main.js \
	console.html \
	infographic.html \
	robots.txt \
	refresh-badges.js \
	sitemap.xml \
	tv.html \
	dash-example.html \
	version.txt \
	$(NULL)

webstaticdir=$(webdir)/static/img
dist_webstatic_DATA = \
    static/img/netdata-logomark.svg \
	$(NULL)

weblibdir=$(webdir)/lib
dist_weblib_DATA = \
	lib/bootstrap-3.3.7.min.js \
	lib/bootstrap-slider-10.0.0.min.js \
	lib/bootstrap-table-1.11.0.min.js \
	lib/bootstrap-table-export-1.11.0.min.js \
	lib/bootstrap-toggle-2.2.2.min.js \
	lib/clipboard-polyfill-be05dad.js \
	lib/d3-4.12.2.min.js \
	lib/d3pie-0.2.1-netdata-3.js \
	lib/dygraph-c91c859.min.js \
	lib/dygraph-smooth-plotter-c91c859.js \
	lib/fontawesome-all-5.0.1.min.js \
	lib/gauge-1.3.2.min.js \
	lib/jquery-2.2.4.min.js \
	lib/jquery.easypiechart-97b5824.min.js \
	lib/jquery.peity-3.2.0.min.js \
	lib/jquery.sparkline-2.1.2.min.js \
	lib/lz-string-1.4.4.min.js \
	lib/pako-1.0.6.min.js \
	lib/perfect-scrollbar-0.6.15.min.js \
	lib/tableExport-1.6.0.min.js \
	$(NULL)

webcssdir=$(webdir)/css
dist_webcss_DATA = \
	css/morris-0.5.1.css \
	css/bootstrap-3.3.7.css \
	css/bootstrap-theme-3.3.7.min.css \
	css/bootstrap-slate-flat-3.3.7.css \
	css/bootstrap-slider-10.0.0.min.css \
	css/bootstrap-toggle-2.2.2.min.css \
	css/c3-0.4.18.min.css \
	$(NULL)

webfontsdir=$(webdir)/fonts
dist_webfonts_DATA = \
	fonts/glyphicons-halflings-regular.eot \
	fonts/glyphicons-halflings-regular.svg \
	fonts/glyphicons-halflings-regular.ttf \
	fonts/glyphicons-halflings-regular.woff \
	fonts/glyphicons-halflings-regular.woff2 \
	$(NULL)

webimagesdir=$(webdir)/images
dist_webimages_DATA = \
	images/netdata-logomark.svg \
	images/alert-128-orange.png \
	images/alert-128-red.png \
	images/alert-multi-size-orange.ico \
	images/alert-multi-size-red.ico \
	images/animated.gif \
	images/check-mark-2-128-green.png \
	images/check-mark-2-multi-size-green.ico \
	images/netdata.svg \
	images/post.png \
	images/android-icon-36x36.png \
	images/android-icon-48x48.png \
	images/android-icon-72x72.png \
	images/android-icon-96x96.png \
	images/android-icon-144x144.png \
	images/android-icon-192x192.png \
    images/apple-icon-57x57.png \
    images/apple-icon-60x60.png \
    images/apple-icon-72x72.png \
    images/apple-icon-76x76.png \
    images/apple-icon-114x114.png \
    images/apple-icon-120x120.png \
    images/apple-icon-144x144.png \
    images/apple-icon-152x152.png \
    images/apple-icon-180x180.png \
    images/apple-icon-precomposed.png \
    images/apple-icon.png \
    images/favicon-16x16.png \
    images/favicon-32x32.png \
    images/favicon-96x96.png \
    images/favicon.ico \
    images/ms-icon-70x70.png \
    images/ms-icon-144x144.png \
    images/ms-icon-150x150.png \
    images/ms-icon-310x310.png \
	images/banner-icon-144x144.png \
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
