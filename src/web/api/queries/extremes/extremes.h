// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_API_QUERY_EXTREMES_H
#define NETDATA_API_QUERY_EXTREMES_H

#include "../query.h"
#include "../rrdr.h"

struct tg_extremes {
    NETDATA_DOUBLE min;      // for negative values
    NETDATA_DOUBLE max;      // for positive values
    size_t pos_count;        // count of positive values
    size_t neg_count;        // count of negative values
    size_t zero_count;       // count of zero values
};

static inline void tg_extremes_create(RRDR *r, const char *options __maybe_unused) {
    r->time_grouping.data = onewayalloc_callocz(r->internal.owa, 1, sizeof(struct tg_extremes));
}

// resets when switches dimensions
// so, clear everything to restart
static inline void tg_extremes_reset(RRDR *r) {
    struct tg_extremes *g = (struct tg_extremes *)r->time_grouping.data;
    g->min = 0;
    g->max = 0;
    g->pos_count = 0;
    g->neg_count = 0;
    g->zero_count = 0;
}

static inline void tg_extremes_free(RRDR *r) {
    onewayalloc_freez(r->internal.owa, r->time_grouping.data);
    r->time_grouping.data = NULL;
}

static inline void tg_extremes_add(RRDR *r, NETDATA_DOUBLE value) {
    struct tg_extremes *g = (struct tg_extremes *)r->time_grouping.data;

    if (value > 0) {
        // For positive values, track the maximum
        if (!g->pos_count || value > g->max) {
            g->max = value;
        }
        g->pos_count++;
    }
    else if (value < 0) {
        // For negative values, track the minimum
        if (!g->neg_count || value < g->min) {
            g->min = value;
        }
        g->neg_count++;
    }
    else {
        // It's a zero
        g->zero_count++;
    }
}

static inline NETDATA_DOUBLE tg_extremes_flush(RRDR *r, RRDR_VALUE_FLAGS *rrdr_value_options_ptr) {
    struct tg_extremes *g = (struct tg_extremes *)r->time_grouping.data;

    NETDATA_DOUBLE value;

    if (unlikely(!g->pos_count && !g->neg_count && !g->zero_count)) {
        // No values at all
        value = 0.0;
        *rrdr_value_options_ptr |= RRDR_VALUE_EMPTY;
    }
    else if (g->pos_count && g->neg_count) {
        // If we have both positive and negative values,
        // return the one with the greatest absolute value
        if (fabsndd(g->max) > fabsndd(g->min))
            value = g->max;
        else
            value = g->min;
    }
    else if (g->pos_count) {
        // Only positive values, return the maximum
        value = g->max;
    }
    else if (g->neg_count) {
        // Only negative values, return the minimum
        value = g->min;
    }
    else {
        // Only zeros
        value = 0.0;
    }

    // Reset the state for the next calculation
    g->min = 0.0;
    g->max = 0.0;
    g->pos_count = 0;
    g->neg_count = 0;
    g->zero_count = 0;

    return value;
}

#endif //NETDATA_API_QUERY_EXTREMES_H