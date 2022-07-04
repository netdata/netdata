// SPDX-License-Identifier: GPL-3.0-or-later

#include "query.h"
#include "web/api/formatters/rrd2json.h"
#include "rrdr.h"

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

    TIER_QUERY_FETCH tier_query_fetch;
} api_v1_data_groups[] = {
        {.name = "average",
                .hash  = 0,
                .value = RRDR_GROUPING_AVERAGE,
                .init  = NULL,
                .create= grouping_create_average,
                .reset = grouping_reset_average,
                .free  = grouping_free_average,
                .add   = grouping_add_average,
                .flush = grouping_flush_average,
                .tier_query_fetch = TIER_QUERY_FETCH_AVERAGE
        },
        {.name = "mean",                           // alias on 'average'
                .hash  = 0,
                .value = RRDR_GROUPING_AVERAGE,
                .init  = NULL,
                .create= grouping_create_average,
                .reset = grouping_reset_average,
                .free  = grouping_free_average,
                .add   = grouping_add_average,
                .flush = grouping_flush_average,
                .tier_query_fetch = TIER_QUERY_FETCH_AVERAGE
        },
        {.name  = "incremental_sum",
                .hash  = 0,
                .value = RRDR_GROUPING_INCREMENTAL_SUM,
                .init  = NULL,
                .create= grouping_create_incremental_sum,
                .reset = grouping_reset_incremental_sum,
                .free  = grouping_free_incremental_sum,
                .add   = grouping_add_incremental_sum,
                .flush = grouping_flush_incremental_sum,
                .tier_query_fetch = TIER_QUERY_FETCH_AVERAGE
        },
        {.name = "incremental-sum",
                .hash  = 0,
                .value = RRDR_GROUPING_INCREMENTAL_SUM,
                .init  = NULL,
                .create= grouping_create_incremental_sum,
                .reset = grouping_reset_incremental_sum,
                .free  = grouping_free_incremental_sum,
                .add   = grouping_add_incremental_sum,
                .flush = grouping_flush_incremental_sum,
                .tier_query_fetch = TIER_QUERY_FETCH_AVERAGE
        },
        {.name = "median",
                .hash  = 0,
                .value = RRDR_GROUPING_MEDIAN,
                .init  = NULL,
                .create= grouping_create_median,
                .reset = grouping_reset_median,
                .free  = grouping_free_median,
                .add   = grouping_add_median,
                .flush = grouping_flush_median,
                .tier_query_fetch = TIER_QUERY_FETCH_AVERAGE
        },
        {.name = "min",
                .hash  = 0,
                .value = RRDR_GROUPING_MIN,
                .init  = NULL,
                .create= grouping_create_min,
                .reset = grouping_reset_min,
                .free  = grouping_free_min,
                .add   = grouping_add_min,
                .flush = grouping_flush_min,
                .tier_query_fetch = TIER_QUERY_FETCH_MIN
        },
        {.name = "max",
                .hash  = 0,
                .value = RRDR_GROUPING_MAX,
                .init  = NULL,
                .create= grouping_create_max,
                .reset = grouping_reset_max,
                .free  = grouping_free_max,
                .add   = grouping_add_max,
                .flush = grouping_flush_max,
                .tier_query_fetch = TIER_QUERY_FETCH_MAX
        },
        {.name = "sum",
                .hash  = 0,
                .value = RRDR_GROUPING_SUM,
                .init  = NULL,
                .create= grouping_create_sum,
                .reset = grouping_reset_sum,
                .free  = grouping_free_sum,
                .add   = grouping_add_sum,
                .flush = grouping_flush_sum,
                .tier_query_fetch = TIER_QUERY_FETCH_SUM
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
                .flush = grouping_flush_stddev,
                .tier_query_fetch = TIER_QUERY_FETCH_AVERAGE
        },
        {.name = "cv",                           // coefficient variation is calculated by stddev
                .hash  = 0,
                .value = RRDR_GROUPING_CV,
                .init  = NULL,
                .create= grouping_create_stddev, // not an error, stddev calculates this too
                .reset = grouping_reset_stddev,  // not an error, stddev calculates this too
                .free  = grouping_free_stddev,   // not an error, stddev calculates this too
                .add   = grouping_add_stddev,    // not an error, stddev calculates this too
                .flush = grouping_flush_coefficient_of_variation,
                .tier_query_fetch = TIER_QUERY_FETCH_AVERAGE
        },
        {.name = "rsd",                          // alias of 'cv'
                .hash  = 0,
                .value = RRDR_GROUPING_CV,
                .init  = NULL,
                .create= grouping_create_stddev, // not an error, stddev calculates this too
                .reset = grouping_reset_stddev,  // not an error, stddev calculates this too
                .free  = grouping_free_stddev,   // not an error, stddev calculates this too
                .add   = grouping_add_stddev,    // not an error, stddev calculates this too
                .flush = grouping_flush_coefficient_of_variation,
                .tier_query_fetch = TIER_QUERY_FETCH_AVERAGE
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
                .flush = grouping_flush_mean,
                .tier_query_fetch = TIER_QUERY_FETCH_AVERAGE
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
                .flush = grouping_flush_variance,
                .tier_query_fetch = TIER_QUERY_FETCH_AVERAGE
        },
        */

        // single exponential smoothing
        {.name = "ses",
                .hash  = 0,
                .value = RRDR_GROUPING_SES,
                .init  = grouping_init_ses,
                .create= grouping_create_ses,
                .reset = grouping_reset_ses,
                .free  = grouping_free_ses,
                .add   = grouping_add_ses,
                .flush = grouping_flush_ses,
                .tier_query_fetch = TIER_QUERY_FETCH_AVERAGE
        },
        {.name = "ema",                         // alias for 'ses'
                .hash  = 0,
                .value = RRDR_GROUPING_SES,
                .init  = NULL,
                .create= grouping_create_ses,
                .reset = grouping_reset_ses,
                .free  = grouping_free_ses,
                .add   = grouping_add_ses,
                .flush = grouping_flush_ses,
                .tier_query_fetch = TIER_QUERY_FETCH_AVERAGE
        },
        {.name = "ewma",                        // alias for ses
                .hash  = 0,
                .value = RRDR_GROUPING_SES,
                .init  = NULL,
                .create= grouping_create_ses,
                .reset = grouping_reset_ses,
                .free  = grouping_free_ses,
                .add   = grouping_add_ses,
                .flush = grouping_flush_ses,
                .tier_query_fetch = TIER_QUERY_FETCH_AVERAGE
        },

        // double exponential smoothing
        {.name = "des",
                .hash  = 0,
                .value = RRDR_GROUPING_DES,
                .init  = grouping_init_des,
                .create= grouping_create_des,
                .reset = grouping_reset_des,
                .free  = grouping_free_des,
                .add   = grouping_add_des,
                .flush = grouping_flush_des,
                .tier_query_fetch = TIER_QUERY_FETCH_AVERAGE
        },

        {.name = "countif",
                .hash  = 0,
                .value = RRDR_GROUPING_COUNTIF,
                .init = NULL,
                .create= grouping_create_countif,
                .reset = grouping_reset_countif,
                .free  = grouping_free_countif,
                .add   = grouping_add_countif,
                .flush = grouping_flush_countif,
                .tier_query_fetch = TIER_QUERY_FETCH_AVERAGE
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
                .flush = grouping_flush_average,
                .tier_query_fetch = TIER_QUERY_FETCH_AVERAGE
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
            r->internal.grouping_create  = api_v1_data_groups[i].create;
            r->internal.grouping_reset   = api_v1_data_groups[i].reset;
            r->internal.grouping_free    = api_v1_data_groups[i].free;
            r->internal.grouping_add     = api_v1_data_groups[i].add;
            r->internal.grouping_flush   = api_v1_data_groups[i].flush;
            r->internal.tier_query_fetch = api_v1_data_groups[i].tier_query_fetch;
            found = 1;
        }
    }
    if(!found) {
        errno = 0;
        internal_error(true, "QUERY: grouping method %u not found. Using 'average'", (unsigned int)group_method);
        r->internal.grouping_create  = grouping_create_average;
        r->internal.grouping_reset   = grouping_reset_average;
        r->internal.grouping_free    = grouping_free_average;
        r->internal.grouping_add     = grouping_add_average;
        r->internal.grouping_flush   = grouping_flush_average;
        r->internal.tier_query_fetch = TIER_QUERY_FETCH_AVERAGE;
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
        // there are a few selected dimensions,
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
// tier management

static int rrddim_find_best_tier_for_timeframe(RRDDIM *rd, time_t after_wanted, time_t before_wanted, long points_wanted) {
    if(unlikely(storage_tiers < 2))
        return 0;

    if(unlikely(after_wanted == before_wanted || points_wanted <= 0 || !rd || !rd->rrdset)) {

        if(!rd)
            internal_error(true, "QUERY: NULL dimension - invalid params to tier calculation");
        else
            internal_error(true, "QUERY: chart '%s' dimension '%s' invalid params to tier calculation",
                           (rd->rrdset)?rd->rrdset->name:"unknown", rd->name);

        return 0;
    }

    //BUFFER *wb = buffer_create(1000);
    //buffer_sprintf(wb, "Best tier for chart '%s', dim '%s', from %ld to %ld (dur %ld, every %d), points %ld",
    //               rd->rrdset->name, rd->name, after_wanted, before_wanted, before_wanted - after_wanted, rd->update_every, points_wanted);

    long weight[storage_tiers];

    for(int tier = 0; tier < storage_tiers ; tier++) {
        if(unlikely(!rd->tiers[tier])) {
            internal_error(true, "QUERY: tier %d of chart '%s' dimension '%s' not initialized",
                           tier, rd->rrdset->name, rd->name);
    //        buffer_free(wb);
            return 0;
        }

        time_t first_t = rd->tiers[tier]->query_ops.oldest_time(rd->tiers[tier]->db_metric_handle);
        time_t last_t  = rd->tiers[tier]->query_ops.latest_time(rd->tiers[tier]->db_metric_handle);

        time_t common_after = MAX(first_t, after_wanted);
        time_t common_before = MIN(last_t, before_wanted);

        long time_coverage = (common_before - common_after) * 1000 / (before_wanted - after_wanted);
        if(time_coverage < 0) time_coverage = 0;

        int update_every = (int)rd->tiers[tier]->tier_grouping * (int)rd->update_every;
        if(unlikely(update_every == 0)) {
            internal_error(true, "QUERY: update_every of tier %d for chart '%s' dimension '%s' is zero. tg = %d, ue = %d",
                           tier, rd->rrdset->name, rd->name, rd->tiers[tier]->tier_grouping, rd->update_every);
    //        buffer_free(wb);
            return 0;
        }

        long points_available = (before_wanted - after_wanted) / update_every;
        long points_delta = points_available - points_wanted;
        long points_coverage = (points_delta < 0) ? points_available * 1000 / points_wanted: 1000;

        if(points_available <= 0)
            weight[tier] = -LONG_MAX;
        else
            weight[tier] = points_coverage;

    //    buffer_sprintf(wb, ": tier %d, first %ld, last %ld (dur %ld, tg %d, every %d), points %ld, tcoverage %ld, pcoverage %ld, weight %ld",
    //                   tier, first_t, last_t, last_t - first_t, rd->tiers[tier]->tier_grouping, update_every,
    //                   points_available, time_coverage, points_coverage, weight[tier]);
    }

    int best_tier = 0;
    for(int tier = 1; tier < storage_tiers ; tier++) {
        if(weight[tier] >= weight[best_tier])
            best_tier = tier;
    }

    if(weight[best_tier] == -LONG_MAX)
        best_tier = 0;

    //buffer_sprintf(wb, ": final best tier %d", best_tier);
    //internal_error(true, "%s", buffer_tostring(wb));
    //buffer_free(wb);

    return best_tier;
}

static int rrdset_find_natural_update_every_for_timeframe(RRDSET *st, time_t after_wanted, time_t before_wanted, long points_wanted, RRDR_OPTIONS options, int tier) {
    int ret = st->update_every;

    if(unlikely(!st->dimensions))
        return ret;

    rrdset_rdlock(st);
    int best_tier;

    if(options & RRDR_OPTION_SELECTED_TIER && tier >= 0 && tier < storage_tiers)
        best_tier = tier;
    else
        best_tier = rrddim_find_best_tier_for_timeframe(st->dimensions, after_wanted, before_wanted, points_wanted);

    if(!st->dimensions->tiers[best_tier]) {
        internal_error(
            true,
            "QUERY: tier %d on chart '%s', is not initialized", best_tier, st->name);
    }
    else {
        ret = (int)st->dimensions->tiers[best_tier]->tier_grouping * (int)st->update_every;
        if(unlikely(!ret)) {
            internal_error(
                true,
                "QUERY: update_every calculated to be zero on chart '%s', tier_grouping %d, update_every %d",
                st->name, st->dimensions->tiers[best_tier]->tier_grouping, st->update_every);

            ret = st->update_every;
        }
    }

    rrdset_unlock(st);

    return ret;
}

// ----------------------------------------------------------------------------
// query ops

typedef struct query_point {
    NETDATA_DOUBLE value;
    SN_FLAGS flags;
    size_t anomaly;
    time_t start_time;
    time_t end_time;
} QUERY_POINT;

QUERY_POINT QUERY_POINT_EMPTY = {
    .value = NAN,
    .flags = SN_EMPTY_SLOT,
    .anomaly = 0,
    .start_time = 0,
    .end_time = 0
};

typedef struct query_plan_entry {
    size_t tier;
    time_t after;
    time_t before;
} QUERY_PLAN_ENTRY;

typedef struct query_plan {
    size_t entries;
    QUERY_PLAN_ENTRY data[RRD_STORAGE_TIERS*2];
} QUERY_PLAN;

typedef struct query_engine_ops {
    // configuration
    RRDR *r;
    RRDDIM *rd;
    time_t view_update_every;
    time_t query_granularity;

    // query planer
    QUERY_PLAN plan;
    size_t current_plan;
    time_t current_plan_expire_time;

    // storage queries
    size_t tier;
    struct rrddim_tier *tier_ptr;
    struct rrddim_query_handle handle;
    NETDATA_DOUBLE (*next_metric)(struct rrddim_query_handle *handle, time_t *current_time, time_t *end_time, SN_FLAGS *flags, uint16_t *count, uint16_t *anomaly_count);
    int (*is_finished)(struct rrddim_query_handle *handle);
    void (*finalize)(struct rrddim_query_handle *handle);

    // aggregating points over time
    void (*grouping_add)(struct rrdresult *r, NETDATA_DOUBLE value);
    NETDATA_DOUBLE (*grouping_flush)(struct rrdresult *r, RRDR_VALUE_FLAGS *rrdr_value_options_ptr);
    size_t group_points_non_zero;
    size_t group_points_added;
    size_t group_anomaly_rate;
    RRDR_VALUE_FLAGS group_value_flags;

    // statistics
    size_t db_total_points_read;
    size_t db_points_read_per_tier[RRD_STORAGE_TIERS];
} QUERY_ENGINE_OPS;


// ----------------------------------------------------------------------------
// query planer

#define query_plan_should_switch_plan(ops, now) ((now) >= (ops).current_plan_expire_time)

static void query_planer_activate_plan(QUERY_ENGINE_OPS *ops, size_t plan_id, time_t overwrite_after) {
    if(unlikely(plan_id >= ops->plan.entries))
        plan_id = ops->plan.entries - 1;

    time_t after = ops->plan.data[plan_id].after;
    time_t before = ops->plan.data[plan_id].before;

    if(overwrite_after > after && overwrite_after < before)
        after = overwrite_after;

    ops->tier = ops->plan.data[plan_id].tier;
    ops->tier_ptr = ops->rd->tiers[ops->tier];
    ops->tier_ptr->query_ops.init(ops->tier_ptr->db_metric_handle, &ops->handle, after, before, ops->r->internal.tier_query_fetch);
    ops->next_metric = ops->tier_ptr->query_ops.next_metric;
    ops->is_finished = ops->tier_ptr->query_ops.is_finished;
    ops->finalize = ops->tier_ptr->query_ops.finalize;
    ops->current_plan = plan_id;
    ops->current_plan_expire_time = ops->plan.data[plan_id].before;
}

static void query_planer_next_plan(QUERY_ENGINE_OPS *ops, time_t now, time_t last_point_end_time) {
    internal_error(now < ops->current_plan_expire_time && now < ops->plan.data[ops->current_plan].before,
                   "QUERY: switching query plan too early!");

    time_t next_plan_before_time;
    do {
        ops->current_plan++;

        if (ops->current_plan >= ops->plan.entries) {
            ops->current_plan = ops->plan.entries - 1;
            return;
        }

        next_plan_before_time = ops->plan.data[ops->current_plan].before;
    } while(now >= next_plan_before_time || last_point_end_time >= next_plan_before_time);

    if(ops->finalize) {
        ops->finalize(&ops->handle);
        ops->finalize = NULL;
    }

    query_planer_activate_plan(ops, ops->current_plan, MIN(now, last_point_end_time));

    // internal_error(true, "QUERY: switched plan to %zu (all is %zu), previous expiration was %ld, this starts at %ld, now is %ld, last_point_end_time %ld", ops->current_plan, ops->plan.entries, ops->plan.data[ops->current_plan-1].before, ops->plan.data[ops->current_plan].after, now, last_point_end_time);
}

static int compare_query_plan_entries_on_start_time(const void *a, const void *b) {
    QUERY_PLAN_ENTRY *p1 = (QUERY_PLAN_ENTRY *)a;
    QUERY_PLAN_ENTRY *p2 = (QUERY_PLAN_ENTRY *)b;
    return (p1->after < p2->after)?-1:1;
}

static void query_plan(QUERY_ENGINE_OPS *ops, time_t after_wanted, time_t before_wanted, long points_wanted) {
    RRDDIM *rd = ops->rd;

    //BUFFER *wb = buffer_create(1000);
    //buffer_sprintf(wb, "QUERY PLAN for chart '%s' dimension '%s', from %ld to %ld:", rd->rrdset->name, rd->name, after_wanted, before_wanted);

    // put our selected tier as the first plan
    size_t selected_tier;

    if(ops->r->internal.query_options & RRDR_OPTION_SELECTED_TIER && ops->r->internal.query_tier >= 0 && ops->r->internal.query_tier < storage_tiers) {
        selected_tier = ops->r->internal.query_tier;
    }
    else {

        selected_tier = rrddim_find_best_tier_for_timeframe(rd, after_wanted, before_wanted, points_wanted);

        if(ops->r->internal.query_options & RRDR_OPTION_SELECTED_TIER)
            ops->r->internal.query_options &= ~RRDR_OPTION_SELECTED_TIER;
    }

    ops->plan.entries = 1;
    ops->plan.data[0].tier = selected_tier;
    ops->plan.data[0].after = rd->tiers[selected_tier]->query_ops.oldest_time(rd->tiers[selected_tier]->db_metric_handle);
    ops->plan.data[0].before = rd->tiers[selected_tier]->query_ops.latest_time(rd->tiers[selected_tier]->db_metric_handle);

    if(!(ops->r->internal.query_options & RRDR_OPTION_SELECTED_TIER)) {
        // the selected tier
        time_t selected_tier_first_time_t = ops->plan.data[0].after;
        time_t selected_tier_last_time_t = ops->plan.data[0].before;

        //buffer_sprintf(wb, ": SELECTED tier %zu, from %ld to %ld", selected_tier, ops->plan.data[0].after, ops->plan.data[0].before);

        // check if our selected tier can start the query
        if (selected_tier_first_time_t > after_wanted) {
            // we need some help from other tiers
            for (int tr = (int)selected_tier + 1; tr < storage_tiers; tr++) {
                // find the first time of this tier
                time_t first_time_t = rd->tiers[tr]->query_ops.oldest_time(rd->tiers[tr]->db_metric_handle);

                //buffer_sprintf(wb, ": EVAL AFTER tier %d, %ld", tier, first_time_t);

                // can it help?
                if (first_time_t < selected_tier_first_time_t) {
                    // it can help us add detail at the beginning of the query
                    QUERY_PLAN_ENTRY t = {
                        .tier = tr,
                        .after = (first_time_t < after_wanted) ? after_wanted : first_time_t,
                        .before = selected_tier_first_time_t};
                    ops->plan.data[ops->plan.entries++] = t;

                    // prepare for the tier
                    selected_tier_first_time_t = t.after;

                    if (t.after <= after_wanted)
                        break;
                }
            }
        }

        // check if our selected tier can finish the query
        if (selected_tier_last_time_t < before_wanted) {
            // we need some help from other tiers
            for (int tr = (int)selected_tier - 1; tr >= 0; tr--) {
                // find the last time of this tier
                time_t last_time_t = rd->tiers[tr]->query_ops.latest_time(rd->tiers[tr]->db_metric_handle);

                //buffer_sprintf(wb, ": EVAL BEFORE tier %d, %ld", tier, last_time_t);

                // can it help?
                if (last_time_t > selected_tier_last_time_t) {
                    // it can help us add detail at the end of the query
                    QUERY_PLAN_ENTRY t = {
                        .tier = tr,
                        .after = selected_tier_last_time_t,
                        .before = (last_time_t > before_wanted) ? before_wanted : last_time_t};
                    ops->plan.data[ops->plan.entries++] = t;

                    // prepare for the tier
                    selected_tier_last_time_t = t.before;

                    if (t.before >= before_wanted)
                        break;
                }
            }
        }
    }

    // sort the query plan
    if(ops->plan.entries > 1)
        qsort(&ops->plan.data, ops->plan.entries, sizeof(QUERY_PLAN_ENTRY), compare_query_plan_entries_on_start_time);

    // make sure it has the whole timeframe we need
    ops->plan.data[0].after = after_wanted;
    ops->plan.data[ops->plan.entries - 1].before = before_wanted;

    //buffer_sprintf(wb, ": FINAL STEPS %zu", ops->plan.entries);

    //for(size_t i = 0; i < ops->plan.entries ;i++)
    //    buffer_sprintf(wb, ": STEP %zu = use tier %zu from %ld to %ld", i+1, ops->plan.data[i].tier, ops->plan.data[i].after, ops->plan.data[i].before);

    //internal_error(true, "%s", buffer_tostring(wb));

    query_planer_activate_plan(ops, 0, 0);
}


// ----------------------------------------------------------------------------
// dimension level query engine

#define query_interpolate_point(this_point, last_point, now)      do {  \
    if(likely(                                                          \
            /* the point to interpolate is more than 1s wide */         \
            (this_point).end_time - (this_point).start_time > 1         \
                                                                        \
            /* the two points are exactly next to each other */         \
         && (last_point).end_time == (this_point).start_time            \
                                                                        \
            /* both points are valid numbers */                         \
         && netdata_double_isnumber((this_point).value)                 \
         && netdata_double_isnumber((last_point).value)                 \
                                                                        \
        )) {                                                            \
            (this_point).value = (last_point).value + ((this_point).value - (last_point).value) * (1.0 - (NETDATA_DOUBLE)((this_point).end_time - (now)) / (NETDATA_DOUBLE)((this_point).end_time - (this_point).start_time)); \
            (this_point).end_time = now;                                \
        }                                                               \
} while(0)

#define query_add_point_to_group(r, point, ops)                   do {  \
    if(likely(netdata_double_isnumber((point).value))) {                \
        if(likely((point).value != 0.0))                                \
            (ops).group_points_non_zero++;                              \
                                                                        \
        if(unlikely((point).flags & SN_EXISTS_RESET))                   \
            (ops).group_value_flags |= RRDR_VALUE_RESET;                \
                                                                        \
        (ops).grouping_add(r, (point).value);                           \
    }                                                                   \
                                                                        \
    (ops).group_points_added++;                                         \
    (ops).group_anomaly_rate += (point).anomaly;                        \
} while(0)

static inline void rrd2rrdr_do_dimension(
    RRDR *r
    , long points_wanted
    , RRDDIM *rd
    , long dim_id_in_rrdr
    , time_t after_wanted
    , time_t before_wanted
){
    time_t max_date = 0,
           min_date = 0;

    size_t points_added = 0;

    QUERY_ENGINE_OPS ops = {
        .r = r,
        .rd = rd,
        .grouping_add = r->internal.grouping_add,
        .grouping_flush = r->internal.grouping_flush,
        .view_update_every = r->update_every,
        .query_granularity = r->update_every / r->group,
        .group_value_flags = RRDR_VALUE_NOTHING
    };

    long rrdr_line = -1;
    bool use_anomaly_bit_as_value = (r->internal.query_options & RRDR_OPTION_ANOMALY_BIT) ? true : false;

    query_plan(&ops, after_wanted, before_wanted, points_wanted);

    NETDATA_DOUBLE min = r->min, max = r->max;

    QUERY_POINT last2_point = QUERY_POINT_EMPTY;
    QUERY_POINT last1_point = QUERY_POINT_EMPTY;
    QUERY_POINT new_point   = QUERY_POINT_EMPTY;

    // The main loop, based on the query granularity we need
    for(time_t now = after_wanted + ops.view_update_every - ops.query_granularity; (long)points_added < points_wanted ; now += ops.view_update_every) {

        if(query_plan_should_switch_plan(ops, now))
            query_planer_next_plan(&ops, now, new_point.end_time);

        // real all the points of the db, prior to the time we need (now)
        size_t count_same_end_time = 0;
        while(count_same_end_time < 100) {
            if(likely(count_same_end_time == 0)) {
                last2_point = last1_point;
                last1_point = new_point;
            }

            if(unlikely(ops.is_finished(&ops.handle))) {
                if(count_same_end_time != 0) {
                    last2_point = last1_point;
                    last1_point = new_point;
                }
                new_point = QUERY_POINT_EMPTY;
                new_point.start_time = last1_point.end_time;
                new_point.end_time   = now;
                break;
            }

            // fetch the new point
            uint16_t query_point_anomaly_count, query_point_count;
            new_point.value = ops.next_metric(&ops.handle, &new_point.start_time, &new_point.end_time, &new_point.flags, &query_point_count, &query_point_anomaly_count);
            ops.db_points_read_per_tier[ops.tier]++;
            ops.db_total_points_read++;

            // calculate the right value for the fetched point
            if(likely(netdata_double_isnumber(new_point.value))) {
                new_point.anomaly = query_point_count ? query_point_anomaly_count * 100 / query_point_count : 0;

                if(unlikely(use_anomaly_bit_as_value))
                    new_point.value = (NETDATA_DOUBLE)new_point.anomaly;
            }
            else {
                new_point.value   = NAN;
                new_point.anomaly = 0;
                new_point.flags   = SN_EMPTY_SLOT;
            }

            if(unlikely(new_point.start_time == new_point.end_time)) {
                internal_error(true, "QUERY: next_metric(%s, %s) returned point %zu start time %ld, end time %ld, that are both equal",
                               rd->rrdset->name, rd->name, ops.db_total_points_read, new_point.start_time, new_point.end_time);

                new_point.start_time = new_point.end_time - ((time_t)ops.tier_ptr->tier_grouping * (time_t)ops.rd->update_every);
            }

            if(unlikely(new_point.end_time <= last1_point.end_time)) {
                internal_error(true, "QUERY: next_metric(%s, %s) returned point %zu from %ld time %ld, before the last point end time %ld, now is %ld",
                               rd->rrdset->name, rd->name, ops.db_total_points_read, new_point.start_time, new_point.end_time, last1_point.end_time, now);

                count_same_end_time++;
                continue;
            }

            count_same_end_time = 0;

            if(new_point.end_time < now)
                query_add_point_to_group(r, new_point, ops);
            else
                break;
        }

        if(count_same_end_time) {
            internal_error(true,
                           "QUERY: the database does not advance the query, it returned an end time less or equal to %ld, %zu times",
                           last1_point.end_time, count_same_end_time);

            new_point.end_time = now;
        }

        // the inner loop
        // we have 3 points in memory: last2, last1, new
        // we select the one to use based on their timestamps

        size_t iterations = 0;
        for ( ; now <= new_point.end_time && (long)points_added < points_wanted; now += ops.view_update_every, iterations++) {
            QUERY_POINT current_point;

            if(likely(now > new_point.start_time)) {
                // it is time for our NEW point to be used
                current_point = new_point;
                query_interpolate_point(current_point, last1_point, now);
            }
            else if(likely(now <= last1_point.end_time)) {
                // our LAST point is still valid
                current_point = last1_point;
                query_interpolate_point(current_point, last2_point, now);
            }
            else {
                // a GAP, we don't have a value this time
                current_point = QUERY_POINT_EMPTY;
            }

            query_add_point_to_group(r, current_point, ops);

            rrdr_line = rrdr_line_init(r, now, rrdr_line);
            size_t rrdr_o_v_index = rrdr_line * r->d + dim_id_in_rrdr;

            if(unlikely(!min_date)) min_date = now;
            max_date = now;

            // find the place to store our values
            RRDR_VALUE_FLAGS *rrdr_value_options_ptr = &r->o[rrdr_o_v_index];

            // update the dimension options
            if(likely(ops.group_points_non_zero))
                r->od[dim_id_in_rrdr] |= RRDR_DIMENSION_NONZERO;

            // store the specific point options
            *rrdr_value_options_ptr = ops.group_value_flags;

            // store the group value
            NETDATA_DOUBLE group_value = ops.grouping_flush(r, rrdr_value_options_ptr);
            r->v[rrdr_o_v_index] = group_value;

            // we only store uint8_t anomaly rates,
            // so let's get double precision by storing
            // anomaly rates in the range 0 - 200
            ops.group_anomaly_rate = (ops.group_anomaly_rate << 1) / ops.group_points_added;
            r->ar[rrdr_o_v_index] = (uint8_t)ops.group_anomaly_rate;

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
            ops.group_points_added = 0;
            ops.group_value_flags = RRDR_VALUE_NOTHING;
            ops.group_points_non_zero = 0;
            ops.group_anomaly_rate = 0;
        }
        // the loop above increased "now" by query_granularity,
        // but the main loop will increase it too,
        // so, let's undo the last iteration of this loop
        if(iterations)
            now -= ops.view_update_every;
    }
    ops.finalize(&ops.handle);

    r->internal.result_points_generated += points_added;
    r->internal.db_points_read += ops.db_total_points_read;
    for(int tr = 0; tr < storage_tiers ; tr++)
        r->internal.tier_points_read[tr] += ops.db_points_read_per_tier[tr];

    r->min = min;
    r->max = max;
    r->before = max_date;
    r->after = min_date - ops.view_update_every + ops.query_granularity;
    rrdr_done(r, rrdr_line);

    internal_error((long)points_added != points_wanted,
                   "QUERY: query on %s/%s requested %zu points, but RRDR added %zu (%zu db points read).",
                   r->st->name, rd->name, (size_t)points_wanted, (size_t)points_added, ops.db_total_points_read);
}

// ----------------------------------------------------------------------------
// fill the gap of a tier

extern void store_metric_at_tier(RRDDIM *rd, struct rrddim_tier *t, time_t now, usec_t point_end_time_ut, NETDATA_DOUBLE n, SN_FLAGS flags);

void rrdr_fill_tier_gap_from_smaller_tiers(RRDDIM *rd, int tier, time_t now) {
    struct rrddim_tier *t = rd->tiers[tier];
    if(unlikely(!t)) return;

    time_t latest_time_t = t->query_ops.latest_time(t->db_metric_handle);
    time_t granularity = (time_t)t->tier_grouping * (time_t)rd->update_every;
    time_t time_diff   = now - latest_time_t;

    // there is really nothing we can do
    if(now <= latest_time_t || latest_time_t <= 0 || time_diff < granularity) return;

    time_t tier0_last_time = rd->tiers[0]->query_ops.latest_time(rd->tiers[0]->db_metric_handle);
    if(tier0_last_time <= latest_time_t) return;  // it is as bad as we are

    long after_wanted = latest_time_t;
    long before_wanted = tier0_last_time;
    long points_wanted = (before_wanted - after_wanted) / rd->update_every;

    if(points_wanted > 20000) {
        // too many points
        after_wanted = before_wanted - (20000 * rd->update_every);
        points_wanted = (before_wanted - after_wanted) / rd->update_every;
    }

    ONEWAYALLOC *owa = onewayalloc_create(0);
    RRDR *r = rrdr_create_for_x_dimensions(owa, 1, points_wanted);
    r->st = rd->rrdset;

    r->group = 1;
    r->update_every = rd->update_every;
    r->after = after_wanted;
    r->before = before_wanted;
    r->internal.points_wanted = points_wanted;
    r->internal.resampling_group = 1;
    r->internal.resampling_divisor = 1.0;
    r->internal.query_options = RRDR_OPTION_SELECTED_TIER;
    r->internal.query_tier = 0;

    rrdr_set_grouping_function(r, RRDR_GROUPING_AVERAGE);
    r->internal.grouping_create(r, NULL);

    // do the query
    rrd2rrdr_do_dimension(r, points_wanted, rd, 0, after_wanted, before_wanted);

    r->internal.grouping_free(r);
    rrdr_query_completed(r->internal.db_points_read, r->internal.result_points_generated);

    internal_error(true, "DBENGINE: backfilling chart '%s', dimension '%s' with update every %d, tier %d (tier grouping %d, granularity %ld, latest time %ld), from %ld to %ld, with %ld points from tier 0",
                   rd->rrdset->name, rd->name, rd->update_every, tier, t->tier_grouping, granularity, latest_time_t, after_wanted, before_wanted, rrdr_rows(r));

    for(long i = 0; i < rrdr_rows(r) ;i++) {
        if(r->o[i] & RRDR_VALUE_EMPTY) continue;

        time_t timestamp = r->t[i];
        NETDATA_DOUBLE value = r->v[i];
        SN_FLAGS flags = r->ar[i] ? 0 : SN_ANOMALY_BIT;
        store_metric_at_tier(rd, t, timestamp, timestamp * USEC_PER_SEC, value, flags);
    }

    rrdr_free(owa, r);
    onewayalloc_destroy(owa);
}

// ----------------------------------------------------------------------------
// fill RRDR for the whole chart

#ifdef NETDATA_INTERNAL_CHECKS
static void rrd2rrdr_log_request_response_metadata(RRDR *r
        , RRDR_OPTIONS options __maybe_unused
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
         "after (got: %zu, want: %zu, req: %ld, db: %zu), "
         "before (got: %zu, want: %zu, req: %ld, db: %zu), "
         "duration (got: %zu, want: %zu, req: %ld, db: %zu), "
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
         , after_requested
         , (size_t)rrdset_first_entry_t_nolock(r->st)

         // before
         , (size_t)r->before
         , (size_t)before_wanted
         , before_requested
         , (size_t)rrdset_last_entry_t_nolock(r->st)

         // duration
         , (size_t)(r->before - r->after + r->st->update_every)
         , (size_t)(before_wanted - after_wanted + r->st->update_every)
         , before_requested - after_requested
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
int rrdr_relative_window_to_absolute(long long *after, long long *before) {
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
            after_requested = -600;

        // since the query engine now returns inclusive timestamps
        // it is awkward to return 6 points when after=-5 is given
        // so for relative queries we add 1 second, to give
        // more predictable results to users.
        after_requested = before_requested + after_requested + 1;
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
        info("QUERY: chart '%s', after:%lld, before:%lld, duration:%lld, points:%ld, res:%ld - wanted => after:%lld, before:%lld, points:%ld, group:%ld, granularity:%ld, resgroup:%ld, resdiv:" NETDATA_DOUBLE_FORMAT_AUTO " %s", st->name, after_requested, before_requested, before_requested - after_requested, points_requested, resampling_time_requested, after_wanted, before_wanted, points_wanted, group, query_granularity, resampling_group, resampling_divisor, buffer_tostring(debug_log)); \
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
        , int tier
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
    bool before_is_aligned_to_db_end = false;

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

    // if the user wants virtual points, make sure we do it
    if(options & RRDR_OPTION_VIRTUAL_POINTS)
        natural_points = false;

    // set the right flag about natural and virtual points
    if(natural_points) {
        options |= RRDR_OPTION_NATURAL_POINTS;

        if(options & RRDR_OPTION_VIRTUAL_POINTS)
            options &= ~RRDR_OPTION_VIRTUAL_POINTS;
    }
    else {
        options |= RRDR_OPTION_VIRTUAL_POINTS;

        if(options & RRDR_OPTION_NATURAL_POINTS)
            options &= ~RRDR_OPTION_NATURAL_POINTS;
    }

    if(after_wanted == 0 || before_wanted == 0) {
        // for non-context queries we have to find the duration of the database
        // for context queries we will assume 600 seconds duration

        if(!context_param_list) {
            relative_period_requested = true;

            rrdset_rdlock(st);
            time_t first_entry_t = rrdset_first_entry_t_nolock(st);
            time_t last_entry_t = rrdset_last_entry_t_nolock(st);
            rrdset_unlock(st);

            if(first_entry_t == 0 || last_entry_t == 0) {
                internal_error(true, "QUERY: chart without data detected on '%s'", st->name);
                query_debug_log_free();
                return NULL;
            }

            query_debug_log(":first_entry_t %ld, last_entry_t %ld", first_entry_t, last_entry_t);

            if (after_wanted == 0) {
                after_wanted = first_entry_t;
                query_debug_log(":zero after_wanted %lld", after_wanted);
            }

            if (before_wanted == 0) {
                before_wanted = last_entry_t;
                before_is_aligned_to_db_end = true;
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
    rrdr_relative_window_to_absolute(&after_wanted, &before_wanted);
    query_debug_log(":relative2absolute after %lld, before %lld", after_wanted, before_wanted);

    if(natural_points && (options & RRDR_OPTION_SELECTED_TIER) && tier > 0 && storage_tiers > 1) {
        update_every = rrdset_find_natural_update_every_for_timeframe(st, after_wanted, before_wanted, points_wanted, options, tier);
        if(update_every <= 0) update_every = st->update_every;
        query_debug_log(":natural update every %d", update_every);
    }

    // this is the update_every of the query
    // it may be different to the update_every of the database
    time_t query_granularity = (natural_points)?update_every:1;
    if(query_granularity <= 0) query_granularity = 1;
    query_debug_log(":query_granularity %ld", query_granularity);

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

    if(points_wanted * group * query_granularity < duration) {
        // the grouping we are going to do, is not enough
        // to cover the entire duration requested, so
        // we have to change the number of points, to make sure we will
        // respect the timeframe as closely as possibly

        // let's see how many points are the optimal
        points_wanted = points_available / group;

        if(points_wanted * group < points_available)
            points_wanted++;

        query_debug_log(":optimal points %ld", points_wanted);
    }

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
        if(before_is_aligned_to_db_end)
            before_wanted -= before_wanted % (group * query_granularity);
        else
            before_wanted += (group * query_granularity) - before_wanted % (group * query_granularity);
        query_debug_log(":align before_wanted %lld", before_wanted);
    }

    after_wanted  = before_wanted - (points_wanted * group * query_granularity) + query_granularity;
    query_debug_log(":final after_wanted %lld", after_wanted);

    duration = before_wanted - after_wanted;
    query_debug_log(":final duration %ld", duration + 1);

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
    r->internal.query_options = options;
    r->internal.query_tier = tier;

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

    for(rd = first_rd, c = 0 ; rd && c < dimensions_count ; rd = rd->next, c++) {

        // if we need a percentage, we need to calculate all dimensions
        if(unlikely(!(options & RRDR_OPTION_PERCENTAGE) && (r->od[c] & RRDR_DIMENSION_HIDDEN))) {
            if(unlikely(r->od[c] & RRDR_DIMENSION_SELECTED)) r->od[c] &= ~RRDR_DIMENSION_SELECTED;
            continue;
        }
        r->od[c] |= RRDR_DIMENSION_SELECTED;

        // reset the grouping for the new dimension
        r->internal.grouping_reset(r);

        rrd2rrdr_do_dimension(r, points_wanted, rd, c, after_wanted, before_wanted);
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
        if (timeout && ((NETDATA_DOUBLE)dt_usec(&query_start_time, &query_current_time) / 1000.0) > timeout) {
            log_access("QUERY CANCELED RUNTIME EXCEEDED %0.2f ms (LIMIT %d ms)",
                       (NETDATA_DOUBLE)dt_usec(&query_start_time, &query_current_time) / 1000.0, timeout);
            r->result_options |= RRDR_RESULT_OPTION_CANCEL;
            break;
        }
    }

#ifdef NETDATA_INTERNAL_CHECKS
    if (dimensions_used) {
        if(r->internal.log)
            rrd2rrdr_log_request_response_metadata(r, options, group_method, aligned, group, resampling_time_requested, resampling_group,
                                                   after_wanted, after_requested, before_wanted, before_requested,
                                                   points_requested, points_wanted, /*after_slot, before_slot,*/
                                                   r->internal.log);

        if(r->rows != points_wanted)
            rrd2rrdr_log_request_response_metadata(r, options, group_method, aligned, group, resampling_time_requested, resampling_group,
                                                   after_wanted, after_requested, before_wanted, before_requested,
                                                   points_requested, points_wanted, /*after_slot, before_slot,*/
                                                   "got 'points' is not wanted 'points'");

        if(aligned && (r->before % (group * query_granularity)) != 0)
            rrd2rrdr_log_request_response_metadata(r, options, group_method, aligned, group, resampling_time_requested, resampling_group,
                                                   after_wanted, after_requested, before_wanted,before_wanted,
                                                   points_requested, points_wanted, /*after_slot, before_slot,*/
                                                   "'before' is not aligned but alignment is required");

        // 'after' should not be aligned, since we start inside the first group
        //if(aligned && (r->after % group) != 0)
        //    rrd2rrdr_log_request_response_metadata(r, options, group_method, aligned, group, resampling_time_requested, resampling_group, after_wanted, after_requested, before_wanted, before_requested, points_requested, points_wanted, after_slot, before_slot, "'after' is not aligned but alignment is required");

        if(r->before != before_wanted)
            rrd2rrdr_log_request_response_metadata(r, options, group_method, aligned, group, resampling_time_requested, resampling_group,
                                                   after_wanted, after_requested, before_wanted, before_requested,
                                                   points_requested, points_wanted, /*after_slot, before_slot,*/
                                                   "chart is not aligned to requested 'before'");

        if(r->before != before_wanted)
            rrd2rrdr_log_request_response_metadata(r, options, group_method, aligned, group, resampling_time_requested, resampling_group,
                                                   after_wanted, after_requested, before_wanted, before_requested,
                                                   points_requested, points_wanted, /*after_slot, before_slot,*/
                                                   "got 'before' is not wanted 'before'");

        // reported 'after' varies, depending on group
        if(r->after != after_wanted)
            rrd2rrdr_log_request_response_metadata(r, options, group_method, aligned, group, resampling_time_requested, resampling_group,
                                                   after_wanted, after_requested, before_wanted, before_requested,
                                                   points_requested, points_wanted, /*after_slot, before_slot,*/
                                                   "got 'after' is not wanted 'after'");

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
