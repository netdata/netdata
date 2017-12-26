#include "common.h"

// --------------------------------------------------------------------------------------------------------------------

inline long double sum_and_count(const long double *series, size_t entries, size_t *count) {
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
    long double sum = 0;

    for(i = 0; i < entries ; i++) {
        long double value = series[i];
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

inline long double sum(const long double *series, size_t entries) {
    return sum_and_count(series, entries, NULL);
}

inline long double average(const long double *series, size_t entries) {
    size_t count = 0;
    long double sum = sum_and_count(series, entries, &count);

    if(unlikely(count == 0))
        return NAN;

    return sum / (long double)count;
}

// --------------------------------------------------------------------------------------------------------------------

long double moving_average(const long double *series, size_t entries, size_t period) {
    if(unlikely(period <= 0))
        return 0.0;

    size_t i, count;
    long double sum = 0, avg = 0;
    long double p[period];

    for(count = 0; count < period ; count++)
        p[count] = 0.0;

    for(i = 0, count = 0; i < entries; i++) {
        long double value = series[i];
        if(unlikely(isnan(value) || isinf(value))) continue;

        if(unlikely(count < period)) {
            sum += value;
            avg = (count == period - 1) ? sum / (long double)period : 0;
        }
        else {
            sum = sum - p[count % period] + value;
            avg = sum / (long double)period;
        }

        p[count % period] = value;
        count++;
    }

    return avg;
}

// --------------------------------------------------------------------------------------------------------------------

static int qsort_compare(const void *a, const void *b) {
    long double *p1 = (long double *)a, *p2 = (long double *)b;
    long double n1 = *p1, n2 = *p2;

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

inline void sort_series(long double *series, size_t entries) {
    qsort(series, entries, sizeof(long double), qsort_compare);
}

inline long double *copy_series(const long double *series, size_t entries) {
    long double *copy = mallocz(sizeof(long double) * entries);
    memcpy(copy, series, sizeof(long double) * entries);
    return copy;
}

long double median_on_sorted_series(const long double *series, size_t entries) {
    if(unlikely(entries == 0))
        return NAN;

    if(unlikely(entries == 1))
        return series[0];

    if(unlikely(entries == 2))
        return (series[0] + series[1]) / 2;

    long double avg;
    if(entries % 2 == 0) {
        size_t m = entries / 2;
        avg = (series[m] + series[m + 1]) / 2;
    }
    else {
        avg = series[entries / 2];
    }

    return avg;
}

long double median(const long double *series, size_t entries) {
    if(unlikely(entries == 0))
        return NAN;

    if(unlikely(entries == 1))
        return series[0];

    if(unlikely(entries == 2))
        return (series[0] + series[1]) / 2;

    long double *copy = copy_series(series, entries);
    sort_series(copy, entries);

    long double avg = median_on_sorted_series(copy, entries);

    freez(copy);
    return avg;
}

// --------------------------------------------------------------------------------------------------------------------

long double moving_median(const long double *series, size_t entries, size_t period) {
    if(entries <= period)
        return median(series, entries);

    long double *data = copy_series(series, entries);

    size_t i;
    for(i = period; i < entries; i++) {
        data[i - period] = median(&series[i - period], period);
    }

    long double avg = median(data, entries - period);
    freez(data);
    return avg;
}

// --------------------------------------------------------------------------------------------------------------------

// http://stackoverflow.com/a/15150143/4525767
long double running_median_estimate(const long double *series, size_t entries) {
    long double median = 0.0f;
    long double average = 0.0f;
    size_t i;

    for(i = 0; i < entries ; i++) {
        long double value = series[i];
        if(unlikely(isnan(value) || isinf(value))) continue;

        average += ( value - average ) * 0.1f; // rough running average.
        median += copysignl( average * 0.01, value - median );
    }

    return median;
}

// --------------------------------------------------------------------------------------------------------------------

long double standard_deviation(const long double *series, size_t entries) {
    if(unlikely(entries < 1))
        return NAN;

    if(unlikely(entries == 1))
        return series[0];

    size_t i, count = 0;
    long double sum = 0;

    for(i = 0; i < entries ; i++) {
        long double value = series[i];
        if(unlikely(isnan(value) || isinf(value))) continue;

        count++;
        sum += value;
    }

    if(unlikely(count == 0))
        return NAN;

    if(unlikely(count == 1))
        return sum;

    long double average = sum / (long double)count;

    for(i = 0, count = 0, sum = 0; i < entries ; i++) {
        long double value = series[i];
        if(unlikely(isnan(value) || isinf(value))) continue;

        count++;
        sum += powl(value - average, 2);
    }

    if(unlikely(count == 0))
        return NAN;

    if(unlikely(count == 1))
        return average;

    long double variance = sum / (long double)(count - 1); // remove -1 to have a population stddev

    long double stddev = sqrtl(variance);
    return stddev;
}

// --------------------------------------------------------------------------------------------------------------------

long double single_exponential_smoothing(const long double *series, size_t entries, long double alpha) {
    size_t i, count = 0;
    long double level = 0, sum = 0;

    if(unlikely(isnan(alpha)))
        alpha = 0.3;

    for(i = 0; i < entries ; i++) {
        long double value = series[i];
        if(unlikely(isnan(value) || isinf(value))) continue;
        count++;

        sum += value;

        long double last_level = level;
        level = alpha * value + (1.0 - alpha) * last_level;
    }

    return level;
}

// --------------------------------------------------------------------------------------------------------------------

// http://grisha.org/blog/2016/02/16/triple-exponential-smoothing-forecasting-part-ii/
long double double_exponential_smoothing(const long double *series, size_t entries, long double alpha, long double beta, long double *forecast) {
    size_t i, count = 0;
    long double level = series[0], trend, sum;

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
        long double value = series[i];
        if(unlikely(isnan(value) || isinf(value))) continue;
        count++;

        sum += value;

        long double last_level = level;

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
        const long double *series,
        int          entries,      // start_time + h

        long double alpha,        // alpha parameter of Holt-Winters Filter.
        long double beta,         // beta  parameter of Holt-Winters Filter. If set to 0, the function will do exponential smoothing.
        long double gamma,        // gamma parameter used for the seasonal component. If set to 0, an non-seasonal model is fitted.

        const int *seasonal,
        const int *period,
        const long double *a,      // Start value for level (a[0]).
        const long double *b,      // Start value for trend (b[0]).
        long double *s,            // Vector of start values for the seasonal component (s_1[0] ... s_p[0])

        /* return values */
        long double *SSE,          // The final sum of squared errors achieved in optimizing
        long double *level,        // Estimated values for the level component (size entries - t + 2)
        long double *trend,        // Estimated values for the trend component (size entries - t + 2)
        long double *season        // Estimated values for the seasonal component (size entries - t + 2)
)
{
    if(unlikely(entries < 4))
        return 0;

    int start_time = 2;

    long double res = 0, xhat = 0, stmp = 0;
    int i, i0, s0;

    /* copy start values to the beginning of the vectors */
    level[0] = *a;
    if(beta > 0) trend[0] = *b;
    if(gamma > 0) memcpy(season, s, *period * sizeof(long double));

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

long double holtwinters(const long double *series, size_t entries, long double alpha, long double beta, long double gamma, long double *forecast) {
    if(unlikely(isnan(alpha)))
        alpha = 0.3;

    if(unlikely(isnan(beta)))
        beta = 0.05;

    if(unlikely(isnan(gamma)))
        gamma = 0;

    int seasonal = 0;
    int period = 0;
    long double a0 = series[0];
    long double b0 = 0;
    long double s[] = {};

    long double errors = 0.0;
    size_t nb_computations = entries;
    long double *estimated_level  = callocz(nb_computations, sizeof(long double));
    long double *estimated_trend  = callocz(nb_computations, sizeof(long double));
    long double *estimated_season = callocz(nb_computations, sizeof(long double));

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

    long double value = estimated_level[nb_computations - 1];

    if(forecast)
        *forecast = 0.0;

    freez(estimated_level);
    freez(estimated_trend);
    freez(estimated_season);

    if(!ret)
        return 0.0;

    return value;
}
