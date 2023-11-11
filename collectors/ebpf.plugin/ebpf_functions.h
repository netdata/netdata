// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_EBPF_FUNCTIONS_H
#define NETDATA_EBPF_FUNCTIONS_H 1

#ifdef NETDATA_DEV_MODE
// Common
static inline void EBPF_PLUGIN_FUNCTIONS(const char *NAME, const char *DESC) {
    fprintf(stdout, "%s \"%s\" 10 \"%s\" \"top\" \"any\" %d\n",
            PLUGINSD_KEYWORD_FUNCTION, NAME, DESC, RRDFUNCTIONS_PRIORITY_DEFAULT);
}
#endif

// configuration file & description
#define NETDATA_DIRECTORY_FUNCTIONS_CONFIG_FILE "functions.conf"
#define NETDATA_EBPF_FUNCTIONS_MODULE_DESC "Show information about current function status."

// Common macros used witth functions
#define NETDATA_EBPF_FUNCTIONS_COMMON_HELP "help"
#define EBPF_FUNCTION_OPTION_PERIOD "period:"
#define EBPF_NOT_IDENFIED "not identified"
#define EBPF_APPS_GROUP_OTHER "other"

// function list
#define EBPF_FUNCTION_THREAD "eBPF-thread"
#define EBPF_FUNCTION_SOCKET "eBPF-socket"
#define EBPF_FUNCTION_CACHESTAT "eBPF-cachestat"
#define EBPF_FUNCTION_FD "eBPF-fd"
#define EBPF_FUNCTION_PROCESS "eBPF-process"
#define EBPF_FUNCTION_SHM "eBPF-shm"
#define EBPF_FUNCTION_SWAP "eBPF-swap"
#define EBPF_FUNCTION_DC "eBPF-dc"
#define EBPF_FUNCTION_VFS "eBPF-vfs"

// socket constants
#define EBPF_PLUGIN_SOCKET_FUNCTION_DESCRIPTION "Detailed information about open sockets."
#define EBPF_FUNCTION_SOCKET_FAMILY "family:"
#define EBPF_FUNCTION_SOCKET_RESOLVE "resolve:"
#define EBPF_FUNCTION_SOCKET_RANGE "range:"
#define EBPF_FUNCTION_SOCKET_PORT "port:"
#define EBPF_FUNCTION_SOCKET_RESET "reset"
#define EBPF_FUNCTION_SOCKET_INTERFACES "interfaces"

// cachestat constants
#define EBPF_PLUGIN_CACHESTAT_FUNCTION_DESCRIPTION "Detailed information about how processes are accessing the linux page cache during runtime."

// fd constants
#define EBPF_PLUGIN_FD_FUNCTION_DESCRIPTION "Detailed information about how processes are opening and closing files."

// Process constants
#define EBPF_PLUGIN_PROCESS_FUNCTION_DESCRIPTION "Detailed information about how processes lifetime."

// SHM constants
#define EBPF_PLUGIN_SHM_FUNCTION_DESCRIPTION "Detailed information about how share memory calls."

// SWAP constants
#define EBPF_PLUGIN_SWAP_FUNCTION_DESCRIPTION "Detailed information about swap memory access."

// DC constants
#define EBPF_PLUGIN_DC_FUNCTION_DESCRIPTION "Detailed information about directory cache access."

// VFS constants
#define EBPF_PLUGIN_VFS_FUNCTION_DESCRIPTION "Detailed information about Virtual File System access."

void *ebpf_function_thread(void *ptr);

#endif
