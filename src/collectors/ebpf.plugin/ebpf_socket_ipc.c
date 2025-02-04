// SPDX-License-Identifier: GPL-3.0-or-later

#include "ebpf.h"
#include "ebpf_socket_ipc.h"

LISTEN_SOCKETS sockets;

static void ebpf_initialize_sockets()
{
    memset(&sockets, 0, sizeof(sockets));

    sockets.config = &collector_config;
}

void *ebpf_socket_thread_ipc(void *ptr)
{
    ebpf_initialize_sockets();
    return NULL;
}
