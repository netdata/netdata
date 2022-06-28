// SPDX-License-Identifier: GPL-3.0-or-later

#include "value.h"


inline NETDATA_DOUBLE
rrdr2value(RRDR *r, long i, RRDR_OPTIONS options, int *all_values_are_null, uint8_t *anomaly_rate, RRDDIM *temp_rd) {
    if (r->st_needs_lock)
        rrdset_check_rdlock(r->st);

    long c;
    RRDDIM *d;

    NETDATA_DOUBLE *cn = &r->v[ i * r->d ];
    RRDR_VALUE_FLAGS *co = &r->o[ i * r->d ];
    uint8_t *ar = &r->ar[ i * r->d ];

    NETDATA_DOUBLE sum = 0, min = 0, max = 0, v;
    int all_null = 1, init = 1;

    NETDATA_DOUBLE total = 1;
    size_t total_anomaly_rate = 0;

    int set_min_max = 0;
    if(unlikely(options & RRDR_OPTION_PERCENTAGE)) {
        total = 0;
        for (c = 0, d = temp_rd ? temp_rd : r->st->dimensions; d && c < r->d; c++, d = d->next) {
            NETDATA_DOUBLE n = cn[c];

            if(likely((options & RRDR_OPTION_ABSOLUTE) && n < 0))
                n = -n;

            total += n;
        }
        // prevent a division by zero
        if(total == 0) total = 1;
        set_min_max = 1;
    }

    // for each dimension
    for (c = 0, d = temp_rd ? temp_rd : r->st->dimensions; d && c < r->d; c++, d = d->next) {
        if(unlikely(r->od[c] & RRDR_DIMENSION_HIDDEN)) continue;
        if(unlikely((options & RRDR_OPTION_NONZERO) && !(r->od[c] & RRDR_DIMENSION_NONZERO))) continue;

        NETDATA_DOUBLE n = cn[c];

        if(likely((options & RRDR_OPTION_ABSOLUTE) && n < 0))
            n = -n;

        if(unlikely(options & RRDR_OPTION_PERCENTAGE)) {
            n = n * 100 / total;

            if(unlikely(set_min_max)) {
                r->min = r->max = n;
                set_min_max = 0;
            }

            if(n < r->min) r->min = n;
            if(n > r->max) r->max = n;
        }

        if(unlikely(init)) {
            if(n > 0) {
                min = 0;
                max = n;
            }
            else {
                min = n;
                max = 0;
            }
            init = 0;
        }

        if(likely(!(co[c] & RRDR_VALUE_EMPTY))) {
            all_null = 0;
            sum += n;
        }

        if(n < min) min = n;
        if(n > max) max = n;

        total_anomaly_rate += ar[c];
    }

    if(anomaly_rate) {
        if(!r->d) *anomaly_rate = 0;
        else *anomaly_rate = total_anomaly_rate / r->d;
    }

    if(unlikely(all_null)) {
        if(likely(all_values_are_null))
            *all_values_are_null = 1;
        return 0;
    }
    else {
        if(likely(all_values_are_null))
            *all_values_are_null = 0;
    }

    if(options & RRDR_OPTION_MIN2MAX)
        v = max - min;
    else
        v = sum;

    return v;
}
