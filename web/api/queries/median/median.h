// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_API_QUERIES_MEDIAN_H
#define NETDATA_API_QUERIES_MEDIAN_H

#include "../query.h"
#include "../rrdr.h"

struct tg_median {
    size_t series_size;
    size_t next_pos;
    NETDATA_DOUBLE percent;

    NETDATA_DOUBLE *series;
};

static inline void tg_median_create_internal(RRDR *r, const char *options, NETDATA_DOUBLE def) {
    long entries = r->view.group;
    if(entries < 10) entries = 10;

    struct tg_median *g = (struct tg_median *)onewayalloc_callocz(r->internal.owa, 1, sizeof(struct tg_median));
    g->series = onewayalloc_mallocz(r->internal.owa, entries * sizeof(NETDATA_DOUBLE));
    g->series_size = (size_t)entries;

    g->percent = def;
    if(options && *options) {
        g->percent = str2ndd(options, NULL);
        if(!netdata_double_isnumber(g->percent)) g->percent = 0.0;
        if(g->percent < 0.0) g->percent = 0.0;
        if(g->percent > 50.0) g->percent = 50.0;
    }

    g->percent = g->percent / 100.0;
    r->time_grouping.data = g;
}

static inline void tg_median_create(RRDR *r, const char *options) {
    tg_median_create_internal(r, options, 0.0);
}
static inline void tg_median_create_trimmed_1(RRDR *r, const char *options) {
    tg_median_create_internal(r, options, 1.0);
}
static inline void tg_median_create_trimmed_2(RRDR *r, const char *options) {
    tg_median_create_internal(r, options, 2.0);
}
static inline void tg_median_create_trimmed_3(RRDR *r, const char *options) {
    tg_median_create_internal(r, options, 3.0);
}
static inline void tg_median_create_trimmed_5(RRDR *r, const char *options) {
    tg_median_create_internal(r, options, 5.0);
}
static inline void tg_median_create_trimmed_10(RRDR *r, const char *options) {
    tg_median_create_internal(r, options, 10.0);
}
static inline void tg_median_create_trimmed_15(RRDR *r, const char *options) {
    tg_median_create_internal(r, options, 15.0);
}
static inline void tg_median_create_trimmed_20(RRDR *r, const char *options) {
    tg_median_create_internal(r, options, 20.0);
}
static inline void tg_median_create_trimmed_25(RRDR *r, const char *options) {
    tg_median_create_internal(r, options, 25.0);
}

// resets when switches dimensions
// so, clear everything to restart
static inline void tg_median_reset(RRDR *r) {
    struct tg_median *g = (struct tg_median *)r->time_grouping.data;
    g->next_pos = 0;
}

static inline void tg_median_free(RRDR *r) {
    struct tg_median *g = (struct tg_median *)r->time_grouping.data;
    if(g) onewayalloc_freez(r->internal.owa, g->series);

    onewayalloc_freez(r->internal.owa, r->time_grouping.data);
    r->time_grouping.data = NULL;
}

static inline void tg_median_add(RRDR *r, NETDATA_DOUBLE value) {
    struct tg_median *g = (struct tg_median *)r->time_grouping.data;

    if(unlikely(g->next_pos >= g->series_size)) {
        g->series = onewayalloc_doublesize( r->internal.owa, g->series, g->series_size * sizeof(NETDATA_DOUBLE));
        g->series_size *= 2;
    }

    g->series[g->next_pos++] = value;
}

static inline NETDATA_DOUBLE tg_median_flush(RRDR *r, RRDR_VALUE_FLAGS *rrdr_value_options_ptr) {
    struct tg_median *g = (struct tg_median *)r->time_grouping.data;

    size_t available_slots = g->next_pos;
    NETDATA_DOUBLE value;

    if(unlikely(!available_slots)) {
        value = 0.0;
        *rrdr_value_options_ptr |= RRDR_VALUE_EMPTY;
    }
    else if(available_slots == 1) {
        value = g->series[0];
    }
    else {
        sort_series(g->series, available_slots);

        size_t start_slot = 0;
        size_t end_slot = available_slots - 1;

        if(g->percent > 0.0) {
            NETDATA_DOUBLE min = g->series[0];
            NETDATA_DOUBLE max = g->series[available_slots - 1];
            NETDATA_DOUBLE delta = (max - min) * g->percent;

            NETDATA_DOUBLE wanted_min = min + delta;
            NETDATA_DOUBLE wanted_max = max - delta;

            for (start_slot = 0; start_slot < available_slots; start_slot++)
                if (g->series[start_slot] >= wanted_min) break;

            for (end_slot = available_slots - 1; end_slot > start_slot; end_slot--)
                if (g->series[end_slot] <= wanted_max) break;
        }

        if(start_slot == end_slot)
            value = g->series[start_slot];
        else
            value = median_on_sorted_series(&g->series[start_slot], end_slot - start_slot + 1);
    }

    if(unlikely(!netdata_double_isnumber(value))) {
        value = 0.0;
        *rrdr_value_options_ptr |= RRDR_VALUE_EMPTY;
    }

    //log_series_to_stderr(g->series, g->next_pos, value, "median");

    g->next_pos = 0;

    return value;
}

#endif //NETDATA_API_QUERIES_MEDIAN_H
