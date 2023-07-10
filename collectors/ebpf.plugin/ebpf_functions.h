// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_EBPF_FUNCTIONS_H
#define NETDATA_EBPF_FUNCTIONS_H 1

// configuration file & description
#define NETDATA_DIRECTORY_FUNCTIONS_CONFIG_FILE "functions.conf"
#define NETDATA_EBPF_FUNCTIONS_MODULE_DESC "Show information about current function status."

// function list
#define EBPF_FUNCTION_THREAD "ebpf_thread"

#define EBPF_PLUGIN_THREAD_FUNCTION_DESCRIPTION "Detailed information about eBPF threads."
#define EBPF_PLUGIN_THREAD_FUNCTION_ERROR_THREAD_NOT_FOUND "ebpf.plugin does not have thread named "

#define EBPF_PLUGIN_FUNCTIONS(NAME, DESC) do { \
    fprintf(stdout, PLUGINSD_KEYWORD_FUNCTION " \"" NAME "\" 10 \"%s\"\n", DESC); \
} while(0)

#define EBPF_THREADS_SELECT_THREAD "thread:"
#define EBPF_THREADS_ENABLE_CATEGORY "enable:"
#define EBPF_THREADS_DISABLE_CATEGORY "disable:"

#define EBPF_THREAD_STATUS_RUNNING "running"
#define EBPF_THREAD_STATUS_STOPPED "stopped"

void *ebpf_function_thread(void *ptr);

#endif
