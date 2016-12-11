#
# Copyright (C) 2015 Alon Bar-Lev <alon.barlev@gmail.com>
#
MAINTAINERCLEANFILES= $(srcdir)/Makefile.in

AM_CPPFLAGS = \
	-DVARLIB_DIR="\"$(varlibdir)\"" \
	-DCACHE_DIR="\"$(cachedir)\"" \
	-DCONFIG_DIR="\"$(configdir)\"" \
	-DLOG_DIR="\"$(logdir)\"" \
	-DPLUGINS_DIR="\"$(pluginsdir)\"" \
	-DRUN_DIR="\"$(localstatedir)/run/netdata\"" \
	-DWEB_DIR="\"$(webdir)\"" \
	$(NULL)
AM_CFLAGS = \
	$(OPTIONAL_MATH_CFLAGS) \
	$(OPTIONAL_NFACCT_CLFAGS) \
	$(OPTIONAL_ZLIB_CFLAGS) \
	$(OPTIONAL_UUID_CFLAGS) \
	$(NULL)

sbin_PROGRAMS = netdata
dist_cache_DATA = .keep
dist_varlib_DATA = .keep
dist_registry_DATA = .keep
dist_log_DATA = .keep
plugins_PROGRAMS = apps.plugin

netdata_SOURCES = \
	appconfig.c appconfig.h \
	avl.c avl.h \
	backends.c backends.h \
	clocks.c clocks.h \
	common.c common.h \
	daemon.c daemon.h \
	dictionary.c dictionary.h \
	eval.c eval.h \
	global_statistics.c global_statistics.h \
	health.c health.h \
	log.c log.h \
	main.c main.h \
	plugin_checks.c plugin_checks.h \
	plugin_idlejitter.c plugin_idlejitter.h \
	plugin_nfacct.c plugin_nfacct.h \
	plugin_tc.c plugin_tc.h \
	plugins_d.c plugins_d.h \
	popen.c popen.h \
	sys_fs_cgroup.c \
	procfile.c procfile.h \
	proc_self_mountinfo.c proc_self_mountinfo.h \
	registry.c registry.h \
	rrd.c rrd.h \
	rrd2json.c rrd2json.h \
	storage_number.c storage_number.h \
	unit_test.c unit_test.h \
	url.c url.h \
	web_buffer.c web_buffer.h \
	web_buffer_svg.c web_buffer_svg.h \
	web_client.c web_client.h \
	web_server.c web_server.h \
	$(NULL)

if FREEBSD
netdata_SOURCES += \
	plugin_freebsd.c plugin_freebsd.h \
	freebsd_sysctl.c \
	$(NULL)
else
netdata_SOURCES += \
	plugin_proc.c plugin_proc.h \
	proc_diskstats.c \
	proc_interrupts.c \
	proc_softirqs.c \
	proc_loadavg.c \
	proc_meminfo.c \
	proc_net_dev.c \
	proc_net_ip_vs_stats.c \
	proc_net_netstat.c \
	proc_net_rpc_nfs.c \
	proc_net_rpc_nfsd.c \
	proc_net_snmp.c \
	proc_net_snmp6.c \
	proc_net_softnet_stat.c \
	proc_net_stat_conntrack.c \
	proc_net_stat_synproxy.c \
	proc_stat.c \
	proc_sys_kernel_random_entropy_avail.c \
	proc_vmstat.c \
	ipc.c ipc.h \
	sys_kernel_mm_ksm.c \
	$(NULL)
endif

netdata_LDADD = \
	$(OPTIONAL_MATH_LIBS) \
	$(OPTIONAL_NFACCT_LIBS) \
	$(OPTIONAL_ZLIB_LIBS) \
	$(OPTIONAL_UUID_LIBS) \
	$(NULL)

apps_plugin_SOURCES = \
	apps_plugin.c \
	avl.c avl.h \
	clocks.c clocks.h \
	common.c common.h \
	log.c log.h \
	procfile.c procfile.h \
	web_buffer.c web_buffer.h \
	$(NULL)

install-data-hook:
	if [ `id -u` == 0 ]; then \
		chown root '$(DESTDIR)$(pluginsdir)/apps.plugin' && \
		chmod 0755 '$(DESTDIR)$(pluginsdir)/apps.plugin' && \
		( setcap cap_dac_read_search,cap_sys_ptrace+ep '$(DESTDIR)$(pluginsdir)/apps.plugin' || \
		  chmod 4755 '$(DESTDIR)$(pluginsdir)/apps.plugin' ); \
	else \
		echo; \
		echo "ATTENTION"; \
		echo; \
		echo "$(pluginsdir)/apps.plugin requires escalated capabilities:"; \
		echo "sudo chown root '$(DESTDIR)$(pluginsdir)/apps.plugin'"; \
		echo "sudo chmod 0755 '$(DESTDIR)$(pluginsdir)/apps.plugin'"; \
		echo "sudo setcap cap_dac_read_search,cap_sys_ptrace+ep '$(DESTDIR)$(pluginsdir)/apps.plugin'"; \
		echo; \
	fi
