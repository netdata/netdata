// SPDX-License-Identifier: GPL-3.0-or-later

#include "average.h"

// ----------------------------------------------------------------------------
// average

struct grouping_average {
    calculated_number sum;
    size_t count;
};

void *grouping_create_average(RRDR *r) {
    (void)r;
    return callocz(1, sizeof(struct grouping_average));
}

// resets when switches dimensions
// so, clear everything to restart
void grouping_reset_average(RRDR *r) {
    struct grouping_average *g = (struct grouping_average *)r->internal.grouping_data;
    g->sum = 0;
    g->count = 0;
}

void grouping_free_average(RRDR *r) {
    freez(r->internal.grouping_data);
    r->internal.grouping_data = NULL;
}

void grouping_add_average(RRDR *r, calculated_number value) {
    if(!isnan(value)) {
        struct grouping_average *g = (struct grouping_average *)r->internal.grouping_data;
        g->sum += value;
        g->count++;
    }
}

calculated_number grouping_flush_average(RRDR *r,  RRDR_VALUE_FLAGS *rrdr_value_options_ptr) {
    struct grouping_average *g = (struct grouping_average *)r->internal.grouping_data;

    calculated_number value;

    if(unlikely(!g->count)) {
        value = 0.0;
        *rrdr_value_options_ptr |= RRDR_VALUE_EMPTY;
    }
    else {
        if(unlikely(r->internal.resampling_group != 1)) {
            if (unlikely(r->result_options & RRDR_RESULT_OPTION_VARIABLE_STEP))
                value = g->sum / g->count / r->internal.resampling_divisor;
            else
                value = g->sum / r->internal.resampling_divisor;
        } else
            value = g->sum / g->count;
    }

    g->sum = 0.0;
    g->count = 0;

    return value;
}
