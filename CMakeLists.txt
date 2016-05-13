
# This file is just a hack to make netdata
# open in Clion
# It cannot build netdata

cmake_minimum_required(VERSION 3.3)
project(netdata)

set(CMAKE_CXX_FLAGS "${CMAKE_CXX_FLAGS} -std=c++11")

set(NETDATA_SOURCE_FILES
	src/appconfig.c
	src/appconfig.h
	src/avl.c
	src/avl.h
	src/common.c
	src/common.h
	src/daemon.c
	src/daemon.h
	src/dictionary.c
	src/dictionary.h
	src/global_statistics.c
	src/global_statistics.h
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
	src/plugin_proc.c
	src/plugin_proc.h
	src/plugins_d.c
	src/plugins_d.h
	src/plugin_tc.c
	src/plugin_tc.h
	src/popen.c
	src/popen.h
	src/proc_diskstats.c
	src/procfile.c
	src/procfile.h
	src/proc_interrupts.c
	src/proc_loadavg.c
	src/proc_meminfo.c
	src/proc_net_dev.c
	src/proc_net_ip_vs_stats.c
	src/proc_net_netstat.c
	src/proc_net_rpc_nfsd.c
	src/proc_net_snmp6.c
	src/proc_net_snmp.c
	src/proc_net_stat_conntrack.c
	src/proc_net_stat_synproxy.c
	src/proc_softirqs.c
	src/proc_stat.c
	src/proc_sys_kernel_random_entropy_avail.c
	src/proc_vmstat.c
	src/registry.c
	src/registry.h
	src/rrd2json.c
	src/rrd2json.h
	src/rrd.c
	src/rrd.h
	src/storage_number.c
	src/storage_number.h
	src/sys_kernel_mm_ksm.c
	src/sys_fs_cgroup.c
	src/unit_test.c
	src/unit_test.h
	src/url.c
	src/url.h
	src/web_buffer.c
	src/web_buffer.h
	src/web_client.c
	src/web_client.h
	src/web_server.c
	src/web_server.h
        config.h)

set(APPS_PLUGIN_SOURCE_FILES
	src/appconfig.c
	src/appconfig.h
	src/apps_plugin.c
	src/avl.c
	src/avl.h
	src/common.c
	src/common.h
	src/log.c
	src/log.h
        config.h)

include_directories(AFTER .)

add_definitions(-DHAVE_CONFIG_H -DNETDATA_INTERNAL_CHECKS=1 -DCACHE_DIR="/tmp" -DCONFIG_DIR="/tmp" -DLOG_DIR="/tmp" -DPLUGINS_DIR="/tmp" -DWEB_DIR="/tmp" -DVARLIB_DIR="/tmp")

add_executable(netdata ${NETDATA_SOURCE_FILES})
add_executable(apps.plugin ${APPS_PLUGIN_SOURCE_FILES})
