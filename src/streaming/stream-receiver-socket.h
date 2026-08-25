// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_STREAM_RECEIVER_SOCKET_H
#define NETDATA_STREAM_RECEIVER_SOCKET_H

#include "libnetdata/libnetdata.h"

#define STREAM_RECEIVER_KEEPALIVE_INTERVAL_SECONDS 10
#define STREAM_RECEIVER_KEEPALIVE_PROBE_COUNT 3

typedef enum {
    STREAM_RECEIVER_KEEPALIVE_UNCHANGED,
    STREAM_RECEIVER_KEEPALIVE_APPLIED,
    STREAM_RECEIVER_KEEPALIVE_BASE_FAILED,
    STREAM_RECEIVER_KEEPALIVE_FAILED,
    STREAM_RECEIVER_KEEPALIVE_NON_TCP,
    STREAM_RECEIVER_KEEPALIVE_OPTION_UNSUPPORTED,
} STREAM_RECEIVER_KEEPALIVE_RESULT;

typedef struct stream_receiver_keepalive_state {
    bool base_attempted;
    bool base_applied;
    bool attempted_enabled;
    bool applied_enabled;
    bool non_tcp;
    bool option_unsupported;
    uint32_t attempted_idle_s;
    uint32_t applied_idle_s;
} STREAM_RECEIVER_KEEPALIVE_STATE;

STREAM_RECEIVER_KEEPALIVE_RESULT stream_receiver_socket_keepalive_reconcile(
    int fd,
    bool enabled,
    uint32_t idle_s,
    STREAM_RECEIVER_KEEPALIVE_STATE *state);

int stream_receiver_socket_unittest(void);

#endif // NETDATA_STREAM_RECEIVER_SOCKET_H
