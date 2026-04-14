// SPDX-License-Identifier: GPL-3.0-or-later

#include "query-internal.h"

// ----------------------------------------------------------------------------
// helpers to find our way in RRDR

ALWAYS_INLINE
static RRDR_VALUE_FLAGS *UNUSED_FUNCTION(rrdr_line_options)(RRDR *r, long rrdr_line) {
    return &r->o[ rrdr_line * r->d ];
}

ALWAYS_INLINE
static NETDATA_DOUBLE *UNUSED_FUNCTION(rrdr_line_values)(RRDR *r, long rrdr_line) {
    return &r->v[ rrdr_line * r->d ];
}

ALWAYS_INLINE
static long rrdr_line_init(RRDR *r __maybe_unused, time_t t __maybe_unused, long rrdr_line) {
    rrdr_line++;

    internal_fatal(rrdr_line >= (long)r->n,
                   "QUERY: requested to step above RRDR size for query '%s'",
                   r->internal.qt->id);

    internal_fatal(r->t[rrdr_line] != t,
                   "QUERY: wrong timestamp at RRDR line %ld, expected %ld, got %ld, of query '%s'",
                   rrdr_line, r->t[rrdr_line], t, r->internal.qt->id);

    return rrdr_line;
}

// ----------------------------------------------------------------------------
// dimension level query engine

#define query_interpolate_point(this_point, last_point, now)      do {  \
    if(likely(                                                          \
            /* the point to interpolate is more than 1s wide */         \
            (this_point).sp.end_time_s - (this_point).sp.start_time_s > 1 \
                                                                        \
            /* the two points are exactly next to each other */         \
         && (last_point).sp.end_time_s == (this_point).sp.start_time_s  \
                                                                        \
            /* both points are valid numbers */                         \
         && netdata_double_isnumber((this_point).value)                 \
         && netdata_double_isnumber((last_point).value)                 \
                                                                        \
        )) {                                                            \
            (this_point).value = (last_point).value + ((this_point).value - (last_point).value) * (1.0 - (NETDATA_DOUBLE)((this_point).sp.end_time_s - (now)) / (NETDATA_DOUBLE)((this_point).sp.end_time_s - (this_point).sp.start_time_s)); \
            (this_point).sp.end_time_s = now;                           \
        }                                                               \
} while(0)

#define query_add_point_to_group(r, point, ops, add_flush)        do {  \
    if(likely(netdata_double_isnumber((point).value))) {                \
        if(likely(fpclassify((point).value) != FP_ZERO))                \
            (ops)->group_points_non_zero++;                             \
                                                                        \
        if(unlikely((point).sp.flags & SN_FLAG_RESET))                  \
            (ops)->group_value_flags |= RRDR_VALUE_RESET;               \
                                                                        \
        time_grouping_add(r, (point).value, add_flush);                 \
                                                                        \
        storage_point_merge_to((ops)->group_point, (point).sp);         \
        if(!(point).added)                                              \
            storage_point_merge_to((ops)->query_point, (point).sp);     \
    }                                                                   \
                                                                        \
    (ops)->group_points_added++;                                        \
} while(0)

NOT_INLINE_HOT void rrd2rrdr_query_execute(RRDR *r, size_t dim_id_in_rrdr, QUERY_ENGINE_OPS *ops) {
    QUERY_TARGET *qt = r->internal.qt;
    QUERY_METRIC *qm = ops->qm;

    const RRDR_TIME_GROUPING add_flush = r->time_grouping.add_flush;

    ops->group_point = STORAGE_POINT_UNSET;
    ops->query_point = STORAGE_POINT_UNSET;

    RRDR_OPTIONS options = qt->window.options;
    size_t points_wanted = qt->window.points;
    time_t after_wanted = qt->window.after;
    time_t before_wanted = qt->window.before; (void)before_wanted;

//    bool debug_this = false;
//    if(strcmp("user", string2str(rd->id)) == 0 && strcmp("system.cpu", string2str(rd->rrdset->id)) == 0)
//        debug_this = true;

    size_t points_added = 0;

    long rrdr_line = -1;
    bool use_anomaly_bit_as_value = (r->internal.qt->window.options & RRDR_OPTION_ANOMALY_BIT) ? true : false;

    NETDATA_DOUBLE min = r->view.min, max = r->view.max;

    QUERY_POINT last2_point = QUERY_POINT_EMPTY;
    QUERY_POINT last1_point = QUERY_POINT_EMPTY;
    QUERY_POINT new_point   = QUERY_POINT_EMPTY;

    // ONE POINT READ-AHEAD
    // when we switch plans, we read-ahead a point from the next plan
    // to join them smoothly at the exact time the next plan begins
    STORAGE_POINT next1_point = STORAGE_POINT_UNSET;

    time_t now_start_time = after_wanted - ops->query_granularity;
    time_t now_end_time   = after_wanted + ops->view_update_every - ops->query_granularity;

    size_t db_points_read_since_plan_switch = 0; (void)db_points_read_since_plan_switch;
    size_t query_is_finished_counter = 0;

    // The main loop, based on the query granularity we need
    for( ; points_added < points_wanted && query_is_finished_counter <= 10 ;
        now_start_time = now_end_time, now_end_time += ops->view_update_every) {

        if(unlikely(query_plan_should_switch_plan(ops, now_end_time))) {
            query_planer_next_plan(ops, now_end_time, new_point.sp.end_time_s);
            db_points_read_since_plan_switch = 0;
        }

        // read all the points of the db, prior to the time we need (now_end_time)

        size_t count_same_end_time = 0;
        while(count_same_end_time < 100) {
            if(likely(count_same_end_time == 0)) {
                last2_point = last1_point;
                last1_point = new_point;
            }

            if(unlikely(storage_engine_query_is_finished(ops->seqh))) {
                query_is_finished_counter++;

                if(count_same_end_time != 0) {
                    last2_point = last1_point;
                    last1_point = new_point;
                }
                new_point = QUERY_POINT_EMPTY;
                new_point.sp.start_time_s = last1_point.sp.end_time_s;
                new_point.sp.end_time_s   = now_end_time;
//
//                if(debug_this) netdata_log_info("QUERY: is finished() returned true");
//
                break;
            }
            else
                query_is_finished_counter = 0;

            // fetch the new point
            {
                STORAGE_POINT sp;
                if(likely(storage_point_is_unset(next1_point))) {
                    db_points_read_since_plan_switch++;
                    sp = storage_engine_query_next_metric(ops->seqh);
                    ops->db_points_read_per_tier[ops->tier]++;
                    ops->db_total_points_read++;

                    if(unlikely(options & RRDR_OPTION_ABSOLUTE))
                        storage_point_make_positive(sp);
                }
                else {
                    // ONE POINT READ-AHEAD
                    sp = next1_point;
                    storage_point_unset(next1_point);
                    db_points_read_since_plan_switch = 1;
                }

                // ONE POINT READ-AHEAD
                if(unlikely(query_plan_should_switch_plan(ops, sp.end_time_s) &&
                    query_planer_next_plan(ops, now_end_time, new_point.sp.end_time_s))) {

                    // The end time of the current point, crosses our plans (tiers)
                    // so, we switched plan (tier)
                    //
                    // There are 2 cases now:
                    //
                    // A. the entire point of the previous plan is to the future of point from the next plan
                    // B. part of the point of the previous plan overlaps with the point from the next plan

                    STORAGE_POINT sp2 = storage_engine_query_next_metric(ops->seqh);
                    ops->db_points_read_per_tier[ops->tier]++;
                    ops->db_total_points_read++;

                    if(unlikely(options & RRDR_OPTION_ABSOLUTE))
                        storage_point_make_positive(sp);

                    if(sp.start_time_s > sp2.start_time_s)
                        // the point from the previous plan is useless
                        sp = sp2;
                    else
                        // let the query run from the previous plan
                        // but setting this will also cut off the interpolation
                        // of the point from the previous plan
                        next1_point = sp2;
                }

                new_point.sp = sp;
                new_point.added = false;
                query_point_set_id(new_point, ops->db_total_points_read);

//                if(debug_this)
//                    netdata_log_info("QUERY: got point %zu, from time %ld to %ld   //   now from %ld to %ld   //   query from %ld to %ld",
//                         new_point.id, new_point.start_time, new_point.end_time, now_start_time, now_end_time, after_wanted, before_wanted);
//
                // get the right value from the point we got
                if(likely(!storage_point_is_unset(sp) && !storage_point_is_gap(sp))) {

                    if(unlikely(use_anomaly_bit_as_value))
                        new_point.value = storage_point_anomaly_rate(new_point.sp);

                    else {
                        switch (ops->tier_query_fetch) {
                            default:
                            case TIER_QUERY_FETCH_AVERAGE:
                                new_point.value = sp.sum / (NETDATA_DOUBLE)sp.count;
                                break;

                            case TIER_QUERY_FETCH_MIN:
                                new_point.value = sp.min;
                                break;

                            case TIER_QUERY_FETCH_MAX:
                                new_point.value = sp.max;
                                break;

                            case TIER_QUERY_FETCH_SUM:
                                new_point.value = sp.sum;
                                break;
                        }
                    }
                }
                else
                    new_point.value      = NAN;
            }

            // check if the db is giving us zero duration points
            if(unlikely(db_points_read_since_plan_switch > 1 &&
                        new_point.sp.start_time_s == new_point.sp.end_time_s)) {

                internal_error(true, "QUERY: '%s', dimension '%s' next_metric() returned "
                                     "point %zu from %ld to %ld, that are both equal",
                               qt->id, query_metric_id(qt, qm),
                               new_point.id, new_point.sp.start_time_s, new_point.sp.end_time_s);

                new_point.sp.start_time_s = new_point.sp.end_time_s - ops->tier_ptr->db_update_every_s;
            }

            // check if the db is advancing the query
            if(unlikely(db_points_read_since_plan_switch > 1 &&
                        new_point.sp.end_time_s <= last1_point.sp.end_time_s)) {

                internal_error(true,
                               "QUERY: '%s', dimension '%s' next_metric() returned "
                               "point %zu from %ld to %ld, before the "
                               "last point %zu from %ld to %ld, "
                               "now is %ld to %ld",
                               qt->id, query_metric_id(qt, qm),
                               new_point.id, new_point.sp.start_time_s, new_point.sp.end_time_s,
                               last1_point.id, last1_point.sp.start_time_s, last1_point.sp.end_time_s,
                               now_start_time, now_end_time);

                count_same_end_time++;
                continue;
            }
            count_same_end_time = 0;

            // decide how to use this point
            if(likely(new_point.sp.end_time_s < now_end_time)) { // likely to favor tier0
                // this db point ends before our now_end_time

                if(likely(new_point.sp.end_time_s >= now_start_time)) { // likely to favor tier0
                    // this db point ends after our now_start time

                    query_add_point_to_group(r, new_point, ops, add_flush);
                    new_point.added = true;
                }
                else {
                    // we don't need this db point
                    // it is totally outside our current time-frame

                    // this is desirable for the first point of the query
                    // because it allows us to interpolate the next point
                    // at exactly the time we will want

                    // we only log if this is not point 1
                    internal_error(new_point.sp.end_time_s < ops->plan_expanded_after &&
                                   db_points_read_since_plan_switch > 1,
                                   "QUERY: '%s', dimension '%s' next_metric() "
                                   "returned point %zu from %ld time %ld, "
                                   "which is entirely before our current timeframe %ld to %ld "
                                   "(and before the entire query, after %ld, before %ld)",
                                   qt->id, query_metric_id(qt, qm),
                                   new_point.id, new_point.sp.start_time_s, new_point.sp.end_time_s,
                                   now_start_time, now_end_time,
                                   ops->plan_expanded_after, ops->plan_expanded_before);
                }

            }
            else {
                // the point ends in the future
                // so, we will interpolate it below, at the inner loop
                break;
            }
        }

        if(unlikely(count_same_end_time)) {
            internal_error(true,
                           "QUERY: '%s', dimension '%s', the database does not advance the query,"
                           " it returned an end time less or equal to the end time of the last "
                           "point we got %ld, %zu times",
                           qt->id, query_metric_id(qt, qm),
                           last1_point.sp.end_time_s, count_same_end_time);

            if(unlikely(new_point.sp.end_time_s <= last1_point.sp.end_time_s))
                new_point.sp.end_time_s = now_end_time;
        }

        time_t stop_time = new_point.sp.end_time_s;
        if(unlikely(!storage_point_is_unset(next1_point) && next1_point.start_time_s >= now_end_time)) {
            // ONE POINT READ-AHEAD
            // the point crosses the start time of the
            // read ahead storage point we have read
            stop_time = next1_point.start_time_s;
        }

        // the inner loop
        // we have 3 points in memory: last2, last1, new
        // we select the one to use based on their timestamps

        internal_fatal(now_end_time > stop_time || points_added >= points_wanted,
            "QUERY: first part of query provides invalid point to interpolate (now_end_time %ld, stop_time %ld",
            now_end_time, stop_time);

        do {
            // now_start_time is wrong in this loop
            // but, we don't need it

            QUERY_POINT current_point;

            if(likely(now_end_time > new_point.sp.start_time_s)) {
                // it is time for our NEW point to be used
                current_point = new_point;
                new_point.added = true; // first copy, then set it, so that new_point will not be added again
                query_interpolate_point(current_point, last1_point, now_end_time);

//                internal_error(current_point.id > 0
//                                && last1_point.id == 0
//                                && current_point.end_time > after_wanted
//                                && current_point.end_time > now_end_time,
//                               "QUERY: '%s', dimension '%s', after %ld, before %ld, view update every %ld,"
//                               " query granularity %ld, interpolating point %zu (from %ld to %ld) at %ld,"
//                               " but we could really favor by having last_point1 in this query.",
//                               qt->id, string2str(qm->dimension.id),
//                               after_wanted, before_wanted,
//                               ops.view_update_every, ops.query_granularity,
//                               current_point.id, current_point.start_time, current_point.end_time,
//                               now_end_time);
            }
            else if(likely(now_end_time <= last1_point.sp.end_time_s)) {
                // our LAST point is still valid
                current_point = last1_point;
                last1_point.added = true; // first copy, then set it, so that last1_point will not be added again
                query_interpolate_point(current_point, last2_point, now_end_time);

//                internal_error(current_point.id > 0
//                                && last2_point.id == 0
//                                && current_point.end_time > after_wanted
//                                && current_point.end_time > now_end_time,
//                               "QUERY: '%s', dimension '%s', after %ld, before %ld, view update every %ld,"
//                               " query granularity %ld, interpolating point %zu (from %ld to %ld) at %ld,"
//                               " but we could really favor by having last_point2 in this query.",
//                               qt->id, string2str(qm->dimension.id),
//                               after_wanted, before_wanted, ops.view_update_every, ops.query_granularity,
//                               current_point.id, current_point.start_time, current_point.end_time,
//                               now_end_time);
            }
            else {
                // a GAP, we don't have a value this time
                current_point = QUERY_POINT_EMPTY;
            }

            query_add_point_to_group(r, current_point, ops, add_flush);

            rrdr_line = rrdr_line_init(r, now_end_time, rrdr_line);
            size_t rrdr_o_v_index = rrdr_line * r->d + dim_id_in_rrdr;

            // find the place to store our values
            RRDR_VALUE_FLAGS *rrdr_value_options_ptr = &r->o[rrdr_o_v_index];

            // update the dimension options
            if(likely(ops->group_points_non_zero))
                r->od[dim_id_in_rrdr] |= RRDR_DIMENSION_NONZERO;

            // store the specific point options
            *rrdr_value_options_ptr = ops->group_value_flags;

            // store the group value
            NETDATA_DOUBLE group_value = time_grouping_flush(r, rrdr_value_options_ptr, add_flush);
            r->v[rrdr_o_v_index] = group_value;

            r->ar[rrdr_o_v_index] = storage_point_anomaly_rate(ops->group_point);

            if(likely(points_added || r->internal.queries_count)) {
                // find the min/max across all dimensions

                if(unlikely(group_value < min)) min = group_value;
                if(unlikely(group_value > max)) max = group_value;

            }
            else {
                // runs only when r->internal.queries_count == 0 && points_added == 0
                // so, on the first point added for the query.
                min = max = group_value;
            }

            points_added++;
            ops->group_points_added = 0;
            ops->group_value_flags = RRDR_VALUE_NOTHING;
            ops->group_points_non_zero = 0;
            ops->group_point = STORAGE_POINT_UNSET;

            now_end_time += ops->view_update_every;
        } while(now_end_time <= stop_time && points_added < points_wanted);

        // the loop above increased "now" by ops->view_update_every,
        // but the main loop will increase it too,
        // so, let's undo the last iteration of this loop
        now_end_time -= ops->view_update_every;
    }
    query_planer_finalize_remaining_plans(ops);

    qm->query_points = ops->query_point;

    // fill the rest of the points with empty values
    while (points_added < points_wanted) {
        rrdr_line++;
        size_t rrdr_o_v_index = rrdr_line * r->d + dim_id_in_rrdr;
        r->o[rrdr_o_v_index] = RRDR_VALUE_EMPTY;
        r->v[rrdr_o_v_index] = 0.0;
        r->ar[rrdr_o_v_index] = 0.0;
        points_added++;
    }

    r->internal.queries_count++;
    r->view.min = min;
    r->view.max = max;

    r->stats.result_points_generated += points_added;
    r->stats.db_points_read += ops->db_total_points_read;
    for(size_t tr = 0; tr < nd_profile.storage_tiers; tr++)
        qt->db.tiers[tr].points += ops->db_points_read_per_tier[tr];
}
