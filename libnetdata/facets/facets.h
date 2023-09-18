#ifndef FACETS_H
#define FACETS_H 1

#include "../libnetdata.h"

#define FACET_VALUE_UNSET "-"

typedef enum __attribute__((packed)) {
    FACETS_ANCHOR_DIRECTION_FORWARD,
    FACETS_ANCHOR_DIRECTION_BACKWARD,
} FACETS_ANCHOR_DIRECTION;

typedef enum __attribute__((packed)) {
    FACET_KEY_OPTION_FACET          = (1 << 0), // filterable values
    FACET_KEY_OPTION_NO_FACET       = (1 << 1), // non-filterable value
    FACET_KEY_OPTION_NEVER_FACET    = (1 << 2), // never enable this field as facet
    FACET_KEY_OPTION_STICKY         = (1 << 3), // should be sticky in the table
    FACET_KEY_OPTION_VISIBLE        = (1 << 4), // should be in the default table
    FACET_KEY_OPTION_FTS            = (1 << 5), // the key is filterable by full text search (FTS)
    FACET_KEY_OPTION_MAIN_TEXT      = (1 << 6), // full width and wrap
    FACET_KEY_OPTION_RICH_TEXT      = (1 << 7),
    FACET_KEY_OPTION_REORDER        = (1 << 8), // give the key a new order id on first encounter
} FACET_KEY_OPTIONS;

typedef enum __attribute__((packed)) {
    FACET_ROW_SEVERITY_DEBUG,       // lowest - not important
    FACET_ROW_SEVERITY_NORMAL,      // the default
    FACET_ROW_SEVERITY_NOTICE,      // bold
    FACET_ROW_SEVERITY_WARNING,     // yellow + bold
    FACET_ROW_SEVERITY_CRITICAL,    // red + bold
} FACET_ROW_SEVERITY;

typedef struct facet_row_key_value {
    const char *tmp;
    BUFFER *wb;
    bool empty;
} FACET_ROW_KEY_VALUE;

typedef struct facet_row {
    usec_t usec;
    DICTIONARY *dict;
    FACET_ROW_SEVERITY severity;
    struct facet_row *prev, *next;
} FACET_ROW;

typedef struct facets FACETS;
typedef struct facet_key FACET_KEY;

#define FACET_STRING_HASH_SIZE 23
void facets_string_hash(const char *src, size_t len, char *out);

typedef void (*facets_key_transformer_t)(FACETS *facets __maybe_unused, BUFFER *wb, void *data);
typedef void (*facet_dynamic_row_t)(FACETS *facets, BUFFER *json_array, FACET_ROW_KEY_VALUE *rkv, FACET_ROW *row, void *data);
FACET_KEY *facets_register_dynamic_key_name(FACETS *facets, const char *key, FACET_KEY_OPTIONS options, facet_dynamic_row_t cb, void *data);
FACET_KEY *facets_register_key_name_transformation(FACETS *facets, const char *key, FACET_KEY_OPTIONS options, facets_key_transformer_t cb, void *data);

typedef enum __attribute__((packed)) {
    FACETS_OPTION_ALL_FACETS_VISIBLE    = (1 << 0), // all facets, should be visible by default in the table
    FACETS_OPTION_ALL_KEYS_FTS          = (1 << 1), // all keys are searchable by full text search
    FACETS_OPTION_DISABLE_ALL_FACETS    = (1 << 2),
    FACETS_OPTION_DISABLE_HISTOGRAM     = (1 << 3),
    FACETS_OPTION_DATA_ONLY             = (1 << 4),
} FACETS_OPTIONS;

FACETS *facets_create(uint32_t items_to_return, FACETS_OPTIONS options, const char *visible_keys, const char *facet_keys, const char *non_facet_keys);
void facets_destroy(FACETS *facets);

void facets_accepted_param(FACETS *facets, const char *param);

void facets_rows_begin(FACETS *facets);
void facets_row_finished(FACETS *facets, usec_t usec);

FACET_KEY *facets_register_key_name(FACETS *facets, const char *key, FACET_KEY_OPTIONS options);
void facets_set_query(FACETS *facets, const char *query);
void facets_set_items(FACETS *facets, uint32_t items);
void facets_set_anchor(FACETS *facets, usec_t anchor, FACETS_ANCHOR_DIRECTION direction);
FACET_KEY *facets_register_facet_id(FACETS *facets, const char *key_id, FACET_KEY_OPTIONS options);
void facets_register_facet_id_filter(FACETS *facets, const char *key_id, char *value_ids, FACET_KEY_OPTIONS options);
void facets_set_histogram(FACETS *facets, const char *chart, usec_t after_ut, usec_t before_ut);

void facets_add_key_value(FACETS *facets, const char *key, const char *value);
void facets_add_key_value_length(FACETS *facets, const char *key, size_t key_len, const char *value, size_t value_len);

void facets_report(FACETS *facets, BUFFER *wb);
void facets_accepted_parameters_to_json_array(FACETS *facets, BUFFER *wb, bool with_keys);
void facets_set_current_row_severity(FACETS *facets, FACET_ROW_SEVERITY severity);
void facets_data_only_mode(FACETS *facets);

#endif
