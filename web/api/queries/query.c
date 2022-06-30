// SPDX-License-Identifier: GPL-3.0-or-later

#include "query.h"
#include "web/api/formatters/rrd2json.h"
#include "rrdr.h"
#include "database/ram/rrddim_mem.h"

#include "average/average.h"
#include "countif/countif.h"
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
    void (*create)(struct rrdresult *r, const char *options);

    // Cleanup collected values, but don't destroy the structures.
    // This is called when the query engine switches dimensions,
    // as part of the same query (so same chart, switching metric).
    void (*reset)(struct rrdresult *r);

    // Free all resources allocated for the query.
    void (*free)(struct rrdresult *r);

    // Add a single value into the calculation.
    // The module may decide to cache it, or use it in the fly.
    void (*add)(struct rrdresult *r, NETDATA_DOUBLE value);

    // Generate a single result for the values added so far.
    // More values and points may be requested later.
    // It is up to the module to reset its internal structures
    // when flushing it (so for a few modules it may be better to
    // continue after a flush as if nothing changed, for others a
    // cleanup of the internal structures may be required).
    NETDATA_DOUBLE (*flush)(struct rrdresult *r, RRDR_VALUE_FLAGS *rrdr_value_options_ptr);
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

        {.name = "countif",
                .hash  = 0,
                .value = RRDR_GROUPING_COUNTIF,
                .init = NULL,
                .create= grouping_create_countif,
                .reset = grouping_reset_countif,
                .free  = grouping_free_countif,
                .add   = grouping_add_countif,
                .flush = grouping_flush_countif
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

const char *web_client_api_request_v1_data_group_to_string(RRDR_GROUPING group) {
    int i;

    for(i = 0; api_v1_data_groups[i].name ; i++)
        if(unlikely(group == api_v1_data_groups[i].value))
            return api_v1_data_groups[i].name;

    return "unknown";
}

static void rrdr_set_grouping_function(RRDR *r, RRDR_GROUPING group_method) {
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
        internal_error(true, "QUERY: grouping method %u not found. Using 'average'", (unsigned int)group_method);
        r->internal.grouping_create = grouping_create_average;
        r->internal.grouping_reset  = grouping_reset_average;
        r->internal.grouping_free   = grouping_free_average;
        r->internal.grouping_add    = grouping_add_average;
        r->internal.grouping_flush  = grouping_flush_average;
    }
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

static inline NETDATA_DOUBLE *UNUSED_FUNCTION(rrdr_line_values)(RRDR *r, long rrdr_line) {
    return &r->v[ rrdr_line * r->d ];
}

static inline long rrdr_line_init(RRDR *r, time_t t, long rrdr_line) {
    rrdr_line++;

    internal_error(rrdr_line >= r->n,
                   "QUERY: requested to step above RRDR size for chart '%s'",
                   r->st->name);

    internal_error(r->t[rrdr_line] != 0 && r->t[rrdr_line] != t,
                   "QUERY: overwriting the timestamp of RRDR line %zu from %zu to %zu, of chart '%s'",
                   (size_t)rrdr_line, (size_t)r->t[rrdr_line], (size_t)t, r->st->name);

    // save the time
    r->t[rrdr_line] = t;

    return rrdr_line;
}

static inline void rrdr_done(RRDR *r, long rrdr_line) {
    r->rows = rrdr_line + 1;
}


// ----------------------------------------------------------------------------
// fill RRDR for a single dimension

static inline NETDATA_DOUBLE interpolate_value(NETDATA_DOUBLE this_value, NETDATA_DOUBLE last_value, time_t last_value_end_t, time_t this_value_start_t, time_t now, time_t this_value_end_t) {
    if(unlikely(
            this_value_start_t + 1 == this_value_end_t ||
            !netdata_double_isnumber(this_value) ||
            !netdata_double_isnumber(last_value) ||
            last_value_end_t != this_value_start_t))
        return this_value;

    return last_value + (this_value - last_value) * ( 1.0 - (NETDATA_DOUBLE)(this_value_end_t - now) / (NETDATA_DOUBLE)(this_value_end_t - this_value_start_t) );
}

static inline void rrd2rrdr_do_dimension(
        RRDR *r
        , long points_wanted
        , RRDDIM *rd
        , long dim_id_in_rrdr
        , time_t after_wanted
        , time_t before_wanted
        , RRDR_OPTIONS options
        , TIER_QUERY_FETCH tier_query_fetch_type
){
    time_t now = after_wanted,
           query_granularity = r->update_every / r->group,
           max_date = 0,
           min_date = 0;

    bool interpolate = query_granularity < rd->update_every;

    long group_points_wanted = r->group,
            points_added = 0, group_points_added = 0, group_points_non_zero = 0,
            rrdr_line = -1;

    size_t group_anomaly_rate = 0;

    RRDR_VALUE_FLAGS group_value_flags = RRDR_VALUE_NOTHING;

    struct rrddim_query_handle handle;

    NETDATA_DOUBLE min = r->min, max = r->max;
    size_t db_points_read = 0;

    struct rrddim_tier *tier = ((options & RRDR_OPTION_TIER1) && rd->tiers[1]) ? rd->tiers[1] : rd->tiers[0];

    // cache the function pointers we need in the loop
    NETDATA_DOUBLE (*next_metric)(struct rrddim_query_handle *handle, time_t *current_time, time_t *end_time, SN_FLAGS *flags, uint16_t *count, uint16_t *anomaly_count) = tier->query_ops.next_metric;
    void (*grouping_add)(struct rrdresult *r, NETDATA_DOUBLE value) = r->internal.grouping_add;
    NETDATA_DOUBLE (*grouping_flush)(struct rrdresult *r, RRDR_VALUE_FLAGS *rrdr_value_options_ptr) = r->internal.grouping_flush;

    NETDATA_DOUBLE last2_point_value;
    //SN_FLAGS last2_point_flags;
    //size_t last2_point_anomaly;
    //time_t last2_point_start_time;
    time_t last2_point_end_time;

    NETDATA_DOUBLE last1_point_value = NAN;
    SN_FLAGS last1_point_flags = SN_EMPTY_SLOT;
    size_t last1_point_anomaly = 0;
    time_t last1_point_start_time = 0;
    time_t last1_point_end_time = 0;

    NETDATA_DOUBLE new_point_value = NAN;
//    storage_number_tier1_t tier_new_point_value;
    uint16_t anomaly_count;
    uint16_t count;
    SN_FLAGS new_point_flags = SN_EMPTY_SLOT;
    size_t new_point_anomaly = 0;
    time_t new_point_start_time = 0;
    time_t new_point_end_time = 0;

    // select the right tier to query
    for(tier->query_ops.init(tier->db_metric_handle, &handle, now, before_wanted, tier_query_fetch_type) ; points_added < points_wanted ; now += query_granularity) {
//    struct rrddim_volatile *state = (rd->state_tier1 && (options & RRDR_OPTION_TIER1)) ? rd->state_tier1 : rd->state;
//    for(tier->query_ops.init(tier->db_metric_handle, &handle, now, before_wanted, tier_query_fetch_type) ; points_added < points_wanted ; now += query_granularity) {

        if(unlikely(now > before_wanted))
            break;

        last2_point_value      = last1_point_value;
        //last2_point_flags      = last1_point_flags;
        //last2_point_anomaly    = last1_point_anomaly;
        //last2_point_start_time = last1_point_start_time;
        last2_point_end_time   = last1_point_end_time;

        last1_point_value      = new_point_value;
        last1_point_flags      = new_point_flags;
        last1_point_anomaly    = new_point_anomaly;
        last1_point_start_time = new_point_start_time;
        last1_point_end_time   = new_point_end_time;

        if(likely(!tier->query_ops.is_finished(&handle))) {
            // fetch the new point
            new_point_value = next_metric(&handle, &new_point_start_time, &new_point_end_time, &new_point_flags, &anomaly_count, &count);
            db_points_read++;

            // dbengine does not take into account the starting time of points
            // and depending on the data collection frequency it may return
            // a point that is just before the wanted one.
            // So, here we fetch the next one.
            if(unlikely(new_point_end_time < now)) {
                internal_error(true, "QUERY: next_metric(%s, %s) returned point %zu from %ld to %ld, before now (now = %ld, after_wanted = %ld, before_wanted = %ld, dt = %ld). Fetching the next one.",
                               rd->rrdset->name, rd->name, db_points_read, new_point_start_time, new_point_end_time, now, after_wanted, before_wanted, query_granularity);

                new_point_value = next_metric(&handle, &new_point_start_time, &new_point_end_time, &new_point_flags, &anomaly_count, &count);
                db_points_read++;
            }

            if(likely(netdata_double_isnumber(new_point_value))) {
                new_point_anomaly = (new_point_flags & SN_ANOMALY_BIT) ? 0 : 100;

                if(unlikely(options & RRDR_OPTION_ANOMALY_BIT))
                    new_point_value = (NETDATA_DOUBLE)new_point_anomaly;
            }
            else {
                new_point_flags   = SN_EMPTY_SLOT;
                new_point_value   = NAN;
                new_point_anomaly = 0;
            }

            if(unlikely(new_point_start_time == new_point_end_time)) {
                internal_error(true, "QUERY: next_metric(%s, %s) returned point %zu start time %ld, end time %ld, that are both equal",
                               rd->rrdset->name, rd->name, db_points_read, new_point_start_time, new_point_end_time);

                new_point_start_time = new_point_end_time - rd->update_every;
            }

            if(unlikely(new_point_start_time < last1_point_start_time && new_point_end_time < last1_point_end_time)) {
                internal_error(true, "QUERY: next_metric(%s, %s) returned point %zu start time %ld, end time %ld, before the last point start time %ld, end time %ld",
                               rd->rrdset->name, rd->name, db_points_read, new_point_start_time, new_point_end_time,
                    last1_point_start_time,
                    last1_point_end_time);

                new_point_value      = last1_point_value;
                new_point_flags      = last1_point_flags;
                new_point_start_time = last1_point_start_time;
                new_point_end_time   = last1_point_end_time;
            }

            if(unlikely(new_point_end_time < last1_point_end_time)) {
                internal_error(true, "QUERY: next_metric(%s, %s) returned point %zu end time %ld, before the last point end time %ld",
                               rd->rrdset->name, rd->name, db_points_read, new_point_end_time,
                    last1_point_end_time);

                new_point_value      = last1_point_value;
                new_point_flags      = last1_point_flags;
                new_point_start_time = last1_point_start_time;
                new_point_end_time   = last1_point_end_time;
            }

            if(unlikely(new_point_end_time < now)) {
                internal_error(true, "QUERY: next_metric(%s, %s) returned point %zu from %ld to %ld, before now (now = %ld, after_wanted = %ld, before_wanted = %ld, dt = %ld)",
                               rd->rrdset->name, rd->name, db_points_read, new_point_start_time, new_point_end_time,
                               now, after_wanted, before_wanted, query_granularity);

                new_point_end_time   = now;
            }
        }
        else {
            new_point_value      = NAN;
            new_point_flags      = SN_EMPTY_SLOT;
            new_point_start_time = last1_point_end_time;
            new_point_end_time   = now;
        }

        // the inner loop
        // we have 3 points in memory: last, new, next
        // we select the one to use based on their timestamps

        size_t iterations = 0;
        for ( ; now <= new_point_end_time && points_added < points_wanted; now += query_granularity, iterations++) {

            NETDATA_DOUBLE current_point_value;
            SN_FLAGS current_point_flags;
            size_t current_point_anomaly;
            //time_t current_point_start_time;
            //time_t current_point_end_time;

            if(likely(now > new_point_start_time)) {
                // it is time for our NEW point to be used
                current_point_value      = interpolate ? interpolate_value(new_point_value, last1_point_value, last1_point_end_time, new_point_start_time, now, new_point_end_time) : new_point_value;
                current_point_flags      = new_point_flags;
                current_point_anomaly    = new_point_anomaly;
                //current_point_start_time = new_point_start_time;
                //current_point_end_time   = new_point_end_time;
            }
            else if(likely(now <= last1_point_end_time)) {
                // our LAST point is still valid
                current_point_value      = interpolate ? interpolate_value(last1_point_value, last2_point_value, last2_point_end_time, last1_point_start_time, now, last1_point_end_time) : last1_point_value;
                current_point_flags      = last1_point_flags;
                current_point_anomaly    = last1_point_anomaly;
                //current_point_start_time = last_point_start_time;
                //current_point_end_time   = last_point_end_time;
            }
            else {
                // a GAP, we don't have a value this time
                current_point_value      = NAN;
                current_point_flags      = SN_EMPTY_SLOT;
                current_point_anomaly    = 0;
                //current_point_start_time = now - dt;
                //current_point_end_time   = now;
            }

            if(likely(netdata_double_isnumber(current_point_value))) {
                if(likely(current_point_value != 0.0))
                    group_points_non_zero++;

                if(unlikely(current_point_flags & SN_EXISTS_RESET))
                    group_value_flags |= RRDR_VALUE_RESET;

                grouping_add(r, current_point_value);
            }

            // add this value for grouping
            group_points_added++;
            group_anomaly_rate += current_point_anomaly;

            if(unlikely(group_points_added == group_points_wanted)) {
                rrdr_line = rrdr_line_init(r, now, rrdr_line);
                size_t rrdr_o_v_index = rrdr_line * r->d + dim_id_in_rrdr;

                if(unlikely(!min_date)) min_date = now;
                max_date = now;

                // find the place to store our values
                RRDR_VALUE_FLAGS *rrdr_value_options_ptr = &r->o[rrdr_o_v_index];

                // update the dimension options
                if(likely(group_points_non_zero))
                    r->od[dim_id_in_rrdr] |= RRDR_DIMENSION_NONZERO;

                // store the specific point options
                *rrdr_value_options_ptr = group_value_flags;

                // store the group value
                NETDATA_DOUBLE group_value = grouping_flush(r, rrdr_value_options_ptr);
                r->v[rrdr_o_v_index] = group_value;

                // we only store uint8_t anomaly rates,
                // so let's get double precision by storing
                // anomaly rates in the range 0 - 200
                group_anomaly_rate = (group_anomaly_rate << 1) / group_points_added;
                r->ar[rrdr_o_v_index] = (uint8_t)group_anomaly_rate;

                if(likely(points_added || dim_id_in_rrdr)) {
                    // find the min/max across all dimensions

                    if(unlikely(group_value < min)) min = group_value;
                    if(unlikely(group_value > max)) max = group_value;

                }
                else {
                    // runs only when dim_id_in_rrdr == 0 && points_added == 0
                    // so, on the first point added for the query.
                    min = max = group_value;
                }

                points_added++;
                group_points_added = 0;
                group_value_flags = RRDR_VALUE_NOTHING;
                group_points_non_zero = 0;
                group_anomaly_rate = 0;
            }
        }
        // the loop above increased "now" by dt,
        // but the main loop will increase it,
        // so, let's undo the last iteration of this loop
        if(iterations)
            now -= query_granularity;
    }
    tier->query_ops.finalize(&handle);

    r->internal.db_points_read += db_points_read;
    r->internal.result_points_generated += points_added;

    r->min = min;
    r->max = max;
    r->before = max_date;
    r->after = min_date - (r->group - 1) * query_granularity;
    rrdr_done(r, rrdr_line);

    internal_error(points_wanted != points_added,
                   "QUERY: query on %s/%s requested %zu points, but RRDR added %zu (%zu db points read).",
                   r->st->name, rd->name, (size_t)points_wanted, (size_t)points_added, db_points_read);
}

// ----------------------------------------------------------------------------
// fill RRDR for the whole chart

#ifdef NETDATA_INTERNAL_CHECKS
static void rrd2rrdr_log_request_response_metadata(RRDR *r
        , RRDR_OPTIONS options
        , RRDR_GROUPING group_method
        , bool aligned
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
         , (size_t)rrdset_first_entry_t_nolock(r->st, options & RRDR_OPTION_TIER1? 1 : 0)

         // before
         , (size_t)r->before
         , (size_t)before_wanted
         , (size_t)before_requested
         , (size_t)rrdset_last_entry_t_nolock(r->st, options & RRDR_OPTION_TIER1? 1 : 0)

         // duration
         , (size_t)(r->before - r->after + r->st->update_every)
         , (size_t)(before_wanted - after_wanted + r->st->update_every)
         , (size_t)(before_requested - after_requested)
         , (size_t)((rrdset_last_entry_t_nolock(r->st, 0) - rrdset_first_entry_t_nolock(r->st,0)) + r->st->update_every)

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
int rrdr_relative_window_to_absolute(long long *after, long long *before, int update_every, long points) {
    time_t now = now_realtime_sec() - 1;

    int absolute_period_requested = -1;
    long long after_requested, before_requested;

    before_requested = *before;
    after_requested = *after;

    // allow relative for before (smaller than API_RELATIVE_TIME_MAX)
    if(ABS(before_requested) <= API_RELATIVE_TIME_MAX) {
        // if the user asked for a positive relative time,
        // flip it to a negative
        if(before_requested > 0)
            before_requested = -before_requested;

        before_requested = now + before_requested;
        absolute_period_requested = 0;
    }

    // allow relative for after (smaller than API_RELATIVE_TIME_MAX)
    if(ABS(after_requested) <= API_RELATIVE_TIME_MAX) {
        if(after_requested > 0)
            after_requested = -after_requested;

        // if the user didn't give an after, use the number of points
        // to give a sane default
        if(after_requested == 0)
            after_requested = -(points * update_every);

        after_requested = before_requested + after_requested;
        absolute_period_requested = 0;
    }

    if(absolute_period_requested == -1)
        absolute_period_requested = 1;

    // check if the parameters are flipped
    if(after_requested > before_requested) {
        long long t = before_requested;
        before_requested = after_requested;
        after_requested = t;
    }

    // if the query requests future data
    // shift the query back to be in the present time
    // (this may also happen because of the rules above)
    if(before_requested > now) {
        long long delta = before_requested - now;
        before_requested -= delta;
        after_requested  -= delta;
    }

    *before = before_requested;
    *after = after_requested;

    return absolute_period_requested;
}

// #define DEBUG_QUERY_LOGIC 1

#ifdef DEBUG_QUERY_LOGIC
#define query_debug_log_init() BUFFER *debug_log = buffer_create(1000)
#define query_debug_log(args...) buffer_sprintf(debug_log, ##args)
#define query_debug_log_fin() { \
        info("QUERY: chart '%s', after:%lld, before:%lld, points:%ld, res:%ld - wanted => after:%lld, before:%lld, points:%ld, group:%ld, granularity:%ld, resgroup:%ld, resdiv:" NETDATA_DOUBLE_FORMAT_AUTO " %s", st->name, after_requested, before_requested, points_requested, resampling_time_requested, after_wanted, before_wanted, points_wanted, group, query_granularity, resampling_group, resampling_divisor, buffer_tostring(debug_log)); \
        buffer_free(debug_log); \
        debug_log = NULL; \
    }
#else
#define query_debug_log_init() debug_dummy()
#define query_debug_log(args...) debug_dummy()
#define query_debug_log_fin() debug_dummy()
#endif

RRDR *rrd2rrdr(
          ONEWAYALLOC *owa
        , RRDSET *st
        , long points_requested
        , long long after_requested
        , long long before_requested
        , RRDR_GROUPING group_method
        , long resampling_time_requested
        , RRDR_OPTIONS options
        , const char *dimensions
        , struct context_param *context_param_list
        , const char *group_options
        , int timeout
) {
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

    long points_wanted = points_requested;
    long long after_wanted = after_requested;
    long long before_wanted = before_requested;
    int update_every = st->update_every;

    bool aligned = !(options & RRDR_OPTION_NOT_ALIGNED);
    bool automatic_natural_points = (points_wanted == 0);
    bool relative_period_requested = false;
    bool natural_points = (options & RRDR_OPTION_NATURAL_POINTS) || automatic_natural_points;

    query_debug_log_init();

    // make sure points_wanted is positive
    if(points_wanted < 0) {
        points_wanted = -points_wanted;
        query_debug_log(":-points_wanted %ld", points_wanted);
    }

    if(ABS(before_requested) <= API_RELATIVE_TIME_MAX || ABS(after_requested) <= API_RELATIVE_TIME_MAX) {
        relative_period_requested = true;
        natural_points = true;
        options |= RRDR_OPTION_NATURAL_POINTS;
        query_debug_log(":relative+natural");
    }

    // this is the update_every of the query
    // it may be different to the update_every of the database
    time_t query_granularity = (natural_points)?update_every:1;
    query_debug_log(":query_granularity %ld", query_granularity);

    if(after_wanted == 0 || before_wanted == 0) {
        // for non-context queries we have to find the duration of the database
        // for context queries we will assume 600 seconds duration

        if(!context_param_list) {
            relative_period_requested = true;

            rrdset_rdlock(st);
            time_t first_entry_t = rrdset_first_entry_t_nolock(st, options & RRDR_OPTION_TIER1? 1 : 0);
            time_t last_entry_t = rrdset_last_entry_t_nolock(st, options & RRDR_OPTION_TIER1? 1 : 0);
            rrdset_unlock(st);

            query_debug_log(":first_entry_t %ld, last_entry_t %ld", first_entry_t, last_entry_t);

            if (after_wanted == 0) {
                after_wanted = first_entry_t;
                query_debug_log(":zero after_wanted %lld", after_wanted);
            }

            if (before_wanted == 0) {
                before_wanted = last_entry_t;
                query_debug_log(":zero before_wanted %lld", before_wanted);
            }

            if(points_wanted == 0) {
                points_wanted = (last_entry_t - first_entry_t) / update_every;
                query_debug_log(":zero points_wanted %ld", points_wanted);
            }
        }

        // if they are still zero, assume 600

        if(after_wanted == 0) {
            after_wanted = -600;
            query_debug_log(":zero600 after_wanted %lld", after_wanted);
        }

        if(points_wanted == 0) {
            points_wanted = 600;
            query_debug_log(":zero600 points_wanted %ld", points_wanted);
        }
    }

    // convert our before_wanted and after_wanted to absolute
    rrdr_relative_window_to_absolute(&after_wanted, &before_wanted, (int)query_granularity, points_wanted);
    query_debug_log(":relative2absolute after %lld, before %lld", after_wanted, before_wanted);

    // align before_wanted and after_wanted to query_granularity
    if (before_wanted % query_granularity) {
        before_wanted -= before_wanted % query_granularity;
        query_debug_log(":granularity align before_wanted %lld", before_wanted);
    }

    if (after_wanted % query_granularity) {
        after_wanted -= after_wanted % query_granularity;
        query_debug_log(":granularity align after_wanted %lld", after_wanted);
    }

    // automatic_natural_points is set when the user wants all the points available in the database
    if(automatic_natural_points) {
        points_wanted = (before_wanted - after_wanted + 1) / query_granularity;
        query_debug_log(":auto natural points_wanted %ld", points_wanted);
    }

    time_t duration = before_wanted - after_wanted;

    // if the resampling time is too big, extend the duration to the past
    if (unlikely(resampling_time_requested > duration)) {
        after_wanted = before_wanted - resampling_time_requested;
        duration = before_wanted - after_wanted;
        query_debug_log(":resampling after_wanted %lld", after_wanted);
    }

    // if the duration is not aligned to resampling time
    // extend the duration to the past, to avoid a gap at the chart
    // only when the missing duration is above 1/10th of a point
    if(resampling_time_requested > query_granularity && duration % resampling_time_requested) {
        time_t delta = duration % resampling_time_requested;
        if(delta > resampling_time_requested / 10) {
            after_wanted -= resampling_time_requested - delta;
            duration = before_wanted - after_wanted;
            query_debug_log(":resampling2 after_wanted %lld", after_wanted);
        }
    }

    // the available points of the query
    long points_available = (duration + 1) / query_granularity;
    query_debug_log(":points_available %ld", points_available);

    if(points_wanted > points_available) {
        points_wanted = points_available;
        query_debug_log(":max points_wanted %ld", points_wanted);
    }

    // calculate the desired grouping of source data points
    long group = points_available / points_wanted;
    if(group <= 0) group = 1;

    // round "group" to the closest integer
    if(points_available % points_wanted > points_wanted / 2)
        group++;

    query_debug_log(":group %ld", group);

    // resampling_time_requested enforces a certain grouping multiple
    NETDATA_DOUBLE resampling_divisor = 1.0;
    long resampling_group = 1;
    if(unlikely(resampling_time_requested > query_granularity)) {
        // the points we should group to satisfy gtime
        resampling_group = resampling_time_requested / query_granularity;
        if(unlikely(resampling_time_requested % query_granularity))
            resampling_group++;

        query_debug_log(":resampling group %ld", resampling_group);

        // adapt group according to resampling_group
        if(unlikely(group < resampling_group)) {
            group  = resampling_group; // do not allow grouping below the desired one
            query_debug_log(":group less res %ld", group);
        }
        if(unlikely(group % resampling_group)) {
            group += resampling_group - (group % resampling_group); // make sure group is multiple of resampling_group
            query_debug_log(":group mod res %ld", group);
        }

        // resampling_divisor = group / resampling_group;
        resampling_divisor = (NETDATA_DOUBLE)(group * query_granularity) / (NETDATA_DOUBLE)resampling_time_requested;
        query_debug_log(":resampling divisor " NETDATA_DOUBLE_FORMAT, resampling_divisor);
    }

    // now that we have group, align the requested timeframe to fit it.
    if(aligned && before_wanted % (group * query_granularity)) {
        // alignment has been requested, so align the end timestamp
        before_wanted += (group * query_granularity) - before_wanted % (group * query_granularity);
        query_debug_log(":align before_wanted %lld", before_wanted);
    }

    after_wanted  = before_wanted - (points_wanted * group * query_granularity) + query_granularity;
    query_debug_log(":final after_wanted %lld", after_wanted);

    duration = before_wanted - after_wanted;
    query_debug_log(":final duration %ld", duration);

    // check the context query based on the starting time of the query
    if (context_param_list && !(context_param_list->flags & CONTEXT_FLAGS_ARCHIVE)) {
        rebuild_context_param_list(owa, context_param_list, after_wanted);
        st = context_param_list->rd ? context_param_list->rd->rrdset : NULL;

        if(unlikely(!st))
            return NULL;
    }

    internal_error(points_wanted != duration / (query_granularity * group) + 1,
                   "QUERY: points_wanted %ld is not points %ld",
                   points_wanted, duration / (query_granularity * group) + 1);

    internal_error(group < resampling_group,
                   "QUERY: group %ld is less than the desired group points %ld",
                   group, resampling_group);

    internal_error(group > resampling_group && group % resampling_group,
                   "QUERY: group %ld is not a multiple of the desired group points %ld",
                   group, resampling_group);

    // -------------------------------------------------------------------------
    // initialize our result set
    // this also locks the chart for us

    RRDR *r = rrdr_create(owa, st, points_wanted, context_param_list);
    if(unlikely(!r)) {
        internal_error(true, "QUERY: cannot create RRDR for %s, after=%u, before=%u, duration=%u, points=%ld",
                       st->id, (uint32_t)after_wanted, (uint32_t)before_wanted, (uint32_t)duration, points_wanted);
        return NULL;
    }

    if(unlikely(!r->d || !points_wanted)) {
        internal_error(true, "QUERY: returning empty RRDR (no dimensions in RRDSET) for %s, after=%u, before=%u, duration=%zu, points=%ld",
                       st->id, (uint32_t)after_wanted, (uint32_t)before_wanted, (size_t)duration, points_wanted);
        return r;
    }

    if(relative_period_requested)
        r->result_options |= RRDR_RESULT_OPTION_RELATIVE;
    else
        r->result_options |= RRDR_RESULT_OPTION_ABSOLUTE;

    // find how many dimensions we have
    long dimensions_count = r->d;

    // -------------------------------------------------------------------------
    // initialize RRDR

    r->group = group;
    r->update_every = (int)(group * query_granularity);
    r->before = before_wanted;
    r->after = after_wanted;
    r->internal.points_wanted = points_wanted;
    r->internal.resampling_group = resampling_group;
    r->internal.resampling_divisor = resampling_divisor;


    // -------------------------------------------------------------------------
    // assign the processor functions
    rrdr_set_grouping_function(r, group_method);

    // allocate any memory required by the grouping method
    r->internal.grouping_create(r, group_options);


    // -------------------------------------------------------------------------
    // disable the not-wanted dimensions

    if (context_param_list && !(context_param_list->flags & CONTEXT_FLAGS_ARCHIVE))
        rrdset_check_rdlock(st);

    if(dimensions)
        rrdr_disable_not_selected_dimensions(r, options, dimensions, context_param_list);


    query_debug_log_fin();

    // -------------------------------------------------------------------------
    // do the work for each dimension

    time_t max_after = 0, min_before = 0;
    long max_rows = 0;

    RRDDIM *first_rd = context_param_list ? context_param_list->rd : st->dimensions;
    RRDDIM *rd;
    long c, dimensions_used = 0, dimensions_nonzero = 0;
    struct timeval query_start_time;
    struct timeval query_current_time;
    if (timeout) now_realtime_timeval(&query_start_time);

    TIER_QUERY_FETCH tier_query_fetch_type = TIER_QUERY_FETCH_AVERAGE;
    for(rd = first_rd, c = 0 ; rd && c < dimensions_count ; rd = rd->next, c++) {

        // if we need a percentage, we need to calculate all dimensions
        if(unlikely(!(options & RRDR_OPTION_PERCENTAGE) && (r->od[c] & RRDR_DIMENSION_HIDDEN))) {
            if(unlikely(r->od[c] & RRDR_DIMENSION_SELECTED)) r->od[c] &= ~RRDR_DIMENSION_SELECTED;
            continue;
        }
        r->od[c] |= RRDR_DIMENSION_SELECTED;

        // reset the grouping for the new dimension
        r->internal.grouping_reset(r);

        rrd2rrdr_do_dimension(r, points_wanted, rd, c, after_wanted, before_wanted, options, tier_query_fetch_type);
        if (timeout)
            now_realtime_timeval(&query_current_time);

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
                internal_error(true, "QUERY: 'after' mismatch between dimensions for chart '%s': max is %zu, dimension '%s' has %zu",
                               st->name, (size_t)max_after, rd->name, (size_t)r->after);

                r->after = (r->after > max_after) ? r->after : max_after;
            }

            if(r->before != min_before) {
                internal_error(true, "QUERY: 'before' mismatch between dimensions for chart '%s': max is %zu, dimension '%s' has %zu",
                               st->name, (size_t)min_before, rd->name, (size_t)r->before);

                r->before = (r->before < min_before) ? r->before : min_before;
            }

            if(r->rows != max_rows) {
                internal_error(true, "QUERY: 'rows' mismatch between dimensions for chart '%s': max is %zu, dimension '%s' has %zu",
                               st->name, (size_t)max_rows, rd->name, (size_t)r->rows);

                r->rows = (r->rows > max_rows) ? r->rows : max_rows;
            }
        }

        dimensions_used++;
        if (timeout && (dt_usec(&query_start_time, &query_current_time) / 1000.0) > timeout) {
            log_access("QUERY CANCELED RUNTIME EXCEEDED %0.2f ms (LIMIT %d ms)",
                       dt_usec(&query_start_time, &query_current_time) / 1000.0, timeout);
            r->result_options |= RRDR_RESULT_OPTION_CANCEL;
            break;
        }
    }

#ifdef NETDATA_INTERNAL_CHECKS
    if (dimensions_used) {
        if(r->internal.log)
            rrd2rrdr_log_request_response_metadata(r, options, group_method, aligned, group, resampling_time_requested, resampling_group, after_wanted,
                after_wanted, before_wanted,
                before_wanted,
                points_wanted, points_wanted, /*after_slot, before_slot,*/ r->internal.log);

        if(r->rows != points_wanted)
            rrd2rrdr_log_request_response_metadata(r, options, group_method, aligned, group, resampling_time_requested, resampling_group, after_wanted,
                after_wanted, before_wanted,
                before_wanted,
                points_wanted, points_wanted, /*after_slot, before_slot,*/ "got 'points' is not wanted 'points'");

        if(aligned && (r->before % (group * query_granularity)) != 0)
            rrd2rrdr_log_request_response_metadata(r, options, group_method, aligned, group, resampling_time_requested, resampling_group, after_wanted,
                after_wanted, before_wanted,
                before_wanted,
                points_wanted, points_wanted, /*after_slot, before_slot,*/ "'before' is not aligned but alignment is required");

        // 'after' should not be aligned, since we start inside the first group
        //if(aligned && (r->after % group) != 0)
        //    rrd2rrdr_log_request_response_metadata(r, options, group_method, aligned, group, resampling_time_requested, resampling_group, after_wanted, after_requested, before_wanted, before_requested, points_requested, points_wanted, after_slot, before_slot, "'after' is not aligned but alignment is required");

        if(r->before != before_wanted)
            rrd2rrdr_log_request_response_metadata(r, options, group_method, aligned, group, resampling_time_requested, resampling_group, after_wanted,
                after_wanted, before_wanted,
                before_wanted,
                points_wanted, points_wanted, /*after_slot, before_slot,*/ "chart is not aligned to requested 'before'");

        if(r->before != before_wanted)
            rrd2rrdr_log_request_response_metadata(r, options, group_method, aligned, group, resampling_time_requested, resampling_group, after_wanted,
                after_wanted, before_wanted,
                before_wanted,
                points_wanted, points_wanted, /*after_slot, before_slot,*/ "got 'before' is not wanted 'before'");

        // reported 'after' varies, depending on group
        if(r->after != after_wanted)
            rrd2rrdr_log_request_response_metadata(r, options, group_method, aligned, group, resampling_time_requested, resampling_group, after_wanted,
                after_wanted, before_wanted,
                before_wanted,
                points_wanted, points_wanted, /*after_slot, before_slot,*/ "got 'after' is not wanted 'after'");
    }
#endif

    // free all resources used by the grouping method
    r->internal.grouping_free(r);

    // when all the dimensions are zero, we should return all of them
    if(unlikely(options & RRDR_OPTION_NONZERO && !dimensions_nonzero && !(r->result_options & RRDR_RESULT_OPTION_CANCEL))) {
        // all the dimensions are zero
        // mark them as NONZERO to send them all
        for(rd = first_rd, c = 0 ; rd && c < dimensions_count ; rd = rd->next, c++) {
            if(unlikely(r->od[c] & RRDR_DIMENSION_HIDDEN)) continue;
            r->od[c] |= RRDR_DIMENSION_NONZERO;
        }
    }

    rrdr_query_completed(r->internal.db_points_read, r->internal.result_points_generated);
    return r;
}
