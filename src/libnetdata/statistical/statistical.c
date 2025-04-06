// SPDX-License-Identifier: GPL-3.0-or-later

#include "../libnetdata.h"

NETDATA_DOUBLE default_single_exponential_smoothing_alpha = 0.1;

void log_series_to_stderr(NETDATA_DOUBLE *series, size_t entries, NETDATA_DOUBLE result, const char *msg) {
    const NETDATA_DOUBLE *value, *end = &series[entries];

    fprintf(stderr, "%s of %zu entries [ ", msg, entries);
    for(value = series; value < end ;value++) {
        if(value != series) fprintf(stderr, ", ");
        fprintf(stderr, "%" NETDATA_DOUBLE_MODIFIER, *value);
    }
    fprintf(stderr, " ] results in " NETDATA_DOUBLE_FORMAT "\n", result);
}

// --------------------------------------------------------------------------------------------------------------------

inline NETDATA_DOUBLE sum_and_count(const NETDATA_DOUBLE *series, size_t entries, size_t *count) {
    const NETDATA_DOUBLE *value, *end = &series[entries];
    NETDATA_DOUBLE sum = 0;
    size_t c = 0;

    for(value = series; value < end ; value++) {
        if(netdata_double_isnumber(*value)) {
            sum += *value;
            c++;
        }
    }

    if(unlikely(!c)) sum = NAN;
    if(likely(count)) *count = c;

    return sum;
}

inline NETDATA_DOUBLE sum(const NETDATA_DOUBLE *series, size_t entries) {
    return sum_and_count(series, entries, NULL);
}

inline NETDATA_DOUBLE average(const NETDATA_DOUBLE *series, size_t entries) {
    size_t count = 0;
    NETDATA_DOUBLE sum = sum_and_count(series, entries, &count);

    if(unlikely(!count)) return NAN;
    return sum / (NETDATA_DOUBLE)count;
}

// --------------------------------------------------------------------------------------------------------------------

NETDATA_DOUBLE moving_average(const NETDATA_DOUBLE *series, size_t entries, size_t period) {
    if(unlikely(period <= 0))
        return 0.0;

    size_t i, count;
    NETDATA_DOUBLE sum = 0, avg = 0;
    NETDATA_DOUBLE p[period];

    for(count = 0; count < period ; count++)
        p[count] = 0.0;

    for(i = 0, count = 0; i < entries; i++) {
        NETDATA_DOUBLE value = series[i];
        if(unlikely(!netdata_double_isnumber(value))) continue;

        if(unlikely(count < period)) {
            sum += value;
            avg = (count == period - 1) ? sum / (NETDATA_DOUBLE)period : 0;
        }
        else {
            sum = sum - p[count % period] + value;
            avg = sum / (NETDATA_DOUBLE)period;
        }

        p[count % period] = value;
        count++;
    }

    return avg;
}

// --------------------------------------------------------------------------------------------------------------------

static int qsort_compare(const void *a, const void *b) {
    NETDATA_DOUBLE *p1 = (NETDATA_DOUBLE *)a, *p2 = (NETDATA_DOUBLE *)b;
    NETDATA_DOUBLE n1 = *p1, n2 = *p2;

    if(unlikely(isnan(n1) || isnan(n2))) {
        if(isnan(n1) && !isnan(n2)) return -1;
        if(!isnan(n1) && isnan(n2)) return 1;
        return 0;
    }
    if(unlikely(isinf(n1) || isinf(n2))) {
        if(!isinf(n1) && isinf(n2)) return -1;
        if(isinf(n1) && !isinf(n2)) return 1;
        return 0;
    }

    if(unlikely(n1 < n2)) return -1;
    if(unlikely(n1 > n2)) return 1;
    return 0;
}

inline void sort_series(NETDATA_DOUBLE *series, size_t entries) {
    qsort(series, entries, sizeof(NETDATA_DOUBLE), qsort_compare);
}

inline NETDATA_DOUBLE *copy_series(const NETDATA_DOUBLE *series, size_t entries) {
    NETDATA_DOUBLE *copy = mallocz(sizeof(NETDATA_DOUBLE) * entries);
    memcpy(copy, series, sizeof(NETDATA_DOUBLE) * entries);
    return copy;
}

NETDATA_DOUBLE percentile_on_sorted_series(const NETDATA_DOUBLE *series, size_t entries, double percentile) {
    if (unlikely(entries == 0)) return NAN;
    if (unlikely(entries == 1)) return series[0];

    // Clamp percentile between 0.0 and 1.0
    percentile = fmax(0.0, fmin(1.0, percentile));

    // Compute fractional index
    NETDATA_DOUBLE index = percentile * (NETDATA_DOUBLE)(entries - 1);
    size_t low_idx = (size_t)floor(index);
    size_t high_idx = (size_t)ceil(index);;

    // If index is an integer or at the last element, return directly
    if (high_idx >= entries || low_idx == high_idx || considered_equal_ndd(index, (NETDATA_DOUBLE)low_idx))
        return series[low_idx];

    // Linear interpolation
    NETDATA_DOUBLE weight = index - (NETDATA_DOUBLE)low_idx;
    return series[low_idx] + weight * (series[high_idx] - series[low_idx]);
}

NETDATA_DOUBLE median_on_sorted_series(const NETDATA_DOUBLE *series, size_t entries) {
    return percentile_on_sorted_series(series, entries, 0.5);
}

NETDATA_DOUBLE median(const NETDATA_DOUBLE *series, size_t entries) {
    if(unlikely(entries == 0)) return NAN;
    if(unlikely(entries == 1)) return series[0];

    if(unlikely(entries == 2))
        return (series[0] + series[1]) / 2;

    NETDATA_DOUBLE *copy = copy_series(series, entries);
    sort_series(copy, entries);

    NETDATA_DOUBLE avg = median_on_sorted_series(copy, entries);

    freez(copy);
    return avg;
}

// --------------------------------------------------------------------------------------------------------------------

NETDATA_DOUBLE moving_median(const NETDATA_DOUBLE *series, size_t entries, size_t period) {
    if(entries <= period)
        return median(series, entries);

    NETDATA_DOUBLE *data = copy_series(series, entries);

    size_t i;
    for(i = period; i < entries; i++) {
        data[i - period] = median(&series[i - period], period);
    }

    NETDATA_DOUBLE avg = median(data, entries - period);
    freez(data);
    return avg;
}

// --------------------------------------------------------------------------------------------------------------------

// http://stackoverflow.com/a/15150143/4525767
NETDATA_DOUBLE running_median_estimate(const NETDATA_DOUBLE *series, size_t entries) {
    NETDATA_DOUBLE median = 0.0f;
    NETDATA_DOUBLE average = 0.0f;
    size_t i;

    for(i = 0; i < entries ; i++) {
        NETDATA_DOUBLE value = series[i];
        if(unlikely(!netdata_double_isnumber(value))) continue;

        average += ( value - average ) * 0.1f; // rough running average.
        median += copysignndd( average * 0.01, value - median );
    }

    return median;
}

// --------------------------------------------------------------------------------------------------------------------

NETDATA_DOUBLE standard_deviation(const NETDATA_DOUBLE *series, size_t entries) {
    if(unlikely(entries == 0)) return NAN;
    if(unlikely(entries == 1)) return series[0];

    const NETDATA_DOUBLE *value, *end = &series[entries];
    size_t count;
    NETDATA_DOUBLE sum;

    for(count = 0, sum = 0, value = series ; value < end ;value++) {
        if(likely(netdata_double_isnumber(*value))) {
            count++;
            sum += *value;
        }
    }

    if(unlikely(count == 0)) return NAN;
    if(unlikely(count == 1)) return sum;

    NETDATA_DOUBLE average = sum / (NETDATA_DOUBLE)count;

    for(count = 0, sum = 0, value = series ; value < end ;value++) {
        if(netdata_double_isnumber(*value)) {
            count++;
            sum += powndd(*value - average, 2);
        }
    }

    if(unlikely(count == 0)) return NAN;
    if(unlikely(count == 1)) return average;

    NETDATA_DOUBLE variance = sum / (NETDATA_DOUBLE)(count); // remove -1 from count to have a population stddev
    NETDATA_DOUBLE stddev = sqrtndd(variance);
    return stddev;
}

// --------------------------------------------------------------------------------------------------------------------

NETDATA_DOUBLE single_exponential_smoothing(const NETDATA_DOUBLE *series, size_t entries, NETDATA_DOUBLE alpha) {
    if(unlikely(entries == 0))
        return NAN;

    if(unlikely(isnan(alpha)))
        alpha = default_single_exponential_smoothing_alpha;

    const NETDATA_DOUBLE *value = series, *end = &series[entries];
    NETDATA_DOUBLE level = (1.0 - alpha) * (*value);

    for(value++ ; value < end; value++) {
        if(likely(netdata_double_isnumber(*value)))
            level = alpha * (*value) + (1.0 - alpha) * level;
    }

    return level;
}

NETDATA_DOUBLE single_exponential_smoothing_reverse(const NETDATA_DOUBLE *series, size_t entries, NETDATA_DOUBLE alpha) {
    if(unlikely(entries == 0))
        return NAN;

    if(unlikely(isnan(alpha)))
        alpha = default_single_exponential_smoothing_alpha;

    const NETDATA_DOUBLE *value = &series[entries -1];
    NETDATA_DOUBLE level = (1.0 - alpha) * (*value);

    for(value++ ; value >= series; value--) {
        if(likely(netdata_double_isnumber(*value)))
            level = alpha * (*value) + (1.0 - alpha) * level;
    }

    return level;
}

// --------------------------------------------------------------------------------------------------------------------

// http://grisha.org/blog/2016/02/16/triple-exponential-smoothing-forecasting-part-ii/
NETDATA_DOUBLE double_exponential_smoothing(const NETDATA_DOUBLE *series, size_t entries,
    NETDATA_DOUBLE alpha,
    NETDATA_DOUBLE beta,
    NETDATA_DOUBLE *forecast) {
    if(unlikely(entries == 0))
        return NAN;

    NETDATA_DOUBLE level, trend;

    if(unlikely(isnan(alpha)))
        alpha = 0.3;

    if(unlikely(isnan(beta)))
        beta = 0.05;

    level = series[0];

    if(likely(entries > 1))
        trend = series[1] - series[0];
    else
        trend = 0;

    const NETDATA_DOUBLE *value = series;
    for(value++ ; value >= series; value--) {
        if(likely(netdata_double_isnumber(*value))) {
            NETDATA_DOUBLE last_level = level;
            level = alpha * *value + (1.0 - alpha) * (level + trend);
            trend = beta * (level - last_level) + (1.0 - beta) * trend;

        }
    }

    if(forecast)
        *forecast = level + trend;

    return level;
}

// --------------------------------------------------------------------------------------------------------------------

/*
 * Based on th R implementation
 *
 * a: level component
 * b: trend component
 * s: seasonal component
 *
 * Additive:
 *
 *   Yhat[t+h] = a[t] + h * b[t] + s[t + 1 + (h - 1) mod p],
 *   a[t] = α (Y[t] - s[t-p]) + (1-α) (a[t-1] + b[t-1])
 *   b[t] = β (a[t] - a[t-1]) + (1-β) b[t-1]
 *   s[t] = γ (Y[t] - a[t]) + (1-γ) s[t-p]
 *
 * Multiplicative:
 *
 *   Yhat[t+h] = (a[t] + h * b[t]) * s[t + 1 + (h - 1) mod p],
 *   a[t] = α (Y[t] / s[t-p]) + (1-α) (a[t-1] + b[t-1])
 *   b[t] = β (a[t] - a[t-1]) + (1-β) b[t-1]
 *   s[t] = γ (Y[t] / a[t]) + (1-γ) s[t-p]
 */
static int __HoltWinters(
        const NETDATA_DOUBLE *series,
        int          entries,      // start_time + h

    NETDATA_DOUBLE alpha,        // alpha parameter of Holt-Winters Filter.
    NETDATA_DOUBLE
        beta,         // beta  parameter of Holt-Winters Filter. If set to 0, the function will do exponential smoothing.
    NETDATA_DOUBLE
        gamma,        // gamma parameter used for the seasonal component. If set to 0, an non-seasonal model is fitted.

        const int *seasonal,
        const int *period,
        const NETDATA_DOUBLE *a,      // Start value for level (a[0]).
        const NETDATA_DOUBLE *b,      // Start value for trend (b[0]).
    NETDATA_DOUBLE *s,            // Vector of start values for the seasonal component (s_1[0] ... s_p[0])

        /* return values */
    NETDATA_DOUBLE *SSE,          // The final sum of squared errors achieved in optimizing
    NETDATA_DOUBLE *level,        // Estimated values for the level component (size entries - t + 2)
    NETDATA_DOUBLE *trend,        // Estimated values for the trend component (size entries - t + 2)
    NETDATA_DOUBLE *season        // Estimated values for the seasonal component (size entries - t + 2)
)
{
    if(unlikely(entries < 4))
        return 0;

    int start_time = 2;

    NETDATA_DOUBLE res = 0, xhat = 0, stmp = 0;
    int i, i0, s0;

    /* copy start values to the beginning of the vectors */
    level[0] = *a;
    if(beta > 0) trend[0] = *b;
    if(gamma > 0) memcpy(season, s, *period * sizeof(NETDATA_DOUBLE));

    for(i = start_time - 1; i < entries; i++) {
        /* indices for period i */
        i0 = i - start_time + 2;
        s0 = i0 + *period - 1;

        /* forecast *for* period i */
        xhat = level[i0 - 1] + (beta > 0 ? trend[i0 - 1] : 0);
        stmp = gamma > 0 ? season[s0 - *period] : (*seasonal != 1);
        if (*seasonal == 1)
            xhat += stmp;
        else
            xhat *= stmp;

        /* Sum of Squared Errors */
        res   = series[i] - xhat;
        *SSE += res * res;

        /* estimate of level *in* period i */
        if (*seasonal == 1)
            level[i0] = alpha       * (series[i] - stmp)
                        + (1 - alpha) * (level[i0 - 1] + trend[i0 - 1]);
        else
            level[i0] = alpha       * (series[i] / stmp)
                        + (1 - alpha) * (level[i0 - 1] + trend[i0 - 1]);

        /* estimate of trend *in* period i */
        if (beta > 0)
            trend[i0] = beta        * (level[i0] - level[i0 - 1])
                        + (1 - beta)  * trend[i0 - 1];

        /* estimate of seasonal component *in* period i */
        if (gamma > 0) {
            if (*seasonal == 1)
                season[s0] = gamma       * (series[i] - level[i0])
                             + (1 - gamma) * stmp;
            else
                season[s0] = gamma       * (series[i] / level[i0])
                             + (1 - gamma) * stmp;
        }
    }

    return 1;
}

NETDATA_DOUBLE holtwinters(const NETDATA_DOUBLE *series, size_t entries,
    NETDATA_DOUBLE alpha,
    NETDATA_DOUBLE beta,
    NETDATA_DOUBLE gamma,
    NETDATA_DOUBLE *forecast) {
    if(unlikely(isnan(alpha)))
        alpha = 0.3;

    if(unlikely(isnan(beta)))
        beta = 0.05;

    if(unlikely(isnan(gamma)))
        gamma = 0;

    int seasonal = 0;
    int period = 0;
    NETDATA_DOUBLE a0 = series[0];
    NETDATA_DOUBLE b0 = 0;
    NETDATA_DOUBLE s[] = {};

    NETDATA_DOUBLE errors = 0.0;
    size_t nb_computations = entries;
    NETDATA_DOUBLE *estimated_level  = callocz(nb_computations, sizeof(NETDATA_DOUBLE));
    NETDATA_DOUBLE *estimated_trend  = callocz(nb_computations, sizeof(NETDATA_DOUBLE));
    NETDATA_DOUBLE *estimated_season = callocz(nb_computations, sizeof(NETDATA_DOUBLE));

    int ret = __HoltWinters(
            series,
            (int)entries,
            alpha,
            beta,
            gamma,
            &seasonal,
            &period,
            &a0,
            &b0,
            s,
            &errors,
            estimated_level,
            estimated_trend,
            estimated_season
    );

    NETDATA_DOUBLE value = estimated_level[nb_computations - 1];

    if(forecast)
        *forecast = 0.0;

    freez(estimated_level);
    freez(estimated_trend);
    freez(estimated_season);

    if(!ret)
        return 0.0;

    return value;
}
