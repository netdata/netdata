// SPDX-License-Identifier: GPL-3.0-or-later

#include "query.h"
#include "web/api/formatters/rrd2json.h"
#include "rrdr.h"

#include "average/average.h"
#include "incremental_sum/incremental_sum.h"
#include "max/max.h"
#include "median/median.h"
#include "min/min.h"
#include "sum/sum.h"
#include "stddev/stddev.h"
#include "ses/ses.h"
#include "des/des.h"

// ----------------------------------------------------------------------------

static struct {
    const char *name;
    uint32_t hash;
    RRDR_GROUPING value;

    // One time initialization for the module.
    // This is called once, when netdata starts.
    void (*init)(void);

    // Allocate all required structures for a query.
    // This is called once for each netdata query.
    void *(*create)(struct rrdresult *r);

    // Cleanup collected values, but don't destroy the structures.
    // This is called when the query engine switches dimensions,
    // as part of the same query (so same chart, switching metric).
    void (*reset)(struct rrdresult *r);

    // Free all resources allocated for the query.
    void (*free)(struct rrdresult *r);

    // Add a single value into the calculation.
    // The module may decide to cache it, or use it in the fly.
    void (*add)(struct rrdresult *r, calculated_number value);

    // Generate a single result for the values added so far.
    // More values and points may be requested later.
    // It is up to the module to reset its internal structures
    // when flushing it (so for a few modules it may be better to
    // continue after a flush as if nothing changed, for others a
    // cleanup of the internal structures may be required).
    calculated_number (*flush)(struct rrdresult *r, RRDR_VALUE_FLAGS *rrdr_value_options_ptr);
} api_v1_data_groups[] = {
        {.name = "average",
                .hash  = 0,
                .value = RRDR_GROUPING_AVERAGE,
                .init  = NULL,
                .create= grouping_create_average,
                .reset = grouping_reset_average,
                .free  = grouping_free_average,
                .add   = grouping_add_average,
                .flush = grouping_flush_average
        },
        {.name = "mean",                           // alias on 'average'
                .hash  = 0,
                .value = RRDR_GROUPING_AVERAGE,
                .init  = NULL,
                .create= grouping_create_average,
                .reset = grouping_reset_average,
                .free  = grouping_free_average,
                .add   = grouping_add_average,
                .flush = grouping_flush_average
        },
        {.name  = "incremental_sum",
                .hash  = 0,
                .value = RRDR_GROUPING_INCREMENTAL_SUM,
                .init  = NULL,
                .create= grouping_create_incremental_sum,
                .reset = grouping_reset_incremental_sum,
                .free  = grouping_free_incremental_sum,
                .add   = grouping_add_incremental_sum,
                .flush = grouping_flush_incremental_sum
        },
        {.name = "incremental-sum",
                .hash  = 0,
                .value = RRDR_GROUPING_INCREMENTAL_SUM,
                .init  = NULL,
                .create= grouping_create_incremental_sum,
                .reset = grouping_reset_incremental_sum,
                .free  = grouping_free_incremental_sum,
                .add   = grouping_add_incremental_sum,
                .flush = grouping_flush_incremental_sum
        },
        {.name = "median",
                .hash  = 0,
                .value = RRDR_GROUPING_MEDIAN,
                .init  = NULL,
                .create= grouping_create_median,
                .reset = grouping_reset_median,
                .free  = grouping_free_median,
                .add   = grouping_add_median,
                .flush = grouping_flush_median
        },
        {.name = "min",
                .hash  = 0,
                .value = RRDR_GROUPING_MIN,
                .init  = NULL,
                .create= grouping_create_min,
                .reset = grouping_reset_min,
                .free  = grouping_free_min,
                .add   = grouping_add_min,
                .flush = grouping_flush_min
        },
        {.name = "max",
                .hash  = 0,
                .value = RRDR_GROUPING_MAX,
                .init  = NULL,
                .create= grouping_create_max,
                .reset = grouping_reset_max,
                .free  = grouping_free_max,
                .add   = grouping_add_max,
                .flush = grouping_flush_max
        },
        {.name = "sum",
                .hash  = 0,
                .value = RRDR_GROUPING_SUM,
                .init  = NULL,
                .create= grouping_create_sum,
                .reset = grouping_reset_sum,
                .free  = grouping_free_sum,
                .add   = grouping_add_sum,
                .flush = grouping_flush_sum
        },

        // standard deviation
        {.name = "stddev",
                .hash  = 0,
                .value = RRDR_GROUPING_STDDEV,
                .init  = NULL,
                .create= grouping_create_stddev,
                .reset = grouping_reset_stddev,
                .free  = grouping_free_stddev,
                .add   = grouping_add_stddev,
                .flush = grouping_flush_stddev
        },
        {.name = "cv",                           // coefficient variation is calculated by stddev
                .hash  = 0,
                .value = RRDR_GROUPING_CV,
                .init  = NULL,
                .create= grouping_create_stddev, // not an error, stddev calculates this too
                .reset = grouping_reset_stddev,  // not an error, stddev calculates this too
                .free  = grouping_free_stddev,   // not an error, stddev calculates this too
                .add   = grouping_add_stddev,    // not an error, stddev calculates this too
                .flush = grouping_flush_coefficient_of_variation
        },
        {.name = "rsd",                          // alias of 'cv'
                .hash  = 0,
                .value = RRDR_GROUPING_CV,
                .init  = NULL,
                .create= grouping_create_stddev, // not an error, stddev calculates this too
                .reset = grouping_reset_stddev,  // not an error, stddev calculates this too
                .free  = grouping_free_stddev,   // not an error, stddev calculates this too
                .add   = grouping_add_stddev,    // not an error, stddev calculates this too
                .flush = grouping_flush_coefficient_of_variation
        },

        /*
        {.name = "mean",                        // same as average, no need to define it again
                .hash  = 0,
                .value = RRDR_GROUPING_MEAN,
                .setup = NULL,
                .create= grouping_create_stddev,
                .reset = grouping_reset_stddev,
                .free  = grouping_free_stddev,
                .add   = grouping_add_stddev,
                .flush = grouping_flush_mean
        },
        */

        /*
        {.name = "variance",                    // meaningless to offer
                .hash  = 0,
                .value = RRDR_GROUPING_VARIANCE,
                .setup = NULL,
                .create= grouping_create_stddev,
                .reset = grouping_reset_stddev,
                .free  = grouping_free_stddev,
                .add   = grouping_add_stddev,
                .flush = grouping_flush_variance
        },
        */

        // single exponential smoothing
        {.name = "ses",
                .hash  = 0,
                .value = RRDR_GROUPING_SES,
                .init = grouping_init_ses,
                .create= grouping_create_ses,
                .reset = grouping_reset_ses,
                .free  = grouping_free_ses,
                .add   = grouping_add_ses,
                .flush = grouping_flush_ses
        },
        {.name = "ema",                         // alias for 'ses'
                .hash  = 0,
                .value = RRDR_GROUPING_SES,
                .init = NULL,
                .create= grouping_create_ses,
                .reset = grouping_reset_ses,
                .free  = grouping_free_ses,
                .add   = grouping_add_ses,
                .flush = grouping_flush_ses
        },
        {.name = "ewma",                        // alias for ses
                .hash  = 0,
                .value = RRDR_GROUPING_SES,
                .init = NULL,
                .create= grouping_create_ses,
                .reset = grouping_reset_ses,
                .free  = grouping_free_ses,
                .add   = grouping_add_ses,
                .flush = grouping_flush_ses
        },

        // double exponential smoothing
        {.name = "des",
                .hash  = 0,
                .value = RRDR_GROUPING_DES,
                .init = grouping_init_des,
                .create= grouping_create_des,
                .reset = grouping_reset_des,
                .free  = grouping_free_des,
                .add   = grouping_add_des,
                .flush = grouping_flush_des
        },

        // terminator
        {.name = NULL,
                .hash  = 0,
                .value = RRDR_GROUPING_UNDEFINED,
                .init = NULL,
                .create= grouping_create_average,
                .reset = grouping_reset_average,
                .free  = grouping_free_average,
                .add   = grouping_add_average,
                .flush = grouping_flush_average
        }
};

void web_client_api_v1_init_grouping(void) {
    int i;

    for(i = 0; api_v1_data_groups[i].name ; i++) {
        api_v1_data_groups[i].hash = simple_hash(api_v1_data_groups[i].name);

        if(api_v1_data_groups[i].init)
            api_v1_data_groups[i].init();
    }
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

static void rrdr_disable_not_selected_dimensions(RRDR *r, RRDR_OPTIONS options, const char *dims,
                                                 struct context_param *context_param_list)
{
    RRDDIM *temp_rd = context_param_list ? context_param_list->rd : NULL;
    int should_lock = (!context_param_list || !(context_param_list->flags & CONTEXT_FLAGS_ARCHIVE));

    if (should_lock)
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
    for(c = 0, d = temp_rd?temp_rd:r->st->dimensions; d ;c++, d = d->next) {
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
        for(c = 0, d = temp_rd?temp_rd:r->st->dimensions; d ;c++, d = d->next)
            if(unlikely(r->od[c] & RRDR_DIMENSION_SELECTED))
                r->od[c] |= RRDR_DIMENSION_NONZERO;
    }
}

// ----------------------------------------------------------------------------
// helpers to find our way in RRDR

static inline RRDR_VALUE_FLAGS *UNUSED_FUNCTION(rrdr_line_options)(RRDR *r, long rrdr_line) {
    return &r->o[ rrdr_line * r->d ];
}

static inline calculated_number *UNUSED_FUNCTION(rrdr_line_values)(RRDR *r, long rrdr_line) {
    return &r->v[ rrdr_line * r->d ];
}

static inline long rrdr_line_init(RRDR *r, time_t t, long rrdr_line) {
    rrdr_line++;

    #ifdef NETDATA_INTERNAL_CHECKS

    if(unlikely(rrdr_line >= r->n))
        error("INTERNAL ERROR: requested to step above RRDR size for chart '%s'", r->st->name);

    if(unlikely(r->t[rrdr_line] != 0 && r->t[rrdr_line] != t))
        error("INTERNAL ERROR: overwriting the timestamp of RRDR line %zu from %zu to %zu, of chart '%s'", (size_t)rrdr_line, (size_t)r->t[rrdr_line], (size_t)t, r->st->name);

    #endif

    // save the time
    r->t[rrdr_line] = t;

    return rrdr_line;
}

static inline void rrdr_done(RRDR *r, long rrdr_line) {
    r->rows = rrdr_line + 1;
}


// ----------------------------------------------------------------------------
// fill RRDR for a single dimension

static inline void do_dimension_variablestep(
          RRDR *r
        , long points_wanted
        , RRDDIM *rd
        , long dim_id_in_rrdr
        , time_t after_wanted
        , time_t before_wanted
        , uint32_t options
){
//  RRDSET *st = r->st;

    time_t
        now = after_wanted,
        dt = r->update_every,
        max_date = 0,
        min_date = 0;

    long
//      group_size = r->group,
        points_added = 0,
        values_in_group = 0,
        values_in_group_non_zero = 0,
        rrdr_line = -1;

    RRDR_VALUE_FLAGS
        group_value_flags = RRDR_VALUE_NOTHING;

    struct rrddim_query_handle handle;

    calculated_number min = r->min, max = r->max;
    size_t db_points_read = 0;
    time_t db_now = now;
    storage_number n_curr, n_prev = SN_EMPTY_SLOT;
    calculated_number value;

    for(rd->state->query_ops.init(rd, &handle, now, before_wanted) ; points_added < points_wanted ; now += dt) {
        // make sure we return data in the proper time range
        if (unlikely(now > before_wanted)) {
#ifdef NETDATA_INTERNAL_CHECKS
            r->internal.log = "stopped, because attempted to access the db after 'wanted before'";
#endif
            break;
        }
        if (unlikely(now < after_wanted)) {
#ifdef NETDATA_INTERNAL_CHECKS
            r->internal.log = "skipped, because attempted to access the db before 'wanted after'";
#endif
            continue;
        }

        while (now >= db_now && (!rd->state->query_ops.is_finished(&handle) ||
                                 does_storage_number_exist(n_prev))) {
            value = NAN;
            if (does_storage_number_exist(n_prev)) {
                // use the previously read database value
                n_curr = n_prev;
            } else {
                // read the value from the database
                n_curr = rd->state->query_ops.next_metric(&handle, &db_now);
            }
            n_prev = SN_EMPTY_SLOT;
            // db_now has a different value than above
            if (likely(now >= db_now)) {
                if (likely(does_storage_number_exist(n_curr))) {
                    if (options & RRDR_OPTION_ANOMALY_BIT)
                        value = (n_curr & SN_ANOMALY_BIT) ? 0.0 : 100.0;
                    else
                        value = unpack_storage_number(n_curr);

                    if (likely(value != 0.0))
                        values_in_group_non_zero++;

                    if (unlikely(did_storage_number_reset(n_curr)))
                        group_value_flags |= RRDR_VALUE_RESET;
                }
            } else {
                // We must postpone processing the value and fill the result with gaps instead
                if (likely(does_storage_number_exist(n_curr))) {
                    n_prev = n_curr;
                }
            }
            // add this value to grouping
            r->internal.grouping_add(r, value);
            values_in_group++;
            db_points_read++;
        }

        if (0 == values_in_group) {
            // add NAN to grouping
            r->internal.grouping_add(r, NAN);
        }

        rrdr_line = rrdr_line_init(r, now, rrdr_line);

        if(unlikely(!min_date)) min_date = now;
        max_date = now;

        // find the place to store our values
        RRDR_VALUE_FLAGS *rrdr_value_options_ptr = &r->o[rrdr_line * r->d + dim_id_in_rrdr];

        // update the dimension options
        if(likely(values_in_group_non_zero))
            r->od[dim_id_in_rrdr] |= RRDR_DIMENSION_NONZERO;

        // store the specific point options
        *rrdr_value_options_ptr = group_value_flags;

        // store the value
        value = r->internal.grouping_flush(r, rrdr_value_options_ptr);
        r->v[rrdr_line * r->d + dim_id_in_rrdr] = value;

        if(likely(points_added || dim_id_in_rrdr)) {
            // find the min/max across all dimensions

            if(unlikely(value < min)) min = value;
            if(unlikely(value > max)) max = value;

        }
        else {
            // runs only when dim_id_in_rrdr == 0 && points_added == 0
            // so, on the first point added for the query.
            min = max = value;
        }

        points_added++;
        values_in_group = 0;
        group_value_flags = RRDR_VALUE_NOTHING;
        values_in_group_non_zero = 0;
    }
    rd->state->query_ops.finalize(&handle);

    r->internal.db_points_read += db_points_read;
    r->internal.result_points_generated += points_added;

    r->min = min;
    r->max = max;
    r->before = max_date;
    r->after = min_date - (r->group - 1) * dt;
    rrdr_done(r, rrdr_line);

    #ifdef NETDATA_INTERNAL_CHECKS
    if(unlikely(r->rows != points_added))
        error("INTERNAL ERROR: %s.%s added %zu rows, but RRDR says I added %zu.", r->st->name, rd->name, (size_t)points_added, (size_t)r->rows);
    #endif
}

static inline void do_dimension_fixedstep(
        RRDR *r
        , long points_wanted
        , RRDDIM *rd
        , long dim_id_in_rrdr
        , time_t after_wanted
        , time_t before_wanted
        , uint32_t options
){
#ifdef NETDATA_INTERNAL_CHECKS
    RRDSET *st = r->st;
#endif

    time_t
            now = after_wanted,
            dt = r->update_every / r->group, /* usually is st->update_every */
            max_date = 0,
            min_date = 0;

    long
            group_size = r->group,
            points_added = 0,
            values_in_group = 0,
            values_in_group_non_zero = 0,
            rrdr_line = -1;

    RRDR_VALUE_FLAGS
            group_value_flags = RRDR_VALUE_NOTHING;

    struct rrddim_query_handle handle;

    calculated_number min = r->min, max = r->max;
    size_t db_points_read = 0;
    time_t db_now = now;
    time_t first_time_t = rrddim_first_entry_t(rd);
    for(rd->state->query_ops.init(rd, &handle, now, before_wanted) ; points_added < points_wanted ; now += dt) {
        // make sure we return data in the proper time range
        if(unlikely(now > before_wanted)) {
#ifdef NETDATA_INTERNAL_CHECKS
            r->internal.log = "stopped, because attempted to access the db after 'wanted before'";
#endif
            break;
        }
        if(unlikely(now < after_wanted)) {
#ifdef NETDATA_INTERNAL_CHECKS
            r->internal.log = "skipped, because attempted to access the db before 'wanted after'";
#endif
            continue;
        }
        // read the value from the database
        //storage_number n = rd->values[slot];
#ifdef NETDATA_INTERNAL_CHECKS
        struct mem_query_handle* mem_handle = (struct mem_query_handle*)handle.handle;
        if ((rd->rrd_memory_mode != RRD_MEMORY_MODE_DBENGINE) &&
            (rrdset_time2slot(st, now) != (long unsigned)(mem_handle->slot))) {
            error("INTERNAL CHECK: Unaligned query for %s, database slot: %lu, expected slot: %lu", rd->id, (long unsigned)mem_handle->slot, rrdset_time2slot(st, now));
        }
#endif
        db_now = now; // this is needed to set db_now in case the next_metric implementation does not set it
        storage_number n;
        if (rd->rrd_memory_mode != RRD_MEMORY_MODE_DBENGINE && now <= first_time_t)
            n = SN_EMPTY_SLOT;
        else
            n = rd->state->query_ops.next_metric(&handle, &db_now);
        if(unlikely(db_now > before_wanted)) {
#ifdef NETDATA_INTERNAL_CHECKS
            r->internal.log = "stopped, because attempted to access the db after 'wanted before'";
#endif
            break;
        }
        for ( ; now <= db_now ; now += dt) {
            calculated_number value = NAN;
            if(likely(now >= db_now && does_storage_number_exist(n))) {
#if defined(NETDATA_INTERNAL_CHECKS) && defined(ENABLE_DBENGINE)
                struct rrdeng_query_handle* rrd_handle = (struct rrdeng_query_handle*)handle.handle;
                if ((rd->rrd_memory_mode == RRD_MEMORY_MODE_DBENGINE) && (now != rrd_handle->now)) {
                    error("INTERNAL CHECK: Unaligned query for %s, database time: %ld, expected time: %ld", rd->id, (long)rrd_handle->now, (long)now);
                }
#endif
                if (options & RRDR_OPTION_ANOMALY_BIT)
                    value = (n & SN_ANOMALY_BIT) ? 0.0 : 100.0;
                else
                    value = unpack_storage_number(n);

                if(likely(value != 0.0))
                    values_in_group_non_zero++;

                if(unlikely(did_storage_number_reset(n)))
                    group_value_flags |= RRDR_VALUE_RESET;

            }

            // add this value for grouping
            r->internal.grouping_add(r, value);
            values_in_group++;
            db_points_read++;

            if(unlikely(values_in_group == group_size)) {
                rrdr_line = rrdr_line_init(r, now, rrdr_line);

                if(unlikely(!min_date)) min_date = now;
                max_date = now;

                // find the place to store our values
                RRDR_VALUE_FLAGS *rrdr_value_options_ptr = &r->o[rrdr_line * r->d + dim_id_in_rrdr];

                // update the dimension options
                if(likely(values_in_group_non_zero))
                    r->od[dim_id_in_rrdr] |= RRDR_DIMENSION_NONZERO;

                // store the specific point options
                *rrdr_value_options_ptr = group_value_flags;

                // store the value
                calculated_number value = r->internal.grouping_flush(r, rrdr_value_options_ptr);
                r->v[rrdr_line * r->d + dim_id_in_rrdr] = value;

                if(likely(points_added || dim_id_in_rrdr)) {
                    // find the min/max across all dimensions

                    if(unlikely(value < min)) min = value;
                    if(unlikely(value > max)) max = value;

                }
                else {
                    // runs only when dim_id_in_rrdr == 0 && points_added == 0
                    // so, on the first point added for the query.
                    min = max = value;
                }

                points_added++;
                values_in_group = 0;
                group_value_flags = RRDR_VALUE_NOTHING;
                values_in_group_non_zero = 0;
            }
        }
        now = db_now;
    }
    rd->state->query_ops.finalize(&handle);

    r->internal.db_points_read += db_points_read;
    r->internal.result_points_generated += points_added;

    r->min = min;
    r->max = max;
    r->before = max_date;
    r->after = min_date - (r->group - 1) * dt;
    rrdr_done(r, rrdr_line);

#ifdef NETDATA_INTERNAL_CHECKS
    if(unlikely(r->rows != points_added))
        error("INTERNAL ERROR: %s.%s added %zu rows, but RRDR says I added %zu.", r->st->name, rd->name, (size_t)points_added, (size_t)r->rows);
#endif
}

// ----------------------------------------------------------------------------
// fill RRDR for the whole chart

#ifdef NETDATA_INTERNAL_CHECKS
static void rrd2rrdr_log_request_response_metadata(RRDR *r
        , RRDR_GROUPING group_method
        , int aligned
        , long group
        , long resampling_time
        , long resampling_group
        , time_t after_wanted
        , time_t after_requested
        , time_t before_wanted
        , time_t before_requested
        , long points_requested
        , long points_wanted
        //, size_t after_slot
        //, size_t before_slot
        , const char *msg
        ) {
    netdata_rwlock_rdlock(&r->st->rrdset_rwlock);
    info("INTERNAL ERROR: rrd2rrdr() on %s update every %d with %s grouping %s (group: %ld, resampling_time: %ld, resampling_group: %ld), "
         "after (got: %zu, want: %zu, req: %zu, db: %zu), "
         "before (got: %zu, want: %zu, req: %zu, db: %zu), "
         "duration (got: %zu, want: %zu, req: %zu, db: %zu), "
         //"slot (after: %zu, before: %zu, delta: %zu), "
         "points (got: %ld, want: %ld, req: %ld, db: %ld), "
         "%s"
         , r->st->name
         , r->st->update_every

         // grouping
         , (aligned) ? "aligned" : "unaligned"
         , group_method2string(group_method)
         , group
         , resampling_time
         , resampling_group

         // after
         , (size_t)r->after
         , (size_t)after_wanted
         , (size_t)after_requested
         , (size_t)rrdset_first_entry_t_nolock(r->st)

         // before
         , (size_t)r->before
         , (size_t)before_wanted
         , (size_t)before_requested
         , (size_t)rrdset_last_entry_t_nolock(r->st)

         // duration
         , (size_t)(r->before - r->after + r->st->update_every)
         , (size_t)(before_wanted - after_wanted + r->st->update_every)
         , (size_t)(before_requested - after_requested)
         , (size_t)((rrdset_last_entry_t_nolock(r->st) - rrdset_first_entry_t_nolock(r->st)) + r->st->update_every)

         // slot
         /*
         , after_slot
         , before_slot
         , (after_slot > before_slot) ? (r->st->entries - after_slot + before_slot) : (before_slot - after_slot)
          */

         // points
         , r->rows
         , points_wanted
         , points_requested
         , r->st->entries

         // message
         , msg
    );
    netdata_rwlock_unlock(&r->st->rrdset_rwlock);
}
#endif // NETDATA_INTERNAL_CHECKS

// Returns 1 if an absolute period was requested or 0 if it was a relative period
static int rrdr_convert_before_after_to_absolute(
        long long *after_requestedp
        , long long *before_requestedp
        , int update_every
        , time_t first_entry_t
        , time_t last_entry_t
        , RRDR_OPTIONS options
) {
    int absolute_period_requested = -1;
    long long after_requested, before_requested;

    before_requested = *before_requestedp;
    after_requested = *after_requestedp;

    if(before_requested == 0 && after_requested == 0) {
        // dump the all the data
        before_requested = last_entry_t;
        after_requested = first_entry_t;
        absolute_period_requested = 0;
    }

    // allow relative for before (smaller than API_RELATIVE_TIME_MAX)
    if(ABS(before_requested) <= API_RELATIVE_TIME_MAX) {
        if(ABS(before_requested) % update_every) {
            // make sure it is multiple of st->update_every
            if(before_requested < 0) before_requested = before_requested - update_every -
                                                        before_requested % update_every;
            else before_requested = before_requested + update_every - before_requested % update_every;
        }
        if(before_requested > 0) before_requested = first_entry_t + before_requested;
        else                     before_requested = last_entry_t  + before_requested; //last_entry_t is not really now_t
        //TODO: fix before_requested to be relative to now_t
        absolute_period_requested = 0;
    }

    // allow relative for after (smaller than API_RELATIVE_TIME_MAX)
    if(ABS(after_requested) <= API_RELATIVE_TIME_MAX) {
        if(after_requested == 0) after_requested = -update_every;
        if(ABS(after_requested) % update_every) {
            // make sure it is multiple of st->update_every
            if(after_requested < 0) after_requested = after_requested - update_every - after_requested % update_every;
            else after_requested = after_requested + update_every - after_requested % update_every;
        }
        after_requested = before_requested + after_requested;
        absolute_period_requested = 0;
    }

    if(absolute_period_requested == -1)
        absolute_period_requested = 1;

    // make sure they are within our timeframe
    if(before_requested > last_entry_t)  before_requested = last_entry_t;
    if(before_requested < first_entry_t && !(options & RRDR_OPTION_ALLOW_PAST))
        before_requested = first_entry_t;

    if(after_requested > last_entry_t)  after_requested = last_entry_t;
    if(after_requested < first_entry_t && !(options & RRDR_OPTION_ALLOW_PAST))
        after_requested = first_entry_t;

    // check if they are reversed
    if(after_requested > before_requested) {
        time_t tmp = before_requested;
        before_requested = after_requested;
        after_requested = tmp;
    }

    *before_requestedp = before_requested;
    *after_requestedp = after_requested;

    return absolute_period_requested;
}

static RRDR *rrd2rrdr_fixedstep(
        RRDSET *st
        , long points_requested
        , long long after_requested
        , long long before_requested
        , RRDR_GROUPING group_method
        , long resampling_time_requested
        , RRDR_OPTIONS options
        , const char *dimensions
        , int update_every
        , time_t first_entry_t
        , time_t last_entry_t
        , int absolute_period_requested
        , struct context_param *context_param_list
) {
    int aligned = !(options & RRDR_OPTION_NOT_ALIGNED);

    // the duration of the chart
    time_t duration = before_requested - after_requested;
    long available_points = duration / update_every;

    RRDDIM *temp_rd = context_param_list ? context_param_list->rd : NULL;

    if(duration <= 0 || available_points <= 0)
        return rrdr_create(st, 1, context_param_list);

    // check the number of wanted points in the result
    if(unlikely(points_requested < 0)) points_requested = -points_requested;
    if(unlikely(points_requested > available_points)) points_requested = available_points;
    if(unlikely(points_requested == 0)) points_requested = available_points;

    // calculate the desired grouping of source data points
    long group = available_points / points_requested;
    if(unlikely(group <= 0)) group = 1;
    if(unlikely(available_points % points_requested > points_requested / 2)) group++; // rounding to the closest integer

    // resampling_time_requested enforces a certain grouping multiple
    calculated_number resampling_divisor = 1.0;
    long resampling_group = 1;
    if(unlikely(resampling_time_requested > update_every)) {
        if (unlikely(resampling_time_requested > duration)) {
            // group_time is above the available duration

            #ifdef NETDATA_INTERNAL_CHECKS
            info("INTERNAL CHECK: %s: requested gtime %ld secs, is greater than the desired duration %ld secs", st->id, resampling_time_requested, duration);
            #endif

            after_requested = before_requested - resampling_time_requested;
            duration = before_requested - after_requested;
            available_points = duration / update_every;
            group = available_points / points_requested;
        }

        // if the duration is not aligned to resampling time
        // extend the duration to the past, to avoid a gap at the chart
        // only when the missing duration is above 1/10th of a point
        if(duration % resampling_time_requested) {
            time_t delta = duration % resampling_time_requested;
            if(delta > resampling_time_requested / 10) {
                after_requested -= resampling_time_requested - delta;
                duration = before_requested - after_requested;
                available_points = duration / update_every;
                group = available_points / points_requested;
            }
        }

        // the points we should group to satisfy gtime
        resampling_group = resampling_time_requested / update_every;
        if(unlikely(resampling_time_requested % update_every)) {
            #ifdef NETDATA_INTERNAL_CHECKS
            info("INTERNAL CHECK: %s: requested gtime %ld secs, is not a multiple of the chart's data collection frequency %d secs", st->id, resampling_time_requested, update_every);
            #endif

            resampling_group++;
        }

        // adapt group according to resampling_group
        if(unlikely(group < resampling_group)) group  = resampling_group; // do not allow grouping below the desired one
        if(unlikely(group % resampling_group)) group += resampling_group - (group % resampling_group); // make sure group is multiple of resampling_group

        //resampling_divisor = group / resampling_group;
        resampling_divisor = (calculated_number)(group * update_every) / (calculated_number)resampling_time_requested;
    }

    // now that we have group,
    // align the requested timeframe to fit it.

    if(aligned) {
        // alignment has been requested, so align the values
        before_requested -= before_requested % (group * update_every);
        after_requested  -= after_requested % (group * update_every);
    }

    // we align the request on requested_before
    time_t before_wanted = before_requested;
    if(likely(before_wanted > last_entry_t)) {
        #ifdef NETDATA_INTERNAL_CHECKS
        error("INTERNAL ERROR: rrd2rrdr() on %s, before_wanted is after db max", st->name);
        #endif

        before_wanted = last_entry_t - (last_entry_t % ( ((aligned)?group:1) * update_every ));
    }
    //size_t before_slot = rrdset_time2slot(st, before_wanted);

    // we need to estimate the number of points, for having
    // an integer number of values per point
    long points_wanted = (before_wanted - after_requested) / (update_every * group);

    time_t after_wanted  = before_wanted - (points_wanted * group * update_every) + update_every;
    if(unlikely(after_wanted < first_entry_t)) {
        // hm... we go to the past, calculate again points_wanted using all the db from before_wanted to the beginning
        points_wanted = (before_wanted - first_entry_t) / group;

        // recalculate after wanted with the new number of points
        after_wanted  = before_wanted - (points_wanted * group * update_every) + update_every;

        if(unlikely(after_wanted < first_entry_t)) {
            #ifdef NETDATA_INTERNAL_CHECKS
            error("INTERNAL ERROR: rrd2rrdr() on %s, after_wanted is before db min", st->name);
            #endif

            after_wanted = first_entry_t - (first_entry_t % ( ((aligned)?group:1) * update_every )) + ( ((aligned)?group:1) * update_every );
        }
    }
    //size_t after_slot = rrdset_time2slot(st, after_wanted);

    // check if they are reversed
    if(unlikely(after_wanted > before_wanted)) {
        #ifdef NETDATA_INTERNAL_CHECKS
        error("INTERNAL ERROR: rrd2rrdr() on %s, reversed wanted after/before", st->name);
        #endif
        time_t tmp = before_wanted;
        before_wanted = after_wanted;
        after_wanted = tmp;
    }

    // recalculate points_wanted using the final time-frame
    points_wanted   = (before_wanted - after_wanted) / update_every / group + 1;
    if(unlikely(points_wanted < 0)) {
        #ifdef NETDATA_INTERNAL_CHECKS
        error("INTERNAL ERROR: rrd2rrdr() on %s, points_wanted is %ld", st->name, points_wanted);
        #endif
        points_wanted = 0;
    }

#ifdef NETDATA_INTERNAL_CHECKS
    duration = before_wanted - after_wanted;

    if(after_wanted < first_entry_t)
        error("INTERNAL CHECK: after_wanted %u is too small, minimum %u", (uint32_t)after_wanted, (uint32_t)first_entry_t);

    if(after_wanted > last_entry_t)
        error("INTERNAL CHECK: after_wanted %u is too big, maximum %u", (uint32_t)after_wanted, (uint32_t)last_entry_t);

    if(before_wanted < first_entry_t)
        error("INTERNAL CHECK: before_wanted %u is too small, minimum %u", (uint32_t)before_wanted, (uint32_t)first_entry_t);

    if(before_wanted > last_entry_t)
        error("INTERNAL CHECK: before_wanted %u is too big, maximum %u", (uint32_t)before_wanted, (uint32_t)last_entry_t);

/*
    if(before_slot >= (size_t)st->entries)
        error("INTERNAL CHECK: before_slot is invalid %zu, expected 0 to %ld", before_slot, st->entries - 1);

    if(after_slot >= (size_t)st->entries)
        error("INTERNAL CHECK: after_slot is invalid %zu, expected 0 to %ld", after_slot, st->entries - 1);
*/

    if(points_wanted > (before_wanted - after_wanted) / group / update_every + 1)
        error("INTERNAL CHECK: points_wanted %ld is more than points %ld", points_wanted, (before_wanted - after_wanted) / group / update_every + 1);

    if(group < resampling_group)
        error("INTERNAL CHECK: group %ld is less than the desired group points %ld", group, resampling_group);

    if(group > resampling_group && group % resampling_group)
        error("INTERNAL CHECK: group %ld is not a multiple of the desired group points %ld", group, resampling_group);
#endif

    // -------------------------------------------------------------------------
    // initialize our result set
    // this also locks the chart for us

    RRDR *r = rrdr_create(st, points_wanted, context_param_list);
    if(unlikely(!r)) {
        #ifdef NETDATA_INTERNAL_CHECKS
        error("INTERNAL CHECK: Cannot create RRDR for %s, after=%u, before=%u, duration=%u, points=%ld", st->id, (uint32_t)after_wanted, (uint32_t)before_wanted, (uint32_t)duration, points_wanted);
        #endif
        return NULL;
    }

    if(unlikely(!r->d || !points_wanted)) {
        #ifdef NETDATA_INTERNAL_CHECKS
        error("INTERNAL CHECK: Returning empty RRDR (no dimensions in RRDSET) for %s, after=%u, before=%u, duration=%zu, points=%ld", st->id, (uint32_t)after_wanted, (uint32_t)before_wanted, (size_t)duration, points_wanted);
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
    // initialize RRDR

    r->group = group;
    r->update_every = (int)group * update_every;
    r->before = before_wanted;
    r->after = after_wanted;
    r->internal.points_wanted = points_wanted;
    r->internal.resampling_group = resampling_group;
    r->internal.resampling_divisor = resampling_divisor;


    // -------------------------------------------------------------------------
    // assign the processor functions

    {
        int i, found = 0;
        for(i = 0; !found && api_v1_data_groups[i].name ;i++) {
            if(api_v1_data_groups[i].value == group_method) {
                r->internal.grouping_create= api_v1_data_groups[i].create;
                r->internal.grouping_reset = api_v1_data_groups[i].reset;
                r->internal.grouping_free  = api_v1_data_groups[i].free;
                r->internal.grouping_add   = api_v1_data_groups[i].add;
                r->internal.grouping_flush = api_v1_data_groups[i].flush;
                found = 1;
            }
        }
        if(!found) {
            errno = 0;
            #ifdef NETDATA_INTERNAL_CHECKS
            error("INTERNAL ERROR: grouping method %u not found for chart '%s'. Using 'average'", (unsigned int)group_method, r->st->name);
            #endif
            r->internal.grouping_create= grouping_create_average;
            r->internal.grouping_reset = grouping_reset_average;
            r->internal.grouping_free  = grouping_free_average;
            r->internal.grouping_add   = grouping_add_average;
            r->internal.grouping_flush = grouping_flush_average;
        }
    }

    // allocate any memory required by the grouping method
    r->internal.grouping_data = r->internal.grouping_create(r);


    // -------------------------------------------------------------------------
    // disable the not-wanted dimensions

    if (context_param_list && !(context_param_list->flags & CONTEXT_FLAGS_ARCHIVE))
        rrdset_check_rdlock(st);

    if(dimensions)
        rrdr_disable_not_selected_dimensions(r, options, dimensions, context_param_list);


    // -------------------------------------------------------------------------
    // do the work for each dimension

    time_t max_after = 0, min_before = 0;
    long max_rows = 0;

    RRDDIM *rd;
    long c, dimensions_used = 0, dimensions_nonzero = 0;
    for(rd = temp_rd?temp_rd:st->dimensions, c = 0 ; rd && c < dimensions_count ; rd = rd->next, c++) {

        // if we need a percentage, we need to calculate all dimensions
        if(unlikely(!(options & RRDR_OPTION_PERCENTAGE) && (r->od[c] & RRDR_DIMENSION_HIDDEN))) {
            if(unlikely(r->od[c] & RRDR_DIMENSION_SELECTED)) r->od[c] &= ~RRDR_DIMENSION_SELECTED;
            continue;
        }
        r->od[c] |= RRDR_DIMENSION_SELECTED;

        // reset the grouping for the new dimension
        r->internal.grouping_reset(r);

        do_dimension_fixedstep(
                r
                , points_wanted
                , rd
                , c
                , after_wanted
                , before_wanted
                , options
                );

        if(r->od[c] & RRDR_DIMENSION_NONZERO)
            dimensions_nonzero++;

        // verify all dimensions are aligned
        if(unlikely(!dimensions_used)) {
            min_before = r->before;
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

            if(r->before != min_before) {
                #ifdef NETDATA_INTERNAL_CHECKS
                error("INTERNAL ERROR: 'before' mismatch between dimensions for chart '%s': max is %zu, dimension '%s' has %zu",
                        st->name, (size_t)min_before, rd->name, (size_t)r->before);
                #endif
                r->before = (r->before < min_before) ? r->before : min_before;
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

    #ifdef NETDATA_INTERNAL_CHECKS
    if (dimensions_used) {
        if(r->internal.log)
            rrd2rrdr_log_request_response_metadata(r, group_method, aligned, group, resampling_time_requested, resampling_group, after_wanted, after_requested, before_wanted, before_requested, points_requested, points_wanted, /*after_slot, before_slot,*/ r->internal.log);

        if(r->rows != points_wanted)
            rrd2rrdr_log_request_response_metadata(r, group_method, aligned, group, resampling_time_requested, resampling_group, after_wanted, after_requested, before_wanted, before_requested, points_requested, points_wanted, /*after_slot, before_slot,*/ "got 'points' is not wanted 'points'");

        if(aligned && (r->before % group) != 0)
            rrd2rrdr_log_request_response_metadata(r, group_method, aligned, group, resampling_time_requested, resampling_group, after_wanted, after_requested, before_wanted, before_requested, points_requested, points_wanted, /*after_slot, before_slot,*/ "'before' is not aligned but alignment is required");

        // 'after' should not be aligned, since we start inside the first group
        //if(aligned && (r->after % group) != 0)
        //    rrd2rrdr_log_request_response_metadata(r, group_method, aligned, group, resampling_time_requested, resampling_group, after_wanted, after_requested, before_wanted, before_requested, points_requested, points_wanted, after_slot, before_slot, "'after' is not aligned but alignment is required");

        if(r->before != before_requested)
            rrd2rrdr_log_request_response_metadata(r, group_method, aligned, group, resampling_time_requested, resampling_group, after_wanted, after_requested, before_wanted, before_requested, points_requested, points_wanted, /*after_slot, before_slot,*/ "chart is not aligned to requested 'before'");

        if(r->before != before_wanted)
            rrd2rrdr_log_request_response_metadata(r, group_method, aligned, group, resampling_time_requested, resampling_group, after_wanted, after_requested, before_wanted, before_requested, points_requested, points_wanted, /*after_slot, before_slot,*/ "got 'before' is not wanted 'before'");

        // reported 'after' varies, depending on group
        if(r->after != after_wanted)
            rrd2rrdr_log_request_response_metadata(r, group_method, aligned, group, resampling_time_requested, resampling_group, after_wanted, after_requested, before_wanted, before_requested, points_requested, points_wanted, /*after_slot, before_slot,*/ "got 'after' is not wanted 'after'");
    }
    #endif

    // free all resources used by the grouping method
    r->internal.grouping_free(r);

    // when all the dimensions are zero, we should return all of them
    if(unlikely(options & RRDR_OPTION_NONZERO && !dimensions_nonzero)) {
        // all the dimensions are zero
        // mark them as NONZERO to send them all
        for(rd = temp_rd?temp_rd:st->dimensions, c = 0 ; rd && c < dimensions_count ; rd = rd->next, c++) {
            if(unlikely(r->od[c] & RRDR_DIMENSION_HIDDEN)) continue;
            r->od[c] |= RRDR_DIMENSION_NONZERO;
        }
    }

    rrdr_query_completed(r->internal.db_points_read, r->internal.result_points_generated);
    return r;
}

#ifdef ENABLE_DBENGINE
static RRDR *rrd2rrdr_variablestep(
        RRDSET *st
        , long points_requested
        , long long after_requested
        , long long before_requested
        , RRDR_GROUPING group_method
        , long resampling_time_requested
        , RRDR_OPTIONS options
        , const char *dimensions
        , int update_every
        , time_t first_entry_t
        , time_t last_entry_t
        , int absolute_period_requested
        , struct rrdeng_region_info *region_info_array
        , struct context_param *context_param_list
) {
    int aligned = !(options & RRDR_OPTION_NOT_ALIGNED);

    // the duration of the chart
    time_t duration = before_requested - after_requested;
    long available_points = duration / update_every;

    RRDDIM *temp_rd = context_param_list ? context_param_list->rd : NULL;

    if(duration <= 0 || available_points <= 0) {
        freez(region_info_array);
        return rrdr_create(st, 1, context_param_list);
    }

    // check the number of wanted points in the result
    if(unlikely(points_requested < 0)) points_requested = -points_requested;
    if(unlikely(points_requested > available_points)) points_requested = available_points;
    if(unlikely(points_requested == 0)) points_requested = available_points;

    // calculate the desired grouping of source data points
    long group = available_points / points_requested;
    if(unlikely(group <= 0)) group = 1;
    if(unlikely(available_points % points_requested > points_requested / 2)) group++; // rounding to the closest integer

    // resampling_time_requested enforces a certain grouping multiple
    calculated_number resampling_divisor = 1.0;
    long resampling_group = 1;
    if(unlikely(resampling_time_requested > update_every)) {
        if (unlikely(resampling_time_requested > duration)) {
            // group_time is above the available duration

            #ifdef NETDATA_INTERNAL_CHECKS
            info("INTERNAL CHECK: %s: requested gtime %ld secs, is greater than the desired duration %ld secs", st->id, resampling_time_requested, duration);
            #endif

            after_requested = before_requested - resampling_time_requested;
            duration = before_requested - after_requested;
            available_points = duration / update_every;
            group = available_points / points_requested;
        }

        // if the duration is not aligned to resampling time
        // extend the duration to the past, to avoid a gap at the chart
        // only when the missing duration is above 1/10th of a point
        if(duration % resampling_time_requested) {
            time_t delta = duration % resampling_time_requested;
            if(delta > resampling_time_requested / 10) {
                after_requested -= resampling_time_requested - delta;
                duration = before_requested - after_requested;
                available_points = duration / update_every;
                group = available_points / points_requested;
            }
        }

        // the points we should group to satisfy gtime
        resampling_group = resampling_time_requested / update_every;
        if(unlikely(resampling_time_requested % update_every)) {
            #ifdef NETDATA_INTERNAL_CHECKS
            info("INTERNAL CHECK: %s: requested gtime %ld secs, is not a multiple of the chart's data collection frequency %d secs", st->id, resampling_time_requested, update_every);
            #endif

            resampling_group++;
        }

        // adapt group according to resampling_group
        if(unlikely(group < resampling_group)) group  = resampling_group; // do not allow grouping below the desired one
        if(unlikely(group % resampling_group)) group += resampling_group - (group % resampling_group); // make sure group is multiple of resampling_group

        //resampling_divisor = group / resampling_group;
        resampling_divisor = (calculated_number)(group * update_every) / (calculated_number)resampling_time_requested;
    }

    // now that we have group,
    // align the requested timeframe to fit it.

    if(aligned) {
        // alignment has been requested, so align the values
        before_requested -= before_requested % (group * update_every);
        after_requested  -= after_requested % (group * update_every);
    }

    // we align the request on requested_before
    time_t before_wanted = before_requested;
    if(likely(before_wanted > last_entry_t)) {
        #ifdef NETDATA_INTERNAL_CHECKS
        error("INTERNAL ERROR: rrd2rrdr() on %s, before_wanted is after db max", st->name);
        #endif

        before_wanted = last_entry_t - (last_entry_t % ( ((aligned)?group:1) * update_every ));
    }
    //size_t before_slot = rrdset_time2slot(st, before_wanted);

    // we need to estimate the number of points, for having
    // an integer number of values per point
    long points_wanted = (before_wanted - after_requested) / (update_every * group);

    time_t after_wanted  = before_wanted - (points_wanted * group * update_every) + update_every;
    if(unlikely(after_wanted < first_entry_t)) {
        // hm... we go to the past, calculate again points_wanted using all the db from before_wanted to the beginning
        points_wanted = (before_wanted - first_entry_t) / group;

        // recalculate after wanted with the new number of points
        after_wanted  = before_wanted - (points_wanted * group * update_every) + update_every;

        if(unlikely(after_wanted < first_entry_t)) {
            #ifdef NETDATA_INTERNAL_CHECKS
            error("INTERNAL ERROR: rrd2rrdr() on %s, after_wanted is before db min", st->name);
            #endif

            after_wanted = first_entry_t - (first_entry_t % ( ((aligned)?group:1) * update_every )) + ( ((aligned)?group:1) * update_every );
        }
    }
    //size_t after_slot = rrdset_time2slot(st, after_wanted);

    // check if they are reversed
    if(unlikely(after_wanted > before_wanted)) {
        #ifdef NETDATA_INTERNAL_CHECKS
        error("INTERNAL ERROR: rrd2rrdr() on %s, reversed wanted after/before", st->name);
            #endif
        time_t tmp = before_wanted;
        before_wanted = after_wanted;
        after_wanted = tmp;
    }

    // recalculate points_wanted using the final time-frame
    points_wanted   = (before_wanted - after_wanted) / update_every / group + 1;
    if(unlikely(points_wanted < 0)) {
        #ifdef NETDATA_INTERNAL_CHECKS
        error("INTERNAL ERROR: rrd2rrdr() on %s, points_wanted is %ld", st->name, points_wanted);
        #endif
        points_wanted = 0;
    }

#ifdef NETDATA_INTERNAL_CHECKS
    duration = before_wanted - after_wanted;

    if(after_wanted < first_entry_t)
        error("INTERNAL CHECK: after_wanted %u is too small, minimum %u", (uint32_t)after_wanted, (uint32_t)first_entry_t);

    if(after_wanted > last_entry_t)
        error("INTERNAL CHECK: after_wanted %u is too big, maximum %u", (uint32_t)after_wanted, (uint32_t)last_entry_t);

    if(before_wanted < first_entry_t)
        error("INTERNAL CHECK: before_wanted %u is too small, minimum %u", (uint32_t)before_wanted, (uint32_t)first_entry_t);

    if(before_wanted > last_entry_t)
        error("INTERNAL CHECK: before_wanted %u is too big, maximum %u", (uint32_t)before_wanted, (uint32_t)last_entry_t);

/*
    if(before_slot >= (size_t)st->entries)
        error("INTERNAL CHECK: before_slot is invalid %zu, expected 0 to %ld", before_slot, st->entries - 1);

    if(after_slot >= (size_t)st->entries)
        error("INTERNAL CHECK: after_slot is invalid %zu, expected 0 to %ld", after_slot, st->entries - 1);
*/

    if(points_wanted > (before_wanted - after_wanted) / group / update_every + 1)
        error("INTERNAL CHECK: points_wanted %ld is more than points %ld", points_wanted, (before_wanted - after_wanted) / group / update_every + 1);

    if(group < resampling_group)
        error("INTERNAL CHECK: group %ld is less than the desired group points %ld", group, resampling_group);

    if(group > resampling_group && group % resampling_group)
        error("INTERNAL CHECK: group %ld is not a multiple of the desired group points %ld", group, resampling_group);
#endif

    // -------------------------------------------------------------------------
    // initialize our result set
    // this also locks the chart for us

    RRDR *r = rrdr_create(st, points_wanted, context_param_list);
    if(unlikely(!r)) {
        #ifdef NETDATA_INTERNAL_CHECKS
        error("INTERNAL CHECK: Cannot create RRDR for %s, after=%u, before=%u, duration=%u, points=%ld", st->id, (uint32_t)after_wanted, (uint32_t)before_wanted, (uint32_t)duration, points_wanted);
        #endif
        freez(region_info_array);
        return NULL;
    }

    if(unlikely(!r->d || !points_wanted)) {
        #ifdef NETDATA_INTERNAL_CHECKS
        error("INTERNAL CHECK: Returning empty RRDR (no dimensions in RRDSET) for %s, after=%u, before=%u, duration=%zu, points=%ld", st->id, (uint32_t)after_wanted, (uint32_t)before_wanted, (size_t)duration, points_wanted);
        #endif
        freez(region_info_array);
        return r;
    }

    r->result_options |= RRDR_RESULT_OPTION_VARIABLE_STEP;
    if(unlikely(absolute_period_requested == 1))
        r->result_options |= RRDR_RESULT_OPTION_ABSOLUTE;
    else
        r->result_options |= RRDR_RESULT_OPTION_RELATIVE;

    // find how many dimensions we have
    long dimensions_count = r->d;

    // -------------------------------------------------------------------------
    // initialize RRDR

    r->group = group;
    r->update_every = (int)group * update_every;
    r->before = before_wanted;
    r->after = after_wanted;
    r->internal.points_wanted = points_wanted;
    r->internal.resampling_group = resampling_group;
    r->internal.resampling_divisor = resampling_divisor;


    // -------------------------------------------------------------------------
    // assign the processor functions

    {
        int i, found = 0;
        for(i = 0; !found && api_v1_data_groups[i].name ;i++) {
            if(api_v1_data_groups[i].value == group_method) {
                r->internal.grouping_create= api_v1_data_groups[i].create;
                r->internal.grouping_reset = api_v1_data_groups[i].reset;
                r->internal.grouping_free  = api_v1_data_groups[i].free;
                r->internal.grouping_add   = api_v1_data_groups[i].add;
                r->internal.grouping_flush = api_v1_data_groups[i].flush;
                found = 1;
            }
        }
        if(!found) {
            errno = 0;
            #ifdef NETDATA_INTERNAL_CHECKS
            error("INTERNAL ERROR: grouping method %u not found for chart '%s'. Using 'average'", (unsigned int)group_method, r->st->name);
            #endif
            r->internal.grouping_create= grouping_create_average;
            r->internal.grouping_reset = grouping_reset_average;
            r->internal.grouping_free  = grouping_free_average;
            r->internal.grouping_add   = grouping_add_average;
            r->internal.grouping_flush = grouping_flush_average;
        }
    }

    // allocate any memory required by the grouping method
    r->internal.grouping_data = r->internal.grouping_create(r);


    // -------------------------------------------------------------------------
    // disable the not-wanted dimensions
    if (context_param_list && !(context_param_list->flags & CONTEXT_FLAGS_ARCHIVE))
        rrdset_check_rdlock(st);

    if(dimensions)
        rrdr_disable_not_selected_dimensions(r, options, dimensions, context_param_list);


    // -------------------------------------------------------------------------
    // do the work for each dimension

    time_t max_after = 0, min_before = 0;
    long max_rows = 0;

    RRDDIM *rd;
    long c, dimensions_used = 0, dimensions_nonzero = 0;
    for(rd = temp_rd?temp_rd:st->dimensions, c = 0 ; rd && c < dimensions_count ; rd = rd->next, c++) {

        // if we need a percentage, we need to calculate all dimensions
        if(unlikely(!(options & RRDR_OPTION_PERCENTAGE) && (r->od[c] & RRDR_DIMENSION_HIDDEN))) {
            if(unlikely(r->od[c] & RRDR_DIMENSION_SELECTED)) r->od[c] &= ~RRDR_DIMENSION_SELECTED;
            continue;
        }
        r->od[c] |= RRDR_DIMENSION_SELECTED;

        // reset the grouping for the new dimension
        r->internal.grouping_reset(r);

        do_dimension_variablestep(
                r
                , points_wanted
                , rd
                , c
                , after_wanted
                , before_wanted
                , options
        );

        if(r->od[c] & RRDR_DIMENSION_NONZERO)
            dimensions_nonzero++;

        // verify all dimensions are aligned
        if(unlikely(!dimensions_used)) {
            min_before = r->before;
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

            if(r->before != min_before) {
                #ifdef NETDATA_INTERNAL_CHECKS
                error("INTERNAL ERROR: 'before' mismatch between dimensions for chart '%s': max is %zu, dimension '%s' has %zu",
                      st->name, (size_t)min_before, rd->name, (size_t)r->before);
                #endif
                r->before = (r->before < min_before) ? r->before : min_before;
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

    #ifdef NETDATA_INTERNAL_CHECKS

    if (dimensions_used) {
        if(r->internal.log)
            rrd2rrdr_log_request_response_metadata(r, group_method, aligned, group, resampling_time_requested, resampling_group, after_wanted, after_requested, before_wanted, before_requested, points_requested, points_wanted, /*after_slot, before_slot,*/ r->internal.log);

        if(r->rows != points_wanted)
            rrd2rrdr_log_request_response_metadata(r, group_method, aligned, group, resampling_time_requested, resampling_group, after_wanted, after_requested, before_wanted, before_requested, points_requested, points_wanted, /*after_slot, before_slot,*/ "got 'points' is not wanted 'points'");

        if(aligned && (r->before % group) != 0)
            rrd2rrdr_log_request_response_metadata(r, group_method, aligned, group, resampling_time_requested, resampling_group, after_wanted, after_requested, before_wanted, before_requested, points_requested, points_wanted, /*after_slot, before_slot,*/ "'before' is not aligned but alignment is required");

        // 'after' should not be aligned, since we start inside the first group
        //if(aligned && (r->after % group) != 0)
        //    rrd2rrdr_log_request_response_metadata(r, group_method, aligned, group, resampling_time_requested, resampling_group, after_wanted, after_requested, before_wanted, before_requested, points_requested, points_wanted, after_slot, before_slot, "'after' is not aligned but alignment is required");

        if(r->before != before_requested)
            rrd2rrdr_log_request_response_metadata(r, group_method, aligned, group, resampling_time_requested, resampling_group, after_wanted, after_requested, before_wanted, before_requested, points_requested, points_wanted, /*after_slot, before_slot,*/ "chart is not aligned to requested 'before'");

        if(r->before != before_wanted)
            rrd2rrdr_log_request_response_metadata(r, group_method, aligned, group, resampling_time_requested, resampling_group, after_wanted, after_requested, before_wanted, before_requested, points_requested, points_wanted, /*after_slot, before_slot,*/ "got 'before' is not wanted 'before'");

        // reported 'after' varies, depending on group
        if(r->after != after_wanted)
            rrd2rrdr_log_request_response_metadata(r, group_method, aligned, group, resampling_time_requested, resampling_group, after_wanted, after_requested, before_wanted, before_requested, points_requested, points_wanted, /*after_slot, before_slot,*/ "got 'after' is not wanted 'after'");
    }
    #endif

    // free all resources used by the grouping method
    r->internal.grouping_free(r);

    // when all the dimensions are zero, we should return all of them
    if(unlikely(options & RRDR_OPTION_NONZERO && !dimensions_nonzero)) {
        // all the dimensions are zero
        // mark them as NONZERO to send them all
        for(rd = temp_rd?temp_rd:st->dimensions, c = 0 ; rd && c < dimensions_count ; rd = rd->next, c++) {
            if(unlikely(r->od[c] & RRDR_DIMENSION_HIDDEN)) continue;
            r->od[c] |= RRDR_DIMENSION_NONZERO;
        }
    }

    rrdr_query_completed(r->internal.db_points_read, r->internal.result_points_generated);
    freez(region_info_array);
    return r;
}
#endif //#ifdef ENABLE_DBENGINE

RRDR *rrd2rrdr(
        RRDSET *st
        , long points_requested
        , long long after_requested
        , long long before_requested
        , RRDR_GROUPING group_method
        , long resampling_time_requested
        , RRDR_OPTIONS options
        , const char *dimensions
        , struct context_param *context_param_list
)
{
    int rrd_update_every;
    int absolute_period_requested;

    time_t first_entry_t;
    time_t last_entry_t;
    if (context_param_list) {
        first_entry_t = context_param_list->first_entry_t;
        last_entry_t = context_param_list->last_entry_t;
    } else {
        rrdset_rdlock(st);
        first_entry_t = rrdset_first_entry_t_nolock(st);
        last_entry_t = rrdset_last_entry_t_nolock(st);
        rrdset_unlock(st);
    }

    rrd_update_every = st->update_every;
    absolute_period_requested = rrdr_convert_before_after_to_absolute(&after_requested, &before_requested,
                                                                      rrd_update_every, first_entry_t,
                                                                      last_entry_t, options);
    if (options & RRDR_OPTION_ALLOW_PAST)
        if (first_entry_t > after_requested)
            first_entry_t = after_requested;

    if (context_param_list && !(context_param_list->flags & CONTEXT_FLAGS_ARCHIVE)) {
        rebuild_context_param_list(context_param_list, after_requested);
        st = context_param_list->rd ? context_param_list->rd->rrdset : NULL;
        if (unlikely(!st))
            return NULL;
    }

#ifdef ENABLE_DBENGINE
    if (st->rrd_memory_mode == RRD_MEMORY_MODE_DBENGINE) {
        struct rrdeng_region_info *region_info_array;
        unsigned regions, max_interval;

        /* This call takes the chart read-lock */
        regions = rrdeng_variable_step_boundaries(st, after_requested, before_requested,
                                                  &region_info_array, &max_interval, context_param_list);
        if (1 == regions) {
            if (region_info_array) {
                if (rrd_update_every != region_info_array[0].update_every) {
                    rrd_update_every = region_info_array[0].update_every;
                    /* recalculate query alignment */
                    absolute_period_requested =
                            rrdr_convert_before_after_to_absolute(&after_requested, &before_requested, rrd_update_every,
                                                                  first_entry_t, last_entry_t, options);
                }
                freez(region_info_array);
            }
            return rrd2rrdr_fixedstep(st, points_requested, after_requested, before_requested, group_method,
                                      resampling_time_requested, options, dimensions, rrd_update_every,
                                      first_entry_t, last_entry_t, absolute_period_requested, context_param_list);
        } else {
            if (rrd_update_every != (uint16_t)max_interval) {
                rrd_update_every = (uint16_t) max_interval;
                /* recalculate query alignment */
                absolute_period_requested = rrdr_convert_before_after_to_absolute(&after_requested, &before_requested,
                                                                                  rrd_update_every, first_entry_t,
                                                                                  last_entry_t, options);
            }
            return rrd2rrdr_variablestep(st, points_requested, after_requested, before_requested, group_method,
                                         resampling_time_requested, options, dimensions, rrd_update_every,
                                         first_entry_t, last_entry_t, absolute_period_requested, region_info_array, context_param_list);
        }
    }
#endif
    return rrd2rrdr_fixedstep(st, points_requested, after_requested, before_requested, group_method,
                              resampling_time_requested, options, dimensions,
                              rrd_update_every, first_entry_t, last_entry_t, absolute_period_requested, context_param_list);
}
