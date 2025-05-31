// SPDX-License-Identifier: GPL-3.0-or-later

#include "query-internal.h"

static bool query_metric_is_valid_tier(QUERY_METRIC *qm, size_t tier) {
    if(!qm->tiers[tier].smh || !qm->tiers[tier].db_first_time_s || !qm->tiers[tier].db_last_time_s || !qm->tiers[tier].db_update_every_s)
        return false;

    return true;
}

static size_t query_metric_first_working_tier(QUERY_METRIC *qm) {
    for(size_t tier = 0; tier < nd_profile.storage_tiers; tier++) {

        // find the db time-range for this tier for all metrics
        STORAGE_METRIC_HANDLE *smh = qm->tiers[tier].smh;
        time_t first_time_s = qm->tiers[tier].db_first_time_s;
        time_t last_time_s  = qm->tiers[tier].db_last_time_s;
        time_t update_every_s = qm->tiers[tier].db_update_every_s;

        if(!smh || !first_time_s || !last_time_s || !update_every_s)
            continue;

        return tier;
    }

    return 0;
}

long query_plan_points_coverage_weight(time_t db_first_time_s, time_t db_last_time_s, time_t db_update_every_s, time_t after_wanted, time_t before_wanted, size_t points_wanted, size_t tier __maybe_unused) {
    if(db_first_time_s == 0 ||
        db_last_time_s == 0 ||
        db_update_every_s == 0 ||
        db_first_time_s > before_wanted ||
        db_last_time_s < after_wanted)
        return -LONG_MAX;

    long long common_first_t = MAX(db_first_time_s, after_wanted);
    long long common_last_t = MIN(db_last_time_s, before_wanted);

    long long time_coverage = (common_last_t - common_first_t) * 1000000LL / (before_wanted - after_wanted);
    long long points_wanted_in_coverage = (long long)points_wanted * time_coverage / 1000000LL;

    long long points_available = (common_last_t - common_first_t) / db_update_every_s;
    long long points_delta = (long)(points_available - points_wanted_in_coverage);
    long long points_coverage = (points_delta < 0) ? (long)(points_available * time_coverage / points_wanted_in_coverage) : time_coverage;

    // a way to benefit higher tiers
    // points_coverage += (long)tier * 10000;

    if(points_available <= 0)
        return -LONG_MAX;

    return (long)(points_coverage + (25000LL * tier)); // 2.5% benefit for each higher tier
}

static size_t query_metric_best_tier_for_timeframe(QUERY_METRIC *qm, time_t after_wanted, time_t before_wanted, size_t points_wanted) {
    if(unlikely(nd_profile.storage_tiers < 2))
        return 0;

    if(unlikely(after_wanted == before_wanted || points_wanted <= 0))
        return query_metric_first_working_tier(qm);

    if(points_wanted < QUERY_PLAN_MIN_POINTS)
        // when selecting tiers, aim for a resolution of at least QUERY_PLAN_MIN_POINTS points
        points_wanted = (before_wanted - after_wanted) > QUERY_PLAN_MIN_POINTS ? QUERY_PLAN_MIN_POINTS : before_wanted - after_wanted;

    time_t min_first_time_s = 0;
    time_t max_last_time_s = 0;

    for(size_t tier = 0; tier < nd_profile.storage_tiers; tier++) {
        time_t first_time_s = qm->tiers[tier].db_first_time_s;
        time_t last_time_s  = qm->tiers[tier].db_last_time_s;

        if(!min_first_time_s || (first_time_s && first_time_s < min_first_time_s))
            min_first_time_s = first_time_s;

        if(!max_last_time_s || (last_time_s && last_time_s > max_last_time_s))
            max_last_time_s = last_time_s;
    }

    for(size_t tier = 0; tier < nd_profile.storage_tiers; tier++) {

        // find the db time-range for this tier for all metrics
        STORAGE_METRIC_HANDLE *smh = qm->tiers[tier].smh;
        time_t first_time_s = qm->tiers[tier].db_first_time_s;
        time_t last_time_s  = qm->tiers[tier].db_last_time_s;
        time_t update_every_s = qm->tiers[tier].db_update_every_s;

        if( !smh ||
            !first_time_s ||
            !last_time_s ||
            !update_every_s ||
            first_time_s > before_wanted ||
            last_time_s < after_wanted
        ) {
            qm->tiers[tier].weight = -LONG_MAX;
            continue;
        }

        internal_fatal(first_time_s > before_wanted || last_time_s < after_wanted, "QUERY: invalid db durations");

        qm->tiers[tier].weight = query_plan_points_coverage_weight(
            min_first_time_s, max_last_time_s, update_every_s,
            after_wanted, before_wanted, points_wanted, tier);
    }

    size_t best_tier = 0;
    for(size_t tier = 1; tier < nd_profile.storage_tiers; tier++) {
        if(qm->tiers[tier].weight >= qm->tiers[best_tier].weight)
            best_tier = tier;
    }

    return best_tier;
}

static size_t rrddim_find_best_tier_for_timeframe(QUERY_TARGET *qt, time_t after_wanted, time_t before_wanted, size_t points_wanted) {
    if(unlikely(nd_profile.storage_tiers < 2))
        return 0;

    if(unlikely(after_wanted == before_wanted || points_wanted <= 0)) {
        internal_error(true, "QUERY: '%s' has invalid params to tier calculation", qt->id);
        return 0;
    }

    long weight[nd_profile.storage_tiers];

    for(size_t tier = 0; tier < nd_profile.storage_tiers; tier++) {

        time_t common_first_time_s = 0;
        time_t common_last_time_s = 0;
        time_t common_update_every_s = 0;

        // find the db time-range for this tier for all metrics
        for(size_t i = 0, used = qt->query.used; i < used ; i++) {
            QUERY_METRIC *qm = query_metric(qt, i);

            time_t first_time_s = qm->tiers[tier].db_first_time_s;
            time_t last_time_s  = qm->tiers[tier].db_last_time_s;
            time_t update_every_s = qm->tiers[tier].db_update_every_s;

            if(!first_time_s || !last_time_s || !update_every_s)
                continue;

            if(!common_first_time_s)
                common_first_time_s = first_time_s;
            else
                common_first_time_s = MIN(first_time_s, common_first_time_s);

            if(!common_last_time_s)
                common_last_time_s = last_time_s;
            else
                common_last_time_s = MAX(last_time_s, common_last_time_s);

            if(!common_update_every_s)
                common_update_every_s = update_every_s;
            else
                common_update_every_s = MIN(update_every_s, common_update_every_s);
        }

        weight[tier] = query_plan_points_coverage_weight(common_first_time_s, common_last_time_s, common_update_every_s, after_wanted, before_wanted, points_wanted, tier);
    }

    size_t best_tier = 0;
    for(size_t tier = 1; tier < nd_profile.storage_tiers; tier++) {
        if(weight[tier] >= weight[best_tier])
            best_tier = tier;
    }

    if(weight[best_tier] == -LONG_MAX)
        best_tier = 0;

    return best_tier;
}

time_t rrdset_find_natural_update_every_for_timeframe(QUERY_TARGET *qt, time_t after_wanted, time_t before_wanted, size_t points_wanted, RRDR_OPTIONS options, size_t tier) {
    size_t best_tier;
    if((options & RRDR_OPTION_SELECTED_TIER) && tier < nd_profile.storage_tiers)
        best_tier = tier;
    else
        best_tier = rrddim_find_best_tier_for_timeframe(qt, after_wanted, before_wanted, points_wanted);

    // find the db minimum update every for this tier for all metrics
    time_t common_update_every_s = nd_profile.update_every;
    for(size_t i = 0, used = qt->query.used; i < used ; i++) {
        QUERY_METRIC *qm = query_metric(qt, i);

        time_t update_every_s = qm->tiers[best_tier].db_update_every_s;

        if(!i)
            common_update_every_s = update_every_s;
        else
            common_update_every_s = MIN(update_every_s, common_update_every_s);
    }

    return common_update_every_s;
}

static size_t query_planer_expand_duration_in_points(time_t this_update_every, time_t next_update_every) {

    time_t delta = this_update_every - next_update_every;
    if(delta < 0) delta = -delta;

    size_t points;
    if(delta < this_update_every * POINTS_TO_EXPAND_QUERY)
        points = POINTS_TO_EXPAND_QUERY;
    else
        points = (delta + this_update_every - 1) / this_update_every;

    return points;
}

static void query_planer_initialize_plans(QUERY_ENGINE_OPS *ops) {
    QUERY_METRIC *qm = ops->qm;

    for(size_t p = 0; p < qm->plan.used ; p++) {
        size_t tier = qm->plan.array[p].tier;
        time_t update_every = qm->tiers[tier].db_update_every_s;

        size_t points_to_add_to_after;
        if(p > 0) {
            // there is another plan before to this

            size_t tier0 = qm->plan.array[p - 1].tier;
            time_t update_every0 = qm->tiers[tier0].db_update_every_s;

            points_to_add_to_after = query_planer_expand_duration_in_points(update_every, update_every0);
        }
        else
            points_to_add_to_after = (tier == 0) ? 0 : POINTS_TO_EXPAND_QUERY;

        size_t points_to_add_to_before;
        if(p + 1 < qm->plan.used) {
            // there is another plan after to this

            size_t tier1 = qm->plan.array[p+1].tier;
            time_t update_every1 = qm->tiers[tier1].db_update_every_s;

            points_to_add_to_before = query_planer_expand_duration_in_points(update_every, update_every1);
        }
        else
            points_to_add_to_before = POINTS_TO_EXPAND_QUERY;

        time_t after = qm->plan.array[p].after - (time_t)(update_every * points_to_add_to_after);
        time_t before = qm->plan.array[p].before + (time_t)(update_every * points_to_add_to_before);

        ops->plans[p].expanded_after = after;
        ops->plans[p].expanded_before = before;

        ops->r->internal.qt->db.tiers[tier].queries++;

        struct query_metric_tier *tier_ptr = &qm->tiers[tier];
        STORAGE_ENGINE *eng = query_metric_storage_engine(ops->r->internal.qt, qm, tier);
        storage_engine_query_init(eng->seb, tier_ptr->smh, &ops->plans[p].handle,
                                  after, before, ops->r->internal.qt->request.priority);

        ops->plans[p].initialized = true;
        ops->plans[p].finalized = false;
    }
}

static void query_planer_finalize_plan(QUERY_ENGINE_OPS *ops, size_t plan_id) {
    // QUERY_METRIC *qm = ops->qm;

    if(ops->plans[plan_id].initialized && !ops->plans[plan_id].finalized) {
        storage_engine_query_finalize(&ops->plans[plan_id].handle);
        ops->plans[plan_id].initialized = false;
        ops->plans[plan_id].finalized = true;
    }
}

void query_planer_finalize_remaining_plans(QUERY_ENGINE_OPS *ops) {
    QUERY_METRIC *qm = ops->qm;

    for(size_t p = 0; p < qm->plan.used ; p++)
        query_planer_finalize_plan(ops, p);
}

static void query_planer_activate_plan(QUERY_ENGINE_OPS *ops, size_t plan_id, time_t overwrite_after __maybe_unused) {
    QUERY_METRIC *qm = ops->qm;

    internal_fatal(plan_id >= qm->plan.used, "QUERY: invalid plan_id given");
    internal_fatal(!ops->plans[plan_id].initialized, "QUERY: plan has not been initialized");
    internal_fatal(ops->plans[plan_id].finalized, "QUERY: plan has been finalized");

    internal_fatal(qm->plan.array[plan_id].after > qm->plan.array[plan_id].before, "QUERY: flipped after/before");

    ops->tier = qm->plan.array[plan_id].tier;
    ops->tier_ptr = &qm->tiers[ops->tier];
    ops->seqh = &ops->plans[plan_id].handle;
    ops->current_plan = plan_id;

    if(plan_id + 1 < qm->plan.used && qm->plan.array[plan_id + 1].after < qm->plan.array[plan_id].before)
        ops->current_plan_expire_time = qm->plan.array[plan_id + 1].after;
    else
        ops->current_plan_expire_time = qm->plan.array[plan_id].before;

    ops->plan_expanded_after = ops->plans[plan_id].expanded_after;
    ops->plan_expanded_before = ops->plans[plan_id].expanded_before;
}

bool query_planer_next_plan(QUERY_ENGINE_OPS *ops, time_t now, time_t last_point_end_time) {
    QUERY_METRIC *qm = ops->qm;

    size_t old_plan = ops->current_plan;

    time_t next_plan_before_time;
    do {
        ops->current_plan++;

        if (ops->current_plan >= qm->plan.used) {
            ops->current_plan = old_plan;
            ops->current_plan_expire_time = ops->r->internal.qt->window.before;
            // let the query run with current plan
            // we will not switch it
            return false;
        }

        next_plan_before_time = qm->plan.array[ops->current_plan].before;
    } while(now >= next_plan_before_time || last_point_end_time >= next_plan_before_time);

    if(!query_metric_is_valid_tier(qm, qm->plan.array[ops->current_plan].tier)) {
        ops->current_plan = old_plan;
        ops->current_plan_expire_time = ops->r->internal.qt->window.before;
        return false;
    }

    query_planer_finalize_plan(ops, old_plan);
    query_planer_activate_plan(ops, ops->current_plan, MIN(now, last_point_end_time));
    return true;
}

static int compare_query_plan_entries_on_start_time(const void *a, const void *b) {
    QUERY_PLAN_ENTRY *p1 = (QUERY_PLAN_ENTRY *)a;
    QUERY_PLAN_ENTRY *p2 = (QUERY_PLAN_ENTRY *)b;
    return (p1->after < p2->after)?-1:1;
}

static bool query_plan(QUERY_ENGINE_OPS *ops, time_t after_wanted, time_t before_wanted, size_t points_wanted) {
    QUERY_METRIC *qm = ops->qm;

    // put our selected tier as the first plan
    size_t selected_tier;
    bool switch_tiers = true;

    if((ops->r->internal.qt->window.options & RRDR_OPTION_SELECTED_TIER)
        && ops->r->internal.qt->window.tier < nd_profile.storage_tiers && query_metric_is_valid_tier(qm, ops->r->internal.qt->window.tier)) {
        selected_tier = ops->r->internal.qt->window.tier;
        switch_tiers = false;
    }
    else {
        selected_tier = query_metric_best_tier_for_timeframe(qm, after_wanted, before_wanted, points_wanted);

        if(!query_metric_is_valid_tier(qm, selected_tier))
            return false;
    }

    if(qm->tiers[selected_tier].db_first_time_s > before_wanted ||
        qm->tiers[selected_tier].db_last_time_s < after_wanted) {
        // we don't have any data to satisfy this query
        return false;
    }

    qm->plan.used = 1;
    qm->plan.array[0].tier = selected_tier;
    qm->plan.array[0].after = (qm->tiers[selected_tier].db_first_time_s < after_wanted) ? after_wanted : qm->tiers[selected_tier].db_first_time_s;
    qm->plan.array[0].before = (qm->tiers[selected_tier].db_last_time_s > before_wanted) ? before_wanted : qm->tiers[selected_tier].db_last_time_s;

    if(switch_tiers) {
        // the selected tier
        time_t selected_tier_first_time_s = qm->plan.array[0].after;
        time_t selected_tier_last_time_s = qm->plan.array[0].before;

        // check if our selected tier can start the query
        if (selected_tier_first_time_s > after_wanted) {
            // we need some help from other tiers
            for (size_t tr = (int)selected_tier + 1; tr < nd_profile.storage_tiers && qm->plan.used < QUERY_PLANS_MAX ; tr++) {
                if(!query_metric_is_valid_tier(qm, tr))
                    continue;

                // find the first time of this tier
                time_t tier_first_time_s = qm->tiers[tr].db_first_time_s;
                time_t tier_last_time_s = qm->tiers[tr].db_last_time_s;

                // can it help?
                if (tier_first_time_s < selected_tier_first_time_s && tier_first_time_s <= before_wanted && tier_last_time_s >= after_wanted) {
                    // it can help us add detail at the beginning of the query
                    QUERY_PLAN_ENTRY t = {
                        .tier = tr,
                        .after = (tier_first_time_s < after_wanted) ? after_wanted : tier_first_time_s,
                        .before = selected_tier_first_time_s,
                    };
                    ops->plans[qm->plan.used].initialized = false;
                    ops->plans[qm->plan.used].finalized = false;
                    qm->plan.array[qm->plan.used++] = t;

                    internal_fatal(!t.after || !t.before, "QUERY: invalid plan selected");

                    // prepare for the tier
                    selected_tier_first_time_s = t.after;

                    if (t.after <= after_wanted)
                        break;
                }
            }
        }

        // check if our selected tier can finish the query
        if (selected_tier_last_time_s < before_wanted) {
            // we need some help from other tiers
            for (int tr = (int)selected_tier - 1; tr >= 0 && qm->plan.used < QUERY_PLANS_MAX ; tr--) {
                if(!query_metric_is_valid_tier(qm, tr))
                    continue;

                // find the last time of this tier
                time_t tier_first_time_s = qm->tiers[tr].db_first_time_s;
                time_t tier_last_time_s = qm->tiers[tr].db_last_time_s;

                //buffer_sprintf(wb, ": EVAL BEFORE tier %d, %ld", tier, last_time_s);

                // can it help?
                if (tier_last_time_s > selected_tier_last_time_s && tier_first_time_s <= before_wanted && tier_last_time_s >= after_wanted) {
                    // it can help us add detail at the end of the query
                    QUERY_PLAN_ENTRY t = {
                        .tier = tr,
                        .after = selected_tier_last_time_s,
                        .before = (tier_last_time_s > before_wanted) ? before_wanted : tier_last_time_s,
                    };
                    ops->plans[qm->plan.used].initialized = false;
                    ops->plans[qm->plan.used].finalized = false;
                    qm->plan.array[qm->plan.used++] = t;

                    // prepare for the tier
                    selected_tier_last_time_s = t.before;

                    internal_fatal(!t.after || !t.before, "QUERY: invalid plan selected");

                    if (t.before >= before_wanted)
                        break;
                }
            }
        }
    }

    // sort the query plan
    if(qm->plan.used > 1)
        qsort(&qm->plan.array, qm->plan.used, sizeof(QUERY_PLAN_ENTRY), compare_query_plan_entries_on_start_time);

    if(!query_metric_is_valid_tier(qm, qm->plan.array[0].tier))
        return false;

#ifdef NETDATA_INTERNAL_CHECKS
    for(size_t p = 0; p < qm->plan.used ;p++) {
        internal_fatal(qm->plan.array[p].after > qm->plan.array[p].before, "QUERY: flipped after/before");
        internal_fatal(qm->plan.array[p].after < after_wanted, "QUERY: too small plan first time");
        internal_fatal(qm->plan.array[p].before > before_wanted, "QUERY: too big plan last time");
    }
#endif

    query_planer_initialize_plans(ops);
    query_planer_activate_plan(ops, 0, 0);

    return true;
}


static __thread QUERY_ENGINE_OPS *released_ops = NULL;

void rrd2rrdr_query_ops_freeall(RRDR *r __maybe_unused) {
    while(released_ops) {
        QUERY_ENGINE_OPS *ops = released_ops;
        released_ops = ops->next;

        onewayalloc_freez(r->internal.owa, ops);
    }
}

void rrd2rrdr_query_ops_release(QUERY_ENGINE_OPS *ops) {
    if(!ops) return;

    ops->next = released_ops;
    released_ops = ops;
}

static QUERY_ENGINE_OPS *rrd2rrdr_query_ops_get(RRDR *r) {
    QUERY_ENGINE_OPS *ops;
    if(released_ops) {
        ops = released_ops;
        released_ops = ops->next;
    }
    else {
        ops = onewayalloc_mallocz(r->internal.owa, sizeof(QUERY_ENGINE_OPS));
    }

    memset(ops, 0, sizeof(*ops));
    return ops;
}

QUERY_ENGINE_OPS *rrd2rrdr_query_ops_prep(RRDR *r, size_t query_metric_id) {
    QUERY_TARGET *qt = r->internal.qt;

    QUERY_ENGINE_OPS *ops = rrd2rrdr_query_ops_get(r);
    *ops = (QUERY_ENGINE_OPS) {
        .r = r,
        .qm = query_metric(qt, query_metric_id),
        .tier_query_fetch = r->time_grouping.tier_query_fetch,
        .view_update_every = r->view.update_every,
        .query_granularity = (time_t)(r->view.update_every / r->view.group),
        .group_value_flags = RRDR_VALUE_NOTHING,
    };

    if(!query_plan(ops, qt->window.after, qt->window.before, qt->window.points)) {
        rrd2rrdr_query_ops_release(ops);
        return NULL;
    }

    return ops;
}
