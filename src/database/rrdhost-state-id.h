// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_RRDHOST_STATE_ID_H
#define NETDATA_RRDHOST_STATE_ID_H

#include "libnetdata/libnetdata.h"

#define RRDHOST_STATE_DISCONNECTED (-100000)

typedef uint32_t RRDHOST_STATE;

struct rrdhost;
RRDHOST_STATE rrdhost_state_id(struct rrdhost *host);

void rrdhost_state_connected(struct rrdhost *host);
void rrdhost_state_disconnected(struct rrdhost *host);

bool rrdhost_state_acquire(struct rrdhost *host, RRDHOST_STATE wanted_state_id);
void rrdhost_state_release(struct rrdhost *host);

#endif //NETDATA_RRDHOST_STATE_ID_H
