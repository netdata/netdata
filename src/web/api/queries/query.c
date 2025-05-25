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

NOT_INLINE_HOT static void rrd2rrdr_query_execute(RRDR *r, size_t dim_id_in_rrdr, QUERY_ENGINE_OPS *ops) {
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

// ----------------------------------------------------------------------------
// fill RRDR for the whole chart

#ifdef NETDATA_INTERNAL_CHECKS
static void rrd2rrdr_log_request_response_metadata(RRDR *r
        , RRDR_OPTIONS options __maybe_unused
        , RRDR_TIME_GROUPING group_method
        , bool aligned
        , size_t group
        , time_t resampling_time
        , size_t resampling_group
        , time_t after_wanted
        , time_t after_requested
        , time_t before_wanted
        , time_t before_requested
        , size_t points_requested
        , size_t points_wanted
        //, size_t after_slot
        //, size_t before_slot
        , const char *msg
        ) {

    QUERY_TARGET *qt = r->internal.qt;
    time_t first_entry_s = qt->db.first_time_s;
    time_t last_entry_s = qt->db.last_time_s;

    internal_error(
    true,
    "rrd2rrdr() on %s update every %ld with %s grouping %s (group: %zu, resampling_time: %ld, resampling_group: %zu), "
         "after (got: %ld, want: %ld, req: %ld, db: %ld), "
         "before (got: %ld, want: %ld, req: %ld, db: %ld), "
         "duration (got: %ld, want: %ld, req: %ld, db: %ld), "
         "points (got: %zu, want: %zu, req: %zu), "
         "%s"
         , qt->id
         , qt->window.query_granularity

         // grouping
         , (aligned) ? "aligned" : "unaligned"
         , time_grouping_id2txt(group_method)
         , group
         , resampling_time
         , resampling_group

         // after
         , r->view.after
         , after_wanted
         , after_requested
         , first_entry_s

         // before
         , r->view.before
         , before_wanted
         , before_requested
         , last_entry_s

         // duration
         , (long)(r->view.before - r->view.after + qt->window.query_granularity)
         , (long)(before_wanted - after_wanted + qt->window.query_granularity)
         , (long)before_requested - after_requested
         , (long)((last_entry_s - first_entry_s) + qt->window.query_granularity)

         // points
         , r->rows
         , points_wanted
         , points_requested

         // message
         , msg
    );
}
#endif // NETDATA_INTERNAL_CHECKS

// #define DEBUG_QUERY_LOGIC 1

#ifdef DEBUG_QUERY_LOGIC
#define query_debug_log_init() BUFFER *debug_log = buffer_create(1000)
#define query_debug_log(args...) buffer_sprintf(debug_log, ##args)
#define query_debug_log_fin() { \
        netdata_log_info("QUERY: '%s', after:%ld, before:%ld, duration:%ld, points:%zu, res:%ld - wanted => after:%ld, before:%ld, points:%zu, group:%zu, granularity:%ld, resgroup:%ld, resdiv:" NETDATA_DOUBLE_FORMAT_AUTO " %s", qt->id, after_requested, before_requested, before_requested - after_requested, points_requested, resampling_time_requested, after_wanted, before_wanted, points_wanted, group, query_granularity, resampling_group, resampling_divisor, buffer_tostring(debug_log)); \
        buffer_free(debug_log); \
        debug_log = NULL; \
    }
#define query_debug_log_free() do { buffer_free(debug_log); } while(0)
#else
#define query_debug_log_init() debug_dummy()
#define query_debug_log(args...) debug_dummy()
#define query_debug_log_fin() debug_dummy()
#define query_debug_log_free() debug_dummy()
#endif

bool query_target_calculate_window(QUERY_TARGET *qt) {
    if (unlikely(!qt)) return false;

    size_t points_requested = (long)qt->request.points;
    time_t after_requested = qt->request.after;
    time_t before_requested = qt->request.before;
    RRDR_TIME_GROUPING group_method = qt->request.time_group_method;
    time_t resampling_time_requested = qt->request.resampling_time;
    RRDR_OPTIONS options = qt->window.options;
    size_t tier = qt->request.tier;
    time_t update_every = qt->db.minimum_latest_update_every_s ? qt->db.minimum_latest_update_every_s : 1;

    // RULES
    // points_requested = 0
    // the user wants all the natural points the database has
    //
    // after_requested = 0
    // the user wants to start the query from the oldest point in our database
    //
    // before_requested = 0
    // the user wants the query to end to the latest point in our database
    //
    // when natural points are wanted, the query has to be aligned to the update_every
    // of the database

    size_t points_wanted = points_requested;
    time_t after_wanted = after_requested;
    time_t before_wanted = before_requested;

    bool aligned = !(options & RRDR_OPTION_NOT_ALIGNED);
    bool automatic_natural_points = (points_wanted == 0);
    bool relative_period_requested = false;
    bool natural_points = (options & RRDR_OPTION_NATURAL_POINTS) || automatic_natural_points;
    bool before_is_aligned_to_db_end = false;

    query_debug_log_init();

    if (ABS(before_requested) <= API_RELATIVE_TIME_MAX || ABS(after_requested) <= API_RELATIVE_TIME_MAX) {
        relative_period_requested = true;
        natural_points = true;
        options |= RRDR_OPTION_NATURAL_POINTS;
        query_debug_log(":relative+natural");
    }

    // if the user wants virtual points, make sure we do it
    if (options & RRDR_OPTION_VIRTUAL_POINTS)
        natural_points = false;

    // set the right flag about natural and virtual points
    if (natural_points) {
        options |= RRDR_OPTION_NATURAL_POINTS;

        if (options & RRDR_OPTION_VIRTUAL_POINTS)
            options &= ~RRDR_OPTION_VIRTUAL_POINTS;
    }
    else {
        options |= RRDR_OPTION_VIRTUAL_POINTS;

        if (options & RRDR_OPTION_NATURAL_POINTS)
            options &= ~RRDR_OPTION_NATURAL_POINTS;
    }

    if (after_wanted == 0 || before_wanted == 0) {
        relative_period_requested = true;

        time_t first_entry_s = qt->db.first_time_s;
        time_t last_entry_s = qt->db.last_time_s;

        if (first_entry_s == 0 || last_entry_s == 0) {
            internal_error(true, "QUERY: no data detected on query '%s' (db first_entry_t = %ld, last_entry_t = %ld)", qt->id, first_entry_s, last_entry_s);
            after_wanted = qt->window.after;
            before_wanted = qt->window.before;

            if(after_wanted == before_wanted)
                after_wanted = before_wanted - update_every;

            if (points_wanted == 0) {
                points_wanted = (before_wanted - after_wanted) / update_every;
                query_debug_log(":zero points_wanted %zu", points_wanted);
            }
        }
        else {
            query_debug_log(":first_entry_t %ld, last_entry_t %ld", first_entry_s, last_entry_s);

            if (after_wanted == 0) {
                after_wanted = first_entry_s;
                query_debug_log(":zero after_wanted %ld", after_wanted);
            }

            if (before_wanted == 0) {
                before_wanted = last_entry_s;
                before_is_aligned_to_db_end = true;
                query_debug_log(":zero before_wanted %ld", before_wanted);
            }

            if (points_wanted == 0) {
                points_wanted = (last_entry_s - first_entry_s) / update_every;
                query_debug_log(":zero points_wanted %zu", points_wanted);
            }
        }
    }

    if (points_wanted == 0) {
        points_wanted = 600;
        query_debug_log(":zero600 points_wanted %zu", points_wanted);
    }

    // convert our before_wanted and after_wanted to absolute
    rrdr_relative_window_to_absolute_query(&after_wanted, &before_wanted, NULL, unittest_running);
    query_debug_log(":relative2absolute after %ld, before %ld", after_wanted, before_wanted);

    if (natural_points && (options & RRDR_OPTION_SELECTED_TIER) && tier > 0 && nd_profile.storage_tiers > 1) {
        update_every = rrdset_find_natural_update_every_for_timeframe(
                qt, after_wanted, before_wanted, points_wanted, options, tier);

        if (update_every <= 0) update_every = qt->db.minimum_latest_update_every_s;
        query_debug_log(":natural update every %ld", update_every);
    }

    // this is the update_every of the query
    // it may be different to the update_every of the database
    time_t query_granularity = (natural_points) ? update_every : 1;
    if (query_granularity <= 0) query_granularity = 1;
    query_debug_log(":query_granularity %ld", query_granularity);

    // align before_wanted and after_wanted to query_granularity
    if (before_wanted % query_granularity) {
        before_wanted -= before_wanted % query_granularity;
        query_debug_log(":granularity align before_wanted %ld", before_wanted);
    }

    if (after_wanted % query_granularity) {
        after_wanted -= after_wanted % query_granularity;
        query_debug_log(":granularity align after_wanted %ld", after_wanted);
    }

    // automatic_natural_points is set when the user wants all the points available in the database
    if (automatic_natural_points) {
        points_wanted = (before_wanted - after_wanted + 1) / query_granularity;
        if (unlikely(points_wanted <= 0)) points_wanted = 1;
        query_debug_log(":auto natural points_wanted %zu", points_wanted);
    }

    time_t duration = before_wanted - after_wanted;

    // if the resampling time is too big, extend the duration to the past
    if (unlikely(resampling_time_requested > duration)) {
        after_wanted = before_wanted - resampling_time_requested;
        duration = before_wanted - after_wanted;
        query_debug_log(":resampling after_wanted %ld", after_wanted);
    }

    // if the duration is not aligned to resampling time
    // extend the duration to the past, to avoid a gap at the chart
    // only when the missing duration is above 1/10th of a point
    if (resampling_time_requested > query_granularity && duration % resampling_time_requested) {
        time_t delta = duration % resampling_time_requested;
        if (delta > resampling_time_requested / 10) {
            after_wanted -= resampling_time_requested - delta;
            duration = before_wanted - after_wanted;
            query_debug_log(":resampling2 after_wanted %ld", after_wanted);
        }
    }

    // the available points of the query
    size_t points_available = (duration + 1) / query_granularity;
    if (unlikely(points_available <= 0)) points_available = 1;
    query_debug_log(":points_available %zu", points_available);

    if (points_wanted > points_available) {
        points_wanted = points_available;
        query_debug_log(":max points_wanted %zu", points_wanted);
    }

    if(points_wanted > 86400 && !unittest_running) {
        points_wanted = 86400;
        query_debug_log(":absolute max points_wanted %zu", points_wanted);
    }

    // calculate the desired grouping of source data points
    size_t group = points_available / points_wanted;
    if (group == 0) group = 1;

    // round "group" to the closest integer
    if (points_available % points_wanted > points_wanted / 2)
        group++;

    query_debug_log(":group %zu", group);

    if (points_wanted * group * query_granularity < (size_t)duration) {
        // the grouping we are going to do, is not enough
        // to cover the entire duration requested, so
        // we have to change the number of points, to make sure we will
        // respect the timeframe as closely as possibly

        // let's see how many points are the optimal
        points_wanted = points_available / group;

        if (points_wanted * group < points_available)
            points_wanted++;

        if (unlikely(points_wanted == 0))
            points_wanted = 1;

        query_debug_log(":optimal points %zu", points_wanted);
    }

    // resampling_time_requested enforces a certain grouping multiple
    NETDATA_DOUBLE resampling_divisor = 1.0;
    size_t resampling_group = 1;
    if (unlikely(resampling_time_requested > query_granularity)) {
        // the points we should group to satisfy gtime
        resampling_group = resampling_time_requested / query_granularity;
        if (unlikely(resampling_time_requested % query_granularity))
            resampling_group++;

        query_debug_log(":resampling group %zu", resampling_group);

        // adapt group according to resampling_group
        if (unlikely(group < resampling_group)) {
            group = resampling_group; // do not allow grouping below the desired one
            query_debug_log(":group less res %zu", group);
        }
        if (unlikely(group % resampling_group)) {
            group += resampling_group - (group % resampling_group); // make sure group is multiple of resampling_group
            query_debug_log(":group mod res %zu", group);
        }

        // resampling_divisor = group / resampling_group;
        resampling_divisor = (NETDATA_DOUBLE) (group * query_granularity) / (NETDATA_DOUBLE) resampling_time_requested;
        query_debug_log(":resampling divisor " NETDATA_DOUBLE_FORMAT, resampling_divisor);
    }

    // now that we have group, align the requested timeframe to fit it.
    if (aligned && before_wanted % (group * query_granularity)) {
        if (before_is_aligned_to_db_end)
            before_wanted -= before_wanted % (time_t)(group * query_granularity);
        else
            before_wanted += (time_t)(group * query_granularity) - before_wanted % (time_t)(group * query_granularity);
        query_debug_log(":align before_wanted %ld", before_wanted);
    }

    after_wanted = before_wanted - (time_t)(points_wanted * group * query_granularity) + query_granularity;
    query_debug_log(":final after_wanted %ld", after_wanted);

    duration = before_wanted - after_wanted;
    query_debug_log(":final duration %ld", duration + 1);

    query_debug_log_fin();

    internal_error(points_wanted != duration / (query_granularity * group) + 1,
                   "QUERY: points_wanted %zu is not points %zu",
                   points_wanted, (size_t)(duration / (query_granularity * group) + 1));

    internal_error(group < resampling_group,
                   "QUERY: group %zu is less than the desired group points %zu",
                   group, resampling_group);

    internal_error(group > resampling_group && group % resampling_group,
                   "QUERY: group %zu is not a multiple of the desired group points %zu",
                   group, resampling_group);

    // -------------------------------------------------------------------------
    // update QUERY_TARGET with our calculations

    qt->window.after = after_wanted;
    qt->window.before = before_wanted;
    qt->window.relative = relative_period_requested;
    qt->window.points = points_wanted;
    qt->window.group = group;
    qt->window.time_group_method = group_method;
    qt->window.time_group_options = qt->request.time_group_options;
    qt->window.query_granularity = query_granularity;
    qt->window.resampling_group = resampling_group;
    qt->window.resampling_divisor = resampling_divisor;
    qt->window.options = options;
    qt->window.tier = tier;
    qt->window.aligned = aligned;

    return true;
}

// ----------------------------------------------------------------------------
// query entry point

RRDR *rrd2rrdr_legacy(
        ONEWAYALLOC *owa,
        RRDSET *st, size_t points, time_t after, time_t before,
        RRDR_TIME_GROUPING group_method, time_t resampling_time, RRDR_OPTIONS options, const char *dimensions,
        const char *group_options, time_t timeout_ms, size_t tier, QUERY_SOURCE query_source,
        STORAGE_PRIORITY priority) {

    QUERY_TARGET_REQUEST qtr = {
            .version = 1,
            .st = st,
            .points = points,
            .after = after,
            .before = before,
            .time_group_method = group_method,
            .resampling_time = resampling_time,
            .options = options,
            .dimensions = dimensions,
            .time_group_options = group_options,
            .timeout_ms = timeout_ms,
            .tier = tier,
            .query_source = query_source,
            .priority = priority,
    };

    QUERY_TARGET *qt = query_target_create(&qtr);
    RRDR *r = rrd2rrdr(owa, qt);
    if(!r) {
        query_target_release(qt);
        return NULL;
    }

    r->internal.release_with_rrdr_qt = qt;
    return r;
}

RRDR *rrd2rrdr(ONEWAYALLOC *owa, QUERY_TARGET *qt) {
    if(!qt || !owa)
        return NULL;

    // qt.window members are the WANTED ones.
    // qt.request members are the REQUESTED ones.

    RRDR *r_tmp = rrd2rrdr_group_by_initialize(owa, qt);
    if(!r_tmp)
        return NULL;

    // the RRDR we group-by at
    RRDR *r = (r_tmp->group_by.r) ? r_tmp->group_by.r : r_tmp;

    // the final RRDR to return to callers
    RRDR *last_r = r_tmp;
    while(last_r->group_by.r)
        last_r = last_r->group_by.r;

    if(qt->window.relative)
        last_r->view.flags |= RRDR_RESULT_FLAG_RELATIVE;
    else
        last_r->view.flags |= RRDR_RESULT_FLAG_ABSOLUTE;

    // -------------------------------------------------------------------------
    // assign the processor functions
    rrdr_set_grouping_function(r_tmp, qt->window.time_group_method);

    // allocate any memory required by the grouping method
    r_tmp->time_grouping.create(r_tmp, qt->window.time_group_options);

    // -------------------------------------------------------------------------
    // do the work for each dimension

    time_t max_after = 0, min_before = 0;
    size_t max_rows = 0;

    long dimensions_used = 0, dimensions_nonzero = 0;
    size_t last_db_points_read = 0;
    size_t last_result_points_generated = 0;

    // internal_fatal(released_ops, "QUERY: released_ops should be NULL when the query starts");

    query_progress_set_finish_line(qt->request.transaction, qt->query.used);

    QUERY_ENGINE_OPS **ops = NULL;
    if(qt->query.used)
        ops = onewayalloc_callocz(owa, qt->query.used, sizeof(QUERY_ENGINE_OPS *));

    size_t capacity = MAX(netdata_conf_cpus() / 2, 4);
    size_t max_queries_to_prepare = (qt->query.used > (capacity - 1)) ? (capacity - 1) : qt->query.used;
    size_t queries_prepared = 0;
    while(queries_prepared < max_queries_to_prepare) {
        // preload another query
        ops[queries_prepared] = rrd2rrdr_query_ops_prep(r_tmp, queries_prepared);
        queries_prepared++;
    }

    QUERY_NODE *last_qn = NULL;
    usec_t last_ut = now_monotonic_usec();
    usec_t last_qn_ut = last_ut;

    for(size_t d = 0; d < qt->query.used ; d++) {
        QUERY_METRIC *qm = query_metric(qt, d);
        QUERY_DIMENSION *qd = query_dimension(qt, qm->link.query_dimension_id);
        QUERY_INSTANCE *qi = query_instance(qt, qm->link.query_instance_id);
        QUERY_CONTEXT *qc = query_context(qt, qm->link.query_context_id);
        QUERY_NODE *qn = query_node(qt, qm->link.query_node_id);

        usec_t now_ut = last_ut;
        if(qn != last_qn) {
            if(last_qn)
                last_qn->duration_ut = now_ut - last_qn_ut;

            last_qn = qn;
            last_qn_ut = now_ut;
        }

        if(queries_prepared < qt->query.used) {
            // preload another query
            ops[queries_prepared] = rrd2rrdr_query_ops_prep(r_tmp, queries_prepared);
            queries_prepared++;
        }

        size_t dim_in_rrdr_tmp = (r_tmp != r) ? 0 : d;

        // set the query target dimension options to rrdr
        r_tmp->od[dim_in_rrdr_tmp] = qm->status;

        // reset the grouping for the new dimension
        r_tmp->time_grouping.reset(r_tmp);

        if(ops[d]) {
            rrd2rrdr_query_execute(r_tmp, dim_in_rrdr_tmp, ops[d]);
            r_tmp->od[dim_in_rrdr_tmp] |= RRDR_DIMENSION_QUERIED;

            now_ut = now_monotonic_usec();
            qm->duration_ut = now_ut - last_ut;
            last_ut = now_ut;

            if(r_tmp != r) {
                // copy back whatever got updated from the temporary r

                // the query updates RRDR_DIMENSION_NONZERO
                qm->status = r_tmp->od[dim_in_rrdr_tmp];

                // the query updates these
                r->view.min = r_tmp->view.min;
                r->view.max = r_tmp->view.max;
                r->view.after = r_tmp->view.after;
                r->view.before = r_tmp->view.before;
                r->rows = r_tmp->rows;

                rrd2rrdr_group_by_add_metric(r, qm->grouped_as.first_slot, r_tmp, dim_in_rrdr_tmp,
                                             qt->request.group_by[0].aggregation, &qm->query_points, 0);
            }

            rrd2rrdr_query_ops_release(ops[d]); // reuse this ops allocation
            ops[d] = NULL;

            qi->metrics.queried++;
            qc->metrics.queried++;
            qn->metrics.queried++;

            qd->status |= QUERY_STATUS_QUERIED;
            qm->status |= RRDR_DIMENSION_QUERIED;

            if(qt->request.version >= 2) {
                // we need to make the query points positive now
                // since we will aggregate it across multiple dimensions
                storage_point_make_positive(qm->query_points);
                storage_point_merge_to(qi->query_points, qm->query_points);
                storage_point_merge_to(qc->query_points, qm->query_points);
                storage_point_merge_to(qn->query_points, qm->query_points);
                storage_point_merge_to(qt->query_points, qm->query_points);
            }
        }
        else {
            qi->metrics.failed++;
            qc->metrics.failed++;
            qn->metrics.failed++;

            qd->status |= QUERY_STATUS_FAILED;
            qm->status |= RRDR_DIMENSION_FAILED;

            continue;
        }

        pulse_queries_rrdr_query_completed(
            1,
            r_tmp->stats.db_points_read - last_db_points_read,
            r_tmp->stats.result_points_generated - last_result_points_generated,
            qt->request.query_source);

        last_db_points_read = r_tmp->stats.db_points_read;
        last_result_points_generated = r_tmp->stats.result_points_generated;

        if(qm->status & RRDR_DIMENSION_NONZERO)
            dimensions_nonzero++;

        // verify all dimensions are aligned
        if(unlikely(!dimensions_used)) {
            min_before = r->view.before;
            max_after = r->view.after;
            max_rows = r->rows;
        }
        else {
            if(r->view.after != max_after) {
                internal_error(true, "QUERY: 'after' mismatch between dimensions for chart '%s': max is %zu, dimension '%s' has %zu",
                               rrdinstance_acquired_id(qi->ria), (size_t)max_after, rrdmetric_acquired_id(qd->rma), (size_t)r->view.after);

                r->view.after = (r->view.after > max_after) ? r->view.after : max_after;
            }

            if(r->view.before != min_before) {
                internal_error(true, "QUERY: 'before' mismatch between dimensions for chart '%s': max is %zu, dimension '%s' has %zu",
                               rrdinstance_acquired_id(qi->ria), (size_t)min_before, rrdmetric_acquired_id(qd->rma), (size_t)r->view.before);

                r->view.before = (r->view.before < min_before) ? r->view.before : min_before;
            }

            if(r->rows != max_rows) {
                internal_error(true, "QUERY: 'rows' mismatch between dimensions for chart '%s': max is %zu, dimension '%s' has %zu",
                               rrdinstance_acquired_id(qi->ria), (size_t)max_rows, rrdmetric_acquired_id(qd->rma), (size_t)r->rows);

                r->rows = (r->rows > max_rows) ? r->rows : max_rows;
            }
        }

        dimensions_used++;

        bool cancel = false;
        if (qt->request.interrupt_callback && qt->request.interrupt_callback(qt->request.interrupt_callback_data)) {
            cancel = true;
            nd_log(NDLS_ACCESS, NDLP_NOTICE, "QUERY INTERRUPTED");
        }

        if (qt->request.timeout_ms && ((NETDATA_DOUBLE)(now_ut - qt->timings.received_ut) / 1000.0) > (NETDATA_DOUBLE)qt->request.timeout_ms) {
            cancel = true;
            nd_log(NDLS_ACCESS, NDLP_WARNING, "QUERY CANCELED RUNTIME EXCEEDED %0.2f ms (LIMIT %lld ms)",
                       (NETDATA_DOUBLE)(now_ut - qt->timings.received_ut) / 1000.0, (long long)qt->request.timeout_ms);
        }

        if(cancel) {
            r->view.flags |= RRDR_RESULT_FLAG_CANCEL;

            for(size_t i = d + 1; i < queries_prepared ; i++) {
                if(ops[i]) {
                    query_planer_finalize_remaining_plans(ops[i]);
                    rrd2rrdr_query_ops_release(ops[i]);
                    ops[i] = NULL;
                }
            }

            break;
        }
        else
            query_progress_done_step(qt->request.transaction, 1);
    }

    // free all resources used by the grouping method
    r_tmp->time_grouping.free(r_tmp);

    // get the final RRDR to send to the caller
    r = rrd2rrdr_group_by_finalize(r_tmp);
    
    // apply cardinality limit if requested
    r = rrd2rrdr_cardinality_limit(r);

#ifdef NETDATA_INTERNAL_CHECKS
    if (dimensions_used && !(r->view.flags & RRDR_RESULT_FLAG_CANCEL)) {
        if(r->internal.log)
            rrd2rrdr_log_request_response_metadata(r, qt->window.options, qt->window.time_group_method, qt->window.aligned, qt->window.group, qt->request.resampling_time, qt->window.resampling_group,
                                                   qt->window.after, qt->request.after, qt->window.before, qt->request.before,
                                                   qt->request.points, qt->window.points, /*after_slot, before_slot,*/
                                                   r->internal.log);

        if(r->rows != qt->window.points)
            rrd2rrdr_log_request_response_metadata(r, qt->window.options, qt->window.time_group_method, qt->window.aligned, qt->window.group, qt->request.resampling_time, qt->window.resampling_group,
                                                   qt->window.after, qt->request.after, qt->window.before, qt->request.before,
                                                   qt->request.points, qt->window.points, /*after_slot, before_slot,*/
                                                   "got 'points' is not wanted 'points'");

        if(qt->window.aligned && (r->view.before % query_view_update_every(qt)) != 0)
            rrd2rrdr_log_request_response_metadata(r, qt->window.options, qt->window.time_group_method, qt->window.aligned, qt->window.group, qt->request.resampling_time, qt->window.resampling_group,
                                                   qt->window.after, qt->request.after, qt->window.before, qt->request.before,
                                                   qt->request.points, qt->window.points, /*after_slot, before_slot,*/
                                                   "'before' is not aligned but alignment is required");

        // 'after' should not be aligned, since we start inside the first group
        //if(qt->window.aligned && (r->after % group) != 0)
        //    rrd2rrdr_log_request_response_metadata(r, qt->window.options, qt->window.group_method, qt->window.aligned, qt->window.group, qt->request.resampling_time, qt->window.resampling_group, qt->window.after, after_requested, before_wanted, before_requested, points_requested, points_wanted, after_slot, before_slot, "'after' is not aligned but alignment is required");

        if(r->view.before != qt->window.before)
            rrd2rrdr_log_request_response_metadata(r, qt->window.options, qt->window.time_group_method, qt->window.aligned, qt->window.group, qt->request.resampling_time, qt->window.resampling_group,
                                                   qt->window.after, qt->request.after, qt->window.before, qt->request.before,
                                                   qt->request.points, qt->window.points, /*after_slot, before_slot,*/
                                                   "chart is not aligned to requested 'before'");

        if(r->view.before != qt->window.before)
            rrd2rrdr_log_request_response_metadata(r, qt->window.options, qt->window.time_group_method, qt->window.aligned, qt->window.group, qt->request.resampling_time, qt->window.resampling_group,
                                                   qt->window.after, qt->request.after, qt->window.before, qt->request.before,
                                                   qt->request.points, qt->window.points, /*after_slot, before_slot,*/
                                                   "got 'before' is not wanted 'before'");

        // reported 'after' varies, depending on group
        if(r->view.after != qt->window.after)
            rrd2rrdr_log_request_response_metadata(r, qt->window.options, qt->window.time_group_method, qt->window.aligned, qt->window.group, qt->request.resampling_time, qt->window.resampling_group,
                                                   qt->window.after, qt->request.after, qt->window.before, qt->request.before,
                                                   qt->request.points, qt->window.points, /*after_slot, before_slot,*/
                                                   "got 'after' is not wanted 'after'");

    }
#endif

    // free the query pipelining ops
    for(size_t d = 0; d < qt->query.used ; d++) {
        rrd2rrdr_query_ops_release(ops[d]);
        ops[d] = NULL;
    }
    rrd2rrdr_query_ops_freeall(r);
    // internal_fatal(released_ops, "QUERY: released_ops should be NULL when the query ends");

    onewayalloc_freez(owa, ops);

    if(likely(dimensions_used && (qt->window.options & RRDR_OPTION_NONZERO) && !dimensions_nonzero))
        // when all the dimensions are zero, we should return all of them
        qt->window.options &= ~RRDR_OPTION_NONZERO;

    qt->timings.executed_ut = now_monotonic_usec();

    return r;
}
