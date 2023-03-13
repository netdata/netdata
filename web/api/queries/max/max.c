// SPDX-License-Identifier: GPL-3.0-or-later

#include "max.h"

// ----------------------------------------------------------------------------
// max

struct grouping_max {
    NETDATA_DOUBLE max;
    size_t count;
};

void grouping_create_max(RRDR *r, const char *options __maybe_unused) {
    r->time_grouping.data = onewayalloc_callocz(r->internal.owa, 1, sizeof(struct grouping_max));
}

// resets when switches dimensions
// so, clear everything to restart
void grouping_reset_max(RRDR *r) {
    struct grouping_max *g = (struct grouping_max *)r->time_grouping.data;
    g->max = 0;
    g->count = 0;
}

void grouping_free_max(RRDR *r) {
    onewayalloc_freez(r->internal.owa, r->time_grouping.data);
    r->time_grouping.data = NULL;
}

void grouping_add_max(RRDR *r, NETDATA_DOUBLE value) {
    struct grouping_max *g = (struct grouping_max *)r->time_grouping.data;

    if(!g->count || fabsndd(value) > fabsndd(g->max)) {
        g->max = value;
        g->count++;
    }
}

NETDATA_DOUBLE grouping_flush_max(RRDR *r, RRDR_VALUE_FLAGS *rrdr_value_options_ptr) {
    struct grouping_max *g = (struct grouping_max *)r->time_grouping.data;

    NETDATA_DOUBLE value;

    if(unlikely(!g->count)) {
        value = 0.0;
        *rrdr_value_options_ptr |= RRDR_VALUE_EMPTY;
    }
    else {
        value = g->max;
    }

    g->max = 0.0;
    g->count = 0;

    return value;
}

