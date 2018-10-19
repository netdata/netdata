//
// Created by costa on 19/10/18.
//

#include "query.h"

// ----------------------------------------------------------------------------
// helpers to find our way in RRDR

static inline uint8_t *rrdr_line_options(RRDR *r, long rrdr_line) {
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
// average

struct grouping_average {
    calculated_number sum;
    size_t count;
};

static void *grouping_init_average(RRDR *r) {
    (void)r;
    return callocz(1, sizeof(struct grouping_average));
}

static void grouping_free_average(RRDR *r, void *data) {
    (void)r;
    freez(data);
}

static void grouping_add_average(RRDR *r, void *data, calculated_number value) {
    (void)r;

    if(!isnan(value)) {
        struct grouping_average *g = (struct grouping_average *)data;
        g->sum += value;
        g->count++;
    }
}

static void grouping_flush_average(RRDR *r, calculated_number *rrdr_value_ptr, uint8_t *rrdr_value_options_ptr, void *data) {
    struct grouping_average *g = (struct grouping_average *)data;

    if(unlikely(!g->count)) {
        *rrdr_value_ptr = 0.0;
        *rrdr_value_options_ptr |= RRDR_EMPTY;
    }
    else {
        if(unlikely(r->group_points != 1))
            *rrdr_value_ptr = g->sum / r->group_sum_divisor;
        else
            *rrdr_value_ptr = g->sum / g->count;
    }

    g->sum = 0.0;
    g->count = 0;
}

// ----------------------------------------------------------------------------
// min

struct grouping_min {
    calculated_number min;
    size_t count;
};

static void *grouping_init_min(RRDR *r) {
    (void)r;
    return callocz(1, sizeof(struct grouping_min));
}

static void grouping_free_min(RRDR *r, void *data) {
    (void)r;
    freez(data);
}

static void grouping_add_min(RRDR *r, void *data, calculated_number value) {
    (void)r;

    if(!isnan(value)) {
        struct grouping_min *g = (struct grouping_min *)data;

        if(!g->count || calculated_number_fabs(value) < calculated_number_fabs(g->min)) {
            g->min = value;
            g->count++;
        }
    }
}

static void grouping_flush_min(RRDR *r, calculated_number *rrdr_value_ptr, uint8_t *rrdr_value_options_ptr, void *data) {
    (void)r;
    struct grouping_min *g = (struct grouping_min *)data;

    if(unlikely(!g->count)) {
        *rrdr_value_ptr = 0.0;
        *rrdr_value_options_ptr |= RRDR_EMPTY;
    }
    else {
        *rrdr_value_ptr = g->min;
    }

    g->min = 0.0;
    g->count = 0;
}

// ----------------------------------------------------------------------------
// max

struct grouping_max {
    calculated_number max;
    size_t count;
};

static void *grouping_init_max(RRDR *r) {
    (void)r;
    return callocz(1, sizeof(struct grouping_max));
}

static void grouping_free_max(RRDR *r, void *data) {
    (void)r;
    freez(data);
}

static void grouping_add_max(RRDR *r, void *data, calculated_number value) {
    (void)r;

    if(!isnan(value)) {
        struct grouping_max *g = (struct grouping_max *)data;

        if(!g->count || calculated_number_fabs(value) > calculated_number_fabs(g->max)) {
            g->max = value;
            g->count++;
        }
    }
}

static void grouping_flush_max(RRDR *r, calculated_number *rrdr_value_ptr, uint8_t *rrdr_value_options_ptr, void *data) {
    (void)r;
    struct grouping_max *g = (struct grouping_max *)data;

    if(unlikely(!g->count)) {
        *rrdr_value_ptr = 0.0;
        *rrdr_value_options_ptr |= RRDR_EMPTY;
    }
    else {
        *rrdr_value_ptr = g->max;
    }

    g->max = 0.0;
    g->count = 0;
}

// ----------------------------------------------------------------------------
// sum

struct grouping_sum {
    calculated_number sum;
    size_t count;
};

static void *grouping_init_sum(RRDR *r) {
    (void)r;
    return callocz(1, sizeof(struct grouping_sum));
}

static void grouping_free_sum(RRDR *r, void *data) {
    (void)r;
    freez(data);
}

static void grouping_add_sum(RRDR *r, void *data, calculated_number value) {
    (void)r;

    if(!isnan(value)) {
        struct grouping_sum *g = (struct grouping_sum *)data;

        if(!g->count || calculated_number_fabs(value) > calculated_number_fabs(g->sum)) {
            g->sum += value;
            g->count++;
        }
    }
}

static void grouping_flush_sum(RRDR *r, calculated_number *rrdr_value_ptr, uint8_t *rrdr_value_options_ptr, void *data) {
    (void)r;
    struct grouping_sum *g = (struct grouping_sum *)data;

    if(unlikely(!g->count)) {
        *rrdr_value_ptr = 0.0;
        *rrdr_value_options_ptr |= RRDR_EMPTY;
    }
    else {
        *rrdr_value_ptr = g->sum;
    }

    g->sum = 0.0;
    g->count = 0;
}


// ----------------------------------------------------------------------------
// incremental sum

struct grouping_incremental_sum {
    calculated_number first;
    calculated_number last;
    size_t count;
};

static void *grouping_init_incremental_sum(RRDR *r) {
    (void)r;
    return callocz(1, sizeof(struct grouping_incremental_sum));
}

static void grouping_free_incremental_sum(RRDR *r, void *data) {
    (void)r;
    freez(data);
}

static void grouping_add_incremental_sum(RRDR *r, void *data, calculated_number value) {
    (void)r;

    if(!isnan(value)) {
        struct grouping_incremental_sum *g = (struct grouping_incremental_sum *)data;

        if(unlikely(!g->count)) {
            g->first = value;
            g->count++;
        }
        else {
            g->last = value;
            g->count++;
        }
    }
}

static void grouping_flush_incremental_sum(RRDR *r, calculated_number *rrdr_value_ptr, uint8_t *rrdr_value_options_ptr, void *data) {
    (void)r;
    struct grouping_incremental_sum *g = (struct grouping_incremental_sum *)data;

    if(unlikely(!g->count)) {
        *rrdr_value_ptr = 0.0;
        *rrdr_value_options_ptr |= RRDR_EMPTY;
    }
    else if(unlikely(g->count == 1)) {
        *rrdr_value_ptr = 0.0;
    }
    else {
        *rrdr_value_ptr = g->last - g->first;
    }

    g->first = 0.0;
    g->last = 0.0;
    g->count = 0;
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
        , int group_method
        , int debug
){
    RRDSET *st = r->st;

    time_t  now = rrdset_slot2time(st, start_at_slot),
            dt = st->update_every,
            group_start_t = 0;

    long
        slot = start_at_slot,
        group = r->group,
        counter = 0,
        stop_now = 0,
        added = 0,
        group_count = 0,
        add_this = 0;

    // do the calculations
    calculated_number   last_values = NAN; // keep the last value of each dimension

    calculated_number   group_value = (group_method == GROUP_MAX || group_method == GROUP_MIN)?NAN:0;
    long                values_in_group = 0; // keep the number of values added to group_value

    uint8_t             group_options = 0;
    uint8_t             found_non_zero = 0;


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

    void *(*grouping_init)(struct rrdresult *r);
    void (*grouping_free)(struct rrdresult *r, void *data);
    void (*grouping_add)(struct rrdresult *r, void *data, calculated_number value);
    void (*grouping_flush)(struct rrdresult *r, calculated_number *rrdr_value_ptr, uint8_t *rrdr_value_options_ptr, void *data);

    switch(group_method) {
        case GROUP_MIN:
            grouping_init  = grouping_init_min;
            grouping_free  = grouping_free_min;
            grouping_add   = grouping_add_min;
            grouping_flush = grouping_flush_min;
            break;

        case GROUP_MAX:
            grouping_init  = grouping_init_max;
            grouping_free  = grouping_free_max;
            grouping_add   = grouping_add_max;
            grouping_flush = grouping_flush_max;
            break;

        case GROUP_SUM:
            grouping_init  = grouping_init_sum;
            grouping_free  = grouping_free_sum;
            grouping_add   = grouping_add_sum;
            grouping_flush = grouping_flush_sum;
            break;

        case GROUP_INCREMENTAL_SUM:
            grouping_init  = grouping_init_incremental_sum;
            grouping_free  = grouping_free_incremental_sum;
            grouping_add   = grouping_add_incremental_sum;
            grouping_flush = grouping_flush_incremental_sum;
            break;

        default:
        case GROUP_AVERAGE:
        case GROUP_UNDEFINED:
            grouping_init  = grouping_init_average;
            grouping_free  = grouping_free_average;
            grouping_add   = grouping_add_average;
            grouping_flush = grouping_flush_average;
            break;
    }

    // void *data = grouping_init(r);

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
        if(likely(does_storage_number_exist(n))) {
            values_in_group++;

            calculated_number value = unpack_storage_number(n);
            if(likely(value != 0.0)) {
                group_options |= RRDR_NONZERO;
                found_non_zero = 1;
            }

            if(unlikely(did_storage_number_reset(n)))
                group_options |= RRDR_RESET;

            switch(group_method) {
                case GROUP_MIN:
                    if(unlikely(isnan(group_value)) ||
                       calculated_number_fabs(value) < calculated_number_fabs(group_value))
                       group_value = value;
                    break;

                case GROUP_MAX:
                    if(unlikely(isnan(group_value)) ||
                       calculated_number_fabs(value) > calculated_number_fabs(group_value))
                       group_value = value;
                    break;

                default:
                case GROUP_SUM:
                case GROUP_AVERAGE:
                case GROUP_UNDEFINED:
                    group_value += value;
                    break;

                case GROUP_INCREMENTAL_SUM:
                    if(unlikely(slot == start_at_slot))
                        last_values = value;

                    group_value += last_values - value;
                    last_values = value;
                    break;
            }
        }

        // add it
        if(unlikely(add_this)) {
            rrdr_line = rrdr_line_init(r, group_start_t, rrdr_line);

            after_to_return = now;

            // find the place to store our values
            calculated_number *rrdr_value_ptr;
            uint8_t *rrdr_value_options_ptr;
            {
                calculated_number *cn = rrdr_line_values(r, rrdr_line);
                rrdr_value_ptr = &cn[dim_id_in_rrdr];

                uint8_t *co = rrdr_line_options(r, rrdr_line);
                rrdr_value_options_ptr = &co[dim_id_in_rrdr];
            }

            // update the dimension options
            if(likely(found_non_zero)) r->od[dim_id_in_rrdr] |= RRDR_NONZERO;

            // store the specific point options
            *rrdr_value_options_ptr = group_options;

            // store the value
            if(unlikely(values_in_group == 0)) {
                *rrdr_value_ptr = 0.0;
                *rrdr_value_options_ptr |= RRDR_EMPTY;
                group_value = (group_method == GROUP_MAX || group_method == GROUP_MIN)?NAN:0;
            }
            else {
                switch(group_method) {
                    case GROUP_MIN:
                    case GROUP_MAX:
                        if(unlikely(isnan(group_value)))
                            *rrdr_value_ptr = 0;
                        else {
                            *rrdr_value_ptr = group_value;
                            group_value = NAN;
                        }
                        break;

                    case GROUP_SUM:
                    case GROUP_INCREMENTAL_SUM:
                        *rrdr_value_ptr = group_value;
                        group_value = 0;
                        break;

                    default:
                    case GROUP_AVERAGE:
                    case GROUP_UNDEFINED:
                        if(unlikely(r->group_points != 1))
                            *rrdr_value_ptr = group_value / r->group_sum_divisor;
                        else
                            *rrdr_value_ptr = group_value / values_in_group;

                        group_value = 0;
                        break;
                }

                // find the min and max for the whole chart
                if(*rrdr_value_ptr < r->min) r->min = *rrdr_value_ptr;
                if(*rrdr_value_ptr > r->max) r->max = *rrdr_value_ptr;

                // reset for the next loop
                values_in_group = 0;
                group_options = 0;
            }

            added++;
            group_count = 0;
            add_this = 0;
        }
    }

    r->after = after_to_return;
    rrdr_done(r, rrdr_line);
}

// ----------------------------------------------------------------------------
// fill RRDR for the whole chart

RRDR *rrd2rrdr(RRDSET *st, long points, long long after, long long before, int group_method, long group_time, int aligned) {
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
    long dimensions = r->d;

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

    rrdset_check_rdlock(st);

    time_t max_after = 0;
    long max_rows = 0;

    RRDDIM *rd;
    long c;
    for(rd = st->dimensions, c = 0 ; rd && c < dimensions ; rd = rd->next, c++) {

        do_dimension(
                r
                , points
                , rd
                , c
                , start_at_slot
                , stop_at_slot
                , after
                , before
                , group_method
                , debug
                );

        if(unlikely(!c)) {
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
    }

    //info("RRD2RRDR(): %s: END %ld loops made, %ld points generated", st->id, counter, rrdr_rows(r));
    //error("SHIFT: %s: wanted %ld points, got %ld", st->id, points, rrdr_rows(r));
    return r;
}
