#
# Copyright (C) 2015 Alon Bar-Lev <alon.barlev@gmail.com>
#
MAINTAINERCLEANFILES= $(srcdir)/Makefile.in

AM_CPPFLAGS = \
	-DCACHE_DIR="\"$(cachedir)\"" \
	-DCONFIG_DIR="\"$(configdir)\"" \
	-DLOG_DIR="\"$(logdir)\"" \
	-DPLUGINS_DIR="\"$(pluginsdir)\"" \
	-DWEB_DIR="\"$(webdir)\"" \
	$(NULL)
AM_CFLAGS = \
	$(OPTIONAL_MATH_CFLAGS) \
	$(OPTIONAL_NFACCT_CLFAGS) \
	$(OPTIONAL_ZLIB_CFLAGS) \
	$(NULL)

sbin_PROGRAMS = netdata
dist_cache_DATA = .keep
dist_log_DATA = .keep
plugins_PROGRAMS = apps.plugin

netdata_SOURCES = \
	appconfig.c appconfig.h \
	avl.c avl.h \
	common.c common.h \
	daemon.c daemon.h \
	dictionary.c dictionary.h \
	global_statistics.c global_statistics.h \
	log.c log.h \
	main.c main.h \
	plugin_checks.c plugin_checks.h \
	plugin_idlejitter.c plugin_idlejitter.h \
	plugin_nfacct.c plugin_nfacct.h \
	plugin_proc.c plugin_proc.h \
	plugin_tc.c plugin_tc.h \
	plugins_d.c plugins_d.h \
	popen.c popen.h \
	proc_diskstats.c \
	proc_interrupts.c \
	proc_softirqs.c \
	proc_loadavg.c \
	proc_meminfo.c \
	proc_net_dev.c \
	proc_net_ip_vs_stats.c \
	proc_net_netstat.c \
	proc_net_rpc_nfsd.c \
	proc_net_snmp.c \
	proc_net_stat_conntrack.c \
	proc_stat.c \
	proc_sys_kernel_random_entropy_avail.c \
	proc_vmstat.c \
	sys_kernel_mm_ksm.c \
	procfile.c procfile.h \
	rrd.c rrd.h \
	rrd2json.c rrd2json.h \
	storage_number.c storage_number.h \
	unit_test.c unit_test.h \
	url.c url.h \
	web_buffer.c web_buffer.h \
	web_client.c web_client.h \
	web_server.c web_server.h \
	$(NULL)
netdata_LDADD = \
	$(OPTIONAL_MATH_LIBS) \
	$(OPTIONAL_NFACCT_LIBS) \
	$(OPTIONAL_ZLIB_LIBS) \
	$(NULL)

apps_plugin_SOURCES = \
	apps_plugin.c \
	avl.c avl.h \
	common.c common.h \
	log.c log.h \
	procfile.c procfile.h \
	$(NULL)

install-data-hook:
	if [ `id -u` == 0 ]; then \
		chown root '$(DESTDIR)$(pluginsdir)/apps.plugin' && \
			chmod 4755 '$(DESTDIR)$(pluginsdir)/apps.plugin'; \
	else \
		echo; \
		echo "ATTENTION"; \
		echo; \
		echo "setuid bit of $(pluginsdir)/apps.plugin must be set, please execute as root:"; \
		echo "chown root '$(pluginsdir)/apps.plugin' && chmod 4755 '$(pluginsdir)/apps.plugin'"; \
		echo; \
	fi
