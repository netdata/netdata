// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_BACKFILL_H
#define NETDATA_BACKFILL_H

#include "database/rrd.h"

typedef void (*backfill_callback_t)(size_t successful_dims, size_t failed_dims, void *data);

void *backfill_thread(void *ptr);
bool backfill_request_add(RRDSET *st, backfill_callback_t cb, void *data);

#endif //NETDATA_BACKFILL_H
