// SPDX-License-Identifier: GPL-3.0-or-later

#include "ebpf_socket_ipc.h"

LISTEN_SOCKETS ipc_sockets;

static void ebpf_initialize_sockets()
{
    memset(&ipc_sockets, 0, sizeof(ipc_sockets));

    ipc_sockets.config = &collector_config;
    ipc_sockets.config_section  = NETDATA_EBPF_IPC_INTEGRATION;
}

void *ebpf_socket_thread_ipc(void *ptr)
{
    (void)ptr;

    ebpf_initialize_sockets();
    return NULL;
}
