// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_STATISTICAL_H
#define NETDATA_STATISTICAL_H 1

#include "../libnetdata.h"

extern void log_series_to_stderr(calculated_number *series, size_t entries, calculated_number result, const char *msg);

extern calculated_number average(const calculated_number *series, size_t entries);
extern calculated_number moving_average(const calculated_number *series, size_t entries, size_t period);
extern calculated_number median(const calculated_number *series, size_t entries);
extern calculated_number moving_median(const calculated_number *series, size_t entries, size_t period);
extern calculated_number running_median_estimate(const calculated_number *series, size_t entries);
extern calculated_number standard_deviation(const calculated_number *series, size_t entries);
extern calculated_number single_exponential_smoothing(const calculated_number *series, size_t entries, calculated_number alpha);
extern calculated_number single_exponential_smoothing_reverse(const calculated_number *series, size_t entries, calculated_number alpha);
extern calculated_number double_exponential_smoothing(const calculated_number *series, size_t entries, calculated_number alpha, calculated_number beta, calculated_number *forecast);
extern calculated_number holtwinters(const calculated_number *series, size_t entries, calculated_number alpha, calculated_number beta, calculated_number gamma, calculated_number *forecast);
extern calculated_number sum_and_count(const calculated_number *series, size_t entries, size_t *count);
extern calculated_number sum(const calculated_number *series, size_t entries);
extern calculated_number median_on_sorted_series(const calculated_number *series, size_t entries);
extern calculated_number *copy_series(const calculated_number *series, size_t entries);
extern void sort_series(calculated_number *series, size_t entries);

#endif //NETDATA_STATISTICAL_H
