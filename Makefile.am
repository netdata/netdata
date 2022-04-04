# SPDX-License-Identifier: GPL-3.0-or-later

AUTOMAKE_OPTIONS = foreign subdir-objects 1.11
ACLOCAL_AMFLAGS = -I build/m4

nodist_netdata_SOURCES=$(NULL)
BUILT_SOURCES=$(NULL)

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
    ml/kmeans/dlib \
    README.md \
    LICENSE \
    REDISTRIBUTED.md \
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
    docs \
    mqtt_websockets \
    netdata.cppcheck \
    netdata.spec \
    packaging/bundle-ebpf.sh \
    packaging/bundle-ebpf-co-re.sh \
    packaging/bundle-judy.sh \
    packaging/bundle-libbpf.sh \
    packaging/check-kernel-config.sh \
    packaging/ebpf.checksums \
    packaging/ebpf.version \
    packaging/go.d.checksums \
    packaging/go.d.version \
    packaging/installer/README.md \
    packaging/installer/UNINSTALL.md \
    packaging/installer/UPDATE.md \
    packaging/jsonc.checksums \
    packaging/jsonc.version \
    packaging/judy.checksums \
    packaging/judy.version \
    packaging/libbpf.checksums \
    packaging/libbpf.version \
    packaging/protobuf.checksums \
    packaging/protobuf.version \
    packaging/version \
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
    netdata-installer.sh
    $(NULL)

# -----------------------------------------------------------------------------
# Compile netdata binaries

SUBDIRS += \
    collectors \
    daemon \
    database \
    exporting \
    health \
    libnetdata \
    registry \
    streaming \
    web \
    claim \
    parser \
    spawn \
    ml \
    $(NULL)

AM_CFLAGS = \
    $(OPTIONAL_MATH_CFLAGS) \
    $(OPTIONAL_NFACCT_CFLAGS) \
    $(OPTIONAL_ZLIB_CFLAGS) \
    $(OPTIONAL_UUID_CFLAGS) \
    $(OPTIONAL_MQTT_CFLAGS) \
    $(OPTIONAL_LIBCAP_LIBS) \
    $(OPTIONAL_IPMIMONITORING_CFLAGS) \
    $(OPTIONAL_CUPS_CFLAGS) \
    $(OPTIONAL_XENSTAT_CFLAGS) \
    $(OPTIONAL_BPF_CFLAGS) \
    $(NULL)

sbin_PROGRAMS =
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
    libnetdata/circular_buffer/circular_buffer.c \
    libnetdata/circular_buffer/circular_buffer.h \
    libnetdata/clocks/clocks.c \
    libnetdata/clocks/clocks.h \
    libnetdata/completion/completion.c \
    libnetdata/completion/completion.h \
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

if ENABLE_PLUGIN_EBPF
    LIBNETDATA_FILES += \
        libnetdata/ebpf/ebpf.c \
        libnetdata/ebpf/ebpf.h \
        $(NULL)
endif

APPS_PLUGIN_FILES = \
    collectors/apps.plugin/apps_plugin.c \
    $(LIBNETDATA_FILES) \
    $(NULL)

CHECKS_PLUGIN_FILES = \
    collectors/checks.plugin/plugin_checks.c \
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

ML_FILES = \
    ml/ml.h \
    ml/ml-dummy.c \
    $(NULL)

if ENABLE_ML

ML_FILES += \
    ml/BitBufferCounter.h \
    ml/BitBufferCounter.cc \
    ml/BitRateWindow.h \
    ml/BitRateWindow.cc \
    ml/Config.h \
    ml/Config.cc \
    ml/Database.h \
    ml/Database.cc \
    ml/Dimension.cc \
    ml/Dimension.h \
    ml/Host.h \
    ml/Host.cc \
    ml/Query.h \
    ml/kmeans/KMeans.h \
    ml/kmeans/KMeans.cc \
    ml/kmeans/SamplesBuffer.h \
    ml/kmeans/SamplesBuffer.cc \
    ml/kmeans/dlib/dlib/all/source.cpp \
    ml/json/single_include/nlohmann/json.hpp \
    ml/ml.cc \
    ml/ml-private.h \
    $(NULL)

# Disable warnings from dlib library
ml/kmeans/dlib/dlib/all/source.$(OBJEXT) : CXXFLAGS += -Wno-sign-compare -Wno-type-limits

endif


if ENABLE_ML_TESTS
ML_TESTS_FILES = \
    ml/kmeans/Tests.cc \
    ml/Tests.cc \
    $(NULL)
endif

IDLEJITTER_PLUGIN_FILES = \
    collectors/idlejitter.plugin/plugin_idlejitter.c \
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
    collectors/diskspace.plugin/plugin_diskspace.c \
    $(NULL)

TIMEX_PLUGIN_FILES = \
    collectors/timex.plugin/plugin_timex.c \
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
    $(LIBNETDATA_FILES) \
    $(NULL)

XENSTAT_PLUGIN_FILES = \
    collectors/xenstat.plugin/xenstat_plugin.c \
    $(LIBNETDATA_FILES) \
    $(NULL)

PERF_PLUGIN_FILES = \
    collectors/perf.plugin/perf_plugin.c \
    $(LIBNETDATA_FILES) \
    $(NULL)

EBPF_PLUGIN_FILES = \
    collectors/ebpf.plugin/ebpf.c \
    collectors/ebpf.plugin/ebpf_cachestat.c \
    collectors/ebpf.plugin/ebpf_cachestat.h \
    collectors/ebpf.plugin/ebpf_dcstat.c \
    collectors/ebpf.plugin/ebpf_dcstat.h \
    collectors/ebpf.plugin/ebpf_disk.c \
    collectors/ebpf.plugin/ebpf_disk.h \
    collectors/ebpf.plugin/ebpf_fd.c \
    collectors/ebpf.plugin/ebpf_fd.h \
    collectors/ebpf.plugin/ebpf_filesystem.c \
    collectors/ebpf.plugin/ebpf_filesystem.h \
    collectors/ebpf.plugin/ebpf_hardirq.c \
    collectors/ebpf.plugin/ebpf_hardirq.h \
    collectors/ebpf.plugin/ebpf_mdflush.c \
    collectors/ebpf.plugin/ebpf_mdflush.h \
    collectors/ebpf.plugin/ebpf_mount.c \
    collectors/ebpf.plugin/ebpf_mount.h \
    collectors/ebpf.plugin/ebpf_oomkill.c \
    collectors/ebpf.plugin/ebpf_oomkill.h \
    collectors/ebpf.plugin/ebpf_process.c \
    collectors/ebpf.plugin/ebpf_process.h \
    collectors/ebpf.plugin/ebpf_shm.c \
    collectors/ebpf.plugin/ebpf_shm.h \
    collectors/ebpf.plugin/ebpf_socket.c \
    collectors/ebpf.plugin/ebpf_socket.h \
    collectors/ebpf.plugin/ebpf_softirq.c \
    collectors/ebpf.plugin/ebpf_softirq.h \
    collectors/ebpf.plugin/ebpf_sync.c \
    collectors/ebpf.plugin/ebpf_sync.h \
    collectors/ebpf.plugin/ebpf_swap.c \
    collectors/ebpf.plugin/ebpf_swap.h \
    collectors/ebpf.plugin/ebpf_vfs.c \
    collectors/ebpf.plugin/ebpf_vfs.h \
    collectors/ebpf.plugin/ebpf.h \
    collectors/ebpf.plugin/ebpf_apps.c \
    collectors/ebpf.plugin/ebpf_apps.h \
    collectors/ebpf.plugin/ebpf_cgroup.c \
    collectors/ebpf.plugin/ebpf_cgroup.h \
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
    collectors/proc.plugin/proc_pressure.c \
    collectors/proc.plugin/proc_pressure.h \
    collectors/proc.plugin/proc_net_dev.c \
    collectors/proc.plugin/proc_net_wireless.c \
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
    collectors/proc.plugin/sys_class_infiniband.c \
    $(NULL)

TC_PLUGIN_FILES = \
    collectors/tc.plugin/plugin_tc.c \
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
    collectors/plugins.d/pluginsd_parser.c \
    collectors/plugins.d/pluginsd_parser.h \
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
    database/rrdlabels.c \
    database/rrd.c \
    database/rrd.h \
    database/rrdset.c \
    database/rrdsetvar.c \
    database/rrdsetvar.h \
    database/rrdvar.c \
    database/rrdvar.h \
    database/rrddim_mem.c \
    database/rrddim_mem.h \
    database/storage_engine.c \
    database/storage_engine.h \
    database/sqlite/sqlite_functions.c \
    database/sqlite/sqlite_functions.h \
    database/sqlite/sqlite_aclk.c \
    database/sqlite/sqlite_aclk.h \
    database/sqlite/sqlite_health.c \
    database/sqlite/sqlite_health.h \
    database/sqlite/sqlite_aclk_node.c \
    database/sqlite/sqlite_aclk_node.h \
    database/sqlite/sqlite_aclk_chart.c \
    database/sqlite/sqlite_aclk_chart.h \
    database/sqlite/sqlite_aclk_alert.c \
    database/sqlite/sqlite_aclk_alert.h \
    database/sqlite/sqlite3.c \
    database/sqlite/sqlite3.h \
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
        database/engine/metadata_log/metadatalog.h \
        database/engine/metadata_log/metadatalogapi.c \
        database/engine/metadata_log/metadatalogapi.h \
        database/engine/metadata_log/logfile.h \
        database/engine/metadata_log/logfile.c \
        database/engine/metadata_log/metadatalogprotocol.h \
        database/engine/metadata_log/metalogpluginsd.c \
        database/engine/metadata_log/metalogpluginsd.h \
        database/engine/metadata_log/compaction.c \
        database/engine/metadata_log/compaction.h \
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
    streaming/compression.c \
    streaming/sender.c \
    streaming/receiver.c \
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

CLAIM_FILES = \
    claim/claim.c \
    claim/claim.h \
    $(NULL)

PARSER_FILES = \
    parser/parser.c \
    parser/parser.h \
    $(NULL)

if ENABLE_ACLK
ACLK_FILES = \
    aclk/aclk.c \
    aclk/aclk.h \
    aclk/aclk_util.c \
    aclk/aclk_util.h \
    aclk/aclk_stats.c \
    aclk/aclk_stats.h \
    aclk/aclk_query.c \
    aclk/aclk_query.h \
    aclk/aclk_query_queue.c \
    aclk/aclk_query_queue.h \
    aclk/aclk_otp.c \
    aclk/aclk_otp.h \
    aclk/aclk_tx_msgs.c \
    aclk/aclk_tx_msgs.h \
    aclk/aclk_rx_msgs.c \
    aclk/aclk_rx_msgs.h \
    aclk/https_client.c \
    aclk/https_client.h \
    mqtt_websockets/src/mqtt_wss_client.c \
    mqtt_websockets/src/include/mqtt_wss_client.h \
    mqtt_websockets/src/mqtt_wss_log.c \
    mqtt_websockets/src/include/mqtt_wss_log.h \
    mqtt_websockets/src/ws_client.c \
    mqtt_websockets/src/include/ws_client.h \
    mqtt_websockets/c-rbuf/src/ringbuffer.c \
    mqtt_websockets/c-rbuf/include/ringbuffer.h \
    mqtt_websockets/c-rbuf/src/ringbuffer_internal.h \
    mqtt_websockets/MQTT-C/src/mqtt.c \
    mqtt_websockets/MQTT-C/include/mqtt.h
    $(NULL)

if ENABLE_NEW_CLOUD_PROTOCOL
ACLK_FILES += \
    aclk/aclk_charts_api.c \
    aclk/aclk_charts_api.h \
    aclk/aclk_alarm_api.c \
    aclk/aclk_alarm_api.h \
    aclk/schema-wrappers/connection.cc \
    aclk/schema-wrappers/connection.h \
    aclk/schema-wrappers/node_connection.cc \
    aclk/schema-wrappers/node_connection.h \
    aclk/schema-wrappers/node_creation.cc \
    aclk/schema-wrappers/node_creation.h \
    aclk/schema-wrappers/chart_stream.cc \
    aclk/schema-wrappers/chart_stream.h \
    aclk/schema-wrappers/chart_config.cc \
    aclk/schema-wrappers/chart_config.h \
    aclk/schema-wrappers/alarm_stream.cc \
    aclk/schema-wrappers/alarm_stream.h \
    aclk/schema-wrappers/alarm_config.cc \
    aclk/schema-wrappers/alarm_config.h \
    aclk/schema-wrappers/node_info.cc \
    aclk/schema-wrappers/node_info.h \
    aclk/schema-wrappers/schema_wrappers.h \
    aclk/schema-wrappers/schema_wrapper_utils.cc \
    aclk/schema-wrappers/schema_wrapper_utils.h \
    $(NULL)

ACLK_PROTO_DEFINITIONS = \
    aclk/aclk-schemas/proto/aclk/v1/lib.proto \
    aclk/aclk-schemas/proto/agent/v1/disconnect.proto \
    aclk/aclk-schemas/proto/agent/v1/connection.proto \
    aclk/aclk-schemas/proto/alarm/v1/config.proto \
    aclk/aclk-schemas/proto/alarm/v1/stream.proto \
    aclk/aclk-schemas/proto/chart/v1/config.proto \
    aclk/aclk-schemas/proto/chart/v1/dimension.proto \
    aclk/aclk-schemas/proto/chart/v1/instance.proto \
    aclk/aclk-schemas/proto/chart/v1/stream.proto \
    aclk/aclk-schemas/proto/nodeinstance/connection/v1/connection.proto \
    aclk/aclk-schemas/proto/nodeinstance/create/v1/creation.proto \
    aclk/aclk-schemas/proto/nodeinstance/info/v1/info.proto \
    $(NULL)

dist_noinst_DATA += $(ACLK_PROTO_DEFINITIONS)

ACLK_PROTO_BUILT_FILES = aclk/aclk-schemas/proto/agent/v1/connection.pb.cc \
    aclk/aclk-schemas/proto/agent/v1/connection.pb.h \
    aclk/aclk-schemas/proto/nodeinstance/connection/v1/connection.pb.cc \
    aclk/aclk-schemas/proto/nodeinstance/connection/v1/connection.pb.h \
    aclk/aclk-schemas/proto/nodeinstance/create/v1/creation.pb.cc \
    aclk/aclk-schemas/proto/nodeinstance/create/v1/creation.pb.h \
    aclk/aclk-schemas/proto/chart/v1/stream.pb.cc \
    aclk/aclk-schemas/proto/chart/v1/stream.pb.h \
    aclk/aclk-schemas/proto/chart/v1/instance.pb.cc \
    aclk/aclk-schemas/proto/chart/v1/instance.pb.h \
    aclk/aclk-schemas/proto/chart/v1/dimension.pb.cc \
    aclk/aclk-schemas/proto/chart/v1/dimension.pb.h \
    aclk/aclk-schemas/proto/chart/v1/config.pb.cc \
    aclk/aclk-schemas/proto/chart/v1/config.pb.h \
    aclk/aclk-schemas/proto/aclk/v1/lib.pb.cc \
    aclk/aclk-schemas/proto/aclk/v1/lib.pb.h \
    aclk/aclk-schemas/proto/agent/v1/disconnect.pb.cc \
    aclk/aclk-schemas/proto/agent/v1/disconnect.pb.h \
    aclk/aclk-schemas/proto/alarm/v1/config.pb.cc \
    aclk/aclk-schemas/proto/alarm/v1/config.pb.h \
    aclk/aclk-schemas/proto/alarm/v1/stream.pb.cc \
    aclk/aclk-schemas/proto/alarm/v1/stream.pb.h \
    aclk/aclk-schemas/proto/nodeinstance/info/v1/info.pb.cc \
    aclk/aclk-schemas/proto/nodeinstance/info/v1/info.pb.h \
    $(NULL)

BUILT_SOURCES += $(ACLK_PROTO_BUILT_FILES)
nodist_netdata_SOURCES += $(ACLK_PROTO_BUILT_FILES)
CLEANFILES += $(ACLK_PROTO_BUILT_FILES)

aclk/aclk-schemas/proto/agent/v1/connection.pb.cc \
aclk/aclk-schemas/proto/agent/v1/connection.pb.h: aclk/aclk-schemas/proto/agent/v1/connection.proto
	$(PROTOC) -I=aclk/aclk-schemas --cpp_out=$(builddir)/aclk/aclk-schemas $^

aclk/aclk-schemas/proto/nodeinstance/connection/v1/connection.pb.cc \
aclk/aclk-schemas/proto/nodeinstance/connection/v1/connection.pb.h: aclk/aclk-schemas/proto/nodeinstance/connection/v1/connection.proto
	$(PROTOC) -I=aclk/aclk-schemas --cpp_out=$(builddir)/aclk/aclk-schemas $^

aclk/aclk-schemas/proto/nodeinstance/create/v1/creation.pb.cc \
aclk/aclk-schemas/proto/nodeinstance/create/v1/creation.pb.h: aclk/aclk-schemas/proto/nodeinstance/create/v1/creation.proto
	$(PROTOC) -I=aclk/aclk-schemas --cpp_out=$(builddir)/aclk/aclk-schemas $^

aclk/aclk-schemas/proto/chart/v1/stream.pb.cc \
aclk/aclk-schemas/proto/chart/v1/stream.pb.h: aclk/aclk-schemas/proto/chart/v1/stream.proto
	$(PROTOC) -I=aclk/aclk-schemas --cpp_out=$(builddir)/aclk/aclk-schemas $^

aclk/aclk-schemas/proto/chart/v1/instance.pb.cc \
aclk/aclk-schemas/proto/chart/v1/instance.pb.h: aclk/aclk-schemas/proto/chart/v1/instance.proto
	$(PROTOC) -I=aclk/aclk-schemas --cpp_out=$(builddir)/aclk/aclk-schemas $^

aclk/aclk-schemas/proto/chart/v1/dimension.pb.cc \
aclk/aclk-schemas/proto/chart/v1/dimension.pb.h: aclk/aclk-schemas/proto/chart/v1/dimension.proto
	$(PROTOC) -I=aclk/aclk-schemas --cpp_out=$(builddir)/aclk/aclk-schemas $^

aclk/aclk-schemas/proto/chart/v1/config.pb.cc \
aclk/aclk-schemas/proto/chart/v1/config.pb.h: aclk/aclk-schemas/proto/chart/v1/config.proto
	$(PROTOC) -I=aclk/aclk-schemas --cpp_out=$(builddir)/aclk/aclk-schemas $^

aclk/aclk-schemas/proto/aclk/v1/lib.pb.cc \
aclk/aclk-schemas/proto/aclk/v1/lib.pb.h: aclk/aclk-schemas/proto/aclk/v1/lib.proto
	$(PROTOC) -I=aclk/aclk-schemas --cpp_out=$(builddir)/aclk/aclk-schemas $^

aclk/aclk-schemas/proto/agent/v1/disconnect.pb.cc \
aclk/aclk-schemas/proto/agent/v1/disconnect.pb.h: aclk/aclk-schemas/proto/agent/v1/disconnect.proto
	$(PROTOC) -I=aclk/aclk-schemas --cpp_out=$(builddir)/aclk/aclk-schemas $^

aclk/aclk-schemas/proto/alarm/v1/config.pb.cc \
aclk/aclk-schemas/proto/alarm/v1/config.pb.h: aclk/aclk-schemas/proto/alarm/v1/config.proto
	$(PROTOC) -I=aclk/aclk-schemas --cpp_out=$(builddir)/aclk/aclk-schemas $^

aclk/aclk-schemas/proto/alarm/v1/stream.pb.cc \
aclk/aclk-schemas/proto/alarm/v1/stream.pb.h: aclk/aclk-schemas/proto/alarm/v1/stream.proto
	$(PROTOC) -I=aclk/aclk-schemas --cpp_out=$(builddir)/aclk/aclk-schemas $^

aclk/aclk-schemas/proto/nodeinstance/info/v1/info.pb.cc \
aclk/aclk-schemas/proto/nodeinstance/info/v1/info.pb.h: aclk/aclk-schemas/proto/nodeinstance/info/v1/info.proto
	$(PROTOC) -I=aclk/aclk-schemas --cpp_out=$(builddir)/aclk/aclk-schemas $^

endif #ENABLE_NEW_CLOUD_PROTOCOL

endif #ENABLE_ACLK

if ENABLE_ACLK
ACLK_COMMON_FILES = \
    aclk/aclk_collector_list.c \
    aclk/aclk_collector_list.h \
    $(NULL)
endif

ACLK_ALWAYS_BUILD_FILES = \
    aclk/aclk_rrdhost_state.h \
    aclk/aclk_api.c \
    aclk/aclk_api.h \
    aclk/aclk_proxy.c \
    aclk/aclk_proxy.h \
    $(NULL)

SPAWN_PLUGIN_FILES = \
    spawn/spawn.c \
    spawn/spawn_server.c \
    spawn/spawn_client.c \
    spawn/spawn.h \
    $(NULL)

EXPORTING_ENGINE_FILES = \
    exporting/exporting_engine.c \
    exporting/exporting_engine.h \
    exporting/graphite/graphite.c \
    exporting/graphite/graphite.h \
    exporting/json/json.c \
    exporting/json/json.h \
    exporting/opentsdb/opentsdb.c \
    exporting/opentsdb/opentsdb.h \
    exporting/prometheus/prometheus.c \
    exporting/prometheus/prometheus.h \
    exporting/read_config.c \
    exporting/clean_connectors.c \
    exporting/init_connectors.c \
    exporting/process_data.c \
    exporting/check_filters.c \
    exporting/send_data.c \
    exporting/send_internal_metrics.c \
    $(NULL)

PROMETHEUS_REMOTE_WRITE_EXPORTING_FILES = \
    exporting/prometheus/remote_write/remote_write.c \
    exporting/prometheus/remote_write/remote_write.h \
    exporting/prometheus/remote_write/remote_write_request.cc \
    exporting/prometheus/remote_write/remote_write_request.h \
    exporting/prometheus/remote_write/remote_write.proto \
    $(NULL)

KINESIS_EXPORTING_FILES = \
    exporting/aws_kinesis/aws_kinesis.c \
    exporting/aws_kinesis/aws_kinesis.h \
    exporting/aws_kinesis/aws_kinesis_put_record.cc \
    exporting/aws_kinesis/aws_kinesis_put_record.h \
    $(NULL)

PUBSUB_EXPORTING_FILES = \
    exporting/pubsub/pubsub.c \
    exporting/pubsub/pubsub.h \
    exporting/pubsub/pubsub_publish.cc \
    exporting/pubsub/pubsub_publish.h \
    $(NULL)

MONGODB_EXPORTING_FILES = \
    exporting/mongodb/mongodb.c \
    exporting/mongodb/mongodb.h \
    $(NULL)

DAEMON_FILES = \
    daemon/buildinfo.c \
    daemon/buildinfo.h \
    daemon/common.c \
    daemon/common.h \
    daemon/daemon.c \
    daemon/daemon.h \
    daemon/global_statistics.c \
    daemon/global_statistics.h \
    daemon/analytics.c \
    daemon/analytics.h \
    daemon/main.c \
    daemon/main.h \
    daemon/signals.c \
    daemon/signals.h \
    daemon/service.c \
    daemon/static_threads.h \
    daemon/static_threads.c \
    daemon/commands.c \
    daemon/commands.h \
    daemon/unit_test.c \
    daemon/unit_test.h \
    $(NULL)

NETDATA_FILES = \
    collectors/all.h \
    $(DAEMON_FILES) \
    $(LIBNETDATA_FILES) \
    $(API_PLUGIN_FILES) \
    $(EXPORTING_ENGINE_FILES) \
    $(CHECKS_PLUGIN_FILES) \
    $(HEALTH_PLUGIN_FILES) \
    $(ML_FILES) \
    $(ML_TESTS_FILES) \
    $(IDLEJITTER_PLUGIN_FILES) \
    $(PLUGINSD_PLUGIN_FILES) \
    $(REGISTRY_PLUGIN_FILES) \
    $(RRD_PLUGIN_FILES) \
    $(STREAMING_PLUGIN_FILES) \
    $(STATSD_PLUGIN_FILES) \
    $(WEB_PLUGIN_FILES) \
    $(CLAIM_FILES) \
    $(PARSER_FILES) \
    $(ACLK_ALWAYS_BUILD_FILES) \
    $(ACLK_COMMON_FILES) \
    $(ACLK_FILES) \
    $(SPAWN_PLUGIN_FILES) \
    $(TIMEX_PLUGIN_FILES) \
    $(NULL)

if FREEBSD
    NETDATA_FILES += \
        daemon/static_threads_freebsd.c \
        $(FREEBSD_PLUGIN_FILES) \
        $(NULL)
endif

if MACOS
    NETDATA_FILES += \
        daemon/static_threads_macos.c \
        $(MACOS_PLUGIN_FILES) \
        $(NULL)
endif

if LINUX
    NETDATA_FILES += \
        daemon/static_threads_linux.c \
        $(CGROUPS_PLUGIN_FILES) \
        $(DISKSPACE_PLUGIN_FILES) \
        $(PROC_PLUGIN_FILES) \
        $(TC_PLUGIN_FILES) \
        $(NULL)
endif

NETDATA_COMMON_LIBS = \
    $(OPTIONAL_MATH_LIBS) \
    $(OPTIONAL_BPF_LIBS) \
    $(OPTIONAL_ZLIB_LIBS) \
    $(OPTIONAL_SSL_LIBS) \
    $(OPTIONAL_UUID_LIBS) \
    $(OPTIONAL_MQTT_LIBS) \
    $(OPTIONAL_UV_LIBS) \
    $(OPTIONAL_LZ4_LIBS) \
    $(OPTIONAL_JUDY_LIBS) \
    $(OPTIONAL_SSL_LIBS) \
    $(OPTIONAL_JSONC_LIBS) \
    $(OPTIONAL_ATOMIC_LIBS) \
    $(NULL)

if LINK_STATIC_JSONC
    NETDATA_COMMON_LIBS += $(abs_top_srcdir)/externaldeps/jsonc/libjson-c.a
endif

NETDATACLI_FILES = \
    daemon/commands.h \
    $(LIBNETDATA_FILES) \
    cli/cli.c \
    cli/cli.h \
    $(NULL)

sbin_PROGRAMS += netdata
netdata_SOURCES = $(NETDATA_FILES)

if LINUX
    NETDATA_COMMON_LIBS += -lrt
endif

netdata_LDADD = \
    $(NETDATA_COMMON_LIBS) \
    $(NULL)

if ENABLE_ACLK
    netdata_LDADD += $(OPTIONAL_PROTOBUF_LIBS) \
    $(OPTIONAL_ATOMIC_LIBS) \
    $(NULL)
endif

if ENABLE_ML_TESTS
    netdata_LDADD += $(OPTIONAL_ML_TESTS_LIBS) \
        $(NULL)
endif

if ENABLE_CXX_LINKER
    netdata_LINK = $(CXXLD) $(CXXFLAGS) $(LDFLAGS) -o $@
else
    netdata_LINK = $(CCLD) $(CFLAGS) $(LDFLAGS) -o $@
endif

sbin_PROGRAMS += netdatacli
netdatacli_SOURCES = $(NETDATACLI_FILES)
netdatacli_LDADD = \
    $(NETDATA_COMMON_LIBS) \
    $(NULL)
if ENABLE_CXX_LINKER
    netdatacli_LINK = $(CXXLD) $(CXXFLAGS) $(LDFLAGS) -o $@
else
    netdatacli_LINK = $(CCLD) $(CFLAGS) $(LDFLAGS) -o $@
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

if ENABLE_PLUGIN_EBPF
    plugins_PROGRAMS += ebpf.plugin
    ebpf_plugin_SOURCES = $(EBPF_PLUGIN_FILES)
    ebpf_plugin_LDADD = \
        $(NETDATA_COMMON_LIBS) \
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

if ENABLE_EXPORTING_KINESIS
    netdata_SOURCES += $(KINESIS_EXPORTING_FILES)
    netdata_LDADD += $(OPTIONAL_KINESIS_LIBS)
endif

if ENABLE_EXPORTING_PUBSUB
    netdata_SOURCES += $(PUBSUB_EXPORTING_FILES)
    netdata_LDADD += $(OPTIONAL_PUBSUB_LIBS)
endif

if ENABLE_EXPORTING_PROMETHEUS_REMOTE_WRITE
    netdata_SOURCES += $(PROMETHEUS_REMOTE_WRITE_EXPORTING_FILES)
    netdata_LDADD += $(OPTIONAL_PROMETHEUS_REMOTE_WRITE_LIBS) \
        $(OPTIONAL_PROTOBUF_LIBS) \
        $(NULL)
    EXPORTING_PROMETHEUS_BUILT_SOURCES = \
        exporting/prometheus/remote_write/remote_write.pb.cc \
        exporting/prometheus/remote_write/remote_write.pb.h \
        $(NULL)
    BUILT_SOURCES += $(EXPORTING_PROMETHEUS_BUILT_SOURCES)
    nodist_netdata_SOURCES += $(EXPORTING_PROMETHEUS_BUILT_SOURCES)

exporting/prometheus/remote_write/remote_write.pb.cc \
exporting/prometheus/remote_write/remote_write.pb.h: exporting/prometheus/remote_write/remote_write.proto
	$(PROTOC) --proto_path=$(srcdir) --cpp_out=$(builddir) $^

endif

if ENABLE_EXPORTING_MONGODB
    netdata_SOURCES += $(MONGODB_EXPORTING_FILES)
    netdata_LDADD += $(OPTIONAL_MONGOC_LIBS)
endif

if ENABLE_UNITTESTS
    check_PROGRAMS = \
        libnetdata/tests/str2ld_testdriver \
        libnetdata/storage_number/tests/storage_number_testdriver \
        exporting/tests/exporting_engine_testdriver \
        web/api/tests/web_api_testdriver \
        web/api/tests/valid_urls_testdriver \
        collectors/cgroups_plugin/tests/cgroups_testdriver \
        $(NULL)

    TESTS = $(check_PROGRAMS)

    XFAIL_TESTS = \
        web/api/tests/web_api_testdriver \
        web/api/tests/valid_urls_testdriver \
        $(NULL)

    web_api_tests_valid_urls_testdriver_LDFLAGS = \
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
        -Wl,--wrap=fatal_int \
        -Wl,--wrap=mysendfile \
        -DREMOVE_MYSENDFILE \
        $(TEST_LDFLAGS) \
        $(NULL)
    web_api_tests_valid_urls_testdriver_SOURCES = \
        web/api/tests/valid_urls.c \
        web/server/web_client.c \
        $(LIBNETDATA_FILES) \
        $(NULL)
    web_api_tests_valid_urls_testdriver_LDADD = $(NETDATA_COMMON_LIBS) $(TEST_LIBS)

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
        -Wl,--wrap=fatal_int \
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

    EXPORTING_ENGINE_TEST_FILES = \
        exporting/tests/test_exporting_engine.c \
        exporting/tests/test_exporting_engine.h \
        exporting/tests/exporting_fixtures.c \
        exporting/tests/exporting_doubles.c \
        exporting/tests/netdata_doubles.c \
        exporting/tests/system_doubles.c \
        $(NULL)
    exporting_tests_exporting_engine_testdriver_SOURCES = \
        $(EXPORTING_ENGINE_TEST_FILES) \
        $(EXPORTING_ENGINE_FILES) \
        $(LIBNETDATA_FILES) \
        $(NULL)
    exporting_tests_exporting_engine_testdriver_CFLAGS = \
        $(AM_CFLAGS) \
        -DUNIT_TESTING \
        $(NULL)
    exporting_tests_exporting_engine_testdriver_LDFLAGS = \
        -Wl,--wrap=read_exporting_config \
        -Wl,--wrap=init_connectors \
        -Wl,--wrap=mark_scheduled_instances \
        -Wl,--wrap=rrdhost_is_exportable \
        -Wl,--wrap=rrdset_is_exportable \
        -Wl,--wrap=exporting_calculate_value_from_stored_data \
        -Wl,--wrap=prepare_buffers \
        -Wl,--wrap=send_internal_metrics \
        -Wl,--wrap=now_realtime_sec \
        -Wl,--wrap=uv_thread_set_name_np \
        -Wl,--wrap=uv_thread_create \
        -Wl,--wrap=uv_mutex_lock \
        -Wl,--wrap=uv_mutex_unlock \
        -Wl,--wrap=uv_cond_signal \
        -Wl,--wrap=uv_cond_wait \
        -Wl,--wrap=strdupz \
        -Wl,--wrap=info_int \
        -Wl,--wrap=recv \
        -Wl,--wrap=send \
        -Wl,--wrap=connect_to_one_of \
        -Wl,--wrap=create_main_rusage_chart \
        -Wl,--wrap=send_main_rusage \
        -Wl,--wrap=simple_connector_end_batch \
        $(TEST_LDFLAGS) \
        $(NULL)
    exporting_tests_exporting_engine_testdriver_LDADD = $(NETDATA_COMMON_LIBS) $(TEST_LIBS)
if ENABLE_EXPORTING_PROMETHEUS_REMOTE_WRITE
    exporting_tests_exporting_engine_testdriver_SOURCES += $(PROMETHEUS_REMOTE_WRITE_EXPORTING_FILES)
    exporting_tests_exporting_engine_testdriver_LDADD += \
        $(OPTIONAL_PROMETHEUS_REMOTE_WRITE_LIBS) \
        $(OPTIONAL_PROTOBUF_LIBS) \
        $(NULL)
    exporting_tests_exporting_engine_testdriver_LDFLAGS += \
        -Wl,--wrap=init_write_request \
        -Wl,--wrap=add_host_info \
        -Wl,--wrap=add_label \
        -Wl,--wrap=add_metric \
        $(NULL)
    nodist_exporting_tests_exporting_engine_testdriver_SOURCES = $(EXPORTING_PROMETHEUS_BUILT_SOURCES)
endif
if ENABLE_EXPORTING_KINESIS
    exporting_tests_exporting_engine_testdriver_SOURCES += $(KINESIS_EXPORTING_FILES)
    exporting_tests_exporting_engine_testdriver_LDADD += $(OPTIONAL_KINESIS_LIBS)
    exporting_tests_exporting_engine_testdriver_LDFLAGS += \
        -Wl,--wrap=aws_sdk_init \
        -Wl,--wrap=kinesis_init \
        -Wl,--wrap=kinesis_put_record \
        -Wl,--wrap=kinesis_get_result \
        $(NULL)
endif
if ENABLE_EXPORTING_PUBSUB
    exporting_tests_exporting_engine_testdriver_SOURCES += $(PUBSUB_EXPORTING_FILES)
    exporting_tests_exporting_engine_testdriver_LDADD += $(OPTIONAL_PUBSUB_LIBS)
    exporting_tests_exporting_engine_testdriver_LDFLAGS += \
        -Wl,--wrap=pubsub_init \
        -Wl,--wrap=pubsub_add_message \
        -Wl,--wrap=pubsub_publish \
        -Wl,--wrap=pubsub_get_result \
        $(NULL)
endif
if ENABLE_EXPORTING_MONGODB
    exporting_tests_exporting_engine_testdriver_SOURCES += $(MONGODB_EXPORTING_FILES)
    exporting_tests_exporting_engine_testdriver_LDADD += $(OPTIONAL_MONGOC_LIBS)
    exporting_tests_exporting_engine_testdriver_LDFLAGS += \
        -Wl,--wrap=mongoc_init \
        -Wl,--wrap=mongoc_uri_new_with_error \
        -Wl,--wrap=mongoc_uri_get_option_as_int32 \
        -Wl,--wrap=mongoc_uri_set_option_as_int32 \
        -Wl,--wrap=mongoc_client_new_from_uri \
        -Wl,--wrap=mongoc_client_set_appname \
        -Wl,--wrap=mongoc_client_get_collection \
        -Wl,--wrap=mongoc_uri_destroy \
        -Wl,--wrap=mongoc_collection_insert_many \
        $(NULL)
endif

    collectors_cgroups_plugin_tests_cgroups_testdriver_SOURCES = \
        collectors/cgroups.plugin/tests/test_cgroups_plugin.c \
        collectors/cgroups.plugin/tests/test_cgroups_plugin.h \
        collectors/cgroups.plugin/tests/test_doubles.c \
        $(CGROUPS_PLUGIN_FILES) \
        database/rrdlabels.c \
        database/rrd.h \
        $(LIBNETDATA_FILES) \
        $(NULL)
    collectors_cgroups_plugin_tests_cgroups_testdriver_LDADD = $(NETDATA_COMMON_LIBS) $(TEST_LIBS)
    collectors_cgroups_plugin_tests_cgroups_testdriver_LDFLAGS = \
        -Wl,--wrap=add_label_to_list \
        $(NULL)

endif
