// SPDX-License-Identifier: GPL-3.0-or-later

#include "query.h"
#include "../rrd2json.h"
#include "rrdr.h"

#include "average/average.h"
#include "incremental_sum/incremental_sum.h"
#include "max/max.h"
#include "median/median.h"
#include "min/min.h"
#include "sum/sum.h"
#include "stddev/stddev.h"

// ----------------------------------------------------------------------------

static struct {
    const char *name;
    uint32_t hash;
    RRDR_GROUPING value;
    void *(*init)(struct rrdresult *r);
    void (*reset)(struct rrdresult *r);
    void (*free)(struct rrdresult *r);
    void (*add)(struct rrdresult *r, calculated_number value);
    void (*flush)(struct rrdresult *r, calculated_number *rrdr_value_ptr, RRDR_VALUE_FLAGS *rrdr_value_options_ptr);
} api_v1_data_groups[] = {
          { "average"         , 0, RRDR_GROUPING_AVERAGE        , grouping_init_average        , grouping_reset_average        , grouping_free_average        , grouping_add_average        , grouping_flush_average }
        , { "median"          , 0, RRDR_GROUPING_MEDIAN         , grouping_init_median         , grouping_reset_median         , grouping_free_median         , grouping_add_median         , grouping_flush_median }
        , { "min"             , 0, RRDR_GROUPING_MIN            , grouping_init_min            , grouping_reset_min            , grouping_free_min            , grouping_add_min            , grouping_flush_min }
        , { "max"             , 0, RRDR_GROUPING_MAX            , grouping_init_max            , grouping_reset_max            , grouping_free_max            , grouping_add_max            , grouping_flush_max }
        , { "sum"             , 0, RRDR_GROUPING_SUM            , grouping_init_sum            , grouping_reset_sum            , grouping_free_sum            , grouping_add_sum            , grouping_flush_sum }
        , { "stddev"          , 0, RRDR_GROUPING_STDDEV         , grouping_init_stddev         , grouping_reset_stddev         , grouping_free_stddev         , grouping_add_stddev         , grouping_flush_stddev }
        , { "incremental_sum" , 0, RRDR_GROUPING_INCREMENTAL_SUM, grouping_init_incremental_sum, grouping_reset_incremental_sum, grouping_free_incremental_sum, grouping_add_incremental_sum, grouping_flush_incremental_sum }
        , { "incremental-sum" , 0, RRDR_GROUPING_INCREMENTAL_SUM, grouping_init_incremental_sum, grouping_reset_incremental_sum, grouping_free_incremental_sum, grouping_add_incremental_sum, grouping_flush_incremental_sum }
        , { NULL              , 0, RRDR_GROUPING_UNDEFINED      , grouping_init_average        , grouping_reset_average        , grouping_free_average        , grouping_add_average        , grouping_flush_average }
};

void web_client_api_v1_init_grouping(void) {
    int i;

    for(i = 0; api_v1_data_groups[i].name ; i++)
        api_v1_data_groups[i].hash = simple_hash(api_v1_data_groups[i].name);
}

const char *group_method2string(RRDR_GROUPING group) {
    int i;

    for(i = 0; api_v1_data_groups[i].name ; i++) {
        if(api_v1_data_groups[i].value == group) {
            return api_v1_data_groups[i].name;
        }
    }

    return "unknown-group-method";
}

RRDR_GROUPING web_client_api_request_v1_data_group(const char *name, RRDR_GROUPING def) {
    int i;

    uint32_t hash = simple_hash(name);
    for(i = 0; api_v1_data_groups[i].name ; i++)
        if(unlikely(hash == api_v1_data_groups[i].hash && !strcmp(name, api_v1_data_groups[i].name)))
            return api_v1_data_groups[i].value;

    return def;
}

// ----------------------------------------------------------------------------

static void rrdr_disable_not_selected_dimensions(RRDR *r, RRDR_OPTIONS options, const char *dims) {
    rrdset_check_rdlock(r->st);

    if(unlikely(!dims || !*dims || (dims[0] == '*' && dims[1] == '\0'))) return;

    int match_ids = 0, match_names = 0;

    if(unlikely(options & RRDR_OPTION_MATCH_IDS))
        match_ids = 1;
    if(unlikely(options & RRDR_OPTION_MATCH_NAMES))
        match_names = 1;

    if(likely(!match_ids && !match_names))
        match_ids = match_names = 1;

    SIMPLE_PATTERN *pattern = simple_pattern_create(dims, ",|\t\r\n\f\v", SIMPLE_PATTERN_EXACT);

    RRDDIM *d;
    long c, dims_selected = 0, dims_not_hidden_not_zero = 0;
    for(c = 0, d = r->st->dimensions; d ;c++, d = d->next) {
        if(    (match_ids   && simple_pattern_matches(pattern, d->id))
               || (match_names && simple_pattern_matches(pattern, d->name))
                ) {
            r->od[c] |= RRDR_DIMENSION_SELECTED;
            if(unlikely(r->od[c] & RRDR_DIMENSION_HIDDEN)) r->od[c] &= ~RRDR_DIMENSION_HIDDEN;
            dims_selected++;

            // since the user needs this dimension
            // make it appear as NONZERO, to return it
            // even if the dimension has only zeros
            // unless option non_zero is set
            if(unlikely(!(options & RRDR_OPTION_NONZERO)))
                r->od[c] |= RRDR_DIMENSION_NONZERO;

            // count the visible dimensions
            if(likely(r->od[c] & RRDR_DIMENSION_NONZERO))
                dims_not_hidden_not_zero++;
        }
        else {
            r->od[c] |= RRDR_DIMENSION_HIDDEN;
            if(unlikely(r->od[c] & RRDR_DIMENSION_SELECTED)) r->od[c] &= ~RRDR_DIMENSION_SELECTED;
        }
    }
    simple_pattern_free(pattern);

    // check if all dimensions are hidden
    if(unlikely(!dims_not_hidden_not_zero && dims_selected)) {
        // there are a few selected dimensions
        // but they are all zero
        // enable the selected ones
        // to avoid returning an empty chart
        for(c = 0, d = r->st->dimensions; d ;c++, d = d->next)
            if(unlikely(r->od[c] & RRDR_DIMENSION_SELECTED))
                r->od[c] |= RRDR_DIMENSION_NONZERO;
    }
}

// ----------------------------------------------------------------------------
// helpers to find our way in RRDR

static inline RRDR_VALUE_FLAGS *rrdr_line_options(RRDR *r, long rrdr_line) {
    return &r->o[ rrdr_line * r->d ];
}

static inline calculated_number *rrdr_line_values(RRDR *r, long rrdr_line) {
    return &r->v[ rrdr_line * r->d ];
}


static inline long rrdr_line_init(RRDR *r, time_t t, long rrdr_line) {
    rrdr_line++;

    if(unlikely(rrdr_line >= r->n)) {
        error("INTERNAL ERROR: requested to step above RRDR size for chart '%s'", r->st->name);
        rrdr_line = r->n - 1;
    }

    // save the time
    if(unlikely(r->t[rrdr_line] != 0 && r->t[rrdr_line] != t))
        error("INTERNAL ERROR: overwriting the timestamp of RRDR line %zu from %zu to %zu, of chart '%s'", (size_t)rrdr_line, (size_t)r->t[rrdr_line], (size_t)t, r->st->name);

    r->t[rrdr_line] = t;

    return rrdr_line;
}

static inline void rrdr_done(RRDR *r, long rrdr_line) {
    r->rows = rrdr_line + 1;
}


// ----------------------------------------------------------------------------
// fill RRDR for a single dimension

static inline void do_dimension(
          RRDR *r
        , long points
        , RRDDIM *rd
        , long dim_id_in_rrdr
        , long start_at_slot
        , long stop_at_slot
        , time_t after
        , time_t before
#ifdef NETDATA_INTERNAL_CHECKS
        , int debug
#endif
){
    RRDSET *st = r->st;

    time_t
        now = rrdset_slot2time(st, start_at_slot),
        dt = st->update_every,
        group_start_t = 0;

    long
        slot = start_at_slot,
        group = r->group,
        counter = 0,
        stop_now = 0,
        added = 0,
        group_count = 0,
        add_this = 0,
        found_non_zero = 0;

    RRDR_VALUE_FLAGS
        group_options = 0;


    #ifdef NETDATA_INTERNAL_CHECKS
    if(unlikely(debug)) debug(D_RRD_STATS, "BEGIN %s after_t: %u (stop_at_t: %ld), before_t: %u (start_at_t: %ld), start_t(now): %u, current_entry: %ld, entries: %ld"
                              , st->id
                              , (uint32_t)after
                              , stop_at_slot
                              , (uint32_t)before
                              , start_at_slot
                              , (uint32_t)now
                              , st->current_entry
                              , st->entries
        );
    #endif

    r->grouping_reset(r);

    time_t after_to_return = now;

    long rrdr_line = -1;
    for(; !stop_now ; now -= dt, slot--, counter++) {
        if(unlikely(slot < 0)) slot = st->entries - 1;
        if(unlikely(slot == stop_at_slot)) stop_now = counter;

        #ifdef NETDATA_INTERNAL_CHECKS
        if(unlikely(debug)) debug(D_RRD_STATS, "ROW %s slot: %ld, entries_counter: %ld, group_count: %ld, added: %ld, now: %ld, %s %s"
                                  , st->id
                                  , slot
                                  , counter
                                  , group_count + 1
                                  , added
                                  , now
                                  , (group_count + 1 == group)?"PRINT":"  -  "
                                  , (now >= after && now <= before)?"RANGE":"  -  "
            );
        #endif

        // make sure we return data in the proper time range
        if(unlikely(now > before)) continue;
        if(unlikely(now < after)) break;

        if(unlikely(group_count == 0)) group_start_t = now;
        group_count++;

        if(unlikely(group_count == group)) {
            if(unlikely(added >= points)) break;
            add_this = 1;
        }

        storage_number n = rd->values[slot];
        calculated_number value = NAN;
        if(likely(does_storage_number_exist(n))) {

            value = unpack_storage_number(n);
            if(likely(value != 0.0))
                found_non_zero = 1;

            if(unlikely(did_storage_number_reset(n)))
                group_options |= RRDR_VALUE_RESET;
        }

        // add this value for grouping
        r->grouping_add(r, value);

        // add it
        if(unlikely(add_this)) {
            rrdr_line = rrdr_line_init(r, group_start_t, rrdr_line);

            after_to_return = now;

            // find the place to store our values
            calculated_number *rrdr_value_ptr;
            RRDR_VALUE_FLAGS *rrdr_value_options_ptr;
            {
                calculated_number *cn = rrdr_line_values(r, rrdr_line);
                rrdr_value_ptr = &cn[dim_id_in_rrdr];

                RRDR_VALUE_FLAGS *co = rrdr_line_options(r, rrdr_line);
                rrdr_value_options_ptr = &co[dim_id_in_rrdr];
            }

            // update the dimension options
            if(likely(found_non_zero)) r->od[dim_id_in_rrdr] |= RRDR_DIMENSION_NONZERO;

            // store the specific point options
            *rrdr_value_options_ptr = group_options;

            // store the value
            r->grouping_flush(r, rrdr_value_ptr, rrdr_value_options_ptr);

            // find the min and max for the whole chart
            if(!(*rrdr_value_options_ptr & RRDR_VALUE_EMPTY)) {
                if(*rrdr_value_ptr < r->min) r->min = *rrdr_value_ptr;
                if(*rrdr_value_ptr > r->max) r->max = *rrdr_value_ptr;
            }

            added++;
            add_this = 0;
            group_count = 0;
            found_non_zero = 0;
        }
    }

    r->after = after_to_return;
    rrdr_done(r, rrdr_line);
}

// ----------------------------------------------------------------------------
// fill RRDR for the whole chart

RRDR *rrd2rrdr(
        RRDSET *st
        , long points
        , long long after
        , long long before
        , RRDR_GROUPING group_method
        , long group_time
        , RRDR_OPTIONS options
        , const char *dimensions
) {
    int aligned = !(options & RRDR_OPTION_NOT_ALIGNED);

#ifdef NETDATA_INTERNAL_CHECKS
    int debug = rrdset_flag_check(st, RRDSET_FLAG_DEBUG)?1:0;
#endif
    int absolute_period_requested = -1;

    time_t first_entry_t = rrdset_first_entry_t(st);
    time_t last_entry_t  = rrdset_last_entry_t(st);

    if(before == 0 && after == 0) {
        // dump the all the data
        before = last_entry_t;
        after = first_entry_t;
        absolute_period_requested = 0;
    }

    // allow relative for before (smaller than API_RELATIVE_TIME_MAX)
    if(((before < 0)?-before:before) <= API_RELATIVE_TIME_MAX) {
        if(abs(before) % st->update_every) {
            // make sure it is multiple of st->update_every
            if(before < 0) before = before - st->update_every - before % st->update_every;
            else           before = before + st->update_every - before % st->update_every;
        }
        if(before > 0) before = first_entry_t + before;
        else           before = last_entry_t  + before;
        absolute_period_requested = 0;
    }

    // allow relative for after (smaller than API_RELATIVE_TIME_MAX)
    if(((after < 0)?-after:after) <= API_RELATIVE_TIME_MAX) {
        if(after == 0) after = -st->update_every;
        if(abs(after) % st->update_every) {
            // make sure it is multiple of st->update_every
            if(after < 0) after = after - st->update_every - after % st->update_every;
            else          after = after + st->update_every - after % st->update_every;
        }
        after = before + after;
        absolute_period_requested = 0;
    }

    if(absolute_period_requested == -1)
        absolute_period_requested = 1;

    // make sure they are within our timeframe
    if(before > last_entry_t)  before = last_entry_t;
    if(before < first_entry_t) before = first_entry_t;

    if(after > last_entry_t)  after = last_entry_t;
    if(after < first_entry_t) after = first_entry_t;

    // check if they are upside down
    if(after > before) {
        time_t tmp = before;
        before = after;
        after = tmp;
    }

    // the duration of the chart
    time_t duration = before - after;
    long available_points = duration / st->update_every;

    if(duration <= 0 || available_points <= 0)
        return rrdr_create(st, 1);

    // check the number of wanted points in the result
    if(unlikely(points < 0)) points = -points;
    if(unlikely(points > available_points)) points = available_points;
    if(unlikely(points == 0)) points = available_points;

    // calculate the desired grouping of source data points
    long group = available_points / points;
    if(unlikely(group <= 0)) group = 1;
    if(unlikely(available_points % points > points / 2)) group++; // rounding to the closest integer

    // group_also time enforces a certain grouping multiple
    calculated_number group_sum_divisor = 1.0;
    long group_points = 1;
    if(unlikely(group_time > st->update_every)) {
        if (unlikely(group_time > duration)) {
            // group_time is above the available duration

#ifdef NETDATA_INTERNAL_CHECKS
            info("INTERNAL CHECK: %s: requested gtime %ld secs, is greater than the desired duration %ld secs", st->id, group_time, duration);
#endif

            group = points; // use all the points
        }
        else {
            // the points we should group to satisfy gtime
            group_points = group_time / st->update_every;
            if(unlikely(group_time % group_points)) {
#ifdef NETDATA_INTERNAL_CHECKS
                info("INTERNAL CHECK: %s: requested gtime %ld secs, is not a multiple of the chart's data collection frequency %d secs", st->id, group_time, st->update_every);
#endif

                group_points++;
            }

            // adapt group according to group_points
            if(unlikely(group < group_points)) group = group_points; // do not allow grouping below the desired one
            if(unlikely(group % group_points)) group += group_points - (group % group_points); // make sure group is multiple of group_points

            //group_sum_divisor = group / group_points;
            group_sum_divisor = (calculated_number)(group * st->update_every) / (calculated_number)group_time;
        }
    }

    time_t after_new  = after  - (after  % ( ((aligned)?group:1) * st->update_every ));
    time_t before_new = before - (before % ( ((aligned)?group:1) * st->update_every ));
    long points_new   = (before_new - after_new) / st->update_every / group;

    // find the starting and ending slots in our round robin db
    long    start_at_slot = rrdset_time2slot(st, before_new),
            stop_at_slot  = rrdset_time2slot(st, after_new);

#ifdef NETDATA_INTERNAL_CHECKS
    if(after_new < first_entry_t)
        error("INTERNAL CHECK: after_new %u is too small, minimum %u", (uint32_t)after_new, (uint32_t)first_entry_t);

    if(after_new > last_entry_t)
        error("INTERNAL CHECK: after_new %u is too big, maximum %u", (uint32_t)after_new, (uint32_t)last_entry_t);

    if(before_new < first_entry_t)
        error("INTERNAL CHECK: before_new %u is too small, minimum %u", (uint32_t)before_new, (uint32_t)first_entry_t);

    if(before_new > last_entry_t)
        error("INTERNAL CHECK: before_new %u is too big, maximum %u", (uint32_t)before_new, (uint32_t)last_entry_t);

    if(start_at_slot < 0 || start_at_slot >= st->entries)
        error("INTERNAL CHECK: start_at_slot is invalid %ld, expected 0 to %ld", start_at_slot, st->entries - 1);

    if(stop_at_slot < 0 || stop_at_slot >= st->entries)
        error("INTERNAL CHECK: stop_at_slot is invalid %ld, expected 0 to %ld", stop_at_slot, st->entries - 1);

    if(points_new > (before_new - after_new) / group / st->update_every + 1)
        error("INTERNAL CHECK: points_new %ld is more than points %ld", points_new, (before_new - after_new) / group / st->update_every + 1);

    if(group < group_points)
        error("INTERNAL CHECK: group %ld is less than the desired group points %ld", group, group_points);

    if(group > group_points && group % group_points)
        error("INTERNAL CHECK: group %ld is not a multiple of the desired group points %ld", group, group_points);
#endif

    //info("RRD2RRDR(): %s: wanted %ld points, got %ld - group=%ld, wanted duration=%u, got %u - wanted %ld - %ld, got %ld - %ld", st->id, points, points_new, group, before - after, before_new - after_new, after, before, after_new, before_new);

    after = after_new;
    before = before_new;
    duration = before - after;
    points = points_new;

    // Now we have:
    // before = the end time of the calculation
    // after = the start time of the calculation
    // duration = the duration of the calculation
    // group = the number of source points to aggregate / group together
    // method = the method of grouping source points
    // points = the number of points to generate


    // -------------------------------------------------------------------------
    // initialize our result set

    RRDR *r = rrdr_create(st, points);
    if(unlikely(!r)) {
#ifdef NETDATA_INTERNAL_CHECKS
        error("INTERNAL CHECK: Cannot create RRDR for %s, after=%u, before=%u, duration=%u, points=%ld", st->id, (uint32_t)after, (uint32_t)before, (uint32_t)duration, points);
#endif
        return NULL;
    }

    if(unlikely(!r->d)) {
#ifdef NETDATA_INTERNAL_CHECKS
        error("INTERNAL CHECK: Returning empty RRDR (no dimensions in RRDSET) for %s, after=%u, before=%u, duration=%u, points=%ld", st->id, (uint32_t)after, (uint32_t)before, (uint32_t)duration, points);
#endif
        return r;
    }

    if(unlikely(absolute_period_requested == 1))
        r->result_options |= RRDR_RESULT_OPTION_ABSOLUTE;
    else
        r->result_options |= RRDR_RESULT_OPTION_RELATIVE;

    // find how many dimensions we have
    long dimensions_count = r->d;

    // -------------------------------------------------------------------------
    // checks for debugging
#ifdef NETDATA_INTERNAL_CHECKS
    if(debug) debug(D_RRD_STATS, "INFO %s first_t: %u, last_t: %u, all_duration: %u, after: %u, before: %u, duration: %u, points: %ld, group: %ld, group_points: %ld"
                    , st->id
                    , (uint32_t)first_entry_t
                    , (uint32_t)last_entry_t
                    , (uint32_t)(last_entry_t - first_entry_t)
                    , (uint32_t)after
                    , (uint32_t)before
                    , (uint32_t)duration
                    , points
                    , group
                    , group_points
        );
#endif

    // -------------------------------------------------------------------------
    // the main loop

    r->group = group;
    r->update_every = (int)group * st->update_every;
    r->before = rrdset_slot2time(st, start_at_slot);
    r->after = rrdset_slot2time(st, stop_at_slot) - st->update_every;
    r->group_points = group_points;
    r->group_sum_divisor = group_sum_divisor;

    //info("RRD2RRDR(): %s: STARTING", st->id);

    {
        int i, found = 0;
        for(i = 0; !found && api_v1_data_groups[i].name ;i++) {
            if(api_v1_data_groups[i].value == group_method) {
                r->grouping_init  = api_v1_data_groups[i].init;
                r->grouping_reset = api_v1_data_groups[i].reset;
                r->grouping_free  = api_v1_data_groups[i].free;
                r->grouping_add   = api_v1_data_groups[i].add;
                r->grouping_flush = api_v1_data_groups[i].flush;
                found = 1;
            }
        }
        if(!found) {
            errno = 0;
            error("INTERNAL ERROR: grouping method %d not found for chart '%s'. Using 'average'", (int)group_method, r->st->name);
            r->grouping_init  = grouping_init_average;
            r->grouping_reset = grouping_reset_average;
            r->grouping_free  = grouping_free_average;
            r->grouping_add   = grouping_add_average;
            r->grouping_flush = grouping_flush_average;
        }
    }

    r->grouping_data = r->grouping_init(r);

    if(dimensions)
        rrdr_disable_not_selected_dimensions(r, options, dimensions);

    rrdset_check_rdlock(st);

    time_t max_after = 0;
    long max_rows = 0;

    RRDDIM *rd;
    long c, dimensions_used = 0, dimensions_nonzero = 0;
    for(rd = st->dimensions, c = 0 ; rd && c < dimensions_count ; rd = rd->next, c++) {
        if(unlikely(!(options & RRDR_OPTION_PERCENTAGE) && (r->od[c] & RRDR_DIMENSION_HIDDEN)))
            continue;

        do_dimension(
                r
                , points
                , rd
                , c
                , start_at_slot
                , stop_at_slot
                , after
                , before
#ifdef NETDATA_INTERNAL_CHECKS
                , debug
#endif
                );

        if(r->od[c] & RRDR_DIMENSION_NONZERO)
            dimensions_nonzero++;

        // verify all dimensions are aligned
        if(unlikely(!dimensions_used)) {
            max_after = r->after;
            max_rows = r->rows;
        }
        else {
            if(r->after != max_after) {
                #ifdef NETDATA_INTERNAL_CHECKS
                error("INTERNAL ERROR: 'after' mismatch between dimensions for chart '%s': max is %zu, dimension '%s' has %zu",
                        st->name, (size_t)max_after, rd->name, (size_t)r->after);
                #endif
                r->after = (r->after > max_after) ? r->after : max_after;
            }

            if(r->rows != max_rows) {
                #ifdef NETDATA_INTERNAL_CHECKS
                error("INTERNAL ERROR: 'rows' mismatch between dimensions for chart '%s': max is %zu, dimension '%s' has %zu",
                        st->name, (size_t)max_rows, rd->name, (size_t)r->rows);
                #endif
                r->rows = (r->rows > max_rows) ? r->rows : max_rows;
            }
        }

        dimensions_used++;
    }

    r->grouping_free(r);

    if(unlikely(options & RRDR_OPTION_NONZERO && !dimensions_nonzero)) {
        // all the dimensions are zero
        // send them all
        for(rd = st->dimensions, c = 0 ; rd && c < dimensions_count ; rd = rd->next, c++) {
            if(unlikely(r->od[c] & RRDR_DIMENSION_HIDDEN)) continue;
            r->od[c] |= RRDR_DIMENSION_NONZERO;
        }
    }

    //info("RRD2RRDR(): %s: END %ld loops made, %ld points generated", st->id, counter, rrdr_rows(r));
    //error("SHIFT: %s: wanted %ld points, got %ld", st->id, points, rrdr_rows(r));
    return r;
}
