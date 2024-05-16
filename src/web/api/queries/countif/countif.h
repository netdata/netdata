// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_API_QUERY_COUNTIF_H
#define NETDATA_API_QUERY_COUNTIF_H

#include "../query.h"
#include "../rrdr.h"

enum tg_countif_cmp {
    TG_COUNTIF_EQUAL,
    TG_COUNTIF_NOTEQUAL,
    TG_COUNTIF_LESS,
    TG_COUNTIF_LESSEQUAL,
    TG_COUNTIF_GREATER,
    TG_COUNTIF_GREATEREQUAL,
};

struct tg_countif {
    enum tg_countif_cmp comparison;
    NETDATA_DOUBLE target;
    size_t count;
    size_t matched;
};

static inline void tg_countif_create(RRDR *r, const char *options __maybe_unused) {
    struct tg_countif *g = onewayalloc_callocz(r->internal.owa, 1, sizeof(struct tg_countif));
    r->time_grouping.data = g;

    if(options && *options) {
        // skip any leading spaces
        while(isspace((uint8_t)*options)) options++;

        // find the comparison function
        switch(*options) {
            case '!':
                options++;
                if(*options != '=' && *options != ':')
                    options--;
                g->comparison = TG_COUNTIF_NOTEQUAL;
                break;

            case '>':
                options++;
                if(*options == '=' || *options == ':') {
                    g->comparison = TG_COUNTIF_GREATEREQUAL;
                }
                else {
                    options--;
                    g->comparison = TG_COUNTIF_GREATER;
                }
                break;

            case '<':
                options++;
                if(*options == '>') {
                    g->comparison = TG_COUNTIF_NOTEQUAL;
                }
                else if(*options == '=' || *options == ':') {
                    g->comparison = TG_COUNTIF_LESSEQUAL;
                }
                else {
                    options--;
                    g->comparison = TG_COUNTIF_LESS;
                }
                break;

            default:
            case '=':
            case ':':
                g->comparison = TG_COUNTIF_EQUAL;
                break;
        }
        if(*options) options++;

        // skip everything up to the first digit
        while(isspace((uint8_t)*options)) options++;

        g->target = str2ndd(options, NULL);
    }
    else {
        g->target = 0.0;
        g->comparison = TG_COUNTIF_EQUAL;
    }
}

// resets when switches dimensions
// so, clear everything to restart
static inline void tg_countif_reset(RRDR *r) {
    struct tg_countif *g = (struct tg_countif *)r->time_grouping.data;
    g->matched = 0;
    g->count = 0;
}

static inline void tg_countif_free(RRDR *r) {
    onewayalloc_freez(r->internal.owa, r->time_grouping.data);
    r->time_grouping.data = NULL;
}

static inline void tg_countif_add(RRDR *r, NETDATA_DOUBLE value) {
    struct tg_countif *g = (struct tg_countif *)r->time_grouping.data;
    switch(g->comparison) {
        case TG_COUNTIF_GREATER:
            if(value > g->target) g->matched++;
            break;

        case TG_COUNTIF_GREATEREQUAL:
            if(value >= g->target) g->matched++;
            break;

        case TG_COUNTIF_LESS:
            if(value < g->target) g->matched++;
            break;

        case TG_COUNTIF_LESSEQUAL:
            if(value <= g->target) g->matched++;
            break;

        case TG_COUNTIF_EQUAL:
            if(value == g->target) g->matched++;
            break;

        case TG_COUNTIF_NOTEQUAL:
            if(value != g->target) g->matched++;
            break;
    }
    g->count++;
}

static inline NETDATA_DOUBLE tg_countif_flush(RRDR *r, RRDR_VALUE_FLAGS *rrdr_value_options_ptr) {
    struct tg_countif *g = (struct tg_countif *)r->time_grouping.data;

    NETDATA_DOUBLE value;

    if(unlikely(!g->count)) {
        value = 0.0;
        *rrdr_value_options_ptr |= RRDR_VALUE_EMPTY;
    }
    else {
        value = (NETDATA_DOUBLE)g->matched * 100 / (NETDATA_DOUBLE)g->count;
    }

    g->matched = 0;
    g->count = 0;

    return value;
}

#endif //NETDATA_API_QUERY_COUNTIF_H
