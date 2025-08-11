// SPDX-License-Identifier: GPL-3.0-or-later

#include "ebpf_socket_ipc.h"

LISTEN_SOCKETS ipc_sockets;

static void ebpf_initialize_sockets()
{
    memset(&ipc_sockets, 0, sizeof(ipc_sockets));

    ipc_sockets.config = &collector_config;
    ipc_sockets.config_section = NETDATA_EBPF_IPC_INTEGRATION;
}

// Receive data
static int ebpf_ipc_rcv_callback(POLLINFO *pi, nd_poll_event_t *events)
{
    (void)pi;
    (void)events;

    return 0;
}

static int ebpf_ipc_snd_callback(POLLINFO *pi __maybe_unused, nd_poll_event_t *events __maybe_unused)
{
    (void)pi;
    (void)events;

    return 0;
}

static bool ebpf_ipc_should_stop(void)
{
    return false;
}

void ebpf_socket_thread_ipc(void *ptr)
{
    (void)ptr;

    ebpf_initialize_sockets();

    poll_events(
        &ipc_sockets,
        NULL,
        NULL,
        ebpf_ipc_rcv_callback,
        ebpf_ipc_snd_callback,
        NULL,
        ebpf_ipc_should_stop,
        NULL // No access control pattern
        ,
        0 // No dns lookups for access control pattern
        ,
        NULL,
        0 // tcp request timeout, 0 = disabled
        ,
        0 // tcp idle timeout, 0 = disabled
        ,
        EBPF_DEFAULT_UPDATE_EVERY * 1000,
        ptr,
        0 // We are going to use UDP
    );
}
