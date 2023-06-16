// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_EBPF_FUNCTIONS_H
#define NETDATA_EBPF_FUNCTIONS_H 1

// configuration file
#define NETDATA_DIRECTORY_FUNCTIONS_CONFIG_FILE "functions.conf"

// function list
#define EBPF_FUNCTION_THREAD "thread"

#define EBPF_PLUGIN_THREAD_FUNCTION_DESCRIPTION "Detailed information on the currently running processes."

#define EBPF_PLUGIN_FUNCTIONS(NAME, DESC) do { \
    fprintf(stdout, PLUGINSD_KEYWORD_FUNCTION " \"" NAME "\" 10 \"%s\"\n", DESC); \
} while(0)


void *ebpf_function_thread(void *ptr);

#endif
