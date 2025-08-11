// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_EBPF_FUNCTIONS_H
#define NETDATA_EBPF_FUNCTIONS_H 1

// Common
static inline void EBPF_PLUGIN_FUNCTIONS(const char *NAME, const char *DESC, int update_every)
{
    fprintf(
        stdout,
        PLUGINSD_KEYWORD_FUNCTION " GLOBAL \"%s\" %d \"%s\" \"top\" " HTTP_ACCESS_FORMAT " %d\n",
        NAME,
        update_every,
        DESC,
        (HTTP_ACCESS_FORMAT_CAST)(HTTP_ACCESS_SIGNED_ID | HTTP_ACCESS_SAME_SPACE | HTTP_ACCESS_SENSITIVE_DATA),
        RRDFUNCTIONS_PRIORITY_DEFAULT);
}

// configuration file & description
#define NETDATA_DIRECTORY_FUNCTIONS_CONFIG_FILE "functions.conf"
#define NETDATA_EBPF_FUNCTIONS_MODULE_DESC "Show information about current function status."

// function list
#define EBPF_FUNCTION_SOCKET "network-sockets-tracing"

// socket constants
#define EBPF_PLUGIN_SOCKET_FUNCTION_DESCRIPTION "Detailed information about open sockets."
#define EBPF_FUNCTION_SOCKET_FAMILY "family:"
#define EBPF_FUNCTION_SOCKET_PERIOD "period:"
#define EBPF_FUNCTION_SOCKET_RESOLVE "resolve:"
#define EBPF_FUNCTION_SOCKET_RANGE "range:"
#define EBPF_FUNCTION_SOCKET_PORT "port:"
#define EBPF_FUNCTION_SOCKET_RESET "reset"
#define EBPF_FUNCTION_SOCKET_INTERFACES "interfaces"

void ebpf_function_thread(void *ptr);

#endif
