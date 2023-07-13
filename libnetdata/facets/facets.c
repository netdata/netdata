#include "facets.h"

#define FACET_VALUE_UNSET "-"

// ----------------------------------------------------------------------------

#define FACET_HASH_SIZE 19

static inline void uint32_to_char(uint32_t num, char *out) {
    static char id_encoding_characters[64 + 1] = "ABCDEFGHIJKLMNOPQRSTUVWXYZ.abcdefghijklmnopqrstuvwxyz_0123456789";

    int i;
    for(i = 5; i >= 0; --i) {
        out[i] = id_encoding_characters[num & 63];
        num >>= 6;
    }
    out[6] = '\0';
}

static inline void hash_keys_and_values(const char *src, char *out) {
    uint32_t hash1 = fnv1a_hash32(src);
    uint32_t hash2 = djb2_hash32(src);
    uint32_t hash3 = larson_hash32(src);

    uint32_to_char(hash1, out);
    uint32_to_char(hash2, &out[6]);
    uint32_to_char(hash3, &out[12]);

    out[18] = '\0';
}

// ----------------------------------------------------------------------------

typedef struct facet_value {
    const char *name;

    bool selected;

    uint32_t rows_matching_facet_value;
    uint32_t final_facet_value_counter;
} FACET_VALUE;

typedef struct facet_key {
    const char *name;

    DICTIONARY *values;

    // members about the current row
    uint32_t key_found_in_row;
    uint32_t key_values_selected_in_row;
    BUFFER *current_value;
} FACET_KEY;

typedef struct facet_row_key_value {
    const char *tmp;
    BUFFER *wb;
} FACET_ROW_KEY_VALUE;

typedef struct facet_row {
    usec_t usec;
    DICTIONARY *dict;
    struct facet_row *prev, *next;
} FACET_ROW;

struct facets {
    SIMPLE_PATTERN *visible_keys;
    SIMPLE_PATTERN *excluded_keys;
    SIMPLE_PATTERN *included_keys;
    DICTIONARY *keys;

    usec_t anchor;
    FACET_ROW *base;    // double linked list of the selected facets rows

    uint32_t items_to_return;
    uint32_t max_items_to_return;

    struct {
        FACET_ROW *last_added;

        size_t evaluated;
        size_t matched;

        size_t first;
        size_t forwards;
        size_t backwards;
        size_t skips_before;
        size_t skips_after;
        size_t prepends;
        size_t appends;
        size_t shifts;
    } operations;
};

// ----------------------------------------------------------------------------

static inline void facet_value_is_used(FACET_KEY *k, FACET_VALUE *v) {
    if(!k->key_found_in_row)
        v->rows_matching_facet_value++;

    k->key_found_in_row++;

    if(v->selected)
        k->key_values_selected_in_row++;
}

static inline bool facets_key_is_filterable(FACETS *facets, const char *key) {
    bool included = true, excluded = false;

    if(facets->included_keys) {
        if (!simple_pattern_matches(facets->included_keys, key))
            included = false;
    }

    if(facets->excluded_keys) {
        if (simple_pattern_matches(facets->excluded_keys, key))
            excluded = true;
    }

    return included && !excluded;
}

// ----------------------------------------------------------------------------
// FACET_VALUE dictionary hooks

static void facet_value_insert_callback(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data) {
    FACET_VALUE *v = value;
    FACET_KEY *k = data;

    if(v->name) {
        // an actual value, not a filter
        v->name = strdupz(v->name);
        facet_value_is_used(k, v);
    }

    v->selected = true;
}

static bool facet_value_conflict_callback(const DICTIONARY_ITEM *item __maybe_unused, void *old_value, void *new_value, void *data) {
    FACET_VALUE *v = old_value;
    FACET_VALUE *nv = new_value;
    FACET_KEY *k = data;

    if(!v->name && nv->name)
        // an actual value, not a filter
        v->name = strdupz(nv->name);

    if(v->name)
        facet_value_is_used(k, v);

    return false;
}

static void facet_value_delete_callback(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused) {
    FACET_VALUE *v = value;
    freez((char *)v->name);
}

// ----------------------------------------------------------------------------
// FACET_KEY dictionary hooks

static inline void facet_key_late_init(FACETS *facets, FACET_KEY *k, const char *name) {
    k->name = strdupz(name);

    if(facets_key_is_filterable(facets, name)) {
        k->values = dictionary_create_advanced(
                DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE,
                NULL, sizeof(FACET_VALUE));
        dictionary_register_insert_callback(k->values, facet_value_insert_callback, k);
        dictionary_register_conflict_callback(k->values, facet_value_conflict_callback, k);
        dictionary_register_delete_callback(k->values, facet_value_delete_callback, k);
    }
}

static void facet_key_insert_callback(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data) {
    FACET_KEY *k = value;
    FACETS *facets = data;

    if(k->name)
        // an actual value, not a filter
        facet_key_late_init(facets, k, k->name);

    k->current_value = buffer_create(0, NULL);
}

static bool facet_key_conflict_callback(const DICTIONARY_ITEM *item __maybe_unused, void *old_value, void *new_value, void *data) {
    FACET_KEY *k = old_value;
    FACET_KEY *nk = new_value;
    FACETS *facets = data;

    if(!k->name && nk->name)
        // an actual value, not a filter
        facet_key_late_init(facets, k, nk->name);

    return false;
}

static void facet_key_delete_callback(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused) {
    FACET_KEY *k = value;
    dictionary_destroy(k->values);
    buffer_free(k->current_value);
    freez((char *)k->name);
}

// ----------------------------------------------------------------------------

FACETS *facets_create(uint32_t items_to_return, usec_t anchor, const char *visible_keys, const char *facet_keys, const char *non_facet_keys) {
    FACETS *facets = callocz(1, sizeof(FACETS));
    facets->keys = dictionary_create_advanced(DICT_OPTION_SINGLE_THREADED|DICT_OPTION_DONT_OVERWRITE_VALUE|DICT_OPTION_FIXED_SIZE, NULL, sizeof(FACET_KEY));
    dictionary_register_insert_callback(facets->keys, facet_key_insert_callback, facets);
    dictionary_register_conflict_callback(facets->keys, facet_key_conflict_callback, facets);
    dictionary_register_delete_callback(facets->keys, facet_key_delete_callback, facets);

    if(facet_keys && *facet_keys)
        facets->included_keys = simple_pattern_create(facet_keys, "|", SIMPLE_PATTERN_EXACT, true);

    if(non_facet_keys && *non_facet_keys)
        facets->excluded_keys = simple_pattern_create(non_facet_keys, "|", SIMPLE_PATTERN_EXACT, true);

    if(visible_keys && *visible_keys)
        facets->visible_keys = simple_pattern_create(visible_keys, "|", SIMPLE_PATTERN_EXACT, true);

    facets->max_items_to_return = items_to_return;
    facets->anchor = anchor;

    return facets;
}

void facets_destroy(FACETS *facets) {
    dictionary_destroy(facets->keys);
    simple_pattern_free(facets->visible_keys);
    simple_pattern_free(facets->included_keys);
    simple_pattern_free(facets->excluded_keys);
    freez(facets);
}

// ----------------------------------------------------------------------------

static inline void facets_check_value(FACETS *facets __maybe_unused, FACET_KEY *k) {
    if(buffer_strlen(k->current_value) == 0)
        buffer_strcat(k->current_value, FACET_VALUE_UNSET);

    if(k->values) {
        FACET_VALUE t = {
            .name = buffer_tostring(k->current_value),
        };
        char hash[FACET_HASH_SIZE];
        hash_keys_and_values(t.name, hash);
        dictionary_set(k->values, hash, &t, sizeof(t));
    }
    else {
        k->key_found_in_row++;
        k->key_values_selected_in_row++;
    }
}

void facets_add_key_value(FACETS *facets, const char *key, const char *value) {
    FACET_KEY t = {
            .name = key,
    };
    char hash[FACET_HASH_SIZE];
    hash_keys_and_values(t.name, hash);
    FACET_KEY *k = dictionary_set(facets->keys, hash, &t, sizeof(t));
    buffer_flush(k->current_value);
    buffer_strcat(k->current_value, value);

    facets_check_value(facets, k);
}

void facets_add_key_value_length(FACETS *facets, const char *key, const char *value, size_t value_len) {
    FACET_KEY t = {
            .name = key,
    };
    char hash[FACET_HASH_SIZE];
    hash_keys_and_values(t.name, hash);
    FACET_KEY *k = dictionary_set(facets->keys, hash, &t, sizeof(t));
    buffer_flush(k->current_value);
    buffer_strncat(k->current_value, value, value_len);

    facets_check_value(facets, k);
}

// ----------------------------------------------------------------------------
// FACET_ROW dictionary hooks

static void facet_row_key_value_insert_callback(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data) {
    FACET_ROW_KEY_VALUE *rkv = value;
    FACET_ROW *row = data; (void)row;

    rkv->wb = buffer_create(0, NULL);
    buffer_strcat(rkv->wb, rkv->tmp && *rkv->tmp ? rkv->tmp : FACET_VALUE_UNSET);
}

static bool facet_row_key_value_conflict_callback(const DICTIONARY_ITEM *item __maybe_unused, void *old_value, void *new_value, void *data) {
    FACET_ROW_KEY_VALUE *rkv = old_value;
    FACET_ROW_KEY_VALUE *n_rkv = new_value;
    FACET_ROW *row = data; (void)row;

    buffer_flush(rkv->wb);
    buffer_strcat(rkv->wb, n_rkv->tmp && *n_rkv->tmp ? n_rkv->tmp : FACET_VALUE_UNSET);

    return false;
}

static void facet_row_key_value_delete_callback(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data) {
    FACET_ROW_KEY_VALUE *rkv = value;
    FACET_ROW *row = data; (void)row;

    buffer_free(rkv->wb);
}

// ----------------------------------------------------------------------------
// FACET_ROW management

static void facets_row_free(FACETS *facets __maybe_unused, FACET_ROW *row) {
    dictionary_destroy(row->dict);
    freez(row);
}

static FACET_ROW *facets_row_create(FACETS *facets, usec_t usec, FACET_ROW *into) {
    FACET_ROW *row;

    if(into)
        row = into;
    else {
        row = callocz(1, sizeof(FACET_ROW));
        row->dict = dictionary_create_advanced(DICT_OPTION_SINGLE_THREADED|DICT_OPTION_DONT_OVERWRITE_VALUE|DICT_OPTION_FIXED_SIZE, NULL, sizeof(FACET_ROW_KEY_VALUE));
        dictionary_register_insert_callback(row->dict, facet_row_key_value_insert_callback, row);
        dictionary_register_conflict_callback(row->dict, facet_row_key_value_conflict_callback, row);
        dictionary_register_delete_callback(row->dict, facet_row_key_value_delete_callback, row);
    }

    row->usec = usec;

    FACET_KEY *k;
    dfe_start_read(facets->keys, k) {
        FACET_ROW_KEY_VALUE t = {
                .tmp = buffer_strlen(k->current_value) ? buffer_tostring(k->current_value) : FACET_VALUE_UNSET,
                .wb = NULL,
        };
        dictionary_set(row->dict, k->name, &t, sizeof(t));
    }
    dfe_done(k);

    return row;
}

// ----------------------------------------------------------------------------

static void facets_row_keep(FACETS *facets, usec_t usec) {
    facets->operations.matched++;

    if(usec < facets->anchor) {
        facets->operations.skips_before++;
        return;
    }

    if(unlikely(!facets->base)) {
        facets->operations.last_added = facets_row_create(facets, usec, NULL);
        DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(facets->base, facets->operations.last_added, prev, next);
        facets->items_to_return++;
        facets->operations.first++;
        return;
    }

    if(likely(usec > facets->base->prev->usec))
        facets->operations.last_added = facets->base->prev;

    FACET_ROW *last = facets->operations.last_added;
    while(last->prev != facets->base->prev && usec > last->prev->usec) {
        last = last->prev;
        facets->operations.backwards++;
    }

    while(last->next && usec < last->next->usec) {
        last = last->next;
        facets->operations.forwards++;
    }

    if(facets->items_to_return >= facets->max_items_to_return) {
        if(last == facets->base->prev && usec < last->usec) {
            facets->operations.skips_after++;
            return;
        }
    }

    facets->items_to_return++;

    if(usec > last->usec) {
        if(facets->items_to_return > facets->max_items_to_return) {
            facets->items_to_return--;
            facets->operations.shifts++;
            facets->operations.last_added = facets->base->prev;
            DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(facets->base, facets->operations.last_added, prev, next);
            facets->operations.last_added = facets_row_create(facets, usec, facets->operations.last_added);
        }
        DOUBLE_LINKED_LIST_PREPEND_ITEM_UNSAFE(facets->base, facets->operations.last_added, prev, next);
        facets->operations.prepends++;
    }
    else {
        facets->operations.last_added = facets_row_create(facets, usec, NULL);
        DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(facets->base, facets->operations.last_added, prev, next);
        facets->operations.appends++;
    }

    while(facets->items_to_return > facets->max_items_to_return) {
        // we have to remove something

        FACET_ROW *tmp = facets->base->prev;
        DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(facets->base, tmp, prev, next);
        facets->items_to_return--;

        if(unlikely(facets->operations.last_added == tmp))
            facets->operations.last_added = facets->base->prev;

        facets_row_free(facets, tmp);
        facets->operations.shifts++;
    }
}

void facets_rows_begin(FACETS *facets) {
    FACET_KEY *k;
    dfe_start_read(facets->keys, k) {
                k->key_found_in_row = 0;
                k->key_values_selected_in_row = 0;
                buffer_flush(k->current_value);
            }
    dfe_done(k);
}

void facets_row_finished(FACETS *facets, usec_t usec) {
    facets->operations.evaluated++;

    uint32_t total_keys = 0;
    uint32_t selected_by = 0;

    FACET_KEY *k;
    dfe_start_read(facets->keys, k) {
        if(!k->key_found_in_row) {
            internal_fatal(buffer_strlen(k->current_value), "key is not found in row but it has a current value");
            // put the FACET_VALUE_UNSET value into it
            facets_check_value(facets, k);
        }

        internal_fatal(!k->key_found_in_row, "all keys should be found in the row at this point");
        internal_fatal(k->key_found_in_row != 1, "all keys should be matched exactly once at this point");
        internal_fatal(k->key_values_selected_in_row > 1, "key values are selected in row more than once");

        k->key_found_in_row = 1;
        k->key_values_selected_in_row = k->key_values_selected_in_row == 0 ? 0 : 1;

        total_keys += k->key_found_in_row;
        selected_by += k->key_values_selected_in_row;
    }
    dfe_done(k);

    if(selected_by >= total_keys - 1) {
        uint32_t found = 0;

        dfe_start_read(facets->keys, k){
            uint32_t counted_by = selected_by;

            if (counted_by != total_keys) {
                counted_by = 0;

                FACET_KEY *m;
                dfe_start_read(facets->keys, m) {
                    if(k == m || m->key_values_selected_in_row)
                        counted_by++;
                }
                dfe_done(m);
            }

            if(counted_by == total_keys) {
                if(k->values) {
                    char hash[FACET_HASH_SIZE];
                    hash_keys_and_values(buffer_tostring(k->current_value), hash);
                    FACET_VALUE *v = dictionary_get(k->values, hash);
                    v->final_facet_value_counter++;
                }

                found++;
            }
        }
        dfe_done(k);

        internal_fatal(!found, "We should find at least one facet to count this row");
        (void)found;
    }

    if(selected_by == total_keys)
        facets_row_keep(facets, usec);

    facets_rows_begin(facets);
}

// ----------------------------------------------------------------------------
// output

void facets_report(FACETS *facets, BUFFER *wb) {
    buffer_json_member_add_array(wb, "facets");
    {
        FACET_KEY *k;
        dfe_start_read(facets->keys, k) {
            if(!k->values)
                continue;

            buffer_json_add_array_item_object(wb); // key
            {
                buffer_json_member_add_string(wb, "id", k_dfe.name);
                buffer_json_member_add_string(wb, "name", k->name);
                buffer_json_member_add_uint64(wb, "order", k_dfe.counter);
                buffer_json_member_add_array(wb, "options");
                {
                    FACET_VALUE *v;
                    dfe_start_read(k->values, v) {
                        buffer_json_add_array_item_object(wb);
                        {
                            buffer_json_member_add_string(wb, "id", v_dfe.name);
                            buffer_json_member_add_string(wb, "name", v->name);
                            buffer_json_member_add_uint64(wb, "count", v->final_facet_value_counter);
                        }
                        buffer_json_object_close(wb);
                    }
                    dfe_done(v);
                }
                buffer_json_array_close(wb); // options
            }
            buffer_json_object_close(wb); // key
        }
        dfe_done(k);
    }
    buffer_json_array_close(wb); // facets

    buffer_json_member_add_object(wb, "columns");
    {
        size_t field_id = 0;
        buffer_rrdf_table_add_field(
                wb, field_id++,
                "timestamp", "Timestamp",
                RRDF_FIELD_TYPE_TIMESTAMP,
                RRDF_FIELD_VISUAL_VALUE,
                RRDF_FIELD_TRANSFORM_DATETIME, 0, NULL, NAN,
                RRDF_FIELD_SORT_DESCENDING,
                NULL,
                RRDF_FIELD_SUMMARY_COUNT,
                RRDF_FIELD_FILTER_RANGE,
                RRDF_FIELD_OPTS_VISIBLE | RRDF_FIELD_OPTS_STICKY | RRDF_FIELD_OPTS_UNIQUE_KEY,
                NULL);

        FACET_KEY *k;
        dfe_start_read(facets->keys, k) {
            bool visible = false;

            if(!facets->visible_keys)
                visible = k->values ? true : false;
            else
                visible = simple_pattern_matches(facets->visible_keys, k->name);

            buffer_rrdf_table_add_field(
                    wb, field_id++,
                    k_dfe.name, k->name ? k->name : k_dfe.name,
                    RRDF_FIELD_TYPE_STRING,
                    RRDF_FIELD_VISUAL_VALUE,
                    RRDF_FIELD_TRANSFORM_NONE, 0, NULL, NAN,
                    RRDF_FIELD_SORT_ASCENDING,
                    NULL,
                    RRDF_FIELD_SUMMARY_COUNT,
                    k->values ? RRDF_FIELD_FILTER_FACET : RRDF_FIELD_FILTER_NONE,
                    visible ? RRDF_FIELD_OPTS_VISIBLE : RRDF_FIELD_OPTS_NONE,
                    FACET_VALUE_UNSET);
        }
        dfe_done(k);
    }
    buffer_json_object_close(wb); // columns

    buffer_json_member_add_array(wb, "data");
    {
        for(FACET_ROW *row = facets->base ; row ;row = row->next) {
            buffer_json_add_array_item_array(wb); // each row
            buffer_json_add_array_item_time_t(wb, row->usec / USEC_PER_SEC);

            FACET_KEY *k;
            dfe_start_read(facets->keys, k)
            {
                FACET_ROW_KEY_VALUE *rkv = dictionary_get(row->dict, k->name);
                buffer_json_add_array_item_string(wb, rkv ? buffer_tostring(rkv->wb) : FACET_VALUE_UNSET);
            }
            dfe_done(k);
            buffer_json_array_close(wb); // each row
        }
    }
    buffer_json_array_close(wb); // data

    buffer_json_member_add_string(wb, "default_sort_column", "timestamp");
    buffer_json_member_add_array(wb, "default_charts");
    buffer_json_array_close(wb);

    buffer_json_member_add_object(wb, "items");
    {
        buffer_json_member_add_uint64(wb, "evaluated", facets->operations.evaluated);
        buffer_json_member_add_uint64(wb, "matched", facets->operations.matched);
        buffer_json_member_add_uint64(wb, "returned", facets->items_to_return);
        buffer_json_member_add_uint64(wb, "max_to_return", facets->max_items_to_return);
        buffer_json_member_add_uint64(wb, "before", facets->operations.skips_before);
        buffer_json_member_add_uint64(wb, "after", facets->operations.skips_after + facets->operations.shifts);
    }
    buffer_json_object_close(wb); // items

    buffer_json_member_add_object(wb, "stats");
    {
        buffer_json_member_add_uint64(wb, "first", facets->operations.first);
        buffer_json_member_add_uint64(wb, "forwards", facets->operations.forwards);
        buffer_json_member_add_uint64(wb, "backwards", facets->operations.backwards);
        buffer_json_member_add_uint64(wb, "skips_before", facets->operations.skips_before);
        buffer_json_member_add_uint64(wb, "skips_after", facets->operations.skips_after);
        buffer_json_member_add_uint64(wb, "prepends", facets->operations.prepends);
        buffer_json_member_add_uint64(wb, "appends", facets->operations.appends);
        buffer_json_member_add_uint64(wb, "shifts", facets->operations.shifts);
    }
    buffer_json_object_close(wb); // items

}
