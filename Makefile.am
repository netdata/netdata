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
    .eslintignore \
    .eslintrc \
    .github/CODEOWNERS \
    build/m4/jemalloc.m4 \
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
    build/m4/ax_compiler_vendor.m4 \
    build/m4/ax_cxx_compile_stdcxx.m4 \
    ml/dlib \
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
    contrib \
    docs \
    mqtt_websockets \
    netdata.cppcheck \
    netdata.spec \
    packaging/bundle-ebpf.sh \
    packaging/bundle-ebpf-co-re.sh \
    packaging/bundle-libbpf.sh \
    packaging/check-kernel-config.sh \
    packaging/ebpf.checksums \
    packaging/ebpf.version \
    packaging/ebpf-co-re.checksums \
    packaging/ebpf-co-re.version \
    packaging/go.d.checksums \
    packaging/go.d.version \
    packaging/installer/README.md \
    packaging/installer/UNINSTALL.md \
    packaging/installer/UPDATE.md \
    packaging/jsonc.checksums \
    packaging/jsonc.version \
    packaging/yaml.checksums \
    packaging/yaml.version \
    packaging/current_libbpf.checksums \
    packaging/current_libbpf.version \
    packaging/libbpf_0_0_9.checksums \
    packaging/libbpf_0_0_9.version \
    packaging/protobuf.checksums \
    packaging/protobuf.version \
    packaging/version \
    database/engine/journalfile_v2.ksy.in \
    httpd/h2o \
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
    spawn \
    $(NULL)

AM_CFLAGS = \
    $(OPTIONAL_MATH_CFLAGS) \
    $(OPTIONAL_NFACCT_CFLAGS) \
    $(ZLIB_CFLAGS) \
    $(OPTIONAL_UUID_CFLAGS) \
    $(OPTIONAL_MQTT_CFLAGS) \
    $(OPTIONAL_LIBCAP_LIBS) \
    $(OPTIONAL_DATACHANNEL_CFLAGS) \
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
    libnetdata/aral/aral.c \
    libnetdata/aral/aral.h \
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
    libnetdata/gorilla/gorilla.h \
    libnetdata/gorilla/gorilla.cc \
    libnetdata/inlined.h \
    libnetdata/july/july.c \
    libnetdata/july/july.h \
    libnetdata/libnetdata.c \
    libnetdata/libnetdata.h \
    libnetdata/required_dummies.h \
    libnetdata/locks/locks.c \
    libnetdata/locks/locks.h \
    libnetdata/log/log.c \
    libnetdata/log/log.h \
    libnetdata/onewayalloc/onewayalloc.c \
    libnetdata/onewayalloc/onewayalloc.h \
    libnetdata/parser/parser.c \
    libnetdata/parser/parser.h \
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
    libnetdata/string/string.c \
    libnetdata/string/string.h \
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
    libnetdata/worker_utilization/worker_utilization.c \
    libnetdata/worker_utilization/worker_utilization.h \
    libnetdata/http/http_defs.h \
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

DEBUGFS_PLUGIN_FILES = \
    collectors/debugfs.plugin/debugfs_plugin.c \
    collectors/debugfs.plugin/debugfs_plugin.h \
    collectors/debugfs.plugin/debugfs_extfrag.c \
    collectors/debugfs.plugin/debugfs_zswap.c \
    $(LIBNETDATA_FILES) \
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
    ml/ad_charts.h \
    ml/ad_charts.cc \
    ml/Config.cc \
    ml/dlib/dlib/all/source.cpp \
    ml/ml-private.h \
    ml/ml.cc \
    $(NULL)

# Disable warnings from dlib library
ml/dlib/dlib/all/source.$(OBJEXT) : CXXFLAGS += -Wno-sign-compare -Wno-type-limits -Wno-aggressive-loop-optimizations -Wno-stringop-overflow -Wno-psabi

# Disable ml warnings
ml/ml.$(OBJEXT) : CXXFLAGS += -Wno-psabi

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
    collectors/ebpf.plugin/ebpf_unittest.c \
    collectors/ebpf.plugin/ebpf_unittest.h \
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
    database/contexts/rrdcontext.c \
    database/contexts/rrdcontext.h \
    database/contexts/metric.c \
    database/contexts/instance.c \
    database/contexts/context.c \
    database/contexts/worker.c \
    database/contexts/query_target.c \
    database/contexts/query_scope.c \
    database/contexts/api_v1.c \
    database/contexts/api_v2.c \
    database/contexts/internal.h \
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
    database/rrdfunctions.c \
    database/rrdfunctions.h \
    database/rrdsetvar.c \
    database/rrdsetvar.h \
    database/rrdvar.c \
    database/rrdvar.h \
    database/storage_engine.c \
    database/storage_engine.h \
    database/ram/rrddim_mem.c \
    database/ram/rrddim_mem.h \
    database/sqlite/sqlite_functions.c \
    database/sqlite/sqlite_functions.h \
    database/sqlite/sqlite_context.c \
    database/sqlite/sqlite_context.h \
    database/sqlite/sqlite_db_migration.c \
    database/sqlite/sqlite_db_migration.h \
    database/sqlite/sqlite_aclk.c \
	database/sqlite/sqlite_aclk.h \
    database/sqlite/sqlite_metadata.c \
    database/sqlite/sqlite_metadata.h \
    database/sqlite/sqlite_health.c \
    database/sqlite/sqlite_health.h \
    database/sqlite/sqlite_aclk_node.c \
    database/sqlite/sqlite_aclk_node.h \
    database/sqlite/sqlite_aclk_alert.c \
    database/sqlite/sqlite_aclk_alert.h \
    database/sqlite/sqlite3.c \
    database/sqlite/sqlite3.h \
    database/KolmogorovSmirnovDist.c \
    database/KolmogorovSmirnovDist.h \
    $(NULL)

database/sqlite/sqlite3.$(OBJEXT) : CFLAGS += -Wno-cast-function-type
database/KolmogorovSmirnovDist.$(OBJEXT) : CFLAGS += -Wno-maybe-uninitialized

noinst_LIBRARIES = libjudy.a

libjudy_a_SOURCES = libnetdata/libjudy/src/Judy.h \
    libnetdata/libjudy/src/JudyCommon/JudyMalloc.c \
    libnetdata/libjudy/src/JudyCommon/JudyPrivate.h \
    libnetdata/libjudy/src/JudyCommon/JudyPrivate1L.h \
    libnetdata/libjudy/src/JudyCommon/JudyPrivateBranch.h \
    libnetdata/libjudy/src/JudyL/JudyL.h \
    libnetdata/libjudy/src/JudyL/JudyLByCount.c \
    libnetdata/libjudy/src/JudyL/JudyLCascade.c \
    libnetdata/libjudy/src/JudyL/JudyLCount.c \
    libnetdata/libjudy/src/JudyL/JudyLCreateBranch.c \
    libnetdata/libjudy/src/JudyL/JudyLDecascade.c \
    libnetdata/libjudy/src/JudyL/JudyLDel.c \
    libnetdata/libjudy/src/JudyL/JudyLFirst.c \
    libnetdata/libjudy/src/JudyL/JudyLFreeArray.c \
    libnetdata/libjudy/src/JudyL/j__udyLGet.c \
    libnetdata/libjudy/src/JudyL/JudyLGet.c \
    libnetdata/libjudy/src/JudyL/JudyLInsArray.c \
    libnetdata/libjudy/src/JudyL/JudyLIns.c \
    libnetdata/libjudy/src/JudyL/JudyLInsertBranch.c \
    libnetdata/libjudy/src/JudyL/JudyLMallocIF.c \
    libnetdata/libjudy/src/JudyL/JudyLMemActive.c \
    libnetdata/libjudy/src/JudyL/JudyLMemUsed.c \
    libnetdata/libjudy/src/JudyL/JudyLNext.c \
    libnetdata/libjudy/src/JudyL/JudyLNextEmpty.c \
    libnetdata/libjudy/src/JudyL/JudyLPrev.c \
    libnetdata/libjudy/src/JudyL/JudyLPrevEmpty.c \
    libnetdata/libjudy/src/JudyL/JudyLTables.c \
    libnetdata/libjudy/src/JudyHS/JudyHS.c \
    $(NULL)

libjudy_a_CFLAGS = $(LIBJUDY_CFLAGS) -DJUDYL -I$(abs_top_srcdir)/libnetdata/libjudy/src -I$(abs_top_srcdir)/libnetdata/libjudy/src/JudyCommon -Wno-sign-compare -Wno-implicit-fallthrough

libnetdata/libjudy/src/JudyL/libjudy_a-JudyLPrev.$(OBJEXT) : CFLAGS += -DJUDYPREV
libnetdata/libjudy/src/JudyL/libjudy_a-JudyLPrevEmpty.$(OBJEXT) : CFLAGS += -DJUDYPREV
libnetdata/libjudy/src/JudyL/libjudy_a-JudyLNext.$(OBJEXT) : CFLAGS += -DJUDYNEXT
libnetdata/libjudy/src/JudyL/libjudy_a-JudyLNextEmpty.$(OBJEXT) : CFLAGS += -DJUDYNEXT
libnetdata/libjudy/src/JudyL/libjudy_a-JudyLByCount.$(OBJEXT) : CFLAGS +=  -DNOSMARTJBB -DNOSMARTJBU -DNOSMARTJLB
libnetdata/libjudy/src/JudyL/libjudy_a-j__udyLGet.$(OBJEXT) : CFLAGS +=  -DJUDYGETINLINE

libjudy_a-JudyLTables.$(OBJEXT) : CFLAGS += -I$(abs_top_srcdir)/libnetdata/libjudy/src/JudyL

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
        database/engine/cache.c \
        database/engine/cache.h \
        database/engine/metric.c \
        database/engine/metric.h \
        database/engine/pdc.c \
        database/engine/pdc.h \
        $(NULL)
    
    RRD_PLUGIN_KSY_BUILTFILES = \
        database/engine/journalfile_v2.ksy \
        database/engine/journalfile_v2_virtmemb.ksy \
        $(NULL)

    BUILT_SOURCES += $(RRD_PLUGIN_KSY_BUILTFILES)
    CLEANFILES += $(RRD_PLUGIN_KSY_BUILTFILES)

database/engine/journalfile_v2.ksy: $(abs_top_srcdir)/database/engine/journalfile_v2.ksy.in
	m4 $(abs_top_srcdir)/database/engine/journalfile_v2.ksy.in > $@

database/engine/journalfile_v2_virtmemb.ksy: $(abs_top_srcdir)/database/engine/journalfile_v2.ksy.in
	m4 -DVIRT_MEMBERS $(abs_top_srcdir)/database/engine/journalfile_v2.ksy.in > $@
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
    web/api/queries/countif/countif.c \
    web/api/queries/countif/countif.h \
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
    web/api/queries/percentile/percentile.c \
    web/api/queries/percentile/percentile.h \
    web/api/queries/trimmed_mean/trimmed_mean.c \
    web/api/queries/trimmed_mean/trimmed_mean.h \
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
    web/api/queries/weights.c \
    web/api/queries/weights.h \
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
    web/api/web_api.c \
    web/api/web_api.h \
    web/api/web_api_v1.c \
    web/api/web_api_v1.h \
    web/api/web_api_v2.c \
    web/api/web_api_v2.h \
    $(NULL)

STREAMING_PLUGIN_FILES = \
    streaming/rrdpush.c \
    streaming/compression.c \
    streaming/sender.c \
    streaming/receiver.c \
    streaming/replication.h \
    streaming/replication.c \
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
    web/rtc/webrtc.c \
    web/rtc/webrtc.h \
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

if ENABLE_ACLK
ACLK_FILES = \
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
    aclk/aclk_alarm_api.c \
    aclk/aclk_alarm_api.h \
    aclk/aclk_contexts_api.c \
    aclk/aclk_contexts_api.h \
    aclk/helpers/mqtt_wss_pal.h \
    aclk/helpers/ringbuffer_pal.h \
    aclk/schema-wrappers/connection.cc \
    aclk/schema-wrappers/connection.h \
    aclk/schema-wrappers/node_connection.cc \
    aclk/schema-wrappers/node_connection.h \
    aclk/schema-wrappers/node_creation.cc \
    aclk/schema-wrappers/node_creation.h \
    aclk/schema-wrappers/alarm_stream.cc \
    aclk/schema-wrappers/alarm_stream.h \
    aclk/schema-wrappers/alarm_config.cc \
    aclk/schema-wrappers/alarm_config.h \
    aclk/schema-wrappers/node_info.cc \
    aclk/schema-wrappers/node_info.h \
    aclk/schema-wrappers/capability.cc \
    aclk/schema-wrappers/capability.h \
    aclk/schema-wrappers/proto_2_json.cc \
    aclk/schema-wrappers/proto_2_json.h \
    aclk/schema-wrappers/schema_wrappers.h \
    aclk/schema-wrappers/schema_wrapper_utils.cc \
    aclk/schema-wrappers/schema_wrapper_utils.h \
    aclk/schema-wrappers/context_stream.cc \
    aclk/schema-wrappers/context_stream.h \
    aclk/schema-wrappers/context.cc \
    aclk/schema-wrappers/context.h \
    aclk/schema-wrappers/agent_cmds.cc \
    aclk/schema-wrappers/agent_cmds.h \
    $(NULL)

noinst_LIBRARIES += libmqttwebsockets.a

libmqttwebsockets_a_SOURCES = \
    mqtt_websockets/src/mqtt_wss_client.c \
    mqtt_websockets/src/include/mqtt_wss_client.h \
    mqtt_websockets/src/mqtt_wss_log.c \
    mqtt_websockets/src/include/mqtt_wss_log.h \
    mqtt_websockets/src/ws_client.c \
    mqtt_websockets/src/include/ws_client.h \
    mqtt_websockets/src/mqtt_ng.c \
    mqtt_websockets/src/include/mqtt_ng.h \
    mqtt_websockets/src/common_public.c \
    mqtt_websockets/src/include/common_public.h \
    mqtt_websockets/src/include/common_internal.h \
    mqtt_websockets/c-rbuf/src/ringbuffer.c \
    mqtt_websockets/c-rbuf/include/ringbuffer.h \
    mqtt_websockets/c-rbuf/src/ringbuffer_internal.h \
    mqtt_websockets/c_rhash/src/c_rhash.c \
    mqtt_websockets/c_rhash/include/c_rhash.h \
    mqtt_websockets/c_rhash/src/c_rhash_internal.h \
    $(NULL)

libmqttwebsockets_a_CFLAGS = $(CFLAGS) -DMQTT_WSS_CUSTOM_ALLOC -DRBUF_CUSTOM_MALLOC -DMQTT_WSS_CPUSTATS -I$(srcdir)/aclk/helpers -I$(srcdir)/mqtt_websockets/c_rhash/include

if MQTT_WSS_DEBUG
libmqttwebsockets_a_CFLAGS += -DMQTT_WSS_DEBUG
endif

mqtt_websockets/src/mqtt_wss_client.$(OBJEXT) : CFLAGS += -Wno-unused-result

ACLK_PROTO_DEFINITIONS = \
    aclk/aclk-schemas/proto/aclk/v1/lib.proto \
    aclk/aclk-schemas/proto/agent/v1/disconnect.proto \
    aclk/aclk-schemas/proto/agent/v1/connection.proto \
    aclk/aclk-schemas/proto/alarm/v1/config.proto \
    aclk/aclk-schemas/proto/alarm/v1/stream.proto \
    aclk/aclk-schemas/proto/nodeinstance/connection/v1/connection.proto \
    aclk/aclk-schemas/proto/nodeinstance/create/v1/creation.proto \
    aclk/aclk-schemas/proto/nodeinstance/info/v1/info.proto \
    aclk/aclk-schemas/proto/context/v1/context.proto \
    aclk/aclk-schemas/proto/context/v1/stream.proto \
    aclk/aclk-schemas/proto/agent/v1/cmds.proto \
    $(NULL)

dist_noinst_DATA += $(ACLK_PROTO_DEFINITIONS)

ACLK_PROTO_BUILT_FILES = aclk/aclk-schemas/proto/agent/v1/connection.pb.cc \
    aclk/aclk-schemas/proto/agent/v1/connection.pb.h \
    aclk/aclk-schemas/proto/nodeinstance/connection/v1/connection.pb.cc \
    aclk/aclk-schemas/proto/nodeinstance/connection/v1/connection.pb.h \
    aclk/aclk-schemas/proto/nodeinstance/create/v1/creation.pb.cc \
    aclk/aclk-schemas/proto/nodeinstance/create/v1/creation.pb.h \
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
    aclk/aclk-schemas/proto/context/v1/context.pb.cc \
    aclk/aclk-schemas/proto/context/v1/context.pb.h \
    aclk/aclk-schemas/proto/context/v1/stream.pb.cc \
    aclk/aclk-schemas/proto/context/v1/stream.pb.h \
    aclk/aclk-schemas/proto/agent/v1/cmds.pb.cc \
    aclk/aclk-schemas/proto/agent/v1/cmds.pb.h \
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

aclk/aclk-schemas/proto/context/v1/context.pb.cc \
aclk/aclk-schemas/proto/context/v1/context.pb.h: aclk/aclk-schemas/proto/context/v1/context.proto
	$(PROTOC) -I=aclk/aclk-schemas --cpp_out=$(builddir)/aclk/aclk-schemas $^

aclk/aclk-schemas/proto/context/v1/stream.pb.cc \
aclk/aclk-schemas/proto/context/v1/stream.pb.h: aclk/aclk-schemas/proto/context/v1/stream.proto
	$(PROTOC) -I=aclk/aclk-schemas --cpp_out=$(builddir)/aclk/aclk-schemas $^

aclk/aclk-schemas/proto/agent/v1/cmds.pb.cc \
aclk/aclk-schemas/proto/agent/v1/cmds.pb.h: aclk/aclk-schemas/proto/agent/v1/cmds.proto
	$(PROTOC) -I=aclk/aclk-schemas --cpp_out=$(builddir)/aclk/aclk-schemas $^

endif #ENABLE_ACLK

ACLK_ALWAYS_BUILD_FILES = \
    aclk/aclk_rrdhost_state.h \
    aclk/aclk_proxy.c \
    aclk/aclk_proxy.h \
    aclk/aclk.c \
    aclk/aclk.h \
    aclk/aclk_capas.c \
    aclk/aclk_capas.h \
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
	daemon/event_loop.c \
    daemon/event_loop.h \
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

HTTPD_FILES = \
    httpd/http_server.c \
    httpd/http_server.h \
    httpd/h2o_utils.c \
    httpd/h2o_utils.h \
    $(NULL)

libh2o_a_SOURCES = \
    httpd/h2o/deps/cloexec/cloexec.c \
    httpd/h2o/deps/libgkc/gkc.c \
    httpd/h2o/deps/libyrmcds/close.c \
    httpd/h2o/deps/libyrmcds/connect.c \
    httpd/h2o/deps/libyrmcds/recv.c \
    httpd/h2o/deps/libyrmcds/send.c \
    httpd/h2o/deps/libyrmcds/send_text.c \
    httpd/h2o/deps/libyrmcds/socket.c \
    httpd/h2o/deps/libyrmcds/strerror.c \
    httpd/h2o/deps/libyrmcds/text_mode.c \
    httpd/h2o/deps/picohttpparser/picohttpparser.c \
    httpd/h2o/lib/common/cache.c \
    httpd/h2o/lib/common/file.c \
    httpd/h2o/lib/common/filecache.c \
    httpd/h2o/lib/common/hostinfo.c \
    httpd/h2o/lib/common/http1client.c \
    httpd/h2o/lib/common/memcached.c \
    httpd/h2o/lib/common/memory.c \
    httpd/h2o/lib/common/multithread.c \
    httpd/h2o/lib/common/serverutil.c \
    httpd/h2o/lib/common/socket.c \
    httpd/h2o/lib/common/socketpool.c \
    httpd/h2o/lib/common/string.c \
    httpd/h2o/lib/common/time.c \
    httpd/h2o/lib/common/timeout.c \
    httpd/h2o/lib/common/url.c \
    httpd/h2o/lib/core/config.c \
    httpd/h2o/lib/core/configurator.c \
    httpd/h2o/lib/core/context.c \
    httpd/h2o/lib/core/headers.c \
    httpd/h2o/lib/core/logconf.c \
    httpd/h2o/lib/core/proxy.c \
    httpd/h2o/lib/core/request.c \
    httpd/h2o/lib/core/token.c \
    httpd/h2o/lib/core/util.c \
    httpd/h2o/lib/handler/access_log.c \
    httpd/h2o/lib/handler/chunked.c \
    httpd/h2o/lib/handler/compress.c \
    httpd/h2o/lib/handler/compress/gzip.c \
    httpd/h2o/lib/handler/errordoc.c \
    httpd/h2o/lib/handler/expires.c \
    httpd/h2o/lib/handler/fastcgi.c \
    httpd/h2o/lib/handler/file.c \
    httpd/h2o/lib/handler/headers.c \
    httpd/h2o/lib/handler/mimemap.c \
    httpd/h2o/lib/handler/proxy.c \
    httpd/h2o/lib/handler/redirect.c \
    httpd/h2o/lib/handler/reproxy.c \
    httpd/h2o/lib/handler/throttle_resp.c \
    httpd/h2o/lib/handler/status.c \
    httpd/h2o/lib/handler/headers_util.c \
    httpd/h2o/lib/handler/status/events.c \
    httpd/h2o/lib/handler/status/requests.c \
    httpd/h2o/lib/handler/http2_debug_state.c \
    httpd/h2o/lib/handler/status/durations.c \
    httpd/h2o/lib/handler/configurator/access_log.c \
    httpd/h2o/lib/handler/configurator/compress.c \
    httpd/h2o/lib/handler/configurator/errordoc.c \
    httpd/h2o/lib/handler/configurator/expires.c \
    httpd/h2o/lib/handler/configurator/fastcgi.c \
    httpd/h2o/lib/handler/configurator/file.c \
    httpd/h2o/lib/handler/configurator/headers.c \
    httpd/h2o/lib/handler/configurator/proxy.c \
    httpd/h2o/lib/handler/configurator/redirect.c \
    httpd/h2o/lib/handler/configurator/reproxy.c \
    httpd/h2o/lib/handler/configurator/throttle_resp.c \
    httpd/h2o/lib/handler/configurator/status.c \
    httpd/h2o/lib/handler/configurator/http2_debug_state.c \
    httpd/h2o/lib/handler/configurator/headers_util.c \
    httpd/h2o/lib/http1.c \
    httpd/h2o/lib/tunnel.c \
    httpd/h2o/lib/http2/cache_digests.c \
    httpd/h2o/lib/http2/casper.c \
    httpd/h2o/lib/http2/connection.c \
    httpd/h2o/lib/http2/frame.c \
    httpd/h2o/lib/http2/hpack.c \
    httpd/h2o/lib/http2/scheduler.c \
    httpd/h2o/lib/http2/stream.c \
    httpd/h2o/lib/http2/http2_debug_state.c \
    $(NULL)

libh2o_a_INCLUDES = \
    -I$(srcdir)/httpd/h2o/include \
    -I$(srcdir)/httpd/h2o/deps/cloexec \
    -I$(srcdir)/httpd/h2o/deps/brotli/enc \
    -I$(srcdir)/httpd/h2o/deps/golombset \
    -I$(srcdir)/httpd/h2o/deps/libgkc \
    -I$(srcdir)/httpd/h2o/deps/libyrmcds \
    -I$(srcdir)/httpd/h2o/deps/klib \
    -I$(srcdir)/httpd/h2o/deps/neverbleed \
    -I$(srcdir)/httpd/h2o/deps/picohttpparser \
    -I$(srcdir)/httpd/h2o/deps/picotest \
    -I$(srcdir)/httpd/h2o/deps/yaml/include \
    -I$(srcdir)/httpd/h2o/deps/yoml \
    $(NULL)

if ENABLE_HTTPD
noinst_LIBRARIES += libh2o.a

# until h2o updates support for OpenSSL 3.0 we silence the warnings
libh2o_a_CFLAGS = $(CFLAGS) -Wno-deprecated-declarations -Wno-unused-parameter -Wno-sign-compare -Wno-missing-field-initializers -DH2O_USE_LIBUV=0 $(libh2o_a_INCLUDES)

if LINUX
    libh2o_a_CFLAGS += -D_GNU_SOURCE
endif
endif #ENABLE_HTTPD

NETDATA_FILES = \
    collectors/all.h \
    $(DAEMON_FILES) \
    $(LIBNETDATA_FILES) \
    $(API_PLUGIN_FILES) \
    $(EXPORTING_ENGINE_FILES) \
    $(HEALTH_PLUGIN_FILES) \
    $(ML_FILES) \
    $(IDLEJITTER_PLUGIN_FILES) \
    $(PLUGINSD_PLUGIN_FILES) \
    $(REGISTRY_PLUGIN_FILES) \
    $(RRD_PLUGIN_FILES) \
    $(STREAMING_PLUGIN_FILES) \
    $(STATSD_PLUGIN_FILES) \
    $(WEB_PLUGIN_FILES) \
    $(CLAIM_FILES) \
    $(ACLK_ALWAYS_BUILD_FILES) \
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
    $(ZLIB_LIBS) \
    $(OPTIONAL_SSL_LIBS) \
    $(OPTIONAL_UUID_LIBS) \
    $(OPTIONAL_MQTT_LIBS) \
    $(OPTIONAL_UV_LIBS) \
    $(OPTIONAL_LZ4_LIBS) \
    $(OPTIONAL_DATACHANNEL_LIBS) \
    libjudy.a \
    $(OPTIONAL_SSL_LIBS) \
    $(OPTIONAL_JSONC_LIBS) \
    $(OPTIONAL_YAML_LIBS) \
    $(OPTIONAL_ATOMIC_LIBS) \
    $(OPTIONAL_DL_LIBS) \
    $(NULL)

if ENABLE_ACLK
    NETDATA_COMMON_LIBS += libmqttwebsockets.a
endif

if ENABLE_HTTPD
    NETDATA_FILES += $(HTTPD_FILES)
    NETDATA_COMMON_LIBS += libh2o.a
endif

if LINK_STATIC_JSONC
    NETDATA_COMMON_LIBS += $(abs_top_srcdir)/externaldeps/jsonc/libjson-c.a
endif

if LINK_STATIC_YAML
    NETDATA_COMMON_LIBS += $(abs_top_srcdir)/externaldeps/libyaml/libyaml.a
endif

NETDATACLI_FILES = \
    daemon/commands.h \
    libnetdata/buffer/buffer.c \
    libnetdata/buffer/buffer.h \
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

netdata_LINK = $(CXXLD) $(CXXFLAGS) $(LDFLAGS) -o $@

sbin_PROGRAMS += netdatacli
netdatacli_SOURCES = $(NETDATACLI_FILES)
netdatacli_LDADD = \
    $(NETDATA_COMMON_LIBS) \
    $(NULL)

netdatacli_LINK = $(CXXLD) $(CXXFLAGS) $(LDFLAGS) -o $@

if ENABLE_PLUGIN_APPS
    plugins_PROGRAMS += apps.plugin
    apps_plugin_SOURCES = $(APPS_PLUGIN_FILES)
    apps_plugin_LDADD = \
        $(NETDATA_COMMON_LIBS) \
        $(OPTIONAL_LIBCAP_LIBS) \
        $(NULL)
endif

if ENABLE_PLUGIN_DEBUGFS
    plugins_PROGRAMS += debugfs.plugin
    debugfs_plugin_SOURCES = $(DEBUGFS_PLUGIN_FILES)
    debugfs_plugin_LDADD = \
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
        database/rrdlabels.c \
        database/rrdvar.c \
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
        -Wl,--wrap=rrdlabels_add \
        $(NULL)

endif
