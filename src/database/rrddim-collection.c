// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrddim-collection.h"

ALWAYS_INLINE void store_metric_collection_completed() {
    pulse_queries_rrdset_collection_completed(rrdset_done_statistics_points_stored_per_tier);
}

static inline time_t tier_next_point_time_s(RRDDIM *rd, struct rrddim_tier *t, time_t now_s) {
    time_t loop = (time_t)rd->rrdset->update_every * (time_t)t->tier_grouping;
    return now_s + loop - ((now_s + loop) % loop);
}

ALWAYS_INLINE_HOT void store_metric_at_tier(RRDDIM *rd, size_t tier, struct rrddim_tier *t, STORAGE_POINT sp, usec_t now_ut __maybe_unused) {
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

#ifdef NETDATA_LOG_COLLECTION_ERRORS
void rrddim_store_metric_with_trace(RRDDIM *rd, usec_t point_end_time_ut, NETDATA_DOUBLE n, SN_FLAGS flags, const char *function) {
#else // !NETDATA_LOG_COLLECTION_ERRORS
void rrddim_store_metric(RRDDIM *rd, usec_t point_end_time_ut, NETDATA_DOUBLE n, SN_FLAGS flags) {
#endif // !NETDATA_LOG_COLLECTION_ERRORS

    static __thread struct log_stack_entry lgs[] = {
        [0] = ND_LOG_FIELD_STR(NDF_NIDL_DIMENSION, NULL),
        [1] = ND_LOG_FIELD_END(),
    };
    lgs[0].str = rd->id;
    log_stack_push(lgs);

#ifdef NETDATA_LOG_COLLECTION_ERRORS
    rd->rrddim_store_metric_count++;

    if(likely(rd->rrddim_store_metric_count > 1)) {
        usec_t expected = rd->rrddim_store_metric_last_ut + rd->update_every * USEC_PER_SEC;

        if(point_end_time_ut != rd->rrddim_store_metric_last_ut) {
            internal_error(true,
                           "%s COLLECTION: 'host:%s/chart:%s/dim:%s' granularity %d, collection %zu, expected to store at tier 0 a value at %llu, but it gave %llu [%s%llu usec] (called from %s(), previously by %s())",
                           (point_end_time_ut < rd->rrddim_store_metric_last_ut) ? "**PAST**" : "GAP",
                           rrdhost_hostname(rd->rrdset->rrdhost), rrdset_id(rd->rrdset), rrddim_id(rd),
                           rd->update_every,
                           rd->rrddim_store_metric_count,
                           expected, point_end_time_ut,
                           (point_end_time_ut < rd->rrddim_store_metric_last_ut)?"by -" : "gap ",
                           expected - point_end_time_ut,
                           function,
                           rd->rrddim_store_metric_last_caller?rd->rrddim_store_metric_last_caller:"none");
        }
    }

    rd->rrddim_store_metric_last_ut = point_end_time_ut;
    rd->rrddim_store_metric_last_caller = function;
#endif // NETDATA_LOG_COLLECTION_ERRORS

    // store the metric on tier 0
    storage_engine_store_metric(rd->tiers[0].sch, point_end_time_ut,
                                n, 0, 0,
                                1, 0, flags);

    rrdset_done_statistics_points_stored_per_tier[0]++;

    time_t now_s = (time_t)(point_end_time_ut / USEC_PER_SEC);

    STORAGE_POINT sp = {
        .start_time_s = now_s - rd->rrdset->update_every,
        .end_time_s = now_s,
        .min = n,
        .max = n,
        .sum = n,
        .count = 1,
        .anomaly_count = (flags & SN_FLAG_NOT_ANOMALOUS) ? 0 : 1,
        .flags = flags
    };

    for(size_t tier = 1; tier < nd_profile.storage_tiers;tier++) {
        if(unlikely(!rd->tiers[tier].smh)) continue;

        struct rrddim_tier *t = &rd->tiers[tier];

        if(!rrddim_option_check(rd, RRDDIM_OPTION_BACKFILLED_HIGH_TIERS)) {
            // we have not collected this tier before
            // let's fill any gap that may exist
            backfill_tier_from_smaller_tiers(rd, tier, now_s);
        }

        store_metric_at_tier(rd, tier, t, sp, point_end_time_ut);
    }
    rrddim_option_set(rd, RRDDIM_OPTION_BACKFILLED_HIGH_TIERS);

    rrdcontext_collected_rrddim(rd);
    log_stack_pop(&lgs);
}
