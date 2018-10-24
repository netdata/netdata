// SPDX-License-Identifier: GPL-3.0-or-later

#include "sum.h"

// ----------------------------------------------------------------------------
// sum

struct grouping_sum {
    calculated_number sum;
    size_t count;
};

void *grouping_create_sum(RRDR *r) {
    (void)r;
    return callocz(1, sizeof(struct grouping_sum));
}

// resets when switches dimensions
// so, clear everything to restart
void grouping_reset_sum(RRDR *r) {
    struct grouping_sum *g = (struct grouping_sum *)r->internal.grouping_data;
    g->sum = 0;
    g->count = 0;
}

void grouping_free_sum(RRDR *r) {
    freez(r->internal.grouping_data);
    r->internal.grouping_data = NULL;
}

void grouping_add_sum(RRDR *r, calculated_number value) {
    if(!isnan(value)) {
        struct grouping_sum *g = (struct grouping_sum *)r->internal.grouping_data;

        if(!g->count || calculated_number_fabs(value) > calculated_number_fabs(g->sum)) {
            g->sum += value;
            g->count++;
        }
    }
}

calculated_number grouping_flush_sum(RRDR *r, RRDR_VALUE_FLAGS *rrdr_value_options_ptr) {
    struct grouping_sum *g = (struct grouping_sum *)r->internal.grouping_data;

    calculated_number value;

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


