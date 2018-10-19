//
// Created by costa on 19/10/18.
//

#include "query.h"

// ----------------------------------------------------------------------------


static inline uint8_t *rrdr_line_options(RRDR *r) {
    return &r->o[ r->c * r->d ];
}

static inline int rrdr_line_init(RRDR *r, time_t t) {
    r->c++;

    if(unlikely(r->c >= r->n)) {
        error("requested to step above RRDR size for chart %s", r->st->name);
        r->c = r->n - 1;
    }

    // save the time
    r->t[r->c] = t;

    return 1;
}

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

    // group_time enforces a certain grouping multiple
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
    // temp arrays for keeping values per dimension

    calculated_number   last_values[dimensions]; // keep the last value of each dimension
    calculated_number   group_values[dimensions]; // keep sums when grouping
    long                group_counts[dimensions]; // keep the number of values added to group_values
    uint8_t             group_options[dimensions];
    uint8_t             found_non_zero[dimensions];


    // initialize them
    RRDDIM *rd;
    long c;
    rrdset_check_rdlock(st);
    for( rd = st->dimensions, c = 0 ; rd && c < dimensions ; rd = rd->next, c++) {
        last_values[c] = 0;
        group_values[c] = (group_method == GROUP_MAX || group_method == GROUP_MIN)?NAN:0;
        group_counts[c] = 0;
        group_options[c] = 0;
        found_non_zero[c] = 0;
    }


    // -------------------------------------------------------------------------
    // the main loop

    time_t  now = rrdset_slot2time(st, start_at_slot),
            dt = st->update_every,
            group_start_t = 0;

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

    r->group = group;
    r->update_every = (int)group * st->update_every;
    r->before = now;
    r->after = now;

    //info("RRD2RRDR(): %s: STARTING", st->id);

    long slot = start_at_slot, counter = 0, stop_now = 0, added = 0, group_count = 0, add_this = 0;
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

        // do the calculations
        for(rd = st->dimensions, c = 0 ; rd && c < dimensions ; rd = rd->next, c++) {
            storage_number n = rd->values[slot];
            if(unlikely(!does_storage_number_exist(n))) continue;

            group_counts[c]++;

            calculated_number value = unpack_storage_number(n);
            if(likely(value != 0.0)) {
                group_options[c] |= RRDR_NONZERO;
                found_non_zero[c] = 1;
            }

            if(unlikely(did_storage_number_reset(n)))
                group_options[c] |= RRDR_RESET;

            switch(group_method) {
                case GROUP_MIN:
                    if(unlikely(isnan(group_values[c])) ||
                       calculated_number_fabs(value) < calculated_number_fabs(group_values[c]))
                        group_values[c] = value;
                    break;

                case GROUP_MAX:
                    if(unlikely(isnan(group_values[c])) ||
                       calculated_number_fabs(value) > calculated_number_fabs(group_values[c]))
                        group_values[c] = value;
                    break;

                default:
                case GROUP_SUM:
                case GROUP_AVERAGE:
                case GROUP_UNDEFINED:
                    group_values[c] += value;
                    break;

                case GROUP_INCREMENTAL_SUM:
                    if(unlikely(slot == start_at_slot))
                        last_values[c] = value;

                    group_values[c] += last_values[c] - value;
                    last_values[c] = value;
                    break;
            }
        }

        // added it
        if(unlikely(add_this)) {
            if(unlikely(!rrdr_line_init(r, group_start_t))) break;

            r->after = now;

            calculated_number *cn = rrdr_line_values(r);
            uint8_t *co = rrdr_line_options(r);

            for(rd = st->dimensions, c = 0 ; rd && c < dimensions ; rd = rd->next, c++) {

                // update the dimension options
                if(likely(found_non_zero[c])) r->od[c] |= RRDR_NONZERO;

                // store the specific point options
                co[c] = group_options[c];

                // store the value
                if(unlikely(group_counts[c] == 0)) {
                    cn[c] = 0.0;
                    co[c] |= RRDR_EMPTY;
                    group_values[c] = (group_method == GROUP_MAX || group_method == GROUP_MIN)?NAN:0;
                }
                else {
                    switch(group_method) {
                        case GROUP_MIN:
                        case GROUP_MAX:
                            if(unlikely(isnan(group_values[c])))
                                cn[c] = 0;
                            else {
                                cn[c] = group_values[c];
                                group_values[c] = NAN;
                            }
                            break;

                        case GROUP_SUM:
                        case GROUP_INCREMENTAL_SUM:
                            cn[c] = group_values[c];
                            group_values[c] = 0;
                            break;

                        default:
                        case GROUP_AVERAGE:
                        case GROUP_UNDEFINED:
                            if(unlikely(group_points != 1))
                                cn[c] = group_values[c] / group_sum_divisor;
                            else
                                cn[c] = group_values[c] / group_counts[c];

                            group_values[c] = 0;
                            break;
                    }

                    if(cn[c] < r->min) r->min = cn[c];
                    if(cn[c] > r->max) r->max = cn[c];
                }

                // reset for the next loop
                group_counts[c] = 0;
                group_options[c] = 0;
            }

            added++;
            group_count = 0;
            add_this = 0;
        }
    }

    rrdr_done(r);
    //info("RRD2RRDR(): %s: END %ld loops made, %ld points generated", st->id, counter, rrdr_rows(r));
    //error("SHIFT: %s: wanted %ld points, got %ld", st->id, points, rrdr_rows(r));
    return r;
}

