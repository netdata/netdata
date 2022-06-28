// SPDX-License-Identifier: GPL-3.0-or-later

#include "min.h"

// ----------------------------------------------------------------------------
// min

struct grouping_min {
    NETDATA_DOUBLE min;
    size_t count;
};

void grouping_create_min(RRDR *r, const char *options __maybe_unused) {
    r->internal.grouping_data = callocz(1, sizeof(struct grouping_min));
}

// resets when switches dimensions
// so, clear everything to restart
void grouping_reset_min(RRDR *r) {
    struct grouping_min *g = (struct grouping_min *)r->internal.grouping_data;
    g->min = 0;
    g->count = 0;
}

void grouping_free_min(RRDR *r) {
    freez(r->internal.grouping_data);
    r->internal.grouping_data = NULL;
}

void grouping_add_min(RRDR *r, NETDATA_DOUBLE value) {
    struct grouping_min *g = (struct grouping_min *)r->internal.grouping_data;

    if(!g->count || fabsndd(value) < fabsndd(g->min)) {
        g->min = value;
        g->count++;
    }
}

NETDATA_DOUBLE grouping_flush_min(RRDR *r, RRDR_VALUE_FLAGS *rrdr_value_options_ptr) {
    struct grouping_min *g = (struct grouping_min *)r->internal.grouping_data;

    NETDATA_DOUBLE value;

    if(unlikely(!g->count)) {
        value = 0.0;
        *rrdr_value_options_ptr |= RRDR_VALUE_EMPTY;
    }
    else {
        value = g->min;
    }

    g->min = 0.0;
    g->count = 0;

    return value;
}

