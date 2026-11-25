// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_RRDSET_COLLECTION_H
#define NETDATA_RRDSET_COLLECTION_H

#include "rrd.h"

extern __thread size_t rrdset_done_statistics_points_stored_per_tier[RRD_STORAGE_TIERS];

void rrdset_next_usec_unfiltered(RRDSET *st, usec_t microseconds);
void rrdset_next_usec(RRDSET *st, usec_t microseconds);
void rrdset_timed_next(RRDSET *st, struct timeval now, usec_t microseconds);
#define rrdset_next(st) rrdset_next_usec(st, 0ULL)

void rrdset_timed_done(RRDSET *st, struct timeval now, bool pending_rrdset_next);
void rrdset_done(RRDSET *st);

void rrdset_finalize_collection(RRDSET *st, bool dimensions_too);
bool rrdset_update_every_is_valid(RRDHOST *host, time_t update_every_s);
time_t rrdset_set_update_every_s(RRDSET *st, time_t update_every_s);

// The update every of a storage tier of the chart. rrdset_update_every_is_valid() keeps it within INT32_MAX.
static inline uint32_t rrdset_tier_update_every_s(RRDSET *st, size_t tier) {
    return st->rrdhost->db[tier].tier_grouping * (uint32_t)st->update_every;
}

#endif //NETDATA_RRDSET_COLLECTION_H
