// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_BACKFILL_H
#define NETDATA_BACKFILL_H

#include "database/rrd.h"

struct parser;
struct backfill_request_data {
    size_t rrdhost_receiver_state_id;
    struct parser *parser;
    RRDHOST *host;
    RRDSET *st;
    time_t first_entry_child;
    time_t last_entry_child;
    time_t child_wall_clock_time;
};

typedef bool (*backfill_callback_t)(size_t successful_dims, size_t failed_dims, struct backfill_request_data *brd);

void *backfill_thread(void *ptr);
bool backfill_request_add(RRDSET *st, backfill_callback_t cb, struct backfill_request_data *data);

bool backfill_threads_detect_from_stream_conf(void);

#endif //NETDATA_BACKFILL_H
