// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_API_QUERIES_SES_H
#define NETDATA_API_QUERIES_SES_H

#include "../query.h"
#include "../rrdr.h"

struct tg_ses {
    NETDATA_DOUBLE alpha;
    NETDATA_DOUBLE alpha_other;
    NETDATA_DOUBLE level;
    size_t count;
};

static size_t tg_ses_max_window_size = 15;

static inline void tg_ses_init(void) {
    long long ret = inicfg_get_number(&netdata_config, CONFIG_SECTION_WEB, "ses max tg_des_window", (long long)tg_ses_max_window_size);
    if(ret <= 1) {
        inicfg_set_number(&netdata_config, CONFIG_SECTION_WEB, "ses max tg_des_window", (long long)tg_ses_max_window_size);
    }
    else {
        tg_ses_max_window_size = (size_t) ret;
    }
}

static inline NETDATA_DOUBLE tg_ses_window(RRDR *r, struct tg_ses *g) {
    (void)g;

    NETDATA_DOUBLE points;
    if(r->view.group == 1) {
        // provide a running DES
        points = (NETDATA_DOUBLE)r->time_grouping.points_wanted;
    }
    else {
        // provide a SES with flush points
        points = (NETDATA_DOUBLE)r->view.group;
    }

    return (points > (NETDATA_DOUBLE)tg_ses_max_window_size) ? (NETDATA_DOUBLE)tg_ses_max_window_size : points;
}

static inline void tg_ses_set_alpha(RRDR *r, struct tg_ses *g) {
    // https://en.wikipedia.org/wiki/Moving_average#Exponential_moving_average
    // A commonly used value for alpha is 2 / (N + 1)
    g->alpha = 2.0 / (tg_ses_window(r, g) + 1.0);
    g->alpha_other = 1.0 - g->alpha;
}

static inline void tg_ses_create(RRDR *r, const char *options __maybe_unused) {
    struct tg_ses *g = (struct tg_ses *)onewayalloc_callocz(r->internal.owa, 1, sizeof(struct tg_ses));
    tg_ses_set_alpha(r, g);
    g->level = 0.0;
    r->time_grouping.data = g;
}

// resets when switches dimensions
// so, clear everything to restart
static inline void tg_ses_reset(RRDR *r) {
    struct tg_ses *g = (struct tg_ses *)r->time_grouping.data;
    g->level = 0.0;
    g->count = 0;
}

static inline void tg_ses_free(RRDR *r) {
    onewayalloc_freez(r->internal.owa, r->time_grouping.data);
    r->time_grouping.data = NULL;
}

static inline void tg_ses_add(RRDR *r, NETDATA_DOUBLE value) {
    struct tg_ses *g = (struct tg_ses *)r->time_grouping.data;

    if(unlikely(!g->count))
        g->level = value;

    g->level = g->alpha * value + g->alpha_other * g->level;
    g->count++;
}

static inline NETDATA_DOUBLE tg_ses_flush(RRDR *r, RRDR_VALUE_FLAGS *rrdr_value_options_ptr) {
    struct tg_ses *g = (struct tg_ses *)r->time_grouping.data;

    if(unlikely(!g->count || !netdata_double_isnumber(g->level))) {
        *rrdr_value_options_ptr |= RRDR_VALUE_EMPTY;
        return 0.0;
    }

    return g->level;
}

#endif //NETDATA_API_QUERIES_SES_H
