#ifndef FACETS_H
#define FACETS_H 1

#include "../libnetdata.h"

typedef struct facets FACETS;
typedef struct facet_key FACET_KEY;

typedef enum __attribute__((packed)) {
    FACETS_OPTION_ALL_FACETS_VISIBLE    = (1 << 0), // all facets, should be visible by default in the table
} FACETS_OPTIONS;

FACETS *facets_create(uint32_t items_to_return, usec_t anchor, FACETS_OPTIONS options, const char *visible_keys, const char *facet_keys, const char *non_facet_keys);
void facets_destroy(FACETS *facets);

void facets_accepted_param(FACETS *facets, const char *param);

void facets_rows_begin(FACETS *facets);
void facets_row_finished(FACETS *facets, usec_t usec);

typedef enum __attribute__((packed)) {
    FACET_KEY_OPTION_FACET    = (1 << 0), // filterable values
    FACET_KEY_OPTION_NO_FACET = (1 << 1), // non-filterable value
    FACET_KEY_OPTION_STICKY   = (1 << 2), // should be sticky in the table
    FACET_KEY_OPTION_VISIBLE  = (1 << 3), // should be in the default table
} FACET_KEY_OPTIONS;

FACET_KEY *facets_register_key(FACETS *facets, const char *param, FACET_KEY_OPTIONS options);

void facets_add_key_value(FACETS *facets, const char *key, const char *value);
void facets_add_key_value_length(FACETS *facets, const char *key, const char *value, size_t value_len);

void facets_report(FACETS *facets, BUFFER *wb);

#endif
