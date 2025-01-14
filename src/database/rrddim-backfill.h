// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_RRDDIM_BACKFILL_H
#define NETDATA_RRDDIM_BACKFILL_H

typedef enum __attribute__ ((__packed__)) {
    RRD_BACKFILL_NONE = 0,
    RRD_BACKFILL_FULL,
    RRD_BACKFILL_NEW
} RRD_BACKFILL;

#include "rrddim.h"

bool backfill_tier_from_smaller_tiers(RRDDIM *rd, size_t tier, time_t now_s);

#endif //NETDATA_RRDDIM_BACKFILL_H
