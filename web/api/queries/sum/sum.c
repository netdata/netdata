// SPDX-License-Identifier: GPL-3.0-or-later

#include "sum.h"

// ----------------------------------------------------------------------------
// sum

struct grouping_sum {
    NETDATA_DOUBLE sum;
    size_t count;
};

void grouping_create_sum(RRDR *r, const char *options __maybe_unused) {
    r->time_grouping.data = onewayalloc_callocz(r->internal.owa, 1, sizeof(struct grouping_sum));
}

// resets when switches dimensions
// so, clear everything to restart
void grouping_reset_sum(RRDR *r) {
    struct grouping_sum *g = (struct grouping_sum *)r->time_grouping.data;
    g->sum = 0;
    g->count = 0;
}

void grouping_free_sum(RRDR *r) {
    onewayalloc_freez(r->internal.owa, r->time_grouping.data);
    r->time_grouping.data = NULL;
}

void grouping_add_sum(RRDR *r, NETDATA_DOUBLE value) {
    struct grouping_sum *g = (struct grouping_sum *)r->time_grouping.data;
    g->sum += value;
    g->count++;
}

NETDATA_DOUBLE grouping_flush_sum(RRDR *r, RRDR_VALUE_FLAGS *rrdr_value_options_ptr) {
    struct grouping_sum *g = (struct grouping_sum *)r->time_grouping.data;

    NETDATA_DOUBLE value;

    if(unlikely(!g->count)) {
        value = 0.0;
        *rrdr_value_options_ptr |= RRDR_VALUE_EMPTY;
    }
    else {
        value = g->sum;
    }

    g->sum = 0.0;
    g->count = 0;

    return value;
}


