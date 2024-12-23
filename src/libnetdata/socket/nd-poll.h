// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_ND_POLL_H
#define NETDATA_ND_POLL_H

#include "libnetdata/common.h"

typedef enum {
    ND_POLL_READ            = 1 << 0,
    ND_POLL_WRITE           = 1 << 1,
    ND_POLL_ERROR           = 1 << 2,
    ND_POLL_HUP             = 1 << 3,
    ND_POLL_INVALID         = 1 << 4,
    ND_POLL_TIMEOUT         = 1 << 5,
    ND_POLL_OTHER_ERROR     = 1 << 6,
} nd_poll_event_t;

typedef struct {
    nd_poll_event_t events;
    void *data;
} nd_poll_result_t;

typedef struct nd_poll_t nd_poll_t;

nd_poll_t *nd_poll_create();
bool nd_poll_add(nd_poll_t *ndpl, int fd, nd_poll_event_t events, void *data);
bool nd_poll_del(nd_poll_t *ndpl, int fd);
bool nd_poll_upd(nd_poll_t *ndpl, int fd, nd_poll_event_t events, void *data);

// returns -1 = error, 0 = timeout, 1 = event in result
int nd_poll_wait(nd_poll_t *ndpl, int timeout_ms, nd_poll_result_t *result);

void nd_poll_destroy(nd_poll_t *ndpl);

#include "libnetdata/libnetdata.h"

#endif //NETDATA_ND_POLL_H
