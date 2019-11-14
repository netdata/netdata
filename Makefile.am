# SPDX-License-Identifier: GPL-3.0-or-later

AUTOMAKE_OPTIONS = foreign subdir-objects 1.11
ACLOCAL_AMFLAGS = -I build/m4

MAINTAINERCLEANFILES = \
    config.log config.status \
    $(srcdir)/Makefile.in \
    $(srcdir)/config.h.in $(srcdir)/config.h.in~ $(srcdir)/configure \
    $(srcdir)/install-sh $(srcdir)/ltmain.sh $(srcdir)/missing \
    $(srcdir)/compile $(srcdir)/depcomp $(srcdir)/aclocal.m4 \
    $(srcdir)/config.guess $(srcdir)/config.sub \
    $(srcdir)/m4/ltsugar.m4 $(srcdir)/m4/libtool.m4 \
    $(srcdir)/m4/ltversion.m4 $(srcdir)/m4/lt~obsolete.m4 \
    $(srcdir)/m4/ltoptions.m4 \
    $(srcdir)/pkcs11-helper.spec $(srcdir)/config-w32-vc.h

CLEANFILES = \
    $(srcdir)/$(distdir).tar.gz

EXTRA_DIST = \
    .gitignore \
    .csslintrc \
    .eslintignore \
    .eslintrc \
    .github/CODEOWNERS \
    build/m4/jemalloc.m4 \
    build/m4/ax_c___atomic.m4 \
    build/m4/ax_check_enable_debug.m4 \
    build/m4/ax_c_mallinfo.m4 \
    build/m4/ax_gcc_func_attribute.m4 \
    build/m4/ax_check_compile_flag.m4 \
    build/m4/ax_c_statement_expressions.m4 \
    build/m4/ax_pthread.m4 \
    build/m4/ax_c_lto.m4 \
    build/m4/ax_c_mallopt.m4 \
    build/m4/tcmalloc.m4 \
    build/m4/ax_c__generic.m4 \
    README.md \
    CONTRIBUTORS.md \
    CODE_OF_CONDUCT.md \
    LICENSE \
    REDISTRIBUTED.md \
    CONTRIBUTING.md \
    $(NULL)

SUBDIRS = \
    diagrams \
    system \
    tests \
    $(NULL)

dist_noinst_DATA = \
    CHANGELOG.md \
    cppcheck.sh \
    configs.signatures \
    contrib \
    netdata.cppcheck \
    netdata.spec \
    package.json \
    docs \
    packaging/version \
    packaging/go.d.version \
    packaging/go.d.checksums \
    packaging/installer/README.md \
    packaging/installer/UNINSTALL.md \
    packaging/installer/UPDATE.md \
    netlify.toml \
    $(NULL)

# until integrated within build
# should be proper init.d/openrc/systemd usable
dist_noinst_SCRIPTS = \
    coverity-scan.sh \
    packaging/installer/netdata-updater.sh \
    packaging/installer/netdata-uninstaller.sh \
    packaging/installer/kickstart.sh \
    packaging/installer/kickstart-static64.sh \
    packaging/installer/functions.sh \
    netdata-installer.sh \
    docs/generator/buildhtml.sh \
    docs/generator/buildyaml.sh \
    docs/generator/checklinks.sh \
    docs/generator/requirements.txt \
    docs/generator/runtime.txt \
    $(NULL)

# -----------------------------------------------------------------------------
# Compile netdata binaries

SUBDIRS += \
    backends \
    collectors \
    daemon \
    database \
    health \
    libnetdata \
    registry \
    streaming \
    web \
    $(NULL)


AM_CFLAGS = \
    $(OPTIONAL_MATH_CFLAGS) \
    $(OPTIONAL_NFACCT_CFLAGS) \
    $(OPTIONAL_ZLIB_CFLAGS) \
    $(OPTIONAL_UUID_CFLAGS) \
    $(OPTIONAL_LIBCAP_LIBS) \
    $(OPTIONAL_IPMIMONITORING_CFLAGS) \
    $(OPTIONAL_CUPS_CFLAGS) \
    $(OPTIONAL_XENSTAT_CFLAGS) \
    $(NULL)

sbin_PROGRAMS =
dist_cache_DATA = packaging/installer/.keep
dist_varlib_DATA = packaging/installer/.keep
dist_registry_DATA = packaging/installer/.keep
dist_log_DATA = packaging/installer/.keep
plugins_PROGRAMS =

LIBNETDATA_FILES = \
    libnetdata/adaptive_resortable_list/adaptive_resortable_list.c \
    libnetdata/adaptive_resortable_list/adaptive_resortable_list.h \
    libnetdata/config/appconfig.c \
    libnetdata/config/appconfig.h \
    libnetdata/avl/avl.c \
    libnetdata/avl/avl.h \
    libnetdata/buffer/buffer.c \
    libnetdata/buffer/buffer.h \
    libnetdata/clocks/clocks.c \
    libnetdata/clocks/clocks.h \
    libnetdata/dictionary/dictionary.c \
    libnetdata/dictionary/dictionary.h \
    libnetdata/eval/eval.c \
    libnetdata/eval/eval.h \
    libnetdata/inlined.h \
    libnetdata/libnetdata.c \
    libnetdata/libnetdata.h \
    libnetdata/required_dummies.h \
    libnetdata/locks/locks.c \
    libnetdata/locks/locks.h \
    libnetdata/log/log.c \
    libnetdata/log/log.h \
    libnetdata/popen/popen.c \
    libnetdata/popen/popen.h \
    libnetdata/procfile/procfile.c \
    libnetdata/procfile/procfile.h \
    libnetdata/os.c \
    libnetdata/os.h \
    libnetdata/simple_pattern/simple_pattern.c \
    libnetdata/simple_pattern/simple_pattern.h \
    libnetdata/socket/socket.c \
    libnetdata/socket/socket.h \
    libnetdata/socket/security.c \
    libnetdata/socket/security.h \
    libnetdata/statistical/statistical.c \
    libnetdata/statistical/statistical.h \
    libnetdata/storage_number/storage_number.c \
    libnetdata/storage_number/storage_number.h \
    libnetdata/threads/threads.c \
    libnetdata/threads/threads.h \
    libnetdata/url/url.c \
    libnetdata/url/url.h \
    libnetdata/json/json.c \
    libnetdata/json/json.h \
    libnetdata/json/jsmn.c \
    libnetdata/json/jsmn.h \
    libnetdata/health/health.c \
    libnetdata/health/health.h \
    libnetdata/string/utf8.h \
    $(NULL)

APPS_PLUGIN_FILES = \
    collectors/apps.plugin/apps_plugin.c \
    $(LIBNETDATA_FILES) \
    $(NULL)

CHECKS_PLUGIN_FILES = \
    collectors/checks.plugin/plugin_checks.c \
    collectors/checks.plugin/plugin_checks.h \
    $(NULL)

FREEBSD_PLUGIN_FILES = \
    collectors/freebsd.plugin/plugin_freebsd.c \
    collectors/freebsd.plugin/plugin_freebsd.h \
    collectors/freebsd.plugin/freebsd_sysctl.c \
    collectors/freebsd.plugin/freebsd_getmntinfo.c \
    collectors/freebsd.plugin/freebsd_getifaddrs.c \
    collectors/freebsd.plugin/freebsd_devstat.c \
    collectors/freebsd.plugin/freebsd_kstat_zfs.c \
    collectors/freebsd.plugin/freebsd_ipfw.c \
    collectors/proc.plugin/zfs_common.c \
    collectors/proc.plugin/zfs_common.h \
    $(NULL)

HEALTH_PLUGIN_FILES = \
    health/health.c \
    health/health.h \
    health/health_config.c \
    health/health_json.c \
    health/health_log.c \
    $(NULL)

IDLEJITTER_PLUGIN_FILES = \
    collectors/idlejitter.plugin/plugin_idlejitter.c \
    collectors/idlejitter.plugin/plugin_idlejitter.h \
    $(NULL)

CGROUPS_PLUGIN_FILES = \
    collectors/cgroups.plugin/sys_fs_cgroup.c \
    collectors/cgroups.plugin/sys_fs_cgroup.h \
    $(NULL)

CGROUP_NETWORK_FILES = \
    collectors/cgroups.plugin/cgroup-network.c \
    $(LIBNETDATA_FILES) \
    $(NULL)

DISKSPACE_PLUGIN_FILES = \
    collectors/diskspace.plugin/plugin_diskspace.h \
    collectors/diskspace.plugin/plugin_diskspace.c \
    $(NULL)

FREEIPMI_PLUGIN_FILES = \
    collectors/freeipmi.plugin/freeipmi_plugin.c \
    $(LIBNETDATA_FILES) \
    $(NULL)

CUPS_PLUGIN_FILES = \
    collectors/cups.plugin/cups_plugin.c \
    $(LIBNETDATA_FILES) \
    $(NULL)

NFACCT_PLUGIN_FILES = \
    collectors/nfacct.plugin/plugin_nfacct.c \
    $(LIBNETDATA_FILES) \
    $(NULL)

SLABINFO_PLUGIN_FILES = \
    collectors/slabinfo.plugin/slabinfo.c \
    $(LIBNETDATA_FILES)
    $(NULL)

XENSTAT_PLUGIN_FILES = \
    collectors/xenstat.plugin/xenstat_plugin.c \
    $(LIBNETDATA_FILES) \
    $(NULL)

PERF_PLUGIN_FILES = \
    collectors/perf.plugin/perf_plugin.c \
    $(LIBNETDATA_FILES) \
    $(NULL)

PROC_PLUGIN_FILES = \
    collectors/proc.plugin/ipc.c \
    collectors/proc.plugin/plugin_proc.c \
    collectors/proc.plugin/plugin_proc.h \
    collectors/proc.plugin/proc_diskstats.c \
    collectors/proc.plugin/proc_mdstat.c \
    collectors/proc.plugin/proc_interrupts.c \
    collectors/proc.plugin/proc_softirqs.c \
    collectors/proc.plugin/proc_loadavg.c \
    collectors/proc.plugin/proc_meminfo.c \
    collectors/proc.plugin/proc_pagetypeinfo.c \
    collectors/proc.plugin/proc_net_dev.c \
    collectors/proc.plugin/proc_net_ip_vs_stats.c \
    collectors/proc.plugin/proc_net_netstat.c \
    collectors/proc.plugin/proc_net_rpc_nfs.c \
    collectors/proc.plugin/proc_net_rpc_nfsd.c \
    collectors/proc.plugin/proc_net_snmp.c \
    collectors/proc.plugin/proc_net_snmp6.c \
    collectors/proc.plugin/proc_net_sctp_snmp.c \
    collectors/proc.plugin/proc_net_sockstat.c \
    collectors/proc.plugin/proc_net_sockstat6.c \
    collectors/proc.plugin/proc_net_softnet_stat.c \
    collectors/proc.plugin/proc_net_stat_conntrack.c \
    collectors/proc.plugin/proc_net_stat_synproxy.c \
    collectors/proc.plugin/proc_self_mountinfo.c \
    collectors/proc.plugin/proc_self_mountinfo.h \
    collectors/proc.plugin/zfs_common.c \
    collectors/proc.plugin/zfs_common.h \
    collectors/proc.plugin/proc_spl_kstat_zfs.c \
    collectors/proc.plugin/proc_stat.c \
    collectors/proc.plugin/proc_sys_kernel_random_entropy_avail.c \
    collectors/proc.plugin/proc_vmstat.c \
    collectors/proc.plugin/proc_uptime.c \
    collectors/proc.plugin/sys_kernel_mm_ksm.c \
    collectors/proc.plugin/sys_block_zram.c \
    collectors/proc.plugin/sys_devices_system_edac_mc.c \
    collectors/proc.plugin/sys_devices_system_node.c \
    collectors/proc.plugin/sys_fs_btrfs.c \
    collectors/proc.plugin/sys_class_power_supply.c \
    $(NULL)

TC_PLUGIN_FILES = \
    collectors/tc.plugin/plugin_tc.c \
    collectors/tc.plugin/plugin_tc.h \
    $(NULL)

MACOS_PLUGIN_FILES = \
    collectors/macos.plugin/plugin_macos.c \
    collectors/macos.plugin/plugin_macos.h \
    collectors/macos.plugin/macos_sysctl.c \
    collectors/macos.plugin/macos_mach_smi.c \
    collectors/macos.plugin/macos_fw.c \
    $(NULL)

PLUGINSD_PLUGIN_FILES = \
    collectors/plugins.d/plugins_d.c \
    collectors/plugins.d/plugins_d.h \
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

if ENABLE_DBENGINE
    RRD_PLUGIN_FILES += \
        database/engine/rrdengine.c \
        database/engine/rrdengine.h \
        database/engine/rrddiskprotocol.h \
        database/engine/datafile.c \
        database/engine/datafile.h \
        database/engine/journalfile.c \
        database/engine/journalfile.h \
        database/engine/rrdenginelib.c \
        database/engine/rrdenginelib.h \
        database/engine/rrdengineapi.c \
        database/engine/rrdengineapi.h \
        database/engine/pagecache.c \
        database/engine/pagecache.h \
        database/engine/rrdenglocking.c \
        database/engine/rrdenglocking.h \
        $(NULL)
endif

API_PLUGIN_FILES = \
    web/api/badges/web_buffer_svg.c \
    web/api/badges/web_buffer_svg.h \
    web/api/exporters/allmetrics.c \
    web/api/exporters/allmetrics.h \
    web/api/exporters/shell/allmetrics_shell.c \
    web/api/exporters/shell/allmetrics_shell.h \
    web/api/queries/average/average.c \
    web/api/queries/average/average.h \
    web/api/queries/des/des.c \
    web/api/queries/des/des.h \
    web/api/queries/incremental_sum/incremental_sum.c \
    web/api/queries/incremental_sum/incremental_sum.h \
    web/api/queries/max/max.c \
    web/api/queries/max/max.h \
    web/api/queries/median/median.c \
    web/api/queries/median/median.h \
    web/api/queries/min/min.c \
    web/api/queries/min/min.h \
    web/api/queries/query.c \
    web/api/queries/query.h \
    web/api/queries/rrdr.c \
    web/api/queries/rrdr.h \
    web/api/queries/ses/ses.c \
    web/api/queries/ses/ses.h \
    web/api/queries/stddev/stddev.c \
    web/api/queries/stddev/stddev.h \
    web/api/queries/sum/sum.c \
    web/api/queries/sum/sum.h \
    web/api/formatters/rrd2json.c \
    web/api/formatters/rrd2json.h \
    web/api/formatters/csv/csv.c \
    web/api/formatters/csv/csv.h \
    web/api/formatters/json/json.c \
    web/api/formatters/json/json.h \
    web/api/formatters/ssv/ssv.c \
    web/api/formatters/ssv/ssv.h \
    web/api/formatters/value/value.c \
    web/api/formatters/value/value.h \
    web/api/formatters/json_wrapper.c \
    web/api/formatters/json_wrapper.h \
    web/api/formatters/charts2json.c \
    web/api/formatters/charts2json.h \
    web/api/formatters/rrdset2json.c \
    web/api/formatters/rrdset2json.h \
    web/api/health/health_cmdapi.c \
    web/api/health/health_cmdapi.h \
    web/api/web_api_v1.c \
    web/api/web_api_v1.h \
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
    collectors/statsd.plugin/statsd.c \
    collectors/statsd.plugin/statsd.h \
    $(NULL)

WEB_PLUGIN_FILES = \
    web/server/web_client.c \
    web/server/web_client.h \
    web/server/web_server.c \
    web/server/web_server.h \
    web/server/web_client_cache.c \
    web/server/web_client_cache.h \
    web/server/static/static-threaded.c \
    web/server/static/static-threaded.h \
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

KINESIS_BACKEND_FILES = \
    backends/aws_kinesis/aws_kinesis.c \
    backends/aws_kinesis/aws_kinesis.h \
    backends/aws_kinesis/aws_kinesis_put_record.cc \
    backends/aws_kinesis/aws_kinesis_put_record.h \
    $(NULL)

PROMETHEUS_REMOTE_WRITE_BACKEND_FILES = \
    backends/prometheus/remote_write/remote_write.cc \
    backends/prometheus/remote_write/remote_write.h \
    backends/prometheus/remote_write/remote_write.proto \
    $(NULL)

MONGODB_BACKEND_FILES = \
    backends/mongodb/mongodb.c \
    backends/mongodb/mongodb.h \
    $(NULL)

DAEMON_FILES = \
    daemon/common.c \
    daemon/common.h \
    daemon/daemon.c \
    daemon/daemon.h \
    daemon/global_statistics.c \
    daemon/global_statistics.h \
    daemon/main.c \
    daemon/main.h \
    daemon/signals.c \
    daemon/signals.h \
    daemon/unit_test.c \
    daemon/unit_test.h \
    $(NULL)

NETDATA_FILES = \
    collectors/all.h \
    $(DAEMON_FILES) \
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
        $(PROC_PLUGIN_FILES) \
        $(TC_PLUGIN_FILES) \
        $(NULL)

endif

NETDATA_COMMON_LIBS = \
    $(OPTIONAL_MATH_LIBS) \
    $(OPTIONAL_ZLIB_LIBS) \
    $(OPTIONAL_SSL_LIBS) \
    $(OPTIONAL_UUID_LIBS) \
    $(OPTIONAL_UV_LIBS) \
    $(OPTIONAL_LZ4_LIBS) \
    $(OPTIONAL_JUDY_LIBS) \
    $(OPTIONAL_SSL_LIBS) \
    $(OPTIONAL_JSONC_LIBS) \
    $(NULL)

sbin_PROGRAMS += netdata
netdata_SOURCES = $(NETDATA_FILES)
netdata_LDADD = \
    $(NETDATA_COMMON_LIBS) \
    $(NULL)
if ENABLE_CXX_LINKER
    netdata_LINK = $(CXXLD) $(CXXFLAGS) $(LDFLAGS) -o $@
else
    netdata_LINK = $(CCLD) $(CFLAGS) $(LDFLAGS) -o $@
endif

if ENABLE_PLUGIN_APPS
    plugins_PROGRAMS += apps.plugin
    apps_plugin_SOURCES = $(APPS_PLUGIN_FILES)
    apps_plugin_LDADD = \
        $(NETDATA_COMMON_LIBS) \
        $(OPTIONAL_LIBCAP_LIBS) \
        $(NULL)
endif

if ENABLE_PLUGIN_CGROUP_NETWORK
    plugins_PROGRAMS += cgroup-network
    cgroup_network_SOURCES = $(CGROUP_NETWORK_FILES)
    cgroup_network_LDADD = \
        $(NETDATA_COMMON_LIBS) \
        $(NULL)
endif

if ENABLE_PLUGIN_FREEIPMI
    plugins_PROGRAMS += freeipmi.plugin
    freeipmi_plugin_SOURCES = $(FREEIPMI_PLUGIN_FILES)
    freeipmi_plugin_LDADD = \
        $(NETDATA_COMMON_LIBS) \
        $(OPTIONAL_IPMIMONITORING_LIBS) \
        $(NULL)
endif

if ENABLE_PLUGIN_CUPS
    plugins_PROGRAMS += cups.plugin
    cups_plugin_SOURCES = $(CUPS_PLUGIN_FILES)
    cups_plugin_LDADD = \
        $(NETDATA_COMMON_LIBS) \
        $(OPTIONAL_CUPS_LIBS) \
        $(NULL)
endif

if ENABLE_PLUGIN_NFACCT
    plugins_PROGRAMS += nfacct.plugin
    nfacct_plugin_SOURCES = $(NFACCT_PLUGIN_FILES)
    nfacct_plugin_LDADD = \
        $(NETDATA_COMMON_LIBS) \
        $(OPTIONAL_NFACCT_LIBS) \
        $(NULL)
endif

if ENABLE_PLUGIN_XENSTAT
    plugins_PROGRAMS += xenstat.plugin
    xenstat_plugin_SOURCES = $(XENSTAT_PLUGIN_FILES)
    xenstat_plugin_LDADD = \
        $(NETDATA_COMMON_LIBS) \
        $(OPTIONAL_XENSTAT_LIBS) \
        $(NULL)
endif

if ENABLE_PLUGIN_PERF
    plugins_PROGRAMS += perf.plugin
    perf_plugin_SOURCES = $(PERF_PLUGIN_FILES)
    perf_plugin_LDADD = \
        $(NETDATA_COMMON_LIBS) \
        $(NULL)
endif

if ENABLE_PLUGIN_SLABINFO
    plugins_PROGRAMS += slabinfo.plugin
    slabinfo_plugin_SOURCES = $(SLABINFO_PLUGIN_FILES)
    slabinfo_plugin_LDADD = \
        $(NETDATA_COMMON_LIBS) \
        $(NULL)
endif

if ENABLE_BACKEND_KINESIS
    netdata_SOURCES += $(KINESIS_BACKEND_FILES)
    netdata_LDADD += $(OPTIONAL_KINESIS_LIBS)
endif

if ENABLE_BACKEND_PROMETHEUS_REMOTE_WRITE
    netdata_SOURCES += $(PROMETHEUS_REMOTE_WRITE_BACKEND_FILES)
    netdata_LDADD += $(OPTIONAL_PROMETHEUS_REMOTE_WRITE_LIBS)
    BUILT_SOURCES = \
        backends/prometheus/remote_write/remote_write.pb.cc \
        backends/prometheus/remote_write/remote_write.pb.h \
        $(NULL)
    nodist_netdata_SOURCES = $(BUILT_SOURCES)

backends/prometheus/remote_write/remote_write.pb.cc \
backends/prometheus/remote_write/remote_write.pb.h: backends/prometheus/remote_write/remote_write.proto
	$(PROTOC) --proto_path=$(srcdir) --cpp_out=$(builddir) $^

endif

if ENABLE_BACKEND_MONGODB
    netdata_SOURCES += $(MONGODB_BACKEND_FILES)
    netdata_LDADD += $(OPTIONAL_MONGOC_LIBS)
endif

if ENABLE_UNITTESTS
    check_PROGRAMS = \
        libnetdata/tests/str2ld_testdriver \
        libnetdata/storage_number/tests/storage_number_testdriver \
        web/api/tests/web_api_testdriver \
        $(NULL)

    TESTS = $(check_PROGRAMS)

    web_api_tests_web_api_testdriver_LDFLAGS = \
        -Wl,--wrap=rrdhost_find_by_hostname \
        -Wl,--wrap=finished_web_request_statistics \
        -Wl,--wrap=config_get \
        -Wl,--wrap=web_client_api_request_v1 \
        -Wl,--wrap=rrdhost_find_by_guid \
        -Wl,--wrap=rrdset_find_byname \
        -Wl,--wrap=rrdset_find \
        -Wl,--wrap=rrdpush_receiver_thread_spawn \
        -Wl,--wrap=debug_int \
        -Wl,--wrap=error_int \
        -Wl,--wrap=info_int \
        $(TEST_LDFLAGS) \
        $(NULL)
    web_api_tests_web_api_testdriver_SOURCES = \
        web/api/tests/web_api.c \
        web/server/web_client.c \
        $(LIBNETDATA_FILES) \
        $(NULL)
    web_api_tests_web_api_testdriver_LDADD = $(NETDATA_COMMON_LIBS) $(TEST_LIBS)

    libnetdata_tests_str2ld_testdriver_SOURCES = \
        libnetdata/tests/test_str2ld.c \
        $(LIBNETDATA_FILES) \
        $(NULL)
    libnetdata_tests_str2ld_testdriver_LDADD = $(NETDATA_COMMON_LIBS) $(TEST_LIBS)

    libnetdata_storage_number_tests_storage_number_testdriver_SOURCES = \
        libnetdata/storage_number/tests/test_storage_number.c \
        $(LIBNETDATA_FILES) \
        $(NULL)
    libnetdata_storage_number_tests_storage_number_testdriver_LDADD = $(NETDATA_COMMON_LIBS) $(TEST_LIBS)
endif
