#ifndef NETDATA_STATISTICAL_H
#define NETDATA_STATISTICAL_H

extern long double average(long double *series, size_t entries);
extern long double moving_average(long double *series, size_t entries, size_t period);
extern long double median(long double *series, size_t entries);
extern long double moving_median(long double *series, size_t entries, size_t period);
extern long double running_median_estimate(long double *series, size_t entries);
extern long double standard_deviation(long double *series, size_t entries);
extern long double single_exponential_smoothing(long double *series, size_t entries, long double alpha);
extern long double double_exponential_smoothing(long double *series, size_t entries, long double alpha, long double beta, long double *forecast);
extern long double holtwinters(long double *series, size_t entries, long double alpha, long double beta, long double gamma, long double *forecast);
extern long double sum_and_count(long double *series, size_t entries, size_t *count);
extern long double sum(long double *series, size_t entries);
extern long double median_on_sorted_series(long double *series, size_t entries);
extern long double *copy_series(long double *series, size_t entries);
extern void sort_series(long double *series, size_t entries);

#endif //NETDATA_STATISTICAL_H
