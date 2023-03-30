// SPDX-License-Identifier: GPL-3.0-or-later

#include "average.h"

// ----------------------------------------------------------------------------
// average

struct grouping_average {
    NETDATA_DOUBLE sum;
    size_t count;
};

void grouping_create_average(RRDR *r, const char *options __maybe_unused) {
    r->time_grouping.data = onewayalloc_callocz(r->internal.owa, 1, sizeof(struct grouping_average));
}

// resets when switches dimensions
// so, clear everything to restart
void grouping_reset_average(RRDR *r) {
    struct grouping_average *g = (struct grouping_average *)r->time_grouping.data;
    g->sum = 0;
    g->count = 0;
}

void grouping_free_average(RRDR *r) {
    onewayalloc_freez(r->internal.owa, r->time_grouping.data);
    r->time_grouping.data = NULL;
}

void grouping_add_average(RRDR *r, NETDATA_DOUBLE value) {
    struct grouping_average *g = (struct grouping_average *)r->time_grouping.data;
    g->sum += value;
    g->count++;
}

NETDATA_DOUBLE grouping_flush_average(RRDR *r,  RRDR_VALUE_FLAGS *rrdr_value_options_ptr) {
    struct grouping_average *g = (struct grouping_average *)r->time_grouping.data;

    NETDATA_DOUBLE value;

    if(unlikely(!g->count)) {
        value = 0.0;
        *rrdr_value_options_ptr |= RRDR_VALUE_EMPTY;
    }
    else {
        if(unlikely(r->time_grouping.resampling_group != 1))
            value = g->sum / r->time_grouping.resampling_divisor;
        else
            value = g->sum / g->count;
    }

    g->sum = 0.0;
    g->count = 0;

    return value;
}
