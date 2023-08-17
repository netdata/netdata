#ifndef FACETS_H
#define FACETS_H 1

#include "../libnetdata.h"

#define FACET_VALUE_UNSET "-"

typedef enum __attribute__((packed)) {
    FACET_KEY_OPTION_FACET      = (1 << 0), // filterable values
    FACET_KEY_OPTION_NO_FACET   = (1 << 1), // non-filterable value
    FACET_KEY_OPTION_STICKY     = (1 << 2), // should be sticky in the table
    FACET_KEY_OPTION_VISIBLE    = (1 << 3), // should be in the default table
    FACET_KEY_OPTION_FTS        = (1 << 4), // the key is filterable by full text search (FTS)
    FACET_KEY_OPTION_MAIN_TEXT  = (1 << 5), // full width and wrap
    FACET_KEY_OPTION_REORDER    = (1 << 6), // give the key a new order id on first encounter
} FACET_KEY_OPTIONS;

typedef struct facet_row_key_value {
    const char *tmp;
    BUFFER *wb;
} FACET_ROW_KEY_VALUE;

typedef struct facet_row {
    usec_t usec;
    DICTIONARY *dict;
    struct facet_row *prev, *next;
} FACET_ROW;

typedef struct facets FACETS;
typedef struct facet_key FACET_KEY;

#define FACET_STRING_HASH_SIZE 19
void facets_string_hash(const char *src, char *out);

typedef void (*facets_key_transformer_t)(FACETS *facets __maybe_unused, BUFFER *wb, void *data);
typedef void (*facet_dynamic_row_t)(FACETS *facets, BUFFER *json_array, FACET_ROW_KEY_VALUE *rkv, FACET_ROW *row, void *data);
FACET_KEY *facets_register_dynamic_key(FACETS *facets, const char *key, FACET_KEY_OPTIONS options, facet_dynamic_row_t cb, void *data);
FACET_KEY *facets_register_key_transformation(FACETS *facets, const char *key, FACET_KEY_OPTIONS options, facets_key_transformer_t cb, void *data);

typedef enum __attribute__((packed)) {
    FACETS_OPTION_ALL_FACETS_VISIBLE    = (1 << 0), // all facets, should be visible by default in the table
    FACETS_OPTION_ALL_KEYS_FTS          = (1 << 1), // all keys are searchable by full text search
} FACETS_OPTIONS;

FACETS *facets_create(uint32_t items_to_return, usec_t anchor, FACETS_OPTIONS options, const char *visible_keys, const char *facet_keys, const char *non_facet_keys);
void facets_destroy(FACETS *facets);

void facets_accepted_param(FACETS *facets, const char *param);

void facets_rows_begin(FACETS *facets);
void facets_row_finished(FACETS *facets, usec_t usec);

FACET_KEY *facets_register_key(FACETS *facets, const char *param, FACET_KEY_OPTIONS options);
void facets_set_query(FACETS *facets, const char *query);
void facets_set_items(FACETS *facets, uint32_t items);
void facets_set_anchor(FACETS *facets, usec_t anchor);
void facets_register_facet_filter(FACETS *facets, const char *key_id, char *value_ids, FACET_KEY_OPTIONS options);

void facets_add_key_value(FACETS *facets, const char *key, const char *value);
void facets_add_key_value_length(FACETS *facets, const char *key, const char *value, size_t value_len);

void facets_report(FACETS *facets, BUFFER *wb);

#endif
