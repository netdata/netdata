// SPDX-License-Identifier: GPL-3.0-or-later

#include "min.h"

// ----------------------------------------------------------------------------
// min

struct grouping_min {
    calculated_number min;
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

void grouping_add_min(RRDR *r, calculated_number value) {
    struct grouping_min *g = (struct grouping_min *)r->internal.grouping_data;

    if(!g->count || calculated_number_fabs(value) < calculated_number_fabs(g->min)) {
        g->min = value;
        g->count++;
    }
}

calculated_number grouping_flush_min(RRDR *r, RRDR_VALUE_FLAGS *rrdr_value_options_ptr) {
    struct grouping_min *g = (struct grouping_min *)r->internal.grouping_data;

    calculated_number value;

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

