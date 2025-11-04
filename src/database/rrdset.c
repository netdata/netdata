// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrdset.h"
#include "storage-engine.h"

void rrdset_metadata_updated(RRDSET *st) {
    __atomic_add_fetch(&st->version, 1, __ATOMIC_RELAXED);
    rrdcontext_updated_rrdset(st);
}

// ----------------------------------------------------------------------------

// get the timestamp of the last entry in the round-robin database
time_t rrdset_last_entry_s(RRDSET *st) {
    RRDDIM *rd;
    time_t last_entry_s  = 0;

    rrddim_foreach_read(rd, st) {
        time_t t = rrddim_last_entry_s(rd);
        if(t > last_entry_s) last_entry_s = t;
    }
    rrddim_foreach_done(rd);

    return last_entry_s;
}

time_t rrdset_last_entry_s_of_tier(RRDSET *st, size_t tier) {
    RRDDIM *rd;
    time_t last_entry_s  = 0;

    rrddim_foreach_read(rd, st) {
                time_t t = rrddim_last_entry_s_of_tier(rd, tier);
                if(t > last_entry_s) last_entry_s = t;
            }
    rrddim_foreach_done(rd);

    return last_entry_s;
}

// get the timestamp of first entry in the round-robin database
time_t rrdset_first_entry_s(RRDSET *st) {
    RRDDIM *rd;
    time_t first_entry_s = LONG_MAX;

    rrddim_foreach_read(rd, st) {
        time_t t = rrddim_first_entry_s(rd);
        if(t < first_entry_s)
            first_entry_s = t;
    }
    rrddim_foreach_done(rd);

    if (unlikely(LONG_MAX == first_entry_s)) return 0;
    return first_entry_s;
}

time_t rrdset_first_entry_s_of_tier(RRDSET *st, size_t tier) {
    if(unlikely(tier >= nd_profile.storage_tiers))
        return 0;

    RRDDIM *rd;
    time_t first_entry_s = LONG_MAX;

    rrddim_foreach_read(rd, st) {
        time_t t = rrddim_first_entry_s_of_tier(rd, tier);
        if(t && t < first_entry_s)
            first_entry_s = t;
    }
    rrddim_foreach_done(rd);

    if (unlikely(LONG_MAX == first_entry_s)) return 0;
    return first_entry_s;
}

void rrdset_get_retention_of_tier_for_collected_chart(RRDSET *st, time_t *first_time_s, time_t *last_time_s, time_t now_s, size_t tier) {
    if(!now_s)
        now_s = now_realtime_sec();

    time_t db_first_entry_s = rrdset_first_entry_s_of_tier(st, tier);
    time_t db_last_entry_s = st->last_updated.tv_sec; // we assume this is a collected RRDSET

    if(unlikely(!db_last_entry_s)) {
        db_last_entry_s = rrdset_last_entry_s_of_tier(st, tier);

        if (unlikely(!db_last_entry_s)) {
            // we assume this is a collected RRDSET
            db_first_entry_s = 0;
            db_last_entry_s = 0;
        }
    }

    if(unlikely(db_last_entry_s > now_s)) {
        internal_error(db_last_entry_s > now_s + 1,
                       "RRDSET: 'host:%s/chart:%s' latest db time %ld is in the future, adjusting it to now %ld",
                       rrdhost_hostname(st->rrdhost), rrdset_id(st),
                       db_last_entry_s, now_s);
        db_last_entry_s = now_s;
    }

    if(unlikely(db_first_entry_s && db_last_entry_s && db_first_entry_s >= db_last_entry_s)) {
        internal_error(db_first_entry_s > db_last_entry_s,
                       "RRDSET: 'host:%s/chart:%s' oldest db time %ld is bigger than latest db time %ld, adjusting it to (latest time %ld - update every %ld)",
                       rrdhost_hostname(st->rrdhost), rrdset_id(st),
                       db_first_entry_s, db_last_entry_s,
                       db_last_entry_s, (time_t)st->update_every);
        db_first_entry_s = db_last_entry_s - st->update_every;
    }

    if(unlikely(!db_first_entry_s && db_last_entry_s))
        // this can be the case on the first data collection of a chart
        db_first_entry_s = db_last_entry_s - st->update_every;

    *first_time_s = db_first_entry_s;
    *last_time_s = db_last_entry_s;
}

void rrdset_is_obsolete___safe_from_collector_thread(RRDSET *st) {
    if(!st) return;

    rrdset_pluginsd_receive_unslot(st);

    if(unlikely(!(rrdset_flag_check(st, RRDSET_FLAG_OBSOLETE)))) {
//        netdata_log_info("Setting obsolete flag on chart 'host:%s/chart:%s'",
//                rrdhost_hostname(st->rrdhost), rrdset_id(st));

        rrdset_flag_set(st, RRDSET_FLAG_OBSOLETE);
        rrdhost_flag_set(st->rrdhost, RRDHOST_FLAG_PENDING_OBSOLETE_CHARTS);

        st->last_accessed_time_s = now_realtime_sec();

        rrdset_metadata_updated(st);

        // the chart will not get more updates (data collection)
        // so, we have to push its definition now
        stream_sender_send_rrdset_definition_now(st);
        rrdcontext_updated_rrdset_flags(st);
    }
}

void rrdset_isnot_obsolete___safe_from_collector_thread(RRDSET *st) {
    if(unlikely((rrdset_flag_check(st, RRDSET_FLAG_OBSOLETE)))) {

//        netdata_log_info("Clearing obsolete flag on chart 'host:%s/chart:%s'",
//                rrdhost_hostname(st->rrdhost), rrdset_id(st));

        rrdset_flag_clear(st, RRDSET_FLAG_OBSOLETE);
        st->last_accessed_time_s = now_realtime_sec();

        rrdset_metadata_updated(st);

        // the chart will be pushed upstream automatically
        // due to data collection
        rrdcontext_updated_rrdset_flags(st);
    }
}

void rrdset_update_heterogeneous_flag(RRDSET *st) {
    RRDHOST *host = st->rrdhost;
    (void)host;

    RRDDIM *rd;

    rrdset_flag_clear(st, RRDSET_FLAG_HOMOGENEOUS_CHECK);

    bool init = false, is_heterogeneous = false;
    RRD_ALGORITHM algorithm;
    int32_t multiplier;
    int32_t divisor;

    rrddim_foreach_read(rd, st) {
        if(!init) {
            algorithm = rd->algorithm;
            multiplier = rd->multiplier;
            divisor = ABS(rd->divisor);
            init = true;
            continue;
        }

        if(algorithm != rd->algorithm || multiplier != ABS(rd->multiplier) || divisor != ABS(rd->divisor)) {
            if(!rrdset_flag_check(st, RRDSET_FLAG_HETEROGENEOUS)) {
                #ifdef NETDATA_INTERNAL_CHECKS
                netdata_log_info("Dimension '%s' added on chart '%s' of host '%s' is not homogeneous to other dimensions already present "
                     "(algorithm is '%s' vs '%s', multiplier is %d vs %d, "
                     "divisor is %d vs %d).",
                     rrddim_name(rd),
                     rrdset_name(st),
                     rrdhost_hostname(host),
                     rrd_algorithm_name(rd->algorithm), rrd_algorithm_name(algorithm),
                     rd->multiplier, multiplier,
                     rd->divisor, divisor
                );
                #endif
                rrdset_flag_set(st, RRDSET_FLAG_HETEROGENEOUS);
            }

            is_heterogeneous = true;
            break;
        }
    }
    rrddim_foreach_done(rd);

    if(!is_heterogeneous) {
        rrdset_flag_clear(st, RRDSET_FLAG_HETEROGENEOUS);
        rrdcontext_updated_rrdset_flags(st);
    }
}
