// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_METRIC_CORRELATIONS_H
#define NETDATA_METRIC_CORRELATIONS_H 1

void metric_correlations (RRDHOST *host, BUFFER *wb, long long selected_after, long long selected_before, long long reference_after, long long reference_before, long long max_points);

/*
 * The function
 *
 *        double KSfbar (int n, double x);
 *
 * computes the complementary cumulative probability P[D_n >= x] of the
 * 2-sided 1-sample Kolmogorov-Smirnov distribution with sample size n at x.
 * It returns at least 10 decimal digits of precision for n <= 500,
 * at least 6 decimal digits of precision for 500 < n <= 200000,
 * and a few correct decimal digits for n > 200000.
 *
 */

double KSfbar (int n, double x);

#endif //NETDATA_METRIC_CORRELATIONS_H
