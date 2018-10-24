// SPDX-License-Identifier: GPL-3.0-or-later

#include "ses.h"


// ----------------------------------------------------------------------------
// single exponential smoothing

struct grouping_ses {
    calculated_number alpha;
    calculated_number alpha_other;
    calculated_number level;
    size_t count;
};

static inline void set_alpha(RRDR *r, struct grouping_ses *g) {
    // https://en.wikipedia.org/wiki/Moving_average#Exponential_moving_average
    // A commonly used value for alpha is 2 / (N + 1)
    g->alpha = 2.0 / ((calculated_number)r->group + 1.0);
    g->alpha_other = 1 - g->alpha;
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
}

void grouping_free_ses(RRDR *r) {
    freez(r->grouping_data);
    r->grouping_data = NULL;
}

void grouping_add_ses(RRDR *r, calculated_number value) {
    struct grouping_ses *g = (struct grouping_ses *)r->grouping_data;

    if(isnormal(value)) {
        if(unlikely(!g->count))
            g->level = value;

        g->level = g->alpha * value + g->alpha_other * g->level;
        g->count++;
    }
}

calculated_number grouping_flush_ses(RRDR *r, RRDR_VALUE_FLAGS *rrdr_value_options_ptr) {
    struct grouping_ses *g = (struct grouping_ses *)r->grouping_data;

    if(unlikely(!g->count || !isnormal(g->level))) {
        *rrdr_value_options_ptr |= RRDR_VALUE_EMPTY;
        return 0.0;
    }

    return g->level;
}
