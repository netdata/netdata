// SPDX-License-Identifier: GPL-3.0-or-later

#include "ses.h"


// ----------------------------------------------------------------------------
// single exponential smoothing

struct grouping_ses {
    calculated_number alpha;
    calculated_number level;
    size_t count;
    size_t global_count;
};

void *grouping_init_ses(RRDR *r) {
    struct grouping_ses *g = (struct grouping_ses *)callocz(1, sizeof(struct grouping_ses));
    g->alpha = 1.0 / r->group / 2;
    g->level = 0.0;
    return g;
}

void grouping_reset_ses(RRDR *r) {
    struct grouping_ses *g = (struct grouping_ses *)r->grouping_data;
    g->level = 0.0;
    g->count = 0;
}

void grouping_free_ses(RRDR *r) {
    freez(r->grouping_data);
    r->grouping_data = NULL;
}

void grouping_add_ses(RRDR *r, calculated_number value) {
    struct grouping_ses *g = (struct grouping_ses *)r->grouping_data;

    if(isnormal(value)) {
        if(unlikely(!g->global_count))
            g->level = (1.0 - g->alpha) * value;

        else
            g->level = g->alpha * value + (1.0 - g->alpha) * g->level;

        g->count++;
        g->global_count++;
    }
}

void grouping_flush_ses(RRDR *r, calculated_number *rrdr_value_ptr, RRDR_VALUE_FLAGS *rrdr_value_options_ptr) {
    struct grouping_ses *g = (struct grouping_ses *)r->grouping_data;

    if(unlikely(!g->count || !isnormal(g->level))) {
        *rrdr_value_ptr = 0.0;
        *rrdr_value_options_ptr |= RRDR_VALUE_EMPTY;
    }
    else {
        *rrdr_value_ptr = g->level;
    }

    g->count = 0;
}

