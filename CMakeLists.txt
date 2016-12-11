
# This file is only used for development (open netdata in Clion)
# It can build netdata, but you are on your own...

cmake_minimum_required(VERSION 3.3)
project(netdata C)

find_package (Threads)

set(CMAKE_C_FLAGS "${CMAKE_C_FLAGS} -Wall -Wextra -Wformat-signedness -Werror=format-security -DNETDATA_INTERNAL_CHECKS=1")

set(NETDATA_SOURCE_FILES
        src/appconfig.c
        src/appconfig.h
        src/avl.c
        src/avl.h
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
        src/ipc.c
        src/ipc.h
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
        src/proc_net_rpc_nfs.c
        src/proc_net_rpc_nfsd.c
        src/proc_net_snmp6.c
        src/proc_net_snmp.c
        src/proc_net_softnet_stat.c
        src/proc_net_stat_conntrack.c
        src/proc_net_stat_synproxy.c
        src/proc_self_mountinfo.c
        src/proc_self_mountinfo.h
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
        src/sys_fs_cgroup.c
        src/sys_kernel_mm_ksm.c
        src/unit_test.c
        src/unit_test.h
        src/url.c
        src/url.h
        src/web_buffer.c
        src/web_buffer.h
        src/web_buffer_svg.c
        src/web_buffer_svg.h
        src/web_client.c
        src/web_client.h
        src/web_server.c
        src/web_server.h
)

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
        src/procfile.c
        src/procfile.h
        src/web_buffer.c
        src/web_buffer.h
        config.h)

include_directories(AFTER .)

add_definitions(-DHAVE_CONFIG_H -DCACHE_DIR="/var/cache/netdata" -DCONFIG_DIR="/etc/netdata" -DLOG_DIR="/var/log/netdata" -DPLUGINS_DIR="/usr/libexec/netdata" -DWEB_DIR="/usr/share/netdata" -DVARLIB_DIR="/var/lib/netdata")

add_executable(netdata ${NETDATA_SOURCE_FILES})
target_link_libraries (netdata m z uuid ${CMAKE_THREAD_LIBS_INIT})

add_executable(apps.plugin ${APPS_PLUGIN_SOURCE_FILES})
target_link_libraries (apps.plugin m ${CMAKE_THREAD_LIBS_INIT})
