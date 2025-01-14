// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrddim-collection.h"

void store_metric_collection_completed() {
    pulse_queries_rrdset_collection_completed(rrdset_done_statistics_points_stored_per_tier);
}

static inline time_t tier_next_point_time_s(RRDDIM *rd, struct rrddim_tier *t, time_t now_s) {
    time_t loop = (time_t)rd->rrdset->update_every * (time_t)t->tier_grouping;
    return now_s + loop - ((now_s + loop) % loop);
}

void store_metric_at_tier(RRDDIM *rd, size_t tier, struct rrddim_tier *t, STORAGE_POINT sp, usec_t now_ut __maybe_unused) {
    if (unlikely(!t->next_point_end_time_s))
        t->next_point_end_time_s = tier_next_point_time_s(rd, t, sp.end_time_s);

    if(unlikely(sp.start_time_s >= t->next_point_end_time_s)) {
        // flush the virtual point, it is done

        if (likely(!storage_point_is_unset(t->virtual_point))) {

            storage_engine_store_metric(
                t->sch,
                t->next_point_end_time_s * USEC_PER_SEC,
                t->virtual_point.sum,
                t->virtual_point.min,
                t->virtual_point.max,
                t->virtual_point.count,
                t->virtual_point.anomaly_count,
                t->virtual_point.flags);
        }
        else {
            storage_engine_store_metric(
                t->sch,
                t->next_point_end_time_s * USEC_PER_SEC,
                NAN,
                NAN,
                NAN,
                0,
                0, SN_FLAG_NONE);
        }

        rrdset_done_statistics_points_stored_per_tier[tier]++;
        t->virtual_point.count = 0; // make the point unset
        t->next_point_end_time_s = tier_next_point_time_s(rd, t, sp.end_time_s);
    }

    // merge the dates into our virtual point
    if (unlikely(sp.start_time_s < t->virtual_point.start_time_s))
        t->virtual_point.start_time_s = sp.start_time_s;

    if (likely(sp.end_time_s > t->virtual_point.end_time_s))
        t->virtual_point.end_time_s = sp.end_time_s;

    // merge the values into our virtual point
    if (likely(!storage_point_is_gap(sp))) {
        // we aggregate only non NULLs into higher tiers

        if (likely(!storage_point_is_unset(t->virtual_point))) {
            // merge the collected point to our virtual one
            t->virtual_point.sum += sp.sum;
            t->virtual_point.min = MIN(t->virtual_point.min, sp.min);
            t->virtual_point.max = MAX(t->virtual_point.max, sp.max);
            t->virtual_point.count += sp.count;
            t->virtual_point.anomaly_count += sp.anomaly_count;
            t->virtual_point.flags |= sp.flags;
        }
        else {
            // reset our virtual point to this one
            t->virtual_point = sp;
        }
    }
}
