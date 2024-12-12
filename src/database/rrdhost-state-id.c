// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrdhost-state-id.h"
#include "rrd.h"

uint32_t rrdhost_state_id(struct rrdhost *host) {
    return __atomic_load_n(&host->stream.rcv.status.state_id, __ATOMIC_RELAXED);
}

uint32_t stream_receiver_host_state_id_increment(RRDHOST *host) {
    return __atomic_add_fetch(&host->stream.rcv.status.state_id, 1, __ATOMIC_RELAXED);
}

