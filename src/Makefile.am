# SPDX-License-Identifier: GPL-3.0-or-later

AUTOMAKE_OPTIONS = subdir-objects
MAINTAINERCLEANFILES= $(srcdir)/Makefile.in

SUBDIRS = \
    api \
    backends \
    database \
    health \
    libnetdata \
    plugins \
    registry \
    streaming \
    webserver \
	$(NULL)

AM_CFLAGS = \
	$(OPTIONAL_MATH_CFLAGS) \
	$(OPTIONAL_NFACCT_CLFAGS) \
	$(OPTIONAL_ZLIB_CFLAGS) \
	$(OPTIONAL_UUID_CFLAGS) \
	$(OPTIONAL_LIBCAP_LIBS) \
	$(OPTIONAL_IPMIMONITORING_CFLAGS) \
	$(NULL)

sbin_PROGRAMS =
dist_cache_DATA = .keep
dist_varlib_DATA = .keep
dist_registry_DATA = .keep
dist_log_DATA = .keep
plugins_PROGRAMS =

LIBNETDATA_FILES = \
    libnetdata/adaptive_resortable_list.c \
    libnetdata/adaptive_resortable_list.h \
    libnetdata/appconfig.c \
    libnetdata/appconfig.h \
    libnetdata/avl.c \
    libnetdata/avl.h \
    libnetdata/clocks.c \
    libnetdata/clocks.h \
    libnetdata/common.c \
    libnetdata/dictionary.c \
    libnetdata/dictionary.h \
    libnetdata/eval.c \
    libnetdata/eval.h \
    libnetdata/inlined.h \
    libnetdata/libnetdata.h \
    libnetdata/locks.c \
    libnetdata/locks.h \
    libnetdata/log.c \
    libnetdata/log.h \
    libnetdata/popen.c \
    libnetdata/popen.h \
    libnetdata/procfile.c \
    libnetdata/procfile.h \
    libnetdata/os.c \
    libnetdata/os.h \
    libnetdata/simple_pattern.c \
    libnetdata/simple_pattern.h \
    libnetdata/socket.c \
    libnetdata/socket.h \
    libnetdata/statistical.c \
    libnetdata/statistical.h \
    libnetdata/storage_number.c \
    libnetdata/storage_number.h \
    libnetdata/threads.c \
    libnetdata/threads.h \
    libnetdata/web_buffer.c \
    libnetdata/web_buffer.h \
	libnetdata/url.c \
	libnetdata/url.h \
    $(NULL)

APPS_PLUGIN_FILES = \
	plugins/apps.plugin/apps_plugin.c \
	$(LIBNETDATA_FILES) \
    $(NULL)

CHECKS_PLUGIN_FILES = \
    plugins/checks.plugin/plugin_checks.c \
    plugins/checks.plugin/plugin_checks.h \
    $(NULL)

FREEBSD_PLUGIN_FILES = \
	plugins/freebsd.plugin/plugin_freebsd.c \
	plugins/freebsd.plugin/plugin_freebsd.h \
	plugins/freebsd.plugin/freebsd_sysctl.c \
	plugins/freebsd.plugin/freebsd_getmntinfo.c \
	plugins/freebsd.plugin/freebsd_getifaddrs.c \
	plugins/freebsd.plugin/freebsd_devstat.c \
	plugins/freebsd.plugin/freebsd_kstat_zfs.c \
	plugins/freebsd.plugin/freebsd_ipfw.c \
	plugins/linux-proc.plugin/zfs_common.c \
	plugins/linux-proc.plugin/zfs_common.h \
    $(NULL)

HEALTH_PLUGIN_FILES = \
	health/health.c \
	health/health.h \
	health/health_config.c \
	health/health_json.c \
	health/health_log.c \
    $(NULL)

IDLEJITTER_PLUGIN_FILES = \
	plugins/idlejitter.plugin/plugin_idlejitter.c \
	plugins/idlejitter.plugin/plugin_idlejitter.h \
    $(NULL)

CGROUPS_PLUGIN_FILES = \
    plugins/linux-cgroups.plugin/sys_fs_cgroup.c \
    plugins/linux-cgroups.plugin/sys_fs_cgroup.h \
    $(NULL)

CGROUP_NETWORK_FILES = \
    plugins/linux-cgroups.plugin/cgroup-network.c \
    $(LIBNETDATA_FILES) \
    $(NULL)

DISKSPACE_PLUGIN_FILES = \
    plugins/linux-diskspace.plugin/plugin_diskspace.h \
    plugins/linux-diskspace.plugin/plugin_diskspace.c \
    $(NULL)

FREEIPMI_PLUGIN_FILES = \
    plugins/linux-freeipmi.plugin/freeipmi_plugin.c \
    $(LIBNETDATA_FILES) \
    $(NULL)

NFACCT_PLUGIN_FILES = \
	plugins/linux-nfacct.plugin/plugin_nfacct.c \
	plugins/linux-nfacct.plugin/plugin_nfacct.h \
    $(NULL)

PROC_PLUGIN_FILES = \
	plugins/linux-proc.plugin/ipc.c \
	plugins/linux-proc.plugin/plugin_proc.c \
	plugins/linux-proc.plugin/plugin_proc.h \
	plugins/linux-proc.plugin/proc_diskstats.c \
	plugins/linux-proc.plugin/proc_interrupts.c \
	plugins/linux-proc.plugin/proc_softirqs.c \
	plugins/linux-proc.plugin/proc_loadavg.c \
	plugins/linux-proc.plugin/proc_meminfo.c \
	plugins/linux-proc.plugin/proc_net_dev.c \
	plugins/linux-proc.plugin/proc_net_ip_vs_stats.c \
	plugins/linux-proc.plugin/proc_net_netstat.c \
	plugins/linux-proc.plugin/proc_net_rpc_nfs.c \
	plugins/linux-proc.plugin/proc_net_rpc_nfsd.c \
	plugins/linux-proc.plugin/proc_net_snmp.c \
	plugins/linux-proc.plugin/proc_net_snmp6.c \
	plugins/linux-proc.plugin/proc_net_sctp_snmp.c \
	plugins/linux-proc.plugin/proc_net_sockstat.c \
	plugins/linux-proc.plugin/proc_net_sockstat6.c \
	plugins/linux-proc.plugin/proc_net_softnet_stat.c \
	plugins/linux-proc.plugin/proc_net_stat_conntrack.c \
	plugins/linux-proc.plugin/proc_net_stat_synproxy.c \
	plugins/linux-proc.plugin/proc_self_mountinfo.c \
	plugins/linux-proc.plugin/proc_self_mountinfo.h \
	plugins/linux-proc.plugin/zfs_common.c \
	plugins/linux-proc.plugin/zfs_common.h \
	plugins/linux-proc.plugin/proc_spl_kstat_zfs.c \
	plugins/linux-proc.plugin/proc_stat.c \
	plugins/linux-proc.plugin/proc_sys_kernel_random_entropy_avail.c \
	plugins/linux-proc.plugin/proc_vmstat.c \
	plugins/linux-proc.plugin/proc_uptime.c \
	plugins/linux-proc.plugin/sys_kernel_mm_ksm.c \
	plugins/linux-proc.plugin/sys_devices_system_edac_mc.c \
	plugins/linux-proc.plugin/sys_devices_system_node.c \
	plugins/linux-proc.plugin/sys_fs_btrfs.c \
    $(NULL)

TC_PLUGIN_FILES = \
	plugins/linux-tc.plugin/plugin_tc.c \
	plugins/linux-tc.plugin/plugin_tc.h \
    $(NULL)

MACOS_PLUGIN_FILES = \
	plugins/macos.plugin/plugin_macos.c \
	plugins/macos.plugin/plugin_macos.h \
	plugins/macos.plugin/macos_sysctl.c \
	plugins/macos.plugin/macos_mach_smi.c \
	plugins/macos.plugin/macos_fw.c \
    $(NULL)

PLUGINSD_PLUGIN_FILES = \
	plugins/plugins.d.plugin/plugins_d.c \
	plugins/plugins.d.plugin/plugins_d.h \
    $(NULL)

RRD_PLUGIN_FILES = \
    database/rrdcalc.c \
    database/rrdcalc.h \
    database/rrdcalctemplate.c \
    database/rrdcalctemplate.h \
    database/rrddim.c \
    database/rrddimvar.c \
    database/rrddimvar.h \
    database/rrdfamily.c \
    database/rrdhost.c \
    database/rrd.c \
    database/rrd.h \
    database/rrdset.c \
    database/rrdsetvar.c \
    database/rrdsetvar.h \
    database/rrdvar.c \
    database/rrdvar.h \
    $(NULL)

API_PLUGIN_FILES = \
    api/rrd2json.c \
    api/rrd2json.h \
    api/web_api_v1.c \
    api/web_api_v1.h \
    api/web_buffer_svg.c \
    api/web_buffer_svg.h \
    $(NULL)

STREAMING_PLUGIN_FILES = \
    streaming/rrdpush.c \
    streaming/rrdpush.h \
    $(NULL)

REGISTRY_PLUGIN_FILES = \
	registry/registry.c \
	registry/registry.h \
	registry/registry_db.c \
	registry/registry_init.c \
	registry/registry_internals.c \
	registry/registry_internals.h \
	registry/registry_log.c \
	registry/registry_machine.c \
	registry/registry_machine.h \
	registry/registry_person.c \
	registry/registry_person.h \
	registry/registry_url.c \
	registry/registry_url.h \
    $(NULL)

STATSD_PLUGIN_FILES = \
	plugins/statsd.plugin/statsd.c \
	plugins/statsd.plugin/statsd.h \
    $(NULL)

WEB_PLGUGIN_FILES = \
    webserver/web_client.c \
    webserver/web_client.h \
    webserver/web_server.c \
    webserver/web_server.h \
    $(NULL)

BACKENDS_PLUGIN_FILES = \
    backends/backends.c \
    backends/backends.h \
    backends/graphite/graphite.c \
    backends/graphite/graphite.h \
    backends/json/json.c \
    backends/json/json.h \
    backends/opentsdb/opentsdb.c \
    backends/opentsdb/opentsdb.h \
    backends/prometheus/backend_prometheus.c \
    backends/prometheus/backend_prometheus.h \
    $(NULL)

WEB_PLUGIN_FILES = \
    webserver/web_client.c \
    webserver/web_client.h \
    webserver/web_server.c \
    webserver/web_server.h \
    $(NULL)

NETDATA_FILES = \
    plugins/all.h \
	common.c \
	common.h \
	daemon.c \
	daemon.h \
	global_statistics.c \
	global_statistics.h \
	main.c \
	main.h \
	signals.c \
	signals.h \
	unit_test.c \
	unit_test.h \
	$(LIBNETDATA_FILES) \
	$(API_PLUGIN_FILES) \
	$(BACKENDS_PLUGIN_FILES) \
	$(CHECKS_PLUGIN_FILES) \
	$(HEALTH_PLUGIN_FILES) \
	$(IDLEJITTER_PLUGIN_FILES) \
	$(PLUGINSD_PLUGIN_FILES) \
	$(REGISTRY_PLUGIN_FILES) \
	$(RRD_PLUGIN_FILES) \
	$(STREAMING_PLUGIN_FILES) \
	$(STATSD_PLUGIN_FILES) \
	$(WEB_PLUGIN_FILES) \
	$(NULL)

if FREEBSD
    NETDATA_FILES += \
        $(FREEBSD_PLUGIN_FILES) \
        $(NULL)
endif

if MACOS
    NETDATA_FILES += \
        $(MACOS_PLUGIN_FILES) \
        $(NULL)
endif

if LINUX
    NETDATA_FILES += \
        $(CGROUPS_PLUGIN_FILES) \
        $(DISKSPACE_PLUGIN_FILES) \
        $(NFACCT_PLUGIN_FILES) \
        $(PROC_PLUGIN_FILES) \
        $(TC_PLUGIN_FILES) \
        $(NULL)

endif

NETDATA_COMMON_LIBS = \
	$(OPTIONAL_MATH_LIBS) \
	$(OPTIONAL_ZLIB_LIBS) \
	$(OPTIONAL_UUID_LIBS) \
	$(NULL)


sbin_PROGRAMS += netdata
netdata_SOURCES = ../config.h $(NETDATA_FILES)
netdata_LDADD = \
	$(NETDATA_COMMON_LIBS) \
	$(OPTIONAL_NFACCT_LIBS) \
	$(NULL)

if ENABLE_PLUGIN_APPS
    plugins_PROGRAMS += apps.plugin
    apps_plugin_SOURCES = ../config.h $(APPS_PLUGIN_FILES)
    apps_plugin_LDADD = \
    	$(NETDATA_COMMON_LIBS) \
        $(OPTIONAL_LIBCAP_LIBS) \
        $(NULL)
endif

if ENABLE_PLUGIN_CGROUP_NETWORK
    plugins_PROGRAMS += cgroup-network
    cgroup_network_SOURCES = ../config.h $(CGROUP_NETWORK_FILES)
    cgroup_network_LDADD = \
    	$(NETDATA_COMMON_LIBS) \
        $(NULL)
endif

if ENABLE_PLUGIN_FREEIPMI
    plugins_PROGRAMS += freeipmi.plugin
    freeipmi_plugin_SOURCES = ../config.h $(FREEIPMI_PLUGIN_FILES)
    freeipmi_plugin_LDADD = \
    	$(NETDATA_COMMON_LIBS) \
        $(OPTIONAL_IPMIMONITORING_LIBS) \
        $(NULL)
endif

