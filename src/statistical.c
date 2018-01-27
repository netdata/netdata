#include "common.h"

// --------------------------------------------------------------------------------------------------------------------

inline LONG_DOUBLE sum_and_count(const LONG_DOUBLE *series, size_t entries, size_t *count) {
    if(unlikely(entries == 0)) {
        if(likely(count))
            *count = 0;

        return NAN;
    }

    if(unlikely(entries == 1)) {
        if(likely(count))
            *count = (isnan(series[0])?0:1);

        return series[0];
    }

    size_t i, c = 0;
    LONG_DOUBLE sum = 0;

    for(i = 0; i < entries ; i++) {
        LONG_DOUBLE value = series[i];
        if(unlikely(isnan(value) || isinf(value))) continue;
        c++;
        sum += value;
    }

    if(likely(count))
        *count = c;

    if(unlikely(c == 0))
        return NAN;

    return sum;
}

inline LONG_DOUBLE sum(const LONG_DOUBLE *series, size_t entries) {
    return sum_and_count(series, entries, NULL);
}

inline LONG_DOUBLE average(const LONG_DOUBLE *series, size_t entries) {
    size_t count = 0;
    LONG_DOUBLE sum = sum_and_count(series, entries, &count);

    if(unlikely(count == 0))
        return NAN;

    return sum / (LONG_DOUBLE)count;
}

// --------------------------------------------------------------------------------------------------------------------

LONG_DOUBLE moving_average(const LONG_DOUBLE *series, size_t entries, size_t period) {
    if(unlikely(period <= 0))
        return 0.0;

    size_t i, count;
    LONG_DOUBLE sum = 0, avg = 0;
    LONG_DOUBLE p[period];

    for(count = 0; count < period ; count++)
        p[count] = 0.0;

    for(i = 0, count = 0; i < entries; i++) {
        LONG_DOUBLE value = series[i];
        if(unlikely(isnan(value) || isinf(value))) continue;

        if(unlikely(count < period)) {
            sum += value;
            avg = (count == period - 1) ? sum / (LONG_DOUBLE)period : 0;
        }
        else {
            sum = sum - p[count % period] + value;
            avg = sum / (LONG_DOUBLE)period;
        }

        p[count % period] = value;
        count++;
    }

    return avg;
}

// --------------------------------------------------------------------------------------------------------------------

static int qsort_compare(const void *a, const void *b) {
    LONG_DOUBLE *p1 = (LONG_DOUBLE *)a, *p2 = (LONG_DOUBLE *)b;
    LONG_DOUBLE n1 = *p1, n2 = *p2;

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

inline void sort_series(LONG_DOUBLE *series, size_t entries) {
    qsort(series, entries, sizeof(LONG_DOUBLE), qsort_compare);
}

inline LONG_DOUBLE *copy_series(const LONG_DOUBLE *series, size_t entries) {
    LONG_DOUBLE *copy = mallocz(sizeof(LONG_DOUBLE) * entries);
    memcpy(copy, series, sizeof(LONG_DOUBLE) * entries);
    return copy;
}

LONG_DOUBLE median_on_sorted_series(const LONG_DOUBLE *series, size_t entries) {
    if(unlikely(entries == 0))
        return NAN;

    if(unlikely(entries == 1))
        return series[0];

    if(unlikely(entries == 2))
        return (series[0] + series[1]) / 2;

    LONG_DOUBLE avg;
    if(entries % 2 == 0) {
        size_t m = entries / 2;
        avg = (series[m] + series[m + 1]) / 2;
    }
    else {
        avg = series[entries / 2];
    }

    return avg;
}

LONG_DOUBLE median(const LONG_DOUBLE *series, size_t entries) {
    if(unlikely(entries == 0))
        return NAN;

    if(unlikely(entries == 1))
        return series[0];

    if(unlikely(entries == 2))
        return (series[0] + series[1]) / 2;

    LONG_DOUBLE *copy = copy_series(series, entries);
    sort_series(copy, entries);

    LONG_DOUBLE avg = median_on_sorted_series(copy, entries);

    freez(copy);
    return avg;
}

// --------------------------------------------------------------------------------------------------------------------

LONG_DOUBLE moving_median(const LONG_DOUBLE *series, size_t entries, size_t period) {
    if(entries <= period)
        return median(series, entries);

    LONG_DOUBLE *data = copy_series(series, entries);

    size_t i;
    for(i = period; i < entries; i++) {
        data[i - period] = median(&series[i - period], period);
    }

    LONG_DOUBLE avg = median(data, entries - period);
    freez(data);
    return avg;
}

// --------------------------------------------------------------------------------------------------------------------

// http://stackoverflow.com/a/15150143/4525767
LONG_DOUBLE running_median_estimate(const LONG_DOUBLE *series, size_t entries) {
    LONG_DOUBLE median = 0.0f;
    LONG_DOUBLE average = 0.0f;
    size_t i;

    for(i = 0; i < entries ; i++) {
        LONG_DOUBLE value = series[i];
        if(unlikely(isnan(value) || isinf(value))) continue;

        average += ( value - average ) * 0.1f; // rough running average.
        median += copysignl( average * 0.01, value - median );
    }

    return median;
}

// --------------------------------------------------------------------------------------------------------------------

LONG_DOUBLE standard_deviation(const LONG_DOUBLE *series, size_t entries) {
    if(unlikely(entries < 1))
        return NAN;

    if(unlikely(entries == 1))
        return series[0];

    size_t i, count = 0;
    LONG_DOUBLE sum = 0;

    for(i = 0; i < entries ; i++) {
        LONG_DOUBLE value = series[i];
        if(unlikely(isnan(value) || isinf(value))) continue;

        count++;
        sum += value;
    }

    if(unlikely(count == 0))
        return NAN;

    if(unlikely(count == 1))
        return sum;

    LONG_DOUBLE average = sum / (LONG_DOUBLE)count;

    for(i = 0, count = 0, sum = 0; i < entries ; i++) {
        LONG_DOUBLE value = series[i];
        if(unlikely(isnan(value) || isinf(value))) continue;

        count++;
        sum += powl(value - average, 2);
    }

    if(unlikely(count == 0))
        return NAN;

    if(unlikely(count == 1))
        return average;

    LONG_DOUBLE variance = sum / (LONG_DOUBLE)(count - 1); // remove -1 to have a population stddev

    LONG_DOUBLE stddev = sqrtl(variance);
    return stddev;
}

// --------------------------------------------------------------------------------------------------------------------

LONG_DOUBLE single_exponential_smoothing(const LONG_DOUBLE *series, size_t entries, LONG_DOUBLE alpha) {
    size_t i, count = 0;
    LONG_DOUBLE level = 0, sum = 0;

    if(unlikely(isnan(alpha)))
        alpha = 0.3;

    for(i = 0; i < entries ; i++) {
        LONG_DOUBLE value = series[i];
        if(unlikely(isnan(value) || isinf(value))) continue;
        count++;

        sum += value;

        LONG_DOUBLE last_level = level;
        level = alpha * value + (1.0 - alpha) * last_level;
    }

    return level;
}

// --------------------------------------------------------------------------------------------------------------------

// http://grisha.org/blog/2016/02/16/triple-exponential-smoothing-forecasting-part-ii/
LONG_DOUBLE double_exponential_smoothing(const LONG_DOUBLE *series, size_t entries, LONG_DOUBLE alpha, LONG_DOUBLE beta, LONG_DOUBLE *forecast) {
    size_t i, count = 0;
    LONG_DOUBLE level = series[0], trend, sum;

    if(unlikely(isnan(alpha)))
        alpha = 0.3;

    if(unlikely(isnan(beta)))
        beta = 0.05;

    if(likely(entries > 1))
        trend = series[1] - series[0];
    else
        trend = 0;

    sum = series[0];

    for(i = 1; i < entries ; i++) {
        LONG_DOUBLE value = series[i];
        if(unlikely(isnan(value) || isinf(value))) continue;
        count++;

        sum += value;

        LONG_DOUBLE last_level = level;

        level = alpha * value + (1.0 - alpha) * (level + trend);
        trend = beta * (level - last_level) + (1.0 - beta) * trend;
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
        const LONG_DOUBLE *series,
        int          entries,      // start_time + h

        LONG_DOUBLE alpha,        // alpha parameter of Holt-Winters Filter.
        LONG_DOUBLE beta,         // beta  parameter of Holt-Winters Filter. If set to 0, the function will do exponential smoothing.
        LONG_DOUBLE gamma,        // gamma parameter used for the seasonal component. If set to 0, an non-seasonal model is fitted.

        const int *seasonal,
        const int *period,
        const LONG_DOUBLE *a,      // Start value for level (a[0]).
        const LONG_DOUBLE *b,      // Start value for trend (b[0]).
        LONG_DOUBLE *s,            // Vector of start values for the seasonal component (s_1[0] ... s_p[0])

        /* return values */
        LONG_DOUBLE *SSE,          // The final sum of squared errors achieved in optimizing
        LONG_DOUBLE *level,        // Estimated values for the level component (size entries - t + 2)
        LONG_DOUBLE *trend,        // Estimated values for the trend component (size entries - t + 2)
        LONG_DOUBLE *season        // Estimated values for the seasonal component (size entries - t + 2)
)
{
    if(unlikely(entries < 4))
        return 0;

    int start_time = 2;

    LONG_DOUBLE res = 0, xhat = 0, stmp = 0;
    int i, i0, s0;

    /* copy start values to the beginning of the vectors */
    level[0] = *a;
    if(beta > 0) trend[0] = *b;
    if(gamma > 0) memcpy(season, s, *period * sizeof(LONG_DOUBLE));

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

LONG_DOUBLE holtwinters(const LONG_DOUBLE *series, size_t entries, LONG_DOUBLE alpha, LONG_DOUBLE beta, LONG_DOUBLE gamma, LONG_DOUBLE *forecast) {
    if(unlikely(isnan(alpha)))
        alpha = 0.3;

    if(unlikely(isnan(beta)))
        beta = 0.05;

    if(unlikely(isnan(gamma)))
        gamma = 0;

    int seasonal = 0;
    int period = 0;
    LONG_DOUBLE a0 = series[0];
    LONG_DOUBLE b0 = 0;
    LONG_DOUBLE s[] = {};

    LONG_DOUBLE errors = 0.0;
    size_t nb_computations = entries;
    LONG_DOUBLE *estimated_level  = callocz(nb_computations, sizeof(LONG_DOUBLE));
    LONG_DOUBLE *estimated_trend  = callocz(nb_computations, sizeof(LONG_DOUBLE));
    LONG_DOUBLE *estimated_season = callocz(nb_computations, sizeof(LONG_DOUBLE));

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

    LONG_DOUBLE value = estimated_level[nb_computations - 1];

    if(forecast)
        *forecast = 0.0;

    freez(estimated_level);
    freez(estimated_trend);
    freez(estimated_season);

    if(!ret)
        return 0.0;

    return value;
}
