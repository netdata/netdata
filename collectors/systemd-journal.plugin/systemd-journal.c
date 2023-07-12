// SPDX-License-Identifier: GPL-3.0-or-later
/*
 *  netdata systemd-journal.plugin
 *  Copyright (C) 2023 Netdata Inc.
 *  GPL v3+
 */

#include "libnetdata/libnetdata.h"
#include "libnetdata/required_dummies.h"

#include <systemd/sd-journal.h>

// ----------------------------------------------------------------------------

typedef struct facet_value {
    bool selected;

    uint32_t rows_matching_facet_value;
} FACET_VALUE;

typedef struct facet_key {
    DICTIONARY *values;

    // global statistics
    uint32_t rows_matched;

    // members about the current row
    SIMPLE_PATTERN *selected_values_pattern;
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

typedef struct facets {
    SIMPLE_PATTERN *excluded_keys;
    SIMPLE_PATTERN *included_keys;
    DICTIONARY *keys;

    usec_t anchor;
    size_t having_rows;
    FACET_ROW *base;    // double linked list of the selected facets rows

    uint32_t items_to_return;
    uint32_t max_items_to_return;
    uint32_t item_matched;

    struct {
        FACET_ROW *last_added;

        size_t first;
        size_t forwards;
        size_t backwards;
        size_t skips_before;
        size_t skips_after;
        size_t prepend;
        size_t append;
        size_t shifts;
    } operations;
} FACETS;

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

void facet_value_insert_callback(const DICTIONARY_ITEM *item, void *value, void *data) {
    FACET_VALUE *v = value;
    FACET_KEY *k = data;

    if(!k->selected_values_pattern || simple_pattern_matches(k->selected_values_pattern, dictionary_acquired_item_name(item)))
        v->selected = true;
    else
        v->selected = false;

    facet_value_is_used(k, v);
}

bool facet_value_conflict_callback(const DICTIONARY_ITEM *item __maybe_unused, void *old_value, void *new_value __maybe_unused, void *data) {
    FACET_VALUE *v = old_value;
    FACET_KEY *k = data;

    facet_value_is_used(k, v);
    return false;
}

void facet_value_delete_callback(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused) {
    FACET_VALUE *v = value;
    (void)v;
}

void facet_key_insert_callback(const DICTIONARY_ITEM *item, void *value, void *data) {
    FACET_KEY *k = value;
    FACETS *facets = data;

    k->current_value = buffer_create(0, NULL);

    if(!facets_key_is_filterable(facets, dictionary_acquired_item_name(item))) {
        k->values = NULL;
        return;
    }

    k->values = dictionary_create_advanced(DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE, NULL, sizeof(FACET_VALUE));
    dictionary_register_insert_callback(k->values, facet_value_insert_callback, k);
    dictionary_register_conflict_callback(k->values, facet_value_conflict_callback, k);
    dictionary_register_delete_callback(k->values, facet_value_delete_callback, k);
}

bool facet_key_conflict_callback(const DICTIONARY_ITEM *item __maybe_unused, void *old_value __maybe_unused, void *new_value __maybe_unused, void *data __maybe_unused) {
    return false;
}

void facet_key_delete_callback(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused) {
    FACET_KEY *k = value;
    dictionary_destroy(k->values);
    buffer_free(k->current_value);
}

FACETS *facets_create(uint32_t items_to_return, usec_t anchor, const char *filtered_keys, const char *non_filtered_keys) {
    FACETS *facets = callocz(1, sizeof(FACETS));
    facets->keys = dictionary_create_advanced(DICT_OPTION_SINGLE_THREADED|DICT_OPTION_DONT_OVERWRITE_VALUE|DICT_OPTION_FIXED_SIZE, NULL, sizeof(FACET_KEY));
    dictionary_register_insert_callback(facets->keys, facet_key_insert_callback, facets);
    dictionary_register_conflict_callback(facets->keys, facet_key_conflict_callback, facets);
    dictionary_register_delete_callback(facets->keys, facet_key_delete_callback, facets);

    if(filtered_keys && *filtered_keys)
        facets->included_keys = simple_pattern_create(filtered_keys, "|", SIMPLE_PATTERN_EXACT, true);

    if(non_filtered_keys && *non_filtered_keys)
        facets->excluded_keys = simple_pattern_create(non_filtered_keys, "|", SIMPLE_PATTERN_EXACT, true);

    facets->items_to_return = items_to_return;
    facets->anchor = anchor;

    return facets;
}

void facets_destroy(FACETS *facets) {
    dictionary_destroy(facets->keys);
    simple_pattern_free(facets->excluded_keys);
    freez(facets);
}

static inline void facets_check_value(FACETS *facets __maybe_unused, FACET_KEY *k) {
    if(buffer_strlen(k->current_value) == 0)
        buffer_strcat(k->current_value, "[UNSET]");

    if(k->values)
        dictionary_set(k->values, buffer_tostring(k->current_value), NULL, sizeof(FACET_VALUE));
}

void facets_add_key_value(FACETS *facets, const char *key, const char *value) {
    FACET_KEY *k = dictionary_set(facets->keys, key, NULL, sizeof(FACET_KEY));
    buffer_flush(k->current_value);
    buffer_strcat(k->current_value, value);

    facets_check_value(facets, k);
}

void facets_add_key_value_length(FACETS *facets, const char *key, const char *value, size_t value_len) {
    FACET_KEY *k = dictionary_set(facets->keys, key, NULL, sizeof(FACET_KEY));
    buffer_flush(k->current_value);
    buffer_strncat(k->current_value, value, value_len);

    facets_check_value(facets, k);
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

static void facets_row_free(FACETS *facets __maybe_unused, FACET_ROW *row) {
    dictionary_destroy(row->dict);
    freez(row);
}

void facet_row_key_value_insert_callback(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data) {
    FACET_ROW_KEY_VALUE *rkv = value;
    FACET_ROW *row = data; (void)row;

    rkv->wb = buffer_create(0, NULL);
    buffer_strcat(rkv->wb, rkv->tmp && *rkv->tmp ? rkv->tmp : "[UNSET]");
}

bool facet_row_key_value_conflict_callback(const DICTIONARY_ITEM *item __maybe_unused, void *old_value, void *new_value, void *data) {
    FACET_ROW_KEY_VALUE *rkv = old_value;
    FACET_ROW_KEY_VALUE *n_rkv = new_value;
    FACET_ROW *row = data; (void)row;

    buffer_flush(rkv->wb);
    buffer_strcat(rkv->wb, n_rkv->tmp && *n_rkv->tmp ? n_rkv->tmp : "[UNSET]");

    return false;
}

void facet_row_key_value_delete_callback(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data) {
    FACET_ROW_KEY_VALUE *rkv = value;
    FACET_ROW *row = data; (void)row;

    buffer_free(rkv->wb);
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
                .tmp = buffer_tostring(k->current_value),
                .wb = NULL,
        };
        dictionary_set(row->dict, k_dfe.name, &t, sizeof(t));
    }
    dfe_done(k);

    return row;
}

static void facets_row_keep(FACETS *facets, usec_t usec) {
    facets->item_matched++;

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
        facets->operations.prepend++;
    }
    else {
        facets->operations.last_added = facets_row_create(facets, usec, NULL);
        DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(facets->base, facets->operations.last_added, prev, next);
        facets->operations.append++;
    }

    while(facets->items_to_return > facets->max_items_to_return) {
        // we have to remove something

        FACET_ROW *tmp = facets->base->prev;
        DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(facets->base, tmp, prev, next);
        facets->items_to_return--;

        if(unlikely(facets->operations.last_added == tmp))
            facets->operations.last_added = facets->base;

        facets_row_free(facets, tmp);
        facets->operations.shifts++;
    }
}

void facets_row_finished(FACETS *facets, usec_t usec) {
    uint32_t total_keys = 0;
    uint32_t selected_by = 0;

    FACET_KEY *k;
    dfe_start_read(facets->keys, k) {
        if(!k->key_found_in_row) {
            internal_fatal(buffer_strlen(k->current_value), "key is not found in row but it has a current value");
            // put the [UNSET] value into it
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
                k->rows_matched++;
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

#define SYSTEMD_KEYS_EXCLUDED_FROM_FACETS "MESSAGE|_STREAM_ID|_SYSTEMD_INVOCATION_ID|_BOOT_ID|_MACHINE_ID|_CMDLINE"

struct worker_query_request {
    usec_t after_ut;
    usec_t before_ut;
    const char *filters;

    BUFFER *wb;
    int response;
};

void *worker_query(void *ptr) {
    struct worker_query_request *c = ptr;

    if(!c->wb)
        c->wb = buffer_create(0, NULL);

    FACETS *facets = facets_create(50, 0, NULL, SYSTEMD_KEYS_EXCLUDED_FROM_FACETS);
    // FIXME initialize facets filters here
    facets_rows_begin(facets);

    sd_journal *j;
    int r;

    // Open the system journal for reading
    r = sd_journal_open(&j, SD_JOURNAL_ALL_NAMESPACES);
    if (r < 0) {
        c->wb->content_type = CT_TEXT_PLAIN;
        buffer_flush(c->wb);
        buffer_strcat(c->wb, "failed to open journal: %s");
        c->response = HTTP_RESP_INTERNAL_SERVER_ERROR;
        return ptr;
    }

    sd_journal_seek_realtime_usec(j, c->after_ut);
    SD_JOURNAL_FOREACH(j) {
        uint64_t msg_ut;
        sd_journal_get_realtime_usec(j, &msg_ut);
        if (msg_ut > c->before_ut)
            break;

        const void *data;
        size_t length;
        SD_JOURNAL_FOREACH_DATA(j, data, length) {
            const char *key = data;
            const char *equal = strchr(key, '=');
            if(unlikely(!equal))
                continue;

            const char *value = ++equal;
            size_t key_length = value - key; // including '\0'

            char key_copy[key_length];
            memcpy(key_copy, key, key_length - 1);
            key_copy[key_length - 1] = '\0';

            size_t value_length = length - key_length; // without '\0'
            facets_add_key_value_length(facets, key_copy, value, value_length);
        }

        facets_row_finished(facets, msg_ut);
    }

    sd_journal_close(j);

    facets_destroy(facets);
    return 0;
}

int main(int argc __maybe_unused, char **argv __maybe_unused) {
    stderror = stderr;
    clocks_init();

    program_name = "systemd-journal.plugin";

    // disable syslog
    error_log_syslog = 0;

    // set errors flood protection to 100 logs per hour
    error_log_errors_per_period = 100;
    error_log_throttle_period = 3600;

    time_t started_t = now_monotonic_sec();

    size_t iteration;
    usec_t step = 1000 * USEC_PER_MS;
    bool tty = isatty(fileno(stderr)) == 1;

    heartbeat_t hb;
    heartbeat_init(&hb);
    for(iteration = 0; 1 ; iteration++) {
        heartbeat_next(&hb, step);

        if(tty)
            fprintf(stdout, "\n");

        fflush(stdout);

        time_t now = now_monotonic_sec();
        if(now - started_t > 86400)
            break;
    }
}
