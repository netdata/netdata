// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrddim-backfill.h"
#include "database/rrddim-collection.h"

// ----------------------------------------------------------------------------
// fill the gap of a tier

NOT_INLINE_HOT bool backfill_tier_from_smaller_tiers(RRDDIM *rd, size_t tier, time_t now_s) {
    if(unlikely(tier >= nd_profile.storage_tiers)) return false;
#ifdef ENABLE_DBENGINE
    if(default_backfill == RRD_BACKFILL_NONE) return false;
#else
    return false;
#endif

    struct rrddim_tier *t = &rd->tiers[tier];
    if(unlikely(!t)) return false;

    time_t latest_time_s = storage_engine_latest_time_s(t->seb, t->smh);
    time_t granularity = (time_t)t->tier_grouping * (time_t)rd->rrdset->update_every;
    time_t time_diff   = now_s - latest_time_s;

    // if the user wants only NEW backfilling, and we don't have any data
#ifdef ENABLE_DBENGINE
    if(default_backfill == RRD_BACKFILL_NEW && latest_time_s <= 0) return false;
#else
    return;
#endif

    // there is really nothing we can do
    if(now_s <= latest_time_s || time_diff < granularity) return false;

    stream_control_backfill_query_started();

    // for each lower tier
    struct storage_engine_query_handle seqh;
    for(int read_tier = (int)tier - 1; read_tier >= 0 ; read_tier--){
        time_t smaller_tier_first_time = storage_engine_oldest_time_s(rd->tiers[read_tier].seb, rd->tiers[read_tier].smh);
        time_t smaller_tier_last_time = storage_engine_latest_time_s(rd->tiers[read_tier].seb, rd->tiers[read_tier].smh);
        if(smaller_tier_last_time <= latest_time_s) continue;  // it is as bad as we are

        long after_wanted = (latest_time_s < smaller_tier_first_time) ? smaller_tier_first_time : latest_time_s;
        long before_wanted = smaller_tier_last_time;

        struct rrddim_tier *tmp = &rd->tiers[read_tier];
        storage_engine_query_init(tmp->seb, tmp->smh, &seqh, after_wanted, before_wanted, STORAGE_PRIORITY_SYNCHRONOUS_FIRST);

        size_t points_read = 0;

        while(!storage_engine_query_is_finished(&seqh)) {

            STORAGE_POINT sp = storage_engine_query_next_metric(&seqh);
            points_read++;

            if(sp.end_time_s > latest_time_s) {
                latest_time_s = sp.end_time_s;
                store_metric_at_tier(rd, tier, t, sp, sp.end_time_s * USEC_PER_SEC);
            }
        }

        storage_engine_query_finalize(&seqh);
        store_metric_collection_completed();
        pulse_queries_backfill_query_completed(points_read);

        //internal_error(true, "DBENGINE: backfilled chart '%s', dimension '%s', tier %d, from %ld to %ld, with %zu points from tier %d",
        //               rd->rrdset->name, rd->name, tier, after_wanted, before_wanted, points, tr);
    }

    stream_control_backfill_query_finished();

    return true;
}
