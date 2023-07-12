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
    size_t count;
} FACET_VALUE;

typedef struct facet_key {
    DICTIONARY *values;
    BUFFER *current_value;
} FACET_KEY;

typedef struct facets {
    SIMPLE_PATTERN *excluded_keys;
    SIMPLE_PATTERN *included_keys;
    DICTIONARY *keys;
} FACETS;


void facet_value_insert_callback(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused) {
    FACET_VALUE *v = value;
    v->count++;
}

bool facet_value_conflict_callback(const DICTIONARY_ITEM *item __maybe_unused, void *old_value __maybe_unused, void *new_value __maybe_unused, void *data __maybe_unused) {
    FACET_VALUE *v = old_value;
    v->count++;
    return true;
}

void facet_value_delete_callback(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused) {
    FACET_VALUE *v = value;
    (void)v;
}

void facet_key_insert_callback(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused) {
    FACET_KEY *k = value;
    k->values = dictionary_create_advanced(DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE, NULL, sizeof(FACET_VALUE));
    dictionary_register_insert_callback(k->values, facet_value_insert_callback, k);
    dictionary_register_conflict_callback(k->values, facet_value_conflict_callback, k);
    dictionary_register_delete_callback(k->values, facet_value_delete_callback, k);
    k->current_value = buffer_create(0, NULL);
}

bool facet_key_conflict_callback(const DICTIONARY_ITEM *item __maybe_unused, void *old_value __maybe_unused, void *new_value __maybe_unused, void *data __maybe_unused) {
    return false;
}

void facet_key_delete_callback(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused) {
    FACET_KEY *k = value;
    dictionary_destroy(k->values);
    buffer_free(k->current_value);
}

FACETS *facets_create(const char *included_keys, const char *excluded_keys) {
    FACETS *facets = callocz(1, sizeof(FACETS));
    facets->keys = dictionary_create_advanced(DICT_OPTION_SINGLE_THREADED|DICT_OPTION_DONT_OVERWRITE_VALUE|DICT_OPTION_FIXED_SIZE, NULL, sizeof(FACET_KEY));
    dictionary_register_insert_callback(facets->keys, facet_key_insert_callback, facets);
    dictionary_register_conflict_callback(facets->keys, facet_key_conflict_callback, facets);
    dictionary_register_delete_callback(facets->keys, facet_key_delete_callback, facets);

    if(included_keys && *included_keys)
        facets->included_keys = simple_pattern_create(included_keys, "|", SIMPLE_PATTERN_EXACT, true);

    if(excluded_keys && *excluded_keys)
        facets->excluded_keys = simple_pattern_create(excluded_keys, "|", SIMPLE_PATTERN_EXACT, true);

    return facets;
}

void facets_destroy(FACETS *facets) {
    dictionary_destroy(facets->keys);
    simple_pattern_free(facets->excluded_keys);
    freez(facets);
}

void facets_reset_current_value(FACETS *facets) {
    FACET_KEY *key;
    dfe_start_read(facets->keys, key) {
        buffer_flush(key->current_value);
    }
    dfe_done(key);
}

static inline bool facets_key_should_be_added(FACETS *facets, const char *key) {
    if(facets->included_keys) {
        if (!simple_pattern_matches(facets->included_keys, key))
            return false;
    }
    else if(facets->excluded_keys) {
        if (simple_pattern_matches(facets->excluded_keys, key))
            return false;
    }

    return true;
}

static inline void facets_check_value(FACETS *facets, FACET_KEY *k) {
    FACET_VALUE *v = dictionary_set(k->values, buffer_tostring(k->current_value), NULL, sizeof(FACET_VALUE));

}

void facets_add_key_value(FACETS *facets, const char *key, const char *value) {
    if(!facets_key_should_be_added(facets, key))
        return;

    FACET_KEY *k = dictionary_set(facets->keys, key, NULL, sizeof(FACET_KEY));
    buffer_flush(k->current_value);
    buffer_strcat(k->current_value, value);

    facets_check_value(facets, k);
}

void facets_add_key_value_length(FACETS *facets, const char *key, const char *value, size_t value_len) {
    if(!facets_key_should_be_added(facets, key))
        return;

    FACET_KEY *k = dictionary_set(facets->keys, key, NULL, sizeof(FACET_KEY));
    buffer_flush(k->current_value);
    buffer_strncat(k->current_value, value, value_len);

    facets_check_value(facets, k);
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

    FACETS *facets = facets_create(NULL, SYSTEMD_KEYS_EXCLUDED_FROM_FACETS);

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

        facets_reset_current_value(facets);

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


    }

    sd_journal_close(j);

    facets_destroy(facets);
    return 0;
}

int main(int argc, char **argv) {
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
    bool global_chart_created = false;
    bool tty = isatty(fileno(stderr)) == 1;

    heartbeat_t hb;
    heartbeat_init(&hb);
    for(iteration = 0; 1 ; iteration++) {
        usec_t dt = heartbeat_next(&hb, step);

        fprintf(stdout, "\n");
        fflush(stdout);

        time_t now = now_monotonic_sec();
        if(now - started_t > 86400)
            break;
    }
}
