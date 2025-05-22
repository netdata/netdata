// SPDX-License-Identifier: GPL-3.0-or-later

#include "query-internal.h"

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
#include "percentile/percentile.h"
#include "trimmed_mean/trimmed_mean.h"
#include "extremes/extremes.h"

// ----------------------------------------------------------------------------

static struct {
    const char *name;
    uint32_t hash;
    RRDR_TIME_GROUPING value;
    RRDR_TIME_GROUPING add_flush;

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
     .add_flush = RRDR_GROUPING_AVERAGE,
     .init  = NULL,
     .create= tg_average_create,
     .reset = tg_average_reset,
     .free  = tg_average_free,
     .add   = tg_average_add,
     .flush = tg_average_flush,
     .tier_query_fetch = TIER_QUERY_FETCH_AVERAGE
    },
    {.name = "avg",                             // alias on 'average'
     .hash  = 0,
     .value = RRDR_GROUPING_AVERAGE,
     .add_flush = RRDR_GROUPING_AVERAGE,
     .init  = NULL,
     .create= tg_average_create,
     .reset = tg_average_reset,
     .free  = tg_average_free,
     .add   = tg_average_add,
     .flush = tg_average_flush,
     .tier_query_fetch = TIER_QUERY_FETCH_AVERAGE
    },
    {.name = "mean",                            // alias on 'average'
     .hash  = 0,
     .value = RRDR_GROUPING_AVERAGE,
     .add_flush = RRDR_GROUPING_AVERAGE,
     .init  = NULL,
     .create= tg_average_create,
     .reset = tg_average_reset,
     .free  = tg_average_free,
     .add   = tg_average_add,
     .flush = tg_average_flush,
     .tier_query_fetch = TIER_QUERY_FETCH_AVERAGE
    },
    {.name = "trimmed-mean1",
     .hash  = 0,
     .value = RRDR_GROUPING_TRIMMED_MEAN1,
     .add_flush = RRDR_GROUPING_TRIMMED_MEAN,
     .init  = NULL,
     .create= tg_trimmed_mean_create_1,
     .reset = tg_trimmed_mean_reset,
     .free  = tg_trimmed_mean_free,
     .add   = tg_trimmed_mean_add,
     .flush = tg_trimmed_mean_flush,
     .tier_query_fetch = TIER_QUERY_FETCH_AVERAGE
    },
    {.name = "trimmed-mean2",
     .hash  = 0,
     .value = RRDR_GROUPING_TRIMMED_MEAN2,
     .add_flush = RRDR_GROUPING_TRIMMED_MEAN,
     .init  = NULL,
     .create= tg_trimmed_mean_create_2,
     .reset = tg_trimmed_mean_reset,
     .free  = tg_trimmed_mean_free,
     .add   = tg_trimmed_mean_add,
     .flush = tg_trimmed_mean_flush,
     .tier_query_fetch = TIER_QUERY_FETCH_AVERAGE
    },
    {.name = "trimmed-mean3",
     .hash  = 0,
     .value = RRDR_GROUPING_TRIMMED_MEAN3,
     .add_flush = RRDR_GROUPING_TRIMMED_MEAN,
     .init  = NULL,
     .create= tg_trimmed_mean_create_3,
     .reset = tg_trimmed_mean_reset,
     .free  = tg_trimmed_mean_free,
     .add   = tg_trimmed_mean_add,
     .flush = tg_trimmed_mean_flush,
     .tier_query_fetch = TIER_QUERY_FETCH_AVERAGE
    },
    {.name = "trimmed-mean5",
     .hash  = 0,
     .value = RRDR_GROUPING_TRIMMED_MEAN,
     .add_flush = RRDR_GROUPING_TRIMMED_MEAN,
     .init  = NULL,
     .create= tg_trimmed_mean_create_5,
     .reset = tg_trimmed_mean_reset,
     .free  = tg_trimmed_mean_free,
     .add   = tg_trimmed_mean_add,
     .flush = tg_trimmed_mean_flush,
     .tier_query_fetch = TIER_QUERY_FETCH_AVERAGE
    },
    {.name = "trimmed-mean10",
     .hash  = 0,
     .value = RRDR_GROUPING_TRIMMED_MEAN10,
     .add_flush = RRDR_GROUPING_TRIMMED_MEAN,
     .init  = NULL,
     .create= tg_trimmed_mean_create_10,
     .reset = tg_trimmed_mean_reset,
     .free  = tg_trimmed_mean_free,
     .add   = tg_trimmed_mean_add,
     .flush = tg_trimmed_mean_flush,
     .tier_query_fetch = TIER_QUERY_FETCH_AVERAGE
    },
    {.name = "trimmed-mean15",
     .hash  = 0,
     .value = RRDR_GROUPING_TRIMMED_MEAN15,
     .add_flush = RRDR_GROUPING_TRIMMED_MEAN,
     .init  = NULL,
     .create= tg_trimmed_mean_create_15,
     .reset = tg_trimmed_mean_reset,
     .free  = tg_trimmed_mean_free,
     .add   = tg_trimmed_mean_add,
     .flush = tg_trimmed_mean_flush,
     .tier_query_fetch = TIER_QUERY_FETCH_AVERAGE
    },
    {.name = "trimmed-mean20",
     .hash  = 0,
     .value = RRDR_GROUPING_TRIMMED_MEAN20,
     .add_flush = RRDR_GROUPING_TRIMMED_MEAN,
     .init  = NULL,
     .create= tg_trimmed_mean_create_20,
     .reset = tg_trimmed_mean_reset,
     .free  = tg_trimmed_mean_free,
     .add   = tg_trimmed_mean_add,
     .flush = tg_trimmed_mean_flush,
     .tier_query_fetch = TIER_QUERY_FETCH_AVERAGE
    },
    {.name = "trimmed-mean25",
     .hash  = 0,
     .value = RRDR_GROUPING_TRIMMED_MEAN25,
     .add_flush = RRDR_GROUPING_TRIMMED_MEAN,
     .init  = NULL,
     .create= tg_trimmed_mean_create_25,
     .reset = tg_trimmed_mean_reset,
     .free  = tg_trimmed_mean_free,
     .add   = tg_trimmed_mean_add,
     .flush = tg_trimmed_mean_flush,
     .tier_query_fetch = TIER_QUERY_FETCH_AVERAGE
    },
    {.name = "trimmed-mean",
     .hash  = 0,
     .value = RRDR_GROUPING_TRIMMED_MEAN,
     .add_flush = RRDR_GROUPING_TRIMMED_MEAN,
     .init  = NULL,
     .create= tg_trimmed_mean_create_5,
     .reset = tg_trimmed_mean_reset,
     .free  = tg_trimmed_mean_free,
     .add   = tg_trimmed_mean_add,
     .flush = tg_trimmed_mean_flush,
     .tier_query_fetch = TIER_QUERY_FETCH_AVERAGE
    },
    {.name  = "incremental_sum",
     .hash  = 0,
     .value = RRDR_GROUPING_INCREMENTAL_SUM,
     .add_flush = RRDR_GROUPING_INCREMENTAL_SUM,
     .init  = NULL,
     .create= tg_incremental_sum_create,
     .reset = tg_incremental_sum_reset,
     .free  = tg_incremental_sum_free,
     .add   = tg_incremental_sum_add,
     .flush = tg_incremental_sum_flush,
     .tier_query_fetch = TIER_QUERY_FETCH_AVERAGE
    },
    {.name = "incremental-sum",
     .hash  = 0,
     .value = RRDR_GROUPING_INCREMENTAL_SUM,
     .add_flush = RRDR_GROUPING_INCREMENTAL_SUM,
     .init  = NULL,
     .create= tg_incremental_sum_create,
     .reset = tg_incremental_sum_reset,
     .free  = tg_incremental_sum_free,
     .add   = tg_incremental_sum_add,
     .flush = tg_incremental_sum_flush,
     .tier_query_fetch = TIER_QUERY_FETCH_AVERAGE
    },
    {.name = "median",
     .hash  = 0,
     .value = RRDR_GROUPING_MEDIAN,
     .add_flush = RRDR_GROUPING_MEDIAN,
     .init  = NULL,
     .create= tg_median_create,
     .reset = tg_median_reset,
     .free  = tg_median_free,
     .add   = tg_median_add,
     .flush = tg_median_flush,
     .tier_query_fetch = TIER_QUERY_FETCH_AVERAGE
    },
    {.name = "trimmed-median1",
     .hash  = 0,
     .value = RRDR_GROUPING_TRIMMED_MEDIAN1,
     .add_flush = RRDR_GROUPING_MEDIAN,
     .init  = NULL,
     .create= tg_median_create_trimmed_1,
     .reset = tg_median_reset,
     .free  = tg_median_free,
     .add   = tg_median_add,
     .flush = tg_median_flush,
     .tier_query_fetch = TIER_QUERY_FETCH_AVERAGE
    },
    {.name = "trimmed-median2",
     .hash  = 0,
     .value = RRDR_GROUPING_TRIMMED_MEDIAN2,
     .add_flush = RRDR_GROUPING_MEDIAN,
     .init  = NULL,
     .create= tg_median_create_trimmed_2,
     .reset = tg_median_reset,
     .free  = tg_median_free,
     .add   = tg_median_add,
     .flush = tg_median_flush,
     .tier_query_fetch = TIER_QUERY_FETCH_AVERAGE
    },
    {.name = "trimmed-median3",
     .hash  = 0,
     .value = RRDR_GROUPING_TRIMMED_MEDIAN3,
     .add_flush = RRDR_GROUPING_MEDIAN,
     .init  = NULL,
     .create= tg_median_create_trimmed_3,
     .reset = tg_median_reset,
     .free  = tg_median_free,
     .add   = tg_median_add,
     .flush = tg_median_flush,
     .tier_query_fetch = TIER_QUERY_FETCH_AVERAGE
    },
    {.name = "trimmed-median5",
     .hash  = 0,
     .value = RRDR_GROUPING_TRIMMED_MEDIAN,
     .add_flush = RRDR_GROUPING_MEDIAN,
     .init  = NULL,
     .create= tg_median_create_trimmed_5,
     .reset = tg_median_reset,
     .free  = tg_median_free,
     .add   = tg_median_add,
     .flush = tg_median_flush,
     .tier_query_fetch = TIER_QUERY_FETCH_AVERAGE
    },
    {.name = "trimmed-median10",
     .hash  = 0,
     .value = RRDR_GROUPING_TRIMMED_MEDIAN10,
     .add_flush = RRDR_GROUPING_MEDIAN,
     .init  = NULL,
     .create= tg_median_create_trimmed_10,
     .reset = tg_median_reset,
     .free  = tg_median_free,
     .add   = tg_median_add,
     .flush = tg_median_flush,
     .tier_query_fetch = TIER_QUERY_FETCH_AVERAGE
    },
    {.name = "trimmed-median15",
     .hash  = 0,
     .value = RRDR_GROUPING_TRIMMED_MEDIAN15,
     .add_flush = RRDR_GROUPING_MEDIAN,
     .init  = NULL,
     .create= tg_median_create_trimmed_15,
     .reset = tg_median_reset,
     .free  = tg_median_free,
     .add   = tg_median_add,
     .flush = tg_median_flush,
     .tier_query_fetch = TIER_QUERY_FETCH_AVERAGE
    },
    {.name = "trimmed-median20",
     .hash  = 0,
     .value = RRDR_GROUPING_TRIMMED_MEDIAN20,
     .add_flush = RRDR_GROUPING_MEDIAN,
     .init  = NULL,
     .create= tg_median_create_trimmed_20,
     .reset = tg_median_reset,
     .free  = tg_median_free,
     .add   = tg_median_add,
     .flush = tg_median_flush,
     .tier_query_fetch = TIER_QUERY_FETCH_AVERAGE
    },
    {.name = "trimmed-median25",
     .hash  = 0,
     .value = RRDR_GROUPING_TRIMMED_MEDIAN25,
     .add_flush = RRDR_GROUPING_MEDIAN,
     .init  = NULL,
     .create= tg_median_create_trimmed_25,
     .reset = tg_median_reset,
     .free  = tg_median_free,
     .add   = tg_median_add,
     .flush = tg_median_flush,
     .tier_query_fetch = TIER_QUERY_FETCH_AVERAGE
    },
    {.name = "trimmed-median",
     .hash  = 0,
     .value = RRDR_GROUPING_TRIMMED_MEDIAN,
     .add_flush = RRDR_GROUPING_MEDIAN,
     .init  = NULL,
     .create= tg_median_create_trimmed_5,
     .reset = tg_median_reset,
     .free  = tg_median_free,
     .add   = tg_median_add,
     .flush = tg_median_flush,
     .tier_query_fetch = TIER_QUERY_FETCH_AVERAGE
    },
    {.name = "percentile25",
     .hash  = 0,
     .value = RRDR_GROUPING_PERCENTILE25,
     .add_flush = RRDR_GROUPING_PERCENTILE,
     .init  = NULL,
     .create= tg_percentile_create_25,
     .reset = tg_percentile_reset,
     .free  = tg_percentile_free,
     .add   = tg_percentile_add,
     .flush = tg_percentile_flush,
     .tier_query_fetch = TIER_QUERY_FETCH_AVERAGE
    },
    {.name = "percentile50",
     .hash  = 0,
     .value = RRDR_GROUPING_PERCENTILE50,
     .add_flush = RRDR_GROUPING_PERCENTILE,
     .init  = NULL,
     .create= tg_percentile_create_50,
     .reset = tg_percentile_reset,
     .free  = tg_percentile_free,
     .add   = tg_percentile_add,
     .flush = tg_percentile_flush,
     .tier_query_fetch = TIER_QUERY_FETCH_AVERAGE
    },
    {.name = "percentile75",
     .hash  = 0,
     .value = RRDR_GROUPING_PERCENTILE75,
     .add_flush = RRDR_GROUPING_PERCENTILE,
     .init  = NULL,
     .create= tg_percentile_create_75,
     .reset = tg_percentile_reset,
     .free  = tg_percentile_free,
     .add   = tg_percentile_add,
     .flush = tg_percentile_flush,
     .tier_query_fetch = TIER_QUERY_FETCH_AVERAGE
    },
    {.name = "percentile80",
     .hash  = 0,
     .value = RRDR_GROUPING_PERCENTILE80,
     .add_flush = RRDR_GROUPING_PERCENTILE,
     .init  = NULL,
     .create= tg_percentile_create_80,
     .reset = tg_percentile_reset,
     .free  = tg_percentile_free,
     .add   = tg_percentile_add,
     .flush = tg_percentile_flush,
     .tier_query_fetch = TIER_QUERY_FETCH_AVERAGE
    },
    {.name = "percentile90",
     .hash  = 0,
     .value = RRDR_GROUPING_PERCENTILE90,
     .add_flush = RRDR_GROUPING_PERCENTILE,
     .init  = NULL,
     .create= tg_percentile_create_90,
     .reset = tg_percentile_reset,
     .free  = tg_percentile_free,
     .add   = tg_percentile_add,
     .flush = tg_percentile_flush,
     .tier_query_fetch = TIER_QUERY_FETCH_AVERAGE
    },
    {.name = "percentile95",
     .hash  = 0,
     .value = RRDR_GROUPING_PERCENTILE,
     .add_flush = RRDR_GROUPING_PERCENTILE,
     .init  = NULL,
     .create= tg_percentile_create_95,
     .reset = tg_percentile_reset,
     .free  = tg_percentile_free,
     .add   = tg_percentile_add,
     .flush = tg_percentile_flush,
     .tier_query_fetch = TIER_QUERY_FETCH_AVERAGE
    },
    {.name = "percentile97",
     .hash  = 0,
     .value = RRDR_GROUPING_PERCENTILE97,
     .add_flush = RRDR_GROUPING_PERCENTILE,
     .init  = NULL,
     .create= tg_percentile_create_97,
     .reset = tg_percentile_reset,
     .free  = tg_percentile_free,
     .add   = tg_percentile_add,
     .flush = tg_percentile_flush,
     .tier_query_fetch = TIER_QUERY_FETCH_AVERAGE
    },
    {.name = "percentile98",
     .hash  = 0,
     .value = RRDR_GROUPING_PERCENTILE98,
     .add_flush = RRDR_GROUPING_PERCENTILE,
     .init  = NULL,
     .create= tg_percentile_create_98,
     .reset = tg_percentile_reset,
     .free  = tg_percentile_free,
     .add   = tg_percentile_add,
     .flush = tg_percentile_flush,
     .tier_query_fetch = TIER_QUERY_FETCH_AVERAGE
    },
    {.name = "percentile99",
     .hash  = 0,
     .value = RRDR_GROUPING_PERCENTILE99,
     .add_flush = RRDR_GROUPING_PERCENTILE,
     .init  = NULL,
     .create= tg_percentile_create_99,
     .reset = tg_percentile_reset,
     .free  = tg_percentile_free,
     .add   = tg_percentile_add,
     .flush = tg_percentile_flush,
     .tier_query_fetch = TIER_QUERY_FETCH_AVERAGE
    },
    {.name = "percentile",
     .hash  = 0,
     .value = RRDR_GROUPING_PERCENTILE,
     .add_flush = RRDR_GROUPING_PERCENTILE,
     .init  = NULL,
     .create= tg_percentile_create_95,
     .reset = tg_percentile_reset,
     .free  = tg_percentile_free,
     .add   = tg_percentile_add,
     .flush = tg_percentile_flush,
     .tier_query_fetch = TIER_QUERY_FETCH_AVERAGE
    },
    {.name = "min",
     .hash  = 0,
     .value = RRDR_GROUPING_MIN,
     .add_flush = RRDR_GROUPING_MIN,
     .init  = NULL,
     .create= tg_min_create,
     .reset = tg_min_reset,
     .free  = tg_min_free,
     .add   = tg_min_add,
     .flush = tg_min_flush,
     .tier_query_fetch = TIER_QUERY_FETCH_MIN
    },
    {.name = "max",
     .hash  = 0,
     .value = RRDR_GROUPING_MAX,
     .add_flush = RRDR_GROUPING_MAX,
     .init  = NULL,
     .create= tg_max_create,
     .reset = tg_max_reset,
     .free  = tg_max_free,
     .add   = tg_max_add,
     .flush = tg_max_flush,
     .tier_query_fetch = TIER_QUERY_FETCH_MAX
    },
    {.name = "sum",
     .hash  = 0,
     .value = RRDR_GROUPING_SUM,
     .add_flush = RRDR_GROUPING_SUM,
     .init  = NULL,
     .create= tg_sum_create,
     .reset = tg_sum_reset,
     .free  = tg_sum_free,
     .add   = tg_sum_add,
     .flush = tg_sum_flush,
     .tier_query_fetch = TIER_QUERY_FETCH_SUM
    },

    // standard deviation
    {.name = "stddev",
     .hash  = 0,
     .value = RRDR_GROUPING_STDDEV,
     .add_flush = RRDR_GROUPING_STDDEV,
     .init  = NULL,
     .create= tg_stddev_create,
     .reset = tg_stddev_reset,
     .free  = tg_stddev_free,
     .add   = tg_stddev_add,
     .flush = tg_stddev_flush,
     .tier_query_fetch = TIER_QUERY_FETCH_AVERAGE
    },
    {.name = "cv",                           // coefficient variation is calculated by stddev
     .hash  = 0,
     .value = RRDR_GROUPING_CV,
     .add_flush = RRDR_GROUPING_CV,
     .init  = NULL,
     .create= tg_stddev_create, // not an error, stddev calculates this too
     .reset = tg_stddev_reset,  // not an error, stddev calculates this too
     .free  = tg_stddev_free,   // not an error, stddev calculates this too
     .add   = tg_stddev_add,    // not an error, stddev calculates this too
     .flush = tg_stddev_coefficient_of_variation_flush,
     .tier_query_fetch = TIER_QUERY_FETCH_AVERAGE
    },
    {.name = "rsd",                          // alias of 'cv'
     .hash  = 0,
     .value = RRDR_GROUPING_CV,
     .add_flush = RRDR_GROUPING_CV,
     .init  = NULL,
     .create= tg_stddev_create, // not an error, stddev calculates this too
     .reset = tg_stddev_reset,  // not an error, stddev calculates this too
     .free  = tg_stddev_free,   // not an error, stddev calculates this too
     .add   = tg_stddev_add,    // not an error, stddev calculates this too
     .flush = tg_stddev_coefficient_of_variation_flush,
     .tier_query_fetch = TIER_QUERY_FETCH_AVERAGE
    },
    {.name = "coefficient-of-variation", // alias of 'cv'
     .hash  = 0,
     .value = RRDR_GROUPING_CV,
     .add_flush = RRDR_GROUPING_CV,
     .init  = NULL,
     .create= tg_stddev_create, // not an error, stddev calculates this too
     .reset = tg_stddev_reset,  // not an error, stddev calculates this too
     .free  = tg_stddev_free,   // not an error, stddev calculates this too
     .add   = tg_stddev_add,    // not an error, stddev calculates this too
     .flush = tg_stddev_coefficient_of_variation_flush,
     .tier_query_fetch = TIER_QUERY_FETCH_AVERAGE
    },

    // single exponential smoothing
    {.name = "ses",
     .hash  = 0,
     .value = RRDR_GROUPING_SES,
     .add_flush = RRDR_GROUPING_SES,
     .init  = tg_ses_init,
     .create= tg_ses_create,
     .reset = tg_ses_reset,
     .free  = tg_ses_free,
     .add   = tg_ses_add,
     .flush = tg_ses_flush,
     .tier_query_fetch = TIER_QUERY_FETCH_AVERAGE
    },
    {.name = "ema",                         // alias for 'ses'
     .hash  = 0,
     .value = RRDR_GROUPING_SES,
     .add_flush = RRDR_GROUPING_SES,
     .init  = NULL,
     .create= tg_ses_create,
     .reset = tg_ses_reset,
     .free  = tg_ses_free,
     .add   = tg_ses_add,
     .flush = tg_ses_flush,
     .tier_query_fetch = TIER_QUERY_FETCH_AVERAGE
    },
    {.name = "ewma",                        // alias for ses
     .hash  = 0,
     .value = RRDR_GROUPING_SES,
     .add_flush = RRDR_GROUPING_SES,
     .init  = NULL,
     .create= tg_ses_create,
     .reset = tg_ses_reset,
     .free  = tg_ses_free,
     .add   = tg_ses_add,
     .flush = tg_ses_flush,
     .tier_query_fetch = TIER_QUERY_FETCH_AVERAGE
    },

    // double exponential smoothing
    {.name = "des",
     .hash  = 0,
     .value = RRDR_GROUPING_DES,
     .add_flush = RRDR_GROUPING_DES,
     .init  = tg_des_init,
     .create= tg_des_create,
     .reset = tg_des_reset,
     .free  = tg_des_free,
     .add   = tg_des_add,
     .flush = tg_des_flush,
     .tier_query_fetch = TIER_QUERY_FETCH_AVERAGE
    },

    {.name = "countif",
     .hash  = 0,
     .value = RRDR_GROUPING_COUNTIF,
     .add_flush = RRDR_GROUPING_COUNTIF,
     .init = NULL,
     .create= tg_countif_create,
     .reset = tg_countif_reset,
     .free  = tg_countif_free,
     .add   = tg_countif_add,
     .flush = tg_countif_flush,
     .tier_query_fetch = TIER_QUERY_FETCH_AVERAGE
    },

    {.name = "extremes",
     .hash  = 0,
     .value = RRDR_GROUPING_EXTREMES,
     .add_flush = RRDR_GROUPING_EXTREMES,
     .init  = NULL,
     .create= tg_extremes_create,
     .reset = tg_extremes_reset,
     .free  = tg_extremes_free,
     .add   = tg_extremes_add,
     .flush = tg_extremes_flush,
     .tier_query_fetch = TIER_QUERY_FETCH_AVERAGE
    },

    // terminator
    {.name = NULL,
     .hash  = 0,
     .value = RRDR_GROUPING_UNDEFINED,
     .add_flush = RRDR_GROUPING_AVERAGE,
     .init = NULL,
     .create= tg_average_create,
     .reset = tg_average_reset,
     .free  = tg_average_free,
     .add   = tg_average_add,
     .flush = tg_average_flush,
     .tier_query_fetch = TIER_QUERY_FETCH_AVERAGE
    }
};

void time_grouping_init(void) {
    int i;

    for(i = 0; api_v1_data_groups[i].name ; i++) {
        api_v1_data_groups[i].hash = simple_hash(api_v1_data_groups[i].name);

        if(api_v1_data_groups[i].init)
            api_v1_data_groups[i].init();
    }
}

const char *time_grouping_id2txt(RRDR_TIME_GROUPING group) {
    int i;

    for(i = 0; api_v1_data_groups[i].name ; i++) {
        if(api_v1_data_groups[i].value == group) {
            return api_v1_data_groups[i].name;
        }
    }

    return "average";
}

RRDR_TIME_GROUPING time_grouping_txt2id(const char *name) {
    int i;

    uint32_t hash = simple_hash(name);
    for(i = 0; api_v1_data_groups[i].name ; i++)
        if(unlikely(hash == api_v1_data_groups[i].hash && !strcmp(name, api_v1_data_groups[i].name)))
            return api_v1_data_groups[i].value;

    return RRDR_GROUPING_AVERAGE;
}

RRDR_TIME_GROUPING time_grouping_parse(const char *name, RRDR_TIME_GROUPING def) {
    int i;

    uint32_t hash = simple_hash(name);
    for(i = 0; api_v1_data_groups[i].name ; i++)
        if(unlikely(hash == api_v1_data_groups[i].hash && !strcmp(name, api_v1_data_groups[i].name)))
            return api_v1_data_groups[i].value;

    return def;
}

const char *time_grouping_tostring(RRDR_TIME_GROUPING group) {
    int i;

    for(i = 0; api_v1_data_groups[i].name ; i++)
        if(unlikely(group == api_v1_data_groups[i].value))
            return api_v1_data_groups[i].name;

    return "unknown";
}

void rrdr_set_grouping_function(RRDR *r, RRDR_TIME_GROUPING group_method) {
    int i, found = 0;
    for(i = 0; !found && api_v1_data_groups[i].name ;i++) {
        if(api_v1_data_groups[i].value == group_method) {
            r->time_grouping.create  = api_v1_data_groups[i].create;
            r->time_grouping.reset   = api_v1_data_groups[i].reset;
            r->time_grouping.free    = api_v1_data_groups[i].free;
            r->time_grouping.add     = api_v1_data_groups[i].add;
            r->time_grouping.flush   = api_v1_data_groups[i].flush;
            r->time_grouping.tier_query_fetch = api_v1_data_groups[i].tier_query_fetch;
            r->time_grouping.add_flush = api_v1_data_groups[i].add_flush;
            found = 1;
        }
    }
    if(!found) {
        errno_clear();
        internal_error(true, "QUERY: grouping method %u not found. Using 'average'", (unsigned int)group_method);
        r->time_grouping.create  = tg_average_create;
        r->time_grouping.reset   = tg_average_reset;
        r->time_grouping.free    = tg_average_free;
        r->time_grouping.add     = tg_average_add;
        r->time_grouping.flush   = tg_average_flush;
        r->time_grouping.tier_query_fetch = TIER_QUERY_FETCH_AVERAGE;
        r->time_grouping.add_flush = RRDR_GROUPING_AVERAGE;
    }
}

ALWAYS_INLINE_HOT_FLATTEN
void time_grouping_add(RRDR *r, NETDATA_DOUBLE value, const RRDR_TIME_GROUPING add_flush) {
    switch(add_flush) {
        case RRDR_GROUPING_AVERAGE:
            tg_average_add(r, value);
            break;

        case RRDR_GROUPING_MAX:
            tg_max_add(r, value);
            break;

        case RRDR_GROUPING_MIN:
            tg_min_add(r, value);
            break;

        case RRDR_GROUPING_MEDIAN:
            tg_median_add(r, value);
            break;

        case RRDR_GROUPING_STDDEV:
        case RRDR_GROUPING_CV:
            tg_stddev_add(r, value);
            break;

        case RRDR_GROUPING_SUM:
            tg_sum_add(r, value);
            break;

        case RRDR_GROUPING_COUNTIF:
            tg_countif_add(r, value);
            break;

        case RRDR_GROUPING_EXTREMES:
            tg_extremes_add(r, value);
            break;

        case RRDR_GROUPING_TRIMMED_MEAN:
            tg_trimmed_mean_add(r, value);
            break;

        case RRDR_GROUPING_PERCENTILE:
            tg_percentile_add(r, value);
            break;

        case RRDR_GROUPING_SES:
            tg_ses_add(r, value);
            break;

        case RRDR_GROUPING_DES:
            tg_des_add(r, value);
            break;

        case RRDR_GROUPING_INCREMENTAL_SUM:
            tg_incremental_sum_add(r, value);
            break;

        default:
            r->time_grouping.add(r, value);
            break;
    }
}

ALWAYS_INLINE_HOT_FLATTEN
NETDATA_DOUBLE time_grouping_flush(RRDR *r, RRDR_VALUE_FLAGS *rrdr_value_options_ptr, const RRDR_TIME_GROUPING add_flush) {
    switch(add_flush) {
        case RRDR_GROUPING_AVERAGE:
            return tg_average_flush(r, rrdr_value_options_ptr);

        case RRDR_GROUPING_MAX:
            return tg_max_flush(r, rrdr_value_options_ptr);

        case RRDR_GROUPING_MIN:
            return tg_min_flush(r, rrdr_value_options_ptr);

        case RRDR_GROUPING_MEDIAN:
            return tg_median_flush(r, rrdr_value_options_ptr);

        case RRDR_GROUPING_STDDEV:
            return tg_stddev_flush(r, rrdr_value_options_ptr);

        case RRDR_GROUPING_CV:
            return tg_stddev_coefficient_of_variation_flush(r, rrdr_value_options_ptr);

        case RRDR_GROUPING_SUM:
            return tg_sum_flush(r, rrdr_value_options_ptr);

        case RRDR_GROUPING_COUNTIF:
            return tg_countif_flush(r, rrdr_value_options_ptr);

        case RRDR_GROUPING_EXTREMES:
            return tg_extremes_flush(r, rrdr_value_options_ptr);

        case RRDR_GROUPING_TRIMMED_MEAN:
            return tg_trimmed_mean_flush(r, rrdr_value_options_ptr);

        case RRDR_GROUPING_PERCENTILE:
            return tg_percentile_flush(r, rrdr_value_options_ptr);

        case RRDR_GROUPING_SES:
            return tg_ses_flush(r, rrdr_value_options_ptr);

        case RRDR_GROUPING_DES:
            return tg_des_flush(r, rrdr_value_options_ptr);

        case RRDR_GROUPING_INCREMENTAL_SUM:
            return tg_incremental_sum_flush(r, rrdr_value_options_ptr);

        default:
            return r->time_grouping.flush(r, rrdr_value_options_ptr);
    }
}
