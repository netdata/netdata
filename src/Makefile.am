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
	$(OPTIONAL_LIBCAP_CFLAGS) \
	$(OPTIONAL_IPMIMONITORING_CFLAGS)\
	$(NULL)

sbin_PROGRAMS = netdata
dist_cache_DATA = .keep
dist_varlib_DATA = .keep
dist_registry_DATA = .keep
dist_log_DATA = .keep
plugins_PROGRAMS =

if ENABLE_PLUGIN_APPS
plugins_PROGRAMS += apps.plugin
endif

if ENABLE_PLUGIN_FREEIPMI
plugins_PROGRAMS += freeipmi.plugin
endif

if ENABLE_PLUGIN_CGROUP_NETWORK
plugins_PROGRAMS += cgroup-network
endif

netdata_SOURCES = \
	adaptive_resortable_list.c \
	adaptive_resortable_list.h \
	appconfig.c \
	appconfig.h \
	avl.c \
	avl.h \
	backend_prometheus.c \
	backend_prometheus.h \
	backends.c \
	backends.h \
	clocks.c \
	clocks.h \
	common.c \
	common.h \
	daemon.c \
	daemon.h \
	dictionary.c \
	dictionary.h \
	eval.c \
	eval.h \
	global_statistics.c \
	global_statistics.h \
	health.c \
	health.h \
	health_config.c \
	health_json.c \
	health_log.c \
	inlined.h \
	locks.c \
	locks.h \
	log.c \
	log.h \
	main.c \
	main.h \
	plugin_checks.c \
	plugin_checks.h \
	plugin_idlejitter.c \
	plugin_idlejitter.h \
	plugins_d.c \
	plugins_d.h \
	popen.c \
	popen.h \
	procfile.c \
	procfile.h \
	registry.c \
	registry.h \
	registry_db.c \
	registry_init.c \
	registry_internals.c \
	registry_internals.h \
	registry_log.c \
	registry_machine.c \
	registry_machine.h \
	registry_person.c \
	registry_person.h \
	registry_url.c \
	registry_url.h \
	rrd.c \
	rrd.h \
	rrd2json.c \
	rrd2json.h \
	rrd2json_api_old.c \
	rrd2json_api_old.h \
	rrdcalc.c \
	rrdcalctemplate.c \
	rrddim.c \
	rrddimvar.c \
	rrdfamily.c \
	rrdhost.c \
	rrdpush.c \
	rrdpush.h \
	rrdset.c \
	rrdsetvar.c \
	rrdvar.c \
	signals.c \
	signals.h \
	simple_pattern.c \
	simple_pattern.h \
	socket.c \
	socket.h \
	statistical.c \
	statistical.h \
	statsd.c \
	statsd.h \
	storage_number.c \
	storage_number.h \
	unit_test.c \
	unit_test.h \
	url.c \
	url.h \
	web_api_old.c \
	web_api_old.h \
	web_api_v1.c \
	web_api_v1.h \
	web_buffer.c \
	web_buffer.h \
	web_buffer_svg.c \
	web_buffer_svg.h \
	web_client.c \
	web_client.h \
	web_server.c \
	web_server.h \
	$(NULL)

if FREEBSD
netdata_SOURCES += \
	plugin_freebsd.c \
	plugin_freebsd.h \
	freebsd_sysctl.c \
	freebsd_getmntinfo.c \
	freebsd_getifaddrs.c \
	freebsd_devstat.c \
	zfs_common.c \
	zfs_common.h \
	freebsd_kstat_zfs.c \
	freebsd_ipfw.c \
	$(NULL)
else
if MACOS
netdata_SOURCES += \
	plugin_macos.c \
	plugin_macos.h \
	macos_sysctl.c \
	macos_mach_smi.c \
	macos_fw.c \
	$(NULL)
else
netdata_SOURCES += \
	ipc.c \
	ipc.h \
	plugin_nfacct.c \
	plugin_nfacct.h \
	plugin_proc.c \
	plugin_proc.h \
	plugin_proc_diskspace.c \
	plugin_proc_diskspace.h \
	plugin_tc.c \
	plugin_tc.h \
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
	proc_net_sockstat.c \
	proc_net_sockstat6.c \
	proc_net_softnet_stat.c \
	proc_net_stat_conntrack.c \
	proc_net_stat_synproxy.c \
	proc_self_mountinfo.c \
	proc_self_mountinfo.h \
	zfs_common.c \
    zfs_common.h \
	proc_spl_kstat_zfs.c \
	proc_stat.c \
	proc_sys_kernel_random_entropy_avail.c \
	proc_vmstat.c \
	proc_uptime.c \
	sys_kernel_mm_ksm.c \
	sys_devices_system_edac_mc.c \
	sys_devices_system_node.c \
	sys_fs_cgroup.c \
	sys_fs_btrfs.c \
	$(NULL)
endif
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
	inlined.h \
	locks.c locks.h \
	log.c log.h \
	procfile.c procfile.h \
	web_buffer.c web_buffer.h \
	$(NULL)

if FREEBSD
apps_plugin_SOURCES += \
	plugin_freebsd.h \
	$(NULL)
endif

apps_plugin_LDADD = \
	$(OPTIONAL_MATH_LIBS) \
	$(OPTIONAL_LIBCAP_LIBS) \
	$(NULL)

freeipmi_plugin_SOURCES = \
	freeipmi_plugin.c \
	clocks.c clocks.h \
	common.c common.h \
	inlined.h \
	locks.c locks.h \
	log.c log.h \
	procfile.c procfile.h \
	$(NULL)

freeipmi_plugin_LDADD = \
	$(OPTIONAL_IPMIMONITORING_LIBS) \
	$(NULL)

cgroup_network_SOURCES = \
    cgroup-network.c \
	clocks.c clocks.h \
    common.c common.h \
    inlined.h \
	locks.c locks.h \
    log.c log.h \
	procfile.c procfile.h \
	popen.c popen.h \
	signals.c signals.h \
    $(NULL)

cgroup_network_LDADD = \
	$(OPTIONAL_MATH_LIBS) \
	$(OPTIONAL_LIBCAP_LIBS) \
	$(NULL)
