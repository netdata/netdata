// SPDX-License-Identifier: GPL-3.0-or-later

#include "ses.h"


// ----------------------------------------------------------------------------
// single exponential smoothing

struct grouping_ses {
    NETDATA_DOUBLE alpha;
    NETDATA_DOUBLE alpha_other;
    NETDATA_DOUBLE level;
    size_t count;
};

static size_t max_window_size = 15;

void grouping_init_ses(void) {
    long long ret = config_get_number(CONFIG_SECTION_WEB, "ses max window", (long long)max_window_size);
    if(ret <= 1) {
        config_set_number(CONFIG_SECTION_WEB, "ses max window", (long long)max_window_size);
    }
    else {
        max_window_size = (size_t) ret;
    }
}

static inline NETDATA_DOUBLE window(RRDR *r, struct grouping_ses *g) {
    (void)g;

    NETDATA_DOUBLE points;
    if(r->group == 1) {
        // provide a running DES
        points = (NETDATA_DOUBLE)r->internal.points_wanted;
    }
    else {
        // provide a SES with flush points
        points = (NETDATA_DOUBLE)r->group;
    }

    return (points > (NETDATA_DOUBLE)max_window_size) ? (NETDATA_DOUBLE)max_window_size : points;
}

static inline void set_alpha(RRDR *r, struct grouping_ses *g) {
    // https://en.wikipedia.org/wiki/Moving_average#Exponential_moving_average
    // A commonly used value for alpha is 2 / (N + 1)
    g->alpha = 2.0 / (window(r, g) + 1.0);
    g->alpha_other = 1.0 - g->alpha;
}

void grouping_create_ses(RRDR *r, const char *options __maybe_unused) {
    struct grouping_ses *g = (struct grouping_ses *)onewayalloc_callocz(r->internal.owa, 1, sizeof(struct grouping_ses));
    set_alpha(r, g);
    g->level = 0.0;
    r->internal.grouping_data = g;
}

// resets when switches dimensions
// so, clear everything to restart
void grouping_reset_ses(RRDR *r) {
    struct grouping_ses *g = (struct grouping_ses *)r->internal.grouping_data;
    g->level = 0.0;
    g->count = 0;
}

void grouping_free_ses(RRDR *r) {
    onewayalloc_freez(r->internal.owa, r->internal.grouping_data);
    r->internal.grouping_data = NULL;
}

void grouping_add_ses(RRDR *r, NETDATA_DOUBLE value) {
    struct grouping_ses *g = (struct grouping_ses *)r->internal.grouping_data;

    if(unlikely(!g->count))
        g->level = value;

    g->level = g->alpha * value + g->alpha_other * g->level;
    g->count++;
}

NETDATA_DOUBLE grouping_flush_ses(RRDR *r, RRDR_VALUE_FLAGS *rrdr_value_options_ptr) {
    struct grouping_ses *g = (struct grouping_ses *)r->internal.grouping_data;

    if(unlikely(!g->count || !netdata_double_isnumber(g->level))) {
        *rrdr_value_options_ptr |= RRDR_VALUE_EMPTY;
        return 0.0;
    }

    return g->level;
}
