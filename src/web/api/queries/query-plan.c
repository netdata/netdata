// SPDX-License-Identifier: GPL-3.0-or-later

#include "query-internal.h"

static bool query_metric_is_valid_tier(QUERY_METRIC *qm, size_t tier) {
    if(!qm->tiers[tier].smh || !qm->tiers[tier].db_first_time_s || !qm->tiers[tier].db_last_time_s || !qm->tiers[tier].db_update_every_s)
        return false;

    return true;
}

static bool query_plan_tier_is_valid(QUERY_METRIC *qm, size_t tier) {
    if(tier >= nd_profile.storage_tiers || tier >= RRD_STORAGE_TIERS)
        return false;

    return query_metric_is_valid_tier(qm, tier);
}

static bool query_plan_entry_is_valid(
    QUERY_METRIC *qm, const QUERY_PLAN_ENTRY *entry, time_t after_wanted, time_t before_wanted) {
    if(!entry->after || !entry->before)
        return false;

    if(entry->after > entry->before)
        return false;

    if(entry->after < after_wanted || entry->before > before_wanted)
        return false;

    return query_plan_tier_is_valid(qm, entry->tier);
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

#define QUERY_PLAN_POINTS_WEIGHT_SCALE 1000000ULL
#define QUERY_PLAN_ACCEPTABLE_POINTS_NUMERATOR 1ULL
#define QUERY_PLAN_ACCEPTABLE_POINTS_DENOMINATOR 2ULL

static bool query_metric_tier_overlaps_timeframe(QUERY_METRIC *qm, size_t tier, time_t after_wanted, time_t before_wanted) {
    if(!query_metric_is_valid_tier(qm, tier))
        return false;

    return qm->tiers[tier].db_first_time_s <= before_wanted &&
           qm->tiers[tier].db_last_time_s >= after_wanted;
}

static long query_plan_points_density_weight(time_t db_update_every_s, time_t after_wanted, time_t before_wanted) {
    if(db_update_every_s <= 0 || before_wanted <= after_wanted)
        return -LONG_MAX;

    uint64_t duration_s = (uint64_t)(before_wanted - after_wanted);

    if(duration_s > (uint64_t)LONG_MAX / QUERY_PLAN_POINTS_WEIGHT_SCALE)
        return LONG_MAX;

    return (long)((duration_s * QUERY_PLAN_POINTS_WEIGHT_SCALE) / (uint64_t)db_update_every_s);
}

static long query_plan_minimum_acceptable_points_weight(size_t points_wanted) {
    if(!points_wanted)
        return 0;

    if((uint64_t)points_wanted > (uint64_t)LONG_MAX / QUERY_PLAN_POINTS_WEIGHT_SCALE)
        return LONG_MAX;

    uint64_t wanted_scaled = (uint64_t)points_wanted * QUERY_PLAN_POINTS_WEIGHT_SCALE;

    if(wanted_scaled > UINT64_MAX / QUERY_PLAN_ACCEPTABLE_POINTS_NUMERATOR)
        return LONG_MAX;

    uint64_t acceptable_scaled =
        (wanted_scaled * QUERY_PLAN_ACCEPTABLE_POINTS_NUMERATOR + QUERY_PLAN_ACCEPTABLE_POINTS_DENOMINATOR - 1) /
        QUERY_PLAN_ACCEPTABLE_POINTS_DENOMINATOR;

    if(acceptable_scaled > (uint64_t)LONG_MAX)
        return LONG_MAX;

    return (long)acceptable_scaled;
}

static bool query_plan_points_density_is_better(
    size_t tier, long weight, bool acceptable,
    size_t best_tier, long best_weight, bool best_acceptable) {
    if(acceptable) {
        if(!best_acceptable)
            return true;

        if(weight < best_weight)
            return true;

        return weight == best_weight && tier > best_tier;
    }

    if(best_acceptable)
        return false;

    if(weight > best_weight)
        return true;

    return weight == best_weight && tier < best_tier;
}

static size_t query_metric_best_tier_for_timeframe(QUERY_METRIC *qm, time_t after_wanted, time_t before_wanted, size_t points_wanted) {
    if(unlikely(nd_profile.storage_tiers < 2))
        return 0;

    if(unlikely(before_wanted <= after_wanted || points_wanted <= 0))
        return query_metric_first_working_tier(qm);

    if(points_wanted < QUERY_PLAN_MIN_POINTS)
        // when selecting tiers, aim for a resolution of at least QUERY_PLAN_MIN_POINTS points
        points_wanted = (before_wanted - after_wanted) > QUERY_PLAN_MIN_POINTS ? QUERY_PLAN_MIN_POINTS : before_wanted - after_wanted;

    long minimum_acceptable_weight = query_plan_minimum_acceptable_points_weight(points_wanted);

    size_t best_tier = 0;
    long best_weight = -LONG_MAX;
    bool best_acceptable = false;
    bool found_candidate = false;

    for(size_t tier = 0; tier < nd_profile.storage_tiers; tier++) {

        time_t update_every_s = qm->tiers[tier].db_update_every_s;

        if(!query_metric_tier_overlaps_timeframe(qm, tier, after_wanted, before_wanted)) {
            qm->tiers[tier].weight = -LONG_MAX;
            continue;
        }

        qm->tiers[tier].weight = query_plan_points_density_weight(update_every_s, after_wanted, before_wanted);
        if(qm->tiers[tier].weight == -LONG_MAX)
            continue;

        bool acceptable = qm->tiers[tier].weight >= minimum_acceptable_weight;

        if(!found_candidate ||
           query_plan_points_density_is_better(
               tier, qm->tiers[tier].weight, acceptable,
               best_tier, best_weight, best_acceptable)) {
            best_tier = tier;
            best_weight = qm->tiers[tier].weight;
            best_acceptable = acceptable;
            found_candidate = true;
        }
    }

    return found_candidate ? best_tier : query_metric_first_working_tier(qm);
}

time_t query_target_min_update_every_for_tier(QUERY_TARGET *qt, size_t tier) {
    if(tier >= nd_profile.storage_tiers)
        return nd_profile.update_every;

    // find the db minimum update every for this tier for all metrics
    time_t common_update_every_s = 0;
    for(size_t i = 0, used = qt->query.used; i < used ; i++) {
        QUERY_METRIC *qm = query_metric(qt, i);

        time_t update_every_s = qm->tiers[tier].db_update_every_s;
        if(!update_every_s)
            continue;

        if(!common_update_every_s)
            common_update_every_s = update_every_s;
        else
            common_update_every_s = MIN(update_every_s, common_update_every_s);
    }

    return common_update_every_s ? common_update_every_s : nd_profile.update_every;
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

static bool query_planer_plan_can_be_activated(QUERY_ENGINE_OPS *ops, size_t plan_id) {
    QUERY_METRIC *qm = ops->qm;

    if(plan_id >= qm->plan.used)
        return false;

    if(!ops->plans[plan_id].initialized || ops->plans[plan_id].finalized)
        return false;

    if(!qm->plan.array[plan_id].after || !qm->plan.array[plan_id].before)
        return false;

    if(qm->plan.array[plan_id].after > qm->plan.array[plan_id].before)
        return false;

    return query_plan_tier_is_valid(qm, qm->plan.array[plan_id].tier);
}

static void query_planer_set_active_plan(QUERY_ENGINE_OPS *ops, size_t plan_id, time_t overwrite_after __maybe_unused) {
    QUERY_METRIC *qm = ops->qm;

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

static bool query_planer_activate_plan(QUERY_ENGINE_OPS *ops, size_t plan_id, time_t overwrite_after __maybe_unused) {
    if(!query_planer_plan_can_be_activated(ops, plan_id))
        return false;

    query_planer_set_active_plan(ops, plan_id, overwrite_after);
    return true;
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

    if(!query_planer_plan_can_be_activated(ops, ops->current_plan)) {
        ops->current_plan = old_plan;
        ops->current_plan_expire_time = ops->r->internal.qt->window.before;
        return false;
    }

    query_planer_finalize_plan(ops, old_plan);
    query_planer_set_active_plan(ops, ops->current_plan, MIN(now, last_point_end_time));
    return true;
}

static int compare_query_plan_entries_on_start_time(const void *a, const void *b) {
    QUERY_PLAN_ENTRY *p1 = (QUERY_PLAN_ENTRY *)a;
    QUERY_PLAN_ENTRY *p2 = (QUERY_PLAN_ENTRY *)b;
    return (p1->after < p2->after)?-1:1;
}

static bool query_plan_build_entries(QUERY_ENGINE_OPS *ops, time_t after_wanted, time_t before_wanted, size_t points_wanted) {
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

                    if(!query_plan_entry_is_valid(qm, &t, after_wanted, before_wanted))
                        return false;

                    ops->plans[qm->plan.used].initialized = false;
                    ops->plans[qm->plan.used].finalized = false;
                    qm->plan.array[qm->plan.used++] = t;

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

                    if(!query_plan_entry_is_valid(qm, &t, after_wanted, before_wanted))
                        return false;

                    ops->plans[qm->plan.used].initialized = false;
                    ops->plans[qm->plan.used].finalized = false;
                    qm->plan.array[qm->plan.used++] = t;

                    // prepare for the tier
                    selected_tier_last_time_s = t.before;

                    if (t.before >= before_wanted)
                        break;
                }
            }
        }
    }

    // sort the query plan
    if(qm->plan.used > 1)
        qsort(&qm->plan.array, qm->plan.used, sizeof(QUERY_PLAN_ENTRY), compare_query_plan_entries_on_start_time);

    for(size_t p = 0; p < qm->plan.used ;p++) {
        if(!query_plan_entry_is_valid(qm, &qm->plan.array[p], after_wanted, before_wanted))
            return false;
    }

    return true;
}

static bool query_plan(QUERY_ENGINE_OPS *ops, time_t after_wanted, time_t before_wanted, size_t points_wanted) {
    if(!query_plan_build_entries(ops, after_wanted, before_wanted, points_wanted))
        return false;

    query_planer_initialize_plans(ops);
    if(!query_planer_activate_plan(ops, 0, 0)) {
        query_planer_finalize_remaining_plans(ops);
        return false;
    }

    return true;
}


void rrd2rrdr_query_ops_freeall(RRDR *r, QUERY_ENGINE_OPS_CACHE *cache) {
    while(cache->released_ops) {
        QUERY_ENGINE_OPS *ops = cache->released_ops;
        cache->released_ops = ops->next;

        onewayalloc_freez(r->internal.owa, ops);
    }
}

void rrd2rrdr_query_ops_release(QUERY_ENGINE_OPS_CACHE *cache, QUERY_ENGINE_OPS *ops) {
    if(!ops) return;

    ops->next = cache->released_ops;
    cache->released_ops = ops;
}

static QUERY_ENGINE_OPS *rrd2rrdr_query_ops_get(RRDR *r, QUERY_ENGINE_OPS_CACHE *cache) {
    QUERY_ENGINE_OPS *ops;
    if(cache->released_ops) {
        ops = cache->released_ops;
        cache->released_ops = ops->next;
    }
    else {
        ops = onewayalloc_mallocz(r->internal.owa, sizeof(QUERY_ENGINE_OPS));
    }

    memset(ops, 0, sizeof(*ops));
    return ops;
}

// the LATEST grouping asks for the most recent collected value; when the
// query wants a single point and its window covers the metric's last stored
// sample, the answer is the collector's cached last_stored_value - serve it
// without building a query plan, so the storage engine is never touched.
// options that change the value semantics (anomaly-bit, natural points,
// a pinned tier) fall back to the normal execution path.
static bool query_latest_fast_path(RRDR *r, QUERY_ENGINE_OPS *ops) {
    QUERY_TARGET *qt = r->internal.qt;

    // natural points are not excluded: with a single output point the
    // whole window is one group, so natural and virtual points agree
    if(r->time_grouping.add_flush != RRDR_GROUPING_LATEST ||
        qt->window.points != 1 ||
        (qt->window.options & (RRDR_OPTION_SELECTED_TIER|RRDR_OPTION_ANOMALY_BIT)))
        return false;

    // the single output bucket spans (after, before]
    time_t db_last = ops->qm->tiers[0].db_last_time_s;
    if(db_last <= qt->window.after || db_last > qt->window.before)
        return false;

    // NAN when there is no live dimension (archived metric), or when the
    // collector's last sample is a gap - the storage query serves those
    QUERY_DIMENSION *qd = query_dimension(qt, ops->qm->link.query_dimension_id);
    NETDATA_DOUBLE v = rrdmetric_acquired_last_stored_value(qd->rma);
    if(!netdata_double_isnumber(v))
        return false;

    ops->latest_fast_path = true;
    ops->latest_fast_path_value = v;
    ops->latest_fast_path_time = db_last;
    return true;
}

QUERY_ENGINE_OPS *rrd2rrdr_query_ops_prep(RRDR *r, QUERY_ENGINE_OPS_CACHE *cache, size_t query_metric_id) {
    QUERY_TARGET *qt = r->internal.qt;

    QUERY_ENGINE_OPS *ops = rrd2rrdr_query_ops_get(r, cache);
    *ops = (QUERY_ENGINE_OPS) {
        .r = r,
        .qm = query_metric(qt, query_metric_id),
        .tier_query_fetch = r->time_grouping.tier_query_fetch,
        .view_update_every = r->view.update_every,
        .query_granularity = (time_t)(r->view.update_every / r->view.group),
        .group_value_flags = RRDR_VALUE_NOTHING,
    };

    if(query_latest_fast_path(r, ops))
        return ops;

    if(!query_plan(ops, qt->window.after, qt->window.before, qt->window.points)) {
        rrd2rrdr_query_ops_release(cache, ops);
        return NULL;
    }

    return ops;
}

static void query_plan_unittest_set_tier(
    QUERY_METRIC *qm, size_t tier, time_t first_time_s, time_t last_time_s, time_t update_every_s) {
    static char smh_stub;

    qm->tiers[tier].smh = (STORAGE_METRIC_HANDLE *)&smh_stub;
    qm->tiers[tier].db_first_time_s = first_time_s;
    qm->tiers[tier].db_last_time_s = last_time_s;
    qm->tiers[tier].db_update_every_s = update_every_s;
}

static int query_plan_unittest_expect_best_tier(
    const char *name, QUERY_METRIC *qm, time_t after, time_t before, size_t points, size_t expected) {
    size_t got = query_metric_best_tier_for_timeframe(qm, after, before, points);
    if(got == expected) {
        fprintf(stderr, "OK query plan tier selection: %s\n", name);
        return 0;
    }

    fprintf(stderr,
            "FAILED query plan tier selection: %s, expected tier %zu, got tier %zu\n",
            name, expected, got);

    for(size_t tier = 0; tier < nd_profile.storage_tiers; tier++)
        fprintf(stderr,
                " tier %zu: first %ld, last %ld, update_every %ld, weight %ld\n",
                tier,
                qm->tiers[tier].db_first_time_s,
                qm->tiers[tier].db_last_time_s,
                qm->tiers[tier].db_update_every_s,
                qm->tiers[tier].weight);

    return 1;
}

static bool query_plan_unittest_build_entries(
    QUERY_METRIC *qm, RRDR_OPTIONS options, size_t selected_tier,
    time_t after, time_t before, size_t points) {
    RRDR r = {0};
    QUERY_TARGET qt = {0};
    QUERY_ENGINE_OPS ops = {
        .r = &r,
        .qm = qm,
    };

    r.internal.qt = &qt;
    qt.window.options = options;
    qt.window.tier = selected_tier;

    return query_plan_build_entries(&ops, after, before, points);
}

static int query_plan_unittest_expect_plan(
    const char *name, QUERY_METRIC *qm, RRDR_OPTIONS options, size_t selected_tier,
    time_t after, time_t before, size_t points,
    const QUERY_PLAN_ENTRY *expected, size_t expected_used) {
    if(!query_plan_unittest_build_entries(qm, options, selected_tier, after, before, points)) {
        fprintf(stderr, "FAILED query plan entries: %s, planner returned false\n", name);
        return 1;
    }

    if(qm->plan.used != expected_used) {
        fprintf(stderr,
                "FAILED query plan entries: %s, expected %zu entries, got %zu\n",
                name, expected_used, qm->plan.used);
        return 1;
    }

    for(size_t i = 0; i < expected_used; i++) {
        if(qm->plan.array[i].tier == expected[i].tier &&
           qm->plan.array[i].after == expected[i].after &&
           qm->plan.array[i].before == expected[i].before)
            continue;

        fprintf(stderr,
                "FAILED query plan entries: %s, entry %zu expected tier %zu after %ld before %ld, got tier %zu after %ld before %ld\n",
                name, i,
                expected[i].tier, expected[i].after, expected[i].before,
                qm->plan.array[i].tier, qm->plan.array[i].after, qm->plan.array[i].before);
        return 1;
    }

    fprintf(stderr, "OK query plan entries: %s\n", name);
    return 0;
}

static int query_plan_unittest_expect_no_plan(
    const char *name, QUERY_METRIC *qm, RRDR_OPTIONS options, size_t selected_tier,
    time_t after, time_t before, size_t points) {
    if(!query_plan_unittest_build_entries(qm, options, selected_tier, after, before, points)) {
        fprintf(stderr, "OK query plan entries: %s\n", name);
        return 0;
    }

    fprintf(stderr, "FAILED query plan entries: %s, expected no plan, got %zu entries\n", name, qm->plan.used);
    return 1;
}

static int query_plan_unittest_expect_update_every(QUERY_TARGET *qt, size_t tier, time_t expected) {
    time_t got = query_target_min_update_every_for_tier(qt, tier);
    if(got == expected) {
        fprintf(stderr, "OK query plan selected-tier natural update_every\n");
        return 0;
    }

    fprintf(stderr,
            "FAILED query plan selected-tier natural update_every: expected %ld, got %ld\n",
            expected, got);

    return 1;
}

static int query_plan_unittest_expect_entry_validity(
    const char *name, QUERY_METRIC *qm, QUERY_PLAN_ENTRY entry, time_t after, time_t before, bool expected) {
    bool got = query_plan_entry_is_valid(qm, &entry, after, before);
    if(got == expected) {
        fprintf(stderr, "OK query plan entry validation: %s\n", name);
        return 0;
    }

    fprintf(stderr,
            "FAILED query plan entry validation: %s, expected %s, got %s\n",
            name, expected ? "valid" : "invalid", got ? "valid" : "invalid");

    return 1;
}

static int query_plan_unittest_expect_activation(
    const char *name, QUERY_ENGINE_OPS *ops, size_t plan_id, bool expected) {
    bool got = query_planer_activate_plan(ops, plan_id, 0);
    if(got == expected) {
        fprintf(stderr, "OK query plan activation: %s\n", name);
        return 0;
    }

    fprintf(stderr,
            "FAILED query plan activation: %s, expected %s, got %s\n",
            name, expected ? "success" : "failure", got ? "success" : "failure");

    return 1;
}

static int query_plan_unittest_expect_ops_cache_is_local(void) {
    ONEWAYALLOC *owa_a = onewayalloc_create(1024);
    ONEWAYALLOC *owa_b = onewayalloc_create(1024);

    RRDR r_a = {
        .internal.owa = owa_a,
    };
    RRDR r_b = {
        .internal.owa = owa_b,
    };

    QUERY_ENGINE_OPS_CACHE cache_a = { 0 };
    QUERY_ENGINE_OPS_CACHE cache_b = { 0 };

    QUERY_ENGINE_OPS *a = rrd2rrdr_query_ops_get(&r_a, &cache_a);
    rrd2rrdr_query_ops_release(&cache_a, a);

    QUERY_ENGINE_OPS *a_reused = rrd2rrdr_query_ops_get(&r_a, &cache_a);
    bool same_cache_reused = (a_reused == a);
    rrd2rrdr_query_ops_release(&cache_a, a_reused);

    QUERY_ENGINE_OPS *b = rrd2rrdr_query_ops_get(&r_b, &cache_b);
    bool separate_cache_isolated = (b != a);
    rrd2rrdr_query_ops_release(&cache_b, b);

    rrd2rrdr_query_ops_freeall(&r_a, &cache_a);
    rrd2rrdr_query_ops_freeall(&r_b, &cache_b);
    onewayalloc_destroy(owa_a);
    onewayalloc_destroy(owa_b);

    if(same_cache_reused && separate_cache_isolated) {
        fprintf(stderr, "OK query ops cache locality\n");
        return 0;
    }

    fprintf(stderr,
            "FAILED query ops cache locality: same_cache_reused=%s, separate_cache_isolated=%s\n",
            same_cache_reused ? "true" : "false",
            separate_cache_isolated ? "true" : "false");
    return 1;
}

int query_plan_unittest(void) {
    size_t old_storage_tiers = nd_profile.storage_tiers;
    time_t old_update_every = nd_profile.update_every;
    int errors = 0;

    nd_profile.storage_tiers = 3;
    nd_profile.update_every = 1;

    {
        QUERY_METRIC qm = {0};
        query_plan_unittest_set_tier(&qm, 0, 10, 100, 10);

        errors += query_plan_unittest_expect_entry_validity(
            "valid in-window entry", &qm, (QUERY_PLAN_ENTRY){ .tier = 0, .after = 20, .before = 80 }, 10, 100, true);
        errors += query_plan_unittest_expect_entry_validity(
            "zero start is invalid", &qm, (QUERY_PLAN_ENTRY){ .tier = 0, .after = 0, .before = 80 }, 10, 100, false);
        errors += query_plan_unittest_expect_entry_validity(
            "flipped entry is invalid", &qm, (QUERY_PLAN_ENTRY){ .tier = 0, .after = 90, .before = 80 }, 10, 100, false);
        errors += query_plan_unittest_expect_entry_validity(
            "entry before requested window is invalid", &qm, (QUERY_PLAN_ENTRY){ .tier = 0, .after = 9, .before = 80 }, 10, 100, false);
        errors += query_plan_unittest_expect_entry_validity(
            "entry after requested window is invalid", &qm, (QUERY_PLAN_ENTRY){ .tier = 0, .after = 20, .before = 101 }, 10, 100, false);
        errors += query_plan_unittest_expect_entry_validity(
            "out-of-range tier is invalid", &qm, (QUERY_PLAN_ENTRY){ .tier = 3, .after = 20, .before = 80 }, 10, 100, false);
    }

    {
        QUERY_METRIC qm = {0};
        QUERY_ENGINE_OPS ops = { .qm = &qm };

        query_plan_unittest_set_tier(&qm, 0, 10, 100, 10);
        qm.plan.used = 1;
        qm.plan.array[0] = (QUERY_PLAN_ENTRY){ .tier = 0, .after = 10, .before = 100 };
        ops.plans[0].initialized = true;

        errors += query_plan_unittest_expect_activation("valid initialized plan", &ops, 0, true);
    }

    {
        QUERY_METRIC qm = {0};
        QUERY_ENGINE_OPS ops = { .qm = &qm };

        query_plan_unittest_set_tier(&qm, 0, 10, 100, 10);
        qm.plan.used = 1;
        qm.plan.array[0] = (QUERY_PLAN_ENTRY){ .tier = 0, .after = 10, .before = 100 };
        ops.plans[0].initialized = true;

        errors += query_plan_unittest_expect_activation("invalid plan id is rejected", &ops, 1, false);
    }

    {
        QUERY_METRIC qm = {0};
        QUERY_ENGINE_OPS ops = { .qm = &qm };

        query_plan_unittest_set_tier(&qm, 0, 10, 100, 10);
        qm.plan.used = 1;
        qm.plan.array[0] = (QUERY_PLAN_ENTRY){ .tier = 0, .after = 10, .before = 100 };

        errors += query_plan_unittest_expect_activation("uninitialized plan is rejected", &ops, 0, false);
    }

    {
        QUERY_METRIC qm = {0};
        QUERY_ENGINE_OPS ops = { .qm = &qm };

        query_plan_unittest_set_tier(&qm, 0, 10, 100, 10);
        qm.plan.used = 1;
        qm.plan.array[0] = (QUERY_PLAN_ENTRY){ .tier = 0, .after = 10, .before = 100 };
        ops.plans[0].initialized = true;
        ops.plans[0].finalized = true;

        errors += query_plan_unittest_expect_activation("finalized plan is rejected", &ops, 0, false);
    }

    {
        QUERY_METRIC qm = {0};
        QUERY_ENGINE_OPS ops = { .qm = &qm };

        query_plan_unittest_set_tier(&qm, 0, 10, 100, 10);
        qm.plan.used = 1;
        qm.plan.array[0] = (QUERY_PLAN_ENTRY){ .tier = 0, .after = 100, .before = 10 };
        ops.plans[0].initialized = true;

        errors += query_plan_unittest_expect_activation("flipped plan is rejected", &ops, 0, false);
    }

    {
        QUERY_METRIC qm = {0};
        QUERY_ENGINE_OPS ops = { .qm = &qm };

        query_plan_unittest_set_tier(&qm, 0, 10, 100, 10);
        qm.plan.used = 1;
        qm.plan.array[0] = (QUERY_PLAN_ENTRY){ .tier = 3, .after = 10, .before = 100 };
        ops.plans[0].initialized = true;

        errors += query_plan_unittest_expect_activation("out-of-range tier is rejected", &ops, 0, false);
    }

    {
        QUERY_METRIC qm = {0};
        query_plan_unittest_set_tier(&qm, 0, 1, 200, 10);
        query_plan_unittest_set_tier(&qm, 1, 1, 200, 600);
        query_plan_unittest_set_tier(&qm, 2, 1, 100, 36000);

        errors += query_plan_unittest_expect_best_tier(
            "sub-resolution window ignores non-overlapping coarser tier", &qm, 103, 108, 5, 0);
    }

    {
        QUERY_METRIC qm = {0};
        query_plan_unittest_set_tier(&qm, 0, 1, 200, 10);
        query_plan_unittest_set_tier(&qm, 1, 1, 200, 600);
        query_plan_unittest_set_tier(&qm, 2, 1, 200, 36000);

        errors += query_plan_unittest_expect_best_tier(
            "sub-resolution window chooses densest overlapping tier", &qm, 103, 108, 5, 0);
    }

    {
        QUERY_METRIC qm = {0};
        query_plan_unittest_set_tier(&qm, 0, 1, 400000, 1);
        query_plan_unittest_set_tier(&qm, 1, 1, 400000, 600);
        query_plan_unittest_set_tier(&qm, 2, 1, 400000, 36000);

        errors += query_plan_unittest_expect_best_tier(
            "50 percent tolerance chooses sparsest acceptable tier", &qm, 1000, 301000, 500, 1);
    }

    {
        QUERY_METRIC qm = {0};
        query_plan_unittest_set_tier(&qm, 0, 1, 1000, 1);
        query_plan_unittest_set_tier(&qm, 1, 1, 1000, 10);
        query_plan_unittest_set_tier(&qm, 2, 1, 1000, 11);

        errors += query_plan_unittest_expect_best_tier(
            "50 percent tolerance includes exact threshold", &qm, 100, 400, 60, 1);
    }

    {
        QUERY_METRIC qm = {0};
        query_plan_unittest_set_tier(&qm, 0, 1, 1000, 10);
        query_plan_unittest_set_tier(&qm, 1, 1, 1000, 600);
        query_plan_unittest_set_tier(&qm, 2, 1, 1000, 36000);

        errors += query_plan_unittest_expect_best_tier(
            "under-resolution request chooses densest tier", &qm, 100, 700, 600, 0);
    }

    {
        QUERY_METRIC qm = {0};
        query_plan_unittest_set_tier(&qm, 0, 1, 50, 10);
        query_plan_unittest_set_tier(&qm, 1, 100, 200, 600);
        query_plan_unittest_set_tier(&qm, 2, 1, 50, 36000);

        errors += query_plan_unittest_expect_best_tier(
            "zero-overlap tiers are not candidates", &qm, 103, 108, 5, 1);
    }

    {
        QUERY_METRIC qm = {0};
        query_plan_unittest_set_tier(&qm, 1, 1, 200, 600);
        query_plan_unittest_set_tier(&qm, 2, 1, 200, 36000);

        errors += query_plan_unittest_expect_best_tier(
            "invalid duration returns first working tier", &qm, 108, 108, 5, 1);
    }

    {
        QUERY_METRIC qm = {0};
        query_plan_unittest_set_tier(&qm, 0, 1, 300, 10);
        query_plan_unittest_set_tier(&qm, 1, 1, 300, 600);

        QUERY_PLAN_ENTRY expected[] = {
            { .tier = 0, .after = 100, .before = 200 },
        };

        errors += query_plan_unittest_expect_plan(
            "selected tier covers full window", &qm, 0, 0, 100, 200, 10, expected, _countof(expected));
    }

    {
        QUERY_METRIC qm = {0};
        query_plan_unittest_set_tier(&qm, 0, 100, 180, 10);
        query_plan_unittest_set_tier(&qm, 1, 50, 150, 30);

        QUERY_PLAN_ENTRY expected[] = {
            { .tier = 1, .after = 50, .before = 100 },
            { .tier = 0, .after = 100, .before = 180 },
        };

        errors += query_plan_unittest_expect_plan(
            "coarser tier fills head gap", &qm, 0, 0, 50, 180, 10, expected, _countof(expected));
    }

    {
        QUERY_METRIC qm = {0};
        query_plan_unittest_set_tier(&qm, 0, 180, 260, 10);
        query_plan_unittest_set_tier(&qm, 1, 100, 200, 30);

        QUERY_PLAN_ENTRY expected[] = {
            { .tier = 1, .after = 100, .before = 200 },
            { .tier = 0, .after = 200, .before = 250 },
        };

        errors += query_plan_unittest_expect_plan(
            "finer tier fills tail gap", &qm, 0, 0, 100, 250, 10, expected, _countof(expected));
    }

    {
        QUERY_METRIC qm = {0};
        query_plan_unittest_set_tier(&qm, 0, 180, 260, 10);
        query_plan_unittest_set_tier(&qm, 1, 100, 200, 30);
        query_plan_unittest_set_tier(&qm, 2, 50, 150, 60);

        QUERY_PLAN_ENTRY expected[] = {
            { .tier = 2, .after = 50, .before = 100 },
            { .tier = 1, .after = 100, .before = 200 },
            { .tier = 0, .after = 200, .before = 250 },
        };

        errors += query_plan_unittest_expect_plan(
            "planner fills both head and tail gaps", &qm, 0, 0, 50, 250, 10, expected, _countof(expected));
    }

    {
        QUERY_METRIC qm = {0};
        query_plan_unittest_set_tier(&qm, 0, 180, 260, 10);
        query_plan_unittest_set_tier(&qm, 1, 100, 200, 30);
        query_plan_unittest_set_tier(&qm, 2, 50, 150, 60);

        QUERY_PLAN_ENTRY expected[] = {
            { .tier = 1, .after = 100, .before = 200 },
        };

        errors += query_plan_unittest_expect_plan(
            "explicit selected tier disables gap filling", &qm, RRDR_OPTION_SELECTED_TIER, 1,
            50, 250, 10, expected, _countof(expected));
    }

    {
        QUERY_METRIC qm = {0};
        query_plan_unittest_set_tier(&qm, 0, 1, 50, 10);
        query_plan_unittest_set_tier(&qm, 1, 60, 90, 30);
        query_plan_unittest_set_tier(&qm, 2, 100, 150, 60);

        errors += query_plan_unittest_expect_no_plan(
            "no overlapping tier fails planning", &qm, 0, 0, 200, 250, 10);
    }

    {
        QUERY_METRIC metrics[2] = {0};
        QUERY_TARGET qt = {0};

        metrics[0].tiers[1].db_update_every_s = 600;
        metrics[1].tiers[1].db_update_every_s = 300;
        qt.query.array = metrics;
        qt.query.used = 2;

        errors += query_plan_unittest_expect_update_every(&qt, 1, 300);
    }

    errors += query_plan_unittest_expect_ops_cache_is_local();

    nd_profile.storage_tiers = old_storage_tiers;
    nd_profile.update_every = old_update_every;

    return errors;
}
