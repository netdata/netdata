// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_EBPF_FUNCTIONS_H
#define NETDATA_EBPF_FUNCTIONS_H 1

// configuration file
#define NETDATA_DIRECTORY_FUNCTIONS_CONFIG_FILE "functions.conf"

// function list
#define EBPF_FUNCTION_THREAD "thread"

void *ebpf_function_thread(void *ptr);

#endif
