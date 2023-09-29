// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_EBPF_FUNCTIONS_H
#define NETDATA_EBPF_FUNCTIONS_H 1

#ifdef NETDATA_DEV_MODE
// Common
static inline void EBPF_PLUGIN_FUNCTIONS(const char *NAME, const char *DESC) {
    fprintf(stdout, "%s \"%s\" 10 \"%s\"\n", PLUGINSD_KEYWORD_FUNCTION, NAME, DESC);
}
#endif

// configuration file & description
#define NETDATA_DIRECTORY_FUNCTIONS_CONFIG_FILE "functions.conf"
#define NETDATA_EBPF_FUNCTIONS_MODULE_DESC "Show information about current function status."

// function list
#define EBPF_FUNCTION_THREAD "ebpf_thread"
#define EBPF_FUNCTION_SOCKET "ebpf_socket"

// thread constants
#define EBPF_PLUGIN_THREAD_FUNCTION_DESCRIPTION "Detailed information about eBPF threads."
#define EBPF_PLUGIN_THREAD_FUNCTION_ERROR_THREAD_NOT_FOUND "ebpf.plugin does not have thread named "

#define EBPF_THREADS_SELECT_THREAD "thread:"
#define EBPF_THREADS_ENABLE_CATEGORY "enable:"
#define EBPF_THREADS_DISABLE_CATEGORY "disable:"

#define EBPF_THREAD_STATUS_RUNNING "running"
#define EBPF_THREAD_STATUS_STOPPED "stopped"

// socket constants
#define EBPF_PLUGIN_SOCKET_FUNCTION_DESCRIPTION "Detailed information about open sockets."
#define EBPF_FUNCTION_SOCKET_FAMILY "family:"
#define EBPF_FUNCTION_SOCKET_PERIOD "period:"
#define EBPF_FUNCTION_SOCKET_RESOLVE "resolve:"
#define EBPF_FUNCTION_SOCKET_RANGE "range:"
#define EBPF_FUNCTION_SOCKET_PORT "port:"
#define EBPF_FUNCTION_SOCKET_RESET "reset"
#define EBPF_FUNCTION_SOCKET_INTERFACES "interfaces"

void *ebpf_function_thread(void *ptr);

#endif
