// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_RRDHOST_STATE_ID_H
#define NETDATA_RRDHOST_STATE_ID_H

#include "libnetdata/libnetdata.h"

typedef uint32_t RRDHOST_STATE;

struct rrdhost;
RRDHOST_STATE rrdhost_state_id(struct rrdhost *host);
RRDHOST_STATE rrdhost_state_id_increment(struct rrdhost *host);

#endif //NETDATA_RRDHOST_STATE_ID_H
