// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_EBPF_SOCKET_IPC_H
# define NETDATA_EBPF_SOCKET_IPC_H 1

#define NETDATA_EBPF_IPC_SECTION "ipc"
#define NETDATA_EBPF_IPC_INTEGRATION "integration"
#define NETDATA_EBPF_IPC_BACKLOG "backlog"
#define NETDATA_EBPF_IPC_BIND_TO "bind to"
#define NETDATA_EBPF_IPC_BIND_TO_DEFAULT "unix:/tmp/netdata_ebpf_sock"

#define NETDATA_EBPF_IPC_INTEGRATION_SHM "shm"
#define NETDATA_EBPF_IPC_INTEGRATION_SOCKET "socket"
#define NETDATA_EBPF_IPC_INTEGRATION_DISABLED "disabled"

#include "ebpf.h"

enum ebpf_integration_list { NETDATA_EBPF_INTEGRATION_DISABLED,
                             NETDATA_EBPF_INTEGRATION_SOCKET,
                             NETDATA_EBPF_INTEGRATION_SHM
};

extern LISTEN_SOCKETS ipc_sockets;

void *ebpf_socket_thread_ipc(void *ptr);

#endif /* NETDATA_EBPF_SOCKET_IPC_H_ */
