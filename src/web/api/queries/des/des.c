// SPDX-License-Identifier: GPL-3.0-or-later

#include <web/api/queries/rrdr.h>
#include "des.h"


// ----------------------------------------------------------------------------
// single exponential smoothing

struct grouping_des {
    calculated_number alpha;
    calculated_number alpha_other;
    calculated_number beta;
    calculated_number beta_other;

    calculated_number level;
    calculated_number trend;

    size_t count;
};

static size_t max_window_size = 15;

void grouping_init_des(void) {
    long long ret = config_get_number(CONFIG_SECTION_WEB, "des max window", (long long)max_window_size);
    if(ret <= 1) {
        config_set_number(CONFIG_SECTION_WEB, "des max window", (long long)max_window_size);
    }
    else {
        max_window_size = (size_t) ret;
    }
}

static inline calculated_number window(RRDR *r, struct grouping_des *g) {
    (void)g;

    calculated_number points;
    if(r->group == 1) {
        // provide a running DES
        points = r->internal.points_wanted;
    }
    else {
        // provide a SES with flush points
        points = r->group;
    }

    // https://en.wikipedia.org/wiki/Moving_average#Exponential_moving_average
    // A commonly used value for alpha is 2 / (N + 1)
    return (points > max_window_size) ? max_window_size : points;
}

static inline void set_alpha(RRDR *r, struct grouping_des *g) {
    // https://en.wikipedia.org/wiki/Moving_average#Exponential_moving_average
    // A commonly used value for alpha is 2 / (N + 1)

    g->alpha = 2.0 / (window(r, g) + 1.0);
    g->alpha_other = 1.0 - g->alpha;

    //info("alpha for chart '%s' is " CALCULATED_NUMBER_FORMAT, r->st->name, g->alpha);
}

static inline void set_beta(RRDR *r, struct grouping_des *g) {
    // https://en.wikipedia.org/wiki/Moving_average#Exponential_moving_average
    // A commonly used value for alpha is 2 / (N + 1)

    g->beta = 2.0 / (window(r, g) + 1.0);
    g->beta_other = 1.0 - g->beta;

    //info("beta for chart '%s' is " CALCULATED_NUMBER_FORMAT, r->st->name, g->beta);
}

void *grouping_create_des(RRDR *r) {
    struct grouping_des *g = (struct grouping_des *)malloc(sizeof(struct grouping_des));
    set_alpha(r, g);
    set_beta(r, g);
    g->level = 0.0;
    g->trend = 0.0;
    g->count = 0;
    return g;
}

// resets when switches dimensions
// so, clear everything to restart
void grouping_reset_des(RRDR *r) {
    struct grouping_des *g = (struct grouping_des *)r->internal.grouping_data;
    g->level = 0.0;
    g->trend = 0.0;
    g->count = 0;

    // fprintf(stderr, "\nDES: ");

}

void grouping_free_des(RRDR *r) {
    freez(r->internal.grouping_data);
    r->internal.grouping_data = NULL;
}

void grouping_add_des(RRDR *r, calculated_number value) {
    struct grouping_des *g = (struct grouping_des *)r->internal.grouping_data;

    if(calculated_number_isnumber(value)) {
        if(likely(g->count > 0)) {
            // we have at least a number so far

            if(unlikely(g->count == 1)) {
                // the second value we got
                g->trend = value - g->trend;
                g->level = value;
            }

            // for the values, except the first
            calculated_number last_level = g->level;
            g->level = (g->alpha * value) + (g->alpha_other * (g->level + g->trend));
            g->trend = (g->beta * (g->level - last_level)) + (g->beta_other * g->trend);
        }
        else {
            // the first value we got
            g->level = g->trend = value;
        }

        g->count++;
    }

    //fprintf(stderr, "value: " CALCULATED_NUMBER_FORMAT ", level: " CALCULATED_NUMBER_FORMAT ", trend: " CALCULATED_NUMBER_FORMAT "\n", value, g->level, g->trend);
}

calculated_number grouping_flush_des(RRDR *r, RRDR_VALUE_FLAGS *rrdr_value_options_ptr) {
    struct grouping_des *g = (struct grouping_des *)r->internal.grouping_data;

    if(unlikely(!g->count || !calculated_number_isnumber(g->level))) {
        *rrdr_value_options_ptr |= RRDR_VALUE_EMPTY;
        return 0.0;
    }

    //fprintf(stderr, " RESULT for %zu values = " CALCULATED_NUMBER_FORMAT " \n", g->count, g->level);

    return g->level;
}
