// SPDX-License-Identifier: GPL-3.0-or-later

#include "ses.h"


// ----------------------------------------------------------------------------
// single exponential smoothing

struct grouping_ses {
    calculated_number alpha;
    calculated_number alpha_older;
    calculated_number level;
    size_t count;
    size_t has_data;
};

static inline void set_alpha(RRDR *r, struct grouping_ses *g) {
    g->alpha = 1.0 / r->group;
    g->alpha_older = 1 - g->alpha;
}

void *grouping_init_ses(RRDR *r) {
    struct grouping_ses *g = (struct grouping_ses *)callocz(1, sizeof(struct grouping_ses));
    set_alpha(r, g);
    g->level = 0.0;
    return g;
}

// resets when switches dimensions
// so, clear everything to restart
void grouping_reset_ses(RRDR *r) {
    struct grouping_ses *g = (struct grouping_ses *)r->grouping_data;
    g->level = 0.0;
    g->count = 0;
    g->has_data = 0;
}

void grouping_free_ses(RRDR *r) {
    freez(r->grouping_data);
    r->grouping_data = NULL;
}

void grouping_add_ses(RRDR *r, calculated_number value) {
    struct grouping_ses *g = (struct grouping_ses *)r->grouping_data;

    if(isnormal(value)) {
        if(unlikely(!g->has_data)) {
            g->level = value;
            g->has_data = 1;
        }

        g->level = g->alpha * value + g->alpha_older * g->level;

        g->count++;
    }
}

calculated_number grouping_flush_ses(RRDR *r, RRDR_VALUE_FLAGS *rrdr_value_options_ptr) {
    struct grouping_ses *g = (struct grouping_ses *)r->grouping_data;

    calculated_number value;

    if(unlikely(!g->count || !isnormal(g->level))) {
        value = 0.0;
        *rrdr_value_options_ptr |= RRDR_VALUE_EMPTY;
    }
    else {
        value = g->level;
    }

    g->count = 0;

    return value;
}
