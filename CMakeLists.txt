
# SPDX-License-Identifier: GPL-3.0-or-later
# This file is only used for development (netdata in Clion)
# It can build netdata, but you are on your own...

cmake_minimum_required(VERSION 3.3)
project(netdata C)

find_package (Threads)

set(CMAKE_C_FLAGS "${CMAKE_C_FLAGS} -Wall -Wextra -Wformat-signedness -Werror=format-security -DNETDATA_INTERNAL_CHECKS=1 -DNETDATA_VERIFY_LOCKS=1 -fstack-protector-all")

set(NETDATA_FREEBSD_FILES
        src/plugin_freebsd.c
        src/plugin_freebsd.h
        src/freebsd_sysctl.c
        src/freebsd_getmntinfo.c
        src/freebsd_getifaddrs.c
        src/freebsd_devstat.c
        src/zfs_common.c
        src/zfs_common.h
        src/freebsd_kstat_zfs.c
        src/freebsd_ipfw.c
        )

set(NETDATA_MACOS_FILES
        src/plugin_macos.c
        src/plugin_macos.h
        src/macos_sysctl.c
        src/macos_mach_smi.c
        src/macos_fw.c
        )

set(NETDATA_LINUX_FILES
        src/ipc.c
        src/ipc.h
        src/plugin_proc.c
        src/plugin_proc.h
        src/plugin_proc_diskspace.c
        src/plugin_proc_diskspace.h
        src/proc_diskstats.c
        src/proc_interrupts.c
        src/proc_softirqs.c
        src/proc_loadavg.c
        src/proc_meminfo.c
        src/proc_net_dev.c
        src/proc_net_ip_vs_stats.c
        src/proc_net_netstat.c
        src/proc_net_rpc_nfs.c
        src/proc_net_rpc_nfsd.c
        src/proc_net_snmp.c
        src/proc_net_snmp6.c
        src/proc_net_sctp_snmp.c
        src/proc_net_sockstat.c
        src/proc_net_sockstat6.c
        src/proc_net_softnet_stat.c
        src/proc_net_stat_conntrack.c
        src/proc_net_stat_synproxy.c
        src/zfs_common.c
        src/zfs_common.h
        src/proc_spl_kstat_zfs.c
        src/proc_stat.c
        src/proc_sys_kernel_random_entropy_avail.c
        src/proc_vmstat.c
        src/proc_uptime.c
        src/sys_kernel_mm_ksm.c
        src/sys_fs_cgroup.c
        src/sys_fs_btrfs.c
        src/threads.c src/threads.h)

set(NETDATA_COMMON_FILES
        src/adaptive_resortable_list.c
        src/adaptive_resortable_list.h
        src/appconfig.c
        src/appconfig.h
        src/avl.c
        src/avl.h
        src/backend_prometheus.c
        src/backend_prometheus.h
        src/backends.c
        src/backends.h
        src/clocks.c
        src/clocks.h
        src/common.c
        src/common.h
        src/daemon.c
        src/daemon.h
        src/dictionary.c
        src/dictionary.h
        src/eval.c
        src/eval.h
        src/global_statistics.c
        src/global_statistics.h
        src/health.c
        src/health.h
        src/health_config.c
        src/health_json.c
        src/health_log.c
        src/inlined.h
        src/locks.c
        src/locks.h
        src/log.c
        src/log.h
        src/main.c
        src/main.h
        src/plugin_checks.c
        src/plugin_checks.h
        src/plugin_idlejitter.c
        src/plugin_idlejitter.h
        src/plugin_nfacct.c
        src/plugin_nfacct.h
        src/plugin_tc.c
        src/plugin_tc.h
        src/plugins_d.c
        src/plugins_d.h
        src/popen.c
        src/popen.h
        src/proc_self_mountinfo.c
        src/proc_self_mountinfo.h
        src/procfile.c
        src/procfile.h
        src/registry.c
        src/registry.h
        src/registry_db.c
        src/registry_init.c
        src/registry_internals.c
        src/registry_internals.h
        src/registry_log.c
        src/registry_machine.c
        src/registry_machine.h
        src/registry_person.c
        src/registry_person.h
        src/registry_url.c
        src/registry_url.h
        src/rrd.c
        src/rrd.h
        src/rrd2json.c
        src/rrd2json.h
        src/rrdcalc.c
        src/rrdcalctemplate.c
        src/rrddim.c
        src/rrddimvar.c
        src/rrdfamily.c
        src/rrdhost.c
        src/rrdpush.c
        src/rrdpush.h
        src/rrdset.c
        src/rrdsetvar.c
        src/rrdvar.c
        src/signals.c
        src/signals.h
        src/simple_pattern.c
        src/simple_pattern.h
        src/socket.c
        src/socket.h
        src/statistical.c
        src/statistical.h
        src/statsd.c
        src/statsd.h
        src/storage_number.c
        src/storage_number.h
        src/sys_devices_system_edac_mc.c
        src/sys_devices_system_node.c
        src/unit_test.c
        src/unit_test.h
        src/url.c
        src/url.h
        src/web_api_v1.c
        src/web_api_v1.h
        src/web_buffer.c
        src/web_buffer.h
        src/web_buffer_svg.c
        src/web_buffer_svg.h
        src/web_client.c
        src/web_client.h
        src/web_server.c
        src/web_server.h
        config.h
        )

set(APPS_PLUGIN_SOURCE_FILES
        src/appconfig.c
        src/appconfig.h
        src/apps_plugin.c
        src/avl.c
        src/avl.h
        src/adaptive_resortable_list.c
        src/adaptive_resortable_list.h
        src/common.c
        src/common.h
        src/clocks.c
        src/clocks.h
        src/inlined.h
        src/locks.c
        src/locks.h
        src/log.c
        src/log.h
        src/procfile.c
        src/procfile.h
        src/threads.c
        src/threads.h
        src/web_buffer.c
        src/web_buffer.h
        config.h
        )

set(FREEIPMI_PLUGIN_SOURCE_FILES
        src/freeipmi_plugin.c
        src/common.c
        src/common.h
        src/clocks.c
        src/clocks.h
        src/inlined.h
        src/locks.c
        src/locks.h
        src/log.c
        src/log.h
        src/procfile.c
        src/procfile.h
        src/threads.c
        src/threads.h
        config.h
        )

set(CGROUP_NETWORK_SOURCE_FILES
        src/cgroup-network.c
        src/common.c
        src/common.h
        src/clocks.c
        src/clocks.h
        src/inlined.h
        src/locks.c
        src/locks.h
        src/log.c
        src/log.h
        src/popen.c
        src/popen.h
        src/procfile.c
        src/procfile.h
        src/signals.c
        src/signals.h
        src/storage_number.c
        src/storage_number.h
        src/threads.c
        src/threads.h
        src/web_buffer.c
        src/web_buffer.h
        config.h
        )

include_directories(AFTER .)

add_definitions(-DHAVE_CONFIG_H -DCACHE_DIR="/var/cache/netdata" -DCONFIG_DIR="/etc/netdata" -DLIBCONFIG_DIR="/usr/lib/netdata/conf.d" -DLOG_DIR="/var/log/netdata" -DPLUGINS_DIR="/usr/libexec/netdata" -DWEB_DIR="/usr/share/netdata" -DVARLIB_DIR="/var/lib/netdata")

add_executable(netdata ${NETDATA_COMMON_FILES} ${NETDATA_LINUX_FILES})
target_link_libraries (netdata m z uuid mnl netfilter_acct ${CMAKE_THREAD_LIBS_INIT})

add_executable(netdata_freebsd ${NETDATA_COMMON_FILES} ${NETDATA_FREEBSD_FILES})
target_link_libraries (netdata_freebsd m z uuid ${CMAKE_THREAD_LIBS_INIT})

add_executable(netdata_macos ${NETDATA_COMMON_FILES} ${NETDATA_MACOS_FILES})
target_link_libraries (netdata_macos m z uuid ${CMAKE_THREAD_LIBS_INIT})

add_executable(apps.plugin ${APPS_PLUGIN_SOURCE_FILES} src/inlined.h)
target_link_libraries (apps.plugin m cap ${CMAKE_THREAD_LIBS_INIT})

add_executable(freeipmi.plugin ${FREEIPMI_PLUGIN_SOURCE_FILES})
target_link_libraries (freeipmi.plugin ipmimonitoring ${CMAKE_THREAD_LIBS_INIT})

add_executable(cgroup-network ${CGROUP_NETWORK_SOURCE_FILES})
target_link_libraries (cgroup-network m ${CMAKE_THREAD_LIBS_INIT})
