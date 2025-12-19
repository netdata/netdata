// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef FACETS_H
#define FACETS_H 1

#include "../libnetdata.h"

#define FACET_VALUE_UNSET "-"
#define FACET_VALUE_UNSAMPLED "[unsampled]"
#define FACET_VALUE_ESTIMATED "[estimated]"

typedef enum __attribute__((packed)) {
    FACETS_ANCHOR_DIRECTION_FORWARD,
    FACETS_ANCHOR_DIRECTION_BACKWARD,
} FACETS_ANCHOR_DIRECTION;

typedef enum __attribute__((packed)) {
    FACETS_TRANSFORM_VALUE,
    FACETS_TRANSFORM_HISTOGRAM,
    FACETS_TRANSFORM_FACET,
    FACETS_TRANSFORM_DATA,
    FACETS_TRANSFORM_FACET_SORT,
} FACETS_TRANSFORMATION_SCOPE;

typedef enum __attribute__((packed)) {
    FACET_KEY_OPTION_NONE           = 0,
    FACET_KEY_OPTION_FACET          = (1 << 0), // filterable values
    FACET_KEY_OPTION_NO_FACET       = (1 << 1), // non-filterable value
    FACET_KEY_OPTION_NEVER_FACET    = (1 << 2), // never enable this field as facet
    FACET_KEY_OPTION_STICKY         = (1 << 3), // should be sticky in the table
    FACET_KEY_OPTION_VISIBLE        = (1 << 4), // should be in the default table
    FACET_KEY_OPTION_FTS            = (1 << 5), // the key is filterable by full text search (FTS)
    FACET_KEY_OPTION_MAIN_TEXT      = (1 << 6), // full width and wrap
    FACET_KEY_OPTION_RICH_TEXT      = (1 << 7),
    FACET_KEY_OPTION_REORDER        = (1 << 8), // give the key a new order id on first encounter
    FACET_KEY_OPTION_REORDER_DONE   = (1 << 9), // done re-ordering for this field
    FACET_KEY_OPTION_TRANSFORM_VIEW = (1 << 10), // when registering the transformation, do it only at the view, not on all data
    FACET_KEY_OPTION_EXPANDED_FILTER = (1 << 11), // the presentation should have this filter expanded by default
    FACET_KEY_OPTION_PRETTY_XML     = (1 << 12), // instruct the UI to parse this as an XML document
    FACET_KEY_OPTION_HIDDEN         = (1 << 13), // do not include this field in the response
    FACET_KEY_OPTION_FILTER_ONLY    = (1 << 14), // the key is filterable, but not to be exposed as facet
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
    uint32_t tmp_len;
    bool empty;
    BUFFER *wb;
} FACET_ROW_KEY_VALUE;

typedef struct facet_row_bin_data {
    void (*cleanup_cb)(void *data);
    void *data;
} FACET_ROW_BIN_DATA;

#define FACET_ROW_BIN_DATA_EMPTY (FACET_ROW_BIN_DATA){.data = NULL, .cleanup_cb = NULL}

typedef struct facet_row {
    usec_t usec;
    DICTIONARY *dict;
    FACET_ROW_SEVERITY severity;
    FACET_ROW_BIN_DATA bin_data;
    struct facet_row *prev, *next;
} FACET_ROW;

typedef struct facets FACETS;
typedef struct facet_key FACET_KEY;

typedef void (*facets_key_transformer_t)(FACETS *facets __maybe_unused, BUFFER *wb, FACETS_TRANSFORMATION_SCOPE scope, void *data);
typedef void (*facet_dynamic_row_t)(FACETS *facets, BUFFER *json_array, FACET_ROW_KEY_VALUE *rkv, FACET_ROW *row, void *data);
typedef FACET_ROW_SEVERITY (*facet_row_severity_t)(FACETS *facets, FACET_ROW *row, void *data);
FACET_KEY *facets_register_dynamic_key_name(FACETS *facets, const char *key, FACET_KEY_OPTIONS options, facet_dynamic_row_t cb, void *data);
FACET_KEY *facets_register_key_name_transformation(FACETS *facets, const char *key, FACET_KEY_OPTIONS options, facets_key_transformer_t cb, void *data);
void facets_register_row_severity(FACETS *facets, facet_row_severity_t cb, void *data);

typedef enum __attribute__((packed)) {
    FACETS_OPTION_ALL_FACETS_VISIBLE            = (1 << 0), // all facets should be visible by default in the table
    FACETS_OPTION_ALL_KEYS_FTS                  = (1 << 1), // all keys are searchable by full text search
    FACETS_OPTION_DONT_SEND_FACETS              = (1 << 2), // "facets" object will not be included in the report
    FACETS_OPTION_DONT_SEND_HISTOGRAM           = (1 << 3), // "histogram" object will not be included in the report
    FACETS_OPTION_DATA_ONLY                     = (1 << 4),
    FACETS_OPTION_DONT_SEND_EMPTY_VALUE_FACETS  = (1 << 5), // empty facet values will not be included in the report
    FACETS_OPTION_SORT_FACETS_ALPHABETICALLY    = (1 << 6),
    FACETS_OPTION_SHOW_DELTAS                   = (1 << 7),
    FACETS_OPTION_HASH_IDS                      = (1 << 8), // when set, the id of the facets, keys and values will be their hash
} FACETS_OPTIONS;

FACETS *facets_create(uint32_t items_to_return, FACETS_OPTIONS options, const char *visible_keys, const char *facet_keys, const char *non_facet_keys);
void facets_destroy(FACETS *facets);

void facets_accepted_param(FACETS *facets, const char *param);

void facets_rows_begin(FACETS *facets);
bool facets_row_finished(FACETS *facets, usec_t usec);

void facets_row_finished_unsampled(FACETS *facets, usec_t usec);
void facets_update_estimations(FACETS *facets, usec_t from_ut, usec_t to_ut, size_t entries);
size_t facets_histogram_slots(FACETS *facets);

FACET_KEY *facets_register_key_name(FACETS *facets, const char *key, FACET_KEY_OPTIONS options);
void facets_set_query(FACETS *facets, const char *query);
void facets_set_items(FACETS *facets, uint32_t items);
void facets_set_anchor(FACETS *facets, usec_t start_ut, usec_t stop_ut, FACETS_ANCHOR_DIRECTION direction);
void facets_enable_slice_mode(FACETS *facets);
bool facets_row_candidate_to_keep(FACETS *facets, usec_t usec);

void facets_reset_and_disable_all_facets(FACETS *facets);

FACET_KEY *facets_register_facet(FACETS *facets, const char *name, FACET_KEY_OPTIONS options);
FACET_KEY *facets_register_facet_id(FACETS *facets, const char *key_id, FACET_KEY_OPTIONS options);

void facets_register_facet_filter(FACETS *facets, const char *key, const char *value, FACET_KEY_OPTIONS options);
void facets_register_facet_filter_id(FACETS *facets, const char *key_id, const char *value_id, FACET_KEY_OPTIONS options);

void facets_set_timeframe_and_histogram_by_id(FACETS *facets, const char *key_id, usec_t after_ut, usec_t before_ut);
void facets_set_timeframe_and_histogram_by_name(FACETS *facets, const char *key_name, usec_t after_ut, usec_t before_ut);

void facets_add_key_value(FACETS *facets, const char *key, const char *value);
void facets_add_key_value_length(FACETS *facets, const char *key, size_t key_len, const char *value, size_t value_len);

void facets_report(FACETS *facets, BUFFER *wb, DICTIONARY *used_hashes_registry);
void facets_accepted_parameters_to_json_array(FACETS *facets, BUFFER *wb, bool with_keys);
void facets_set_current_row_severity(FACETS *facets, FACET_ROW_SEVERITY severity);
void facets_set_additional_options(FACETS *facets, FACETS_OPTIONS options);

bool facets_key_name_is_filter(FACETS *facets, const char *key);
bool facets_key_name_is_facet(FACETS *facets, const char *key);
bool facets_key_name_value_length_is_selected(FACETS *facets, const char *key, size_t key_length, const char *value, size_t value_length);
void facets_add_possible_value_name_to_key(FACETS *facets, const char *key, size_t key_length, const char *value, size_t value_length);

void facets_sort_and_reorder_keys(FACETS *facets, DICTIONARY *column_order_registry);
usec_t facets_row_oldest_ut(FACETS *facets);
usec_t facets_row_newest_ut(FACETS *facets);
uint32_t facets_rows(FACETS *facets);

void facets_table_config(FACETS *facets, BUFFER *wb);

const char *facets_severity_to_string(FACET_ROW_SEVERITY severity);

typedef bool (*facets_foreach_selected_value_in_key_t)(FACETS *facets, size_t id, const char *key, const char *value, void *data);
bool facets_foreach_selected_value_in_key(FACETS *facets, const char *key, size_t key_length, DICTIONARY *used_hashes_registry, facets_foreach_selected_value_in_key_t cb, void *data);

void facets_row_bin_data_set(FACETS *facets, void (*cleanup_cb)(void *data), void *data);
void *facets_row_bin_data_get(FACETS *facets __maybe_unused, FACET_ROW *row);

void facets_use_hashes_for_ids(FACETS *facets, bool set);

#endif
