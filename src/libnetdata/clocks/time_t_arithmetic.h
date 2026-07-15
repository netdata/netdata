// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_TIME_T_ARITHMETIC_H
#define NETDATA_TIME_T_ARITHMETIC_H 1

#include <limits.h>
#include <stdint.h>
#include <time.h>

#ifdef __cplusplus
static_assert((time_t)-1 < (time_t)0, "time arithmetic requires signed time_t");
static_assert(sizeof(time_t) <= sizeof(intmax_t), "time_t must fit in intmax_t");
#else
_Static_assert((time_t)-1 < (time_t)0, "time arithmetic requires signed time_t");
_Static_assert(sizeof(time_t) <= sizeof(intmax_t), "time_t must fit in intmax_t");
#endif

static inline time_t nd_time_t_max(void) {
    return (time_t)(((uintmax_t)1 << (sizeof(time_t) * CHAR_BIT - 1)) - 1);
}

static inline time_t nd_time_t_min(void) {
    return -nd_time_t_max() - 1;
}

// Compare the mathematical sum to a time_t without first narrowing the sum.
static inline int nd_time_t_add_compare(time_t base, intmax_t offset, time_t value) {
    intmax_t sum;
    if(__builtin_add_overflow((intmax_t)base, offset, &sum))
        return offset < 0 ? -1 : 1;

    return (sum > (intmax_t)value) - (sum < (intmax_t)value);
}

// Preserve every representable sum and keep unrepresentable timestamps monotonic.
static inline time_t nd_time_t_add_saturating(time_t base, intmax_t offset) {
    intmax_t sum;
    if(__builtin_add_overflow((intmax_t)base, offset, &sum))
        return offset < 0 ? nd_time_t_min() : nd_time_t_max();

    if(sum > (intmax_t)nd_time_t_max())
        return nd_time_t_max();

    if(sum < (intmax_t)nd_time_t_min())
        return nd_time_t_min();

    return (time_t)sum;
}

#endif // NETDATA_TIME_T_ARITHMETIC_H
