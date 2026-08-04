// SPDX-License-Identifier: GPL-3.0-or-later

#include "libnetdata/libnetdata.h"

#include "macos-logs.h"

netdata_mutex_t stdout_mutex;
DICTIONARY *used_hashes_registry = NULL;

static void __attribute__((constructor)) init_mutex(void) {
    netdata_mutex_init(&stdout_mutex);
}

static void __attribute__((destructor)) destroy_mutex(void) {
    netdata_mutex_destroy(&stdout_mutex);
}

static bool plugin_should_exit = false;

struct macos_logs_facet_value_cache {
    const char *key;
    size_t key_length;
    DICTIONARY *values;
    bool full;
};

struct macos_logs_cached_facet_value {
    size_t length;
};

#define MACOS_LOGS_FACET_VALUE_CACHE_ENTRY(field) { field, MACOS_LOGS_FIELD_LENGTH(field), NULL, false }

#define MACOS_LOGS_ALWAYS_VISIBLE_KEYS NULL

#define MACOS_LOGS_KEYS_EXCLUDED_FROM_FACETS \
    "|" MACOS_LOGS_FIELD_MESSAGE             \
    "|" MACOS_LOGS_FIELD_PID                 \
    "|" MACOS_LOGS_FIELD_THREAD_ID           \
    "|" MACOS_LOGS_FIELD_ACTIVITY_ID         \
    "|" MACOS_LOGS_FIELD_PARENT_ACTIVITY_ID  \
    "|" MACOS_LOGS_FIELD_FORMAT_STRING       \
    "|" MACOS_LOGS_FIELD_SIGNPOST_ID         \
    ""

#define MACOS_LOGS_KEYS_INCLUDED_IN_FACETS  \
    "|" MACOS_LOGS_FIELD_LEVEL              \
    "|" MACOS_LOGS_FIELD_PROCESS            \
    "|" MACOS_LOGS_FIELD_SENDER             \
    "|" MACOS_LOGS_FIELD_SUBSYSTEM          \
    "|" MACOS_LOGS_FIELD_CATEGORY           \
    "|" MACOS_LOGS_FIELD_ENTRY_TYPE         \
    "|" MACOS_LOGS_FIELD_STORE_CATEGORY     \
    "|" MACOS_LOGS_FIELD_COMPONENT_COUNT    \
    "|" MACOS_LOGS_FIELD_SIGNPOST_NAME      \
    "|" MACOS_LOGS_FIELD_SIGNPOST_TYPE      \
    ""

static netdata_mutex_t macos_logs_facet_value_cache_mutex;
static struct macos_logs_facet_value_cache macos_logs_facet_value_caches[MACOS_LOGS_FACET_VALUE_CACHE_COUNT] = {
    MACOS_LOGS_FACET_VALUE_CACHE_ENTRY(MACOS_LOGS_FIELD_LEVEL),
    MACOS_LOGS_FACET_VALUE_CACHE_ENTRY(MACOS_LOGS_FIELD_PROCESS),
    MACOS_LOGS_FACET_VALUE_CACHE_ENTRY(MACOS_LOGS_FIELD_SENDER),
    MACOS_LOGS_FACET_VALUE_CACHE_ENTRY(MACOS_LOGS_FIELD_SUBSYSTEM),
    MACOS_LOGS_FACET_VALUE_CACHE_ENTRY(MACOS_LOGS_FIELD_CATEGORY),
    MACOS_LOGS_FACET_VALUE_CACHE_ENTRY(MACOS_LOGS_FIELD_ENTRY_TYPE),
    MACOS_LOGS_FACET_VALUE_CACHE_ENTRY(MACOS_LOGS_FIELD_STORE_CATEGORY),
    MACOS_LOGS_FACET_VALUE_CACHE_ENTRY(MACOS_LOGS_FIELD_COMPONENT_COUNT),
    MACOS_LOGS_FACET_VALUE_CACHE_ENTRY(MACOS_LOGS_FIELD_SIGNPOST_NAME),
    MACOS_LOGS_FACET_VALUE_CACHE_ENTRY(MACOS_LOGS_FIELD_SIGNPOST_TYPE),
};
static bool macos_logs_facet_value_cache_initialized = false;

static void __attribute__((constructor)) macos_logs_facet_value_cache_init(void) {
    netdata_mutex_init(&macos_logs_facet_value_cache_mutex);
}

static void macos_logs_facet_value_cache_ensure_initialized(void) {
    netdata_mutex_lock(&macos_logs_facet_value_cache_mutex);
    if(macos_logs_facet_value_cache_initialized) {
        netdata_mutex_unlock(&macos_logs_facet_value_cache_mutex);
        return;
    }

    for(size_t i = 0; i < sizeof(macos_logs_facet_value_caches) / sizeof(macos_logs_facet_value_caches[0]); i++) {
        macos_logs_facet_value_caches[i].values =
            dictionary_create(DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE);
        macos_logs_facet_value_caches[i].full = false;
    }

    macos_logs_facet_value_cache_initialized = true;
    netdata_mutex_unlock(&macos_logs_facet_value_cache_mutex);
}

static void __attribute__((destructor)) macos_logs_facet_value_cache_destroy_mutex(void) {
    netdata_mutex_destroy(&macos_logs_facet_value_cache_mutex);
}

static void macos_logs_facet_value_cache_cleanup(void) {
    netdata_mutex_lock(&macos_logs_facet_value_cache_mutex);
    if(!macos_logs_facet_value_cache_initialized) {
        netdata_mutex_unlock(&macos_logs_facet_value_cache_mutex);
        return;
    }

    for(size_t i = 0; i < sizeof(macos_logs_facet_value_caches) / sizeof(macos_logs_facet_value_caches[0]); i++) {
        dictionary_destroy(macos_logs_facet_value_caches[i].values);
        macos_logs_facet_value_caches[i].values = NULL;
        macos_logs_facet_value_caches[i].full = false;
    }
    macos_logs_facet_value_cache_initialized = false;
    netdata_mutex_unlock(&macos_logs_facet_value_cache_mutex);
}

void macos_logs_cache_facet_value(MACOS_LOGS_FACET_VALUE_CACHE_ID id, const char *value, size_t value_length) {
    if((int)id < 0 || id >= MACOS_LOGS_FACET_VALUE_CACHE_COUNT || !value || !*value || !value_length)
        return;

    macos_logs_facet_value_cache_ensure_initialized();

    struct macos_logs_facet_value_cache *cache = &macos_logs_facet_value_caches[id];
    if(!cache->values)
        return;

    netdata_mutex_lock(&macos_logs_facet_value_cache_mutex);
    if(cache->full || dictionary_get_advanced(cache->values, value, (ssize_t)value_length)) {
        netdata_mutex_unlock(&macos_logs_facet_value_cache_mutex);
        return;
    }

    if(dictionary_entries(cache->values) < MACOS_LOGS_FACET_VALUE_CACHE_MAX_PER_KEY) {
        struct macos_logs_cached_facet_value cached = { .length = value_length };
        dictionary_set_advanced(cache->values, value, (ssize_t)value_length, &cached, sizeof(cached), NULL);
    }
    else
        cache->full = true;

    netdata_mutex_unlock(&macos_logs_facet_value_cache_mutex);
}

void macos_logs_add_cached_facet_values(FACETS *facets) {
    macos_logs_facet_value_cache_ensure_initialized();

    netdata_mutex_lock(&macos_logs_facet_value_cache_mutex);
    for(size_t i = 0; i < sizeof(macos_logs_facet_value_caches) / sizeof(macos_logs_facet_value_caches[0]); i++) {
        struct macos_logs_facet_value_cache *cache = &macos_logs_facet_value_caches[i];
        if(!cache->key || !*cache->key || !cache->values)
            continue;

        struct macos_logs_cached_facet_value *present;
        dfe_start_read(cache->values, present) {
            const char *value = present_dfe.name;
            if(!value || !*value || !present || !present->length)
                continue;

            facets_add_possible_value_name_to_key(
                facets, cache->key, cache->key_length, value, present->length);
        }
        dfe_done(present);
    }
    netdata_mutex_unlock(&macos_logs_facet_value_cache_mutex);
}

static FACET_ROW_SEVERITY macos_logs_level_to_facet_severity(
    FACETS *facets __maybe_unused, FACET_ROW *row, void *data __maybe_unused) {
    FACET_ROW_KEY_VALUE *level_rkv = dictionary_get(row->dict, MACOS_LOGS_FIELD_LEVEL_ID);
    if(!level_rkv || level_rkv->empty)
        return FACET_ROW_SEVERITY_NORMAL;

    int level = str2i(buffer_tostring(level_rkv->wb));
    switch(level) {
        case 1: // debug
            return FACET_ROW_SEVERITY_DEBUG;
        case 3: // notice
            return FACET_ROW_SEVERITY_NOTICE;
        case 4: // error
        case 5: // fault
            return FACET_ROW_SEVERITY_CRITICAL;
        case 0: // undefined
        case 2: // info
        default:
            return FACET_ROW_SEVERITY_NORMAL;
    }
}

static void macos_logs_register_fields(LOGS_QUERY_STATUS *lqs) {
    FACETS *facets = lqs->facets;
    LOGS_QUERY_REQUEST *rq = &lqs->rq;

    facets_register_row_severity(facets, macos_logs_level_to_facet_severity, NULL);

    facets_register_key_name(
        facets,
        MACOS_LOGS_FIELD_MESSAGE,
        FACET_KEY_OPTION_NEVER_FACET | FACET_KEY_OPTION_MAIN_TEXT | FACET_KEY_OPTION_VISIBLE | FACET_KEY_OPTION_FTS);

    facets_register_key_name(
        facets,
        MACOS_LOGS_FIELD_LEVEL,
        rq->default_facet | FACET_KEY_OPTION_FTS | FACET_KEY_OPTION_EXPANDED_FILTER | FACET_KEY_OPTION_VISIBLE);

    facets_register_key_name(facets, MACOS_LOGS_FIELD_LEVEL_ID, FACET_KEY_OPTION_HIDDEN);

    facets_register_key_name(
        facets,
        MACOS_LOGS_FIELD_PROCESS,
        rq->default_facet | FACET_KEY_OPTION_FTS | FACET_KEY_OPTION_VISIBLE);

    facets_register_key_name(facets, MACOS_LOGS_FIELD_PID, FACET_KEY_OPTION_NEVER_FACET | FACET_KEY_OPTION_FTS);

    facets_register_key_name(
        facets,
        MACOS_LOGS_FIELD_SENDER,
        rq->default_facet | FACET_KEY_OPTION_FTS | FACET_KEY_OPTION_VISIBLE);

    facets_register_key_name(
        facets,
        MACOS_LOGS_FIELD_SUBSYSTEM,
        rq->default_facet | FACET_KEY_OPTION_FTS | FACET_KEY_OPTION_VISIBLE);

    facets_register_key_name(
        facets,
        MACOS_LOGS_FIELD_CATEGORY,
        rq->default_facet | FACET_KEY_OPTION_FTS | FACET_KEY_OPTION_VISIBLE);

    facets_register_key_name(
        facets,
        MACOS_LOGS_FIELD_ENTRY_TYPE,
        rq->default_facet | FACET_KEY_OPTION_FTS);

    facets_register_key_name(
        facets,
        MACOS_LOGS_FIELD_STORE_CATEGORY,
        rq->default_facet | FACET_KEY_OPTION_FTS);

    facets_register_key_name(facets, MACOS_LOGS_FIELD_THREAD_ID, FACET_KEY_OPTION_NEVER_FACET | FACET_KEY_OPTION_FTS);
    facets_register_key_name(facets, MACOS_LOGS_FIELD_ACTIVITY_ID, FACET_KEY_OPTION_NEVER_FACET | FACET_KEY_OPTION_FTS);
    facets_register_key_name(facets, MACOS_LOGS_FIELD_PARENT_ACTIVITY_ID, FACET_KEY_OPTION_NEVER_FACET | FACET_KEY_OPTION_FTS);
    facets_register_key_name(facets, MACOS_LOGS_FIELD_FORMAT_STRING, FACET_KEY_OPTION_NEVER_FACET | FACET_KEY_OPTION_FTS);
    facets_register_key_name(facets, MACOS_LOGS_FIELD_COMPONENT_COUNT, FACET_KEY_OPTION_FACET | FACET_KEY_OPTION_FTS);
    facets_register_key_name(facets, MACOS_LOGS_FIELD_SIGNPOST_ID, FACET_KEY_OPTION_NEVER_FACET | FACET_KEY_OPTION_FTS);
    facets_register_key_name(facets, MACOS_LOGS_FIELD_SIGNPOST_NAME, FACET_KEY_OPTION_FACET | FACET_KEY_OPTION_FTS);
    facets_register_key_name(facets, MACOS_LOGS_FIELD_SIGNPOST_TYPE, FACET_KEY_OPTION_FACET | FACET_KEY_OPTION_FTS);
}

static void buffer_json_macos_logs_versions(BUFFER *wb) {
    buffer_json_member_add_object(wb, "versions");
    {
        buffer_json_member_add_uint64(wb, "sources", 1);
    }
    buffer_json_object_close(wb);
}

static int macos_logs_response(BUFFER *wb, LOGS_QUERY_STATUS *lqs, MACOS_LOGS_QUERY_STATUS status) {
    bool partial = status == MACOS_LOGS_QUERY_TIMED_OUT;

    switch(status) {
        case MACOS_LOGS_QUERY_OK:
            if(lqs->rq.if_modified_since && !lqs->c.rows_useful)
                return rrd_call_function_error(wb, "no useful logs, not modified", HTTP_RESP_NOT_MODIFIED);
            break;

        case MACOS_LOGS_QUERY_TIMED_OUT:
            break;

        case MACOS_LOGS_QUERY_CANCELLED:
            return rrd_call_function_error(wb, "client closed connection", HTTP_RESP_CLIENT_CLOSED_REQUEST);

        case MACOS_LOGS_QUERY_OPEN_FAILED:
            return rrd_call_function_error(wb, "failed to open macOS unified log store", HTTP_RESP_INTERNAL_SERVER_ERROR);

        case MACOS_LOGS_QUERY_ENUMERATOR_FAILED:
            return rrd_call_function_error(wb, "failed to enumerate macOS unified logs", HTTP_RESP_INTERNAL_SERVER_ERROR);

        default:
            return rrd_call_function_error(wb, "unknown macOS unified log query status", HTTP_RESP_INTERNAL_SERVER_ERROR);
    }

    buffer_json_member_add_uint64(wb, "status", HTTP_RESP_OK);
    buffer_json_member_add_boolean(wb, "partial", partial);
    buffer_json_member_add_string(wb, "type", "table");

    if(!lqs->rq.data_only) {
        CLEAN_BUFFER *msg = buffer_create(0, NULL);
        CLEAN_BUFFER *msg_description = buffer_create(0, NULL);
        ND_LOG_FIELD_PRIORITY msg_priority = NDLP_INFO;

        if(status == MACOS_LOGS_QUERY_TIMED_OUT) {
            buffer_strcat(msg, "Query timed out, incomplete data. ");
            buffer_strcat(msg_description, "QUERY TIMEOUT: The query timed out and may not include all data in the selected window. ");
            msg_priority = NDLP_WARNING;
        }
        buffer_json_member_add_object(wb, "message");
        if(buffer_strlen(msg)) {
            buffer_json_member_add_string(wb, "title", buffer_tostring(msg));
            buffer_json_member_add_string(wb, "description", buffer_tostring(msg_description));
            buffer_json_member_add_string(wb, "status", nd_log_id2priority(msg_priority));
        }
        // else send an empty object if there is nothing to tell
        buffer_json_object_close(wb);
    }

    buffer_json_member_add_array(wb, "_sources");
    {
        buffer_json_add_array_item_object(wb);
        {
            usec_t duration_ut = lqs->c.query_finished_ut > lqs->c.query_started_ut ?
                                     lqs->c.query_finished_ut - lqs->c.query_started_ut : 0;

            buffer_json_member_add_string(wb, "_name", "macOS unified log");
            buffer_json_member_add_uint64(wb, "_source_type", MACOS_LOGS_SOURCE_ALL);
            buffer_json_member_add_string(wb, "_source", "all");
            buffer_json_member_add_uint64(wb, "duration_ut", duration_ut);
            buffer_json_member_add_uint64(wb, "rows_read", lqs->c.rows_read);
            buffer_json_member_add_uint64(wb, "rows_useful", lqs->c.rows_useful);
            buffer_json_member_add_double(
                wb, "rows_per_second",
                duration_ut ? (double)lqs->c.rows_read / (double)duration_ut * (double)USEC_PER_SEC : 0.0);
            buffer_json_member_add_uint64(wb, "bytes_read", lqs->c.bytes_read);
            buffer_json_member_add_double(
                wb, "bytes_per_second",
                duration_ut ? (double)lqs->c.bytes_read / (double)duration_ut * (double)USEC_PER_SEC : 0.0);
        }
        buffer_json_object_close(wb);
    }
    buffer_json_array_close(wb);

    if(!lqs->rq.data_only) {
        buffer_json_member_add_time_t(wb, "update_every", 1);
        buffer_json_member_add_string(wb, "help", MACOS_LOGS_FUNCTION_DESCRIPTION);
    }

    if(!lqs->rq.data_only || lqs->rq.tail)
        buffer_json_member_add_uint64(wb, "last_modified", lqs->last_modified);

    if(lqs->rq.slice)
        macos_logs_add_cached_facet_values(lqs->facets);

    facets_sort_and_reorder_keys(lqs->facets);
    facets_report(lqs->facets, wb, used_hashes_registry);

    wb->expires = now_realtime_sec() + (lqs->rq.data_only ? 3600 : 0);
    buffer_json_member_add_time_t(wb, "expires", wb->expires);

    wb->content_type = CT_APPLICATION_JSON;
    wb->response_code = HTTP_RESP_OK;
    return wb->response_code;
}

BUFFER *function_macos_logs_result(
    const char *transaction, char *function, usec_t *stop_monotonic_ut, bool *cancelled,
    BUFFER *payload, HTTP_ACCESS access __maybe_unused,
    const char *source __maybe_unused, void *data __maybe_unused) {
    LOGS_QUERY_STATUS tmp_lqs = {
        .facets = lqs_facets_create(
            LQS_DEFAULT_ITEMS_PER_QUERY,
            FACETS_OPTION_ALL_KEYS_FTS | FACETS_OPTION_HASH_IDS,
            MACOS_LOGS_ALWAYS_VISIBLE_KEYS,
            MACOS_LOGS_KEYS_INCLUDED_IN_FACETS,
            MACOS_LOGS_KEYS_EXCLUDED_FROM_FACETS,
            LQS_DEFAULT_SLICE_MODE),

        .rq = LOGS_QUERY_REQUEST_DEFAULTS(transaction, LQS_DEFAULT_SLICE_MODE, FACETS_ANCHOR_DIRECTION_BACKWARD),

        .cancelled = cancelled,
        .stop_monotonic_ut = stop_monotonic_ut,
    };
    LOGS_QUERY_STATUS *lqs = &tmp_lqs;

    BUFFER *wb = lqs_create_output_buffer();

    if(lqs_request_parse_and_validate(lqs, wb, function, payload, LQS_DEFAULT_SLICE_MODE, MACOS_LOGS_FIELD_LEVEL)) {
        macos_logs_register_fields(lqs);
        buffer_json_macos_logs_versions(wb);

        if(lqs->rq.info)
            lqs_info_response(wb, lqs->facets);
        else {
            lqs_query_timeframe(lqs, MACOS_LOGS_ANCHOR_DELTA_UT);
            MACOS_LOGS_QUERY_STATUS status = macos_logs_query_oslog(lqs);
            if(macos_logs_response(wb, lqs, status) == HTTP_RESP_OK)
                buffer_json_finalize(wb);
        }
    }

    lqs_cleanup(lqs);

    return wb;
}

void function_macos_logs(const char *transaction, char *function, usec_t *stop_monotonic_ut, bool *cancelled,
                         BUFFER *payload, HTTP_ACCESS access,
                         const char *source, void *data) {
    BUFFER *wb = function_macos_logs_result(
        transaction, function, stop_monotonic_ut, cancelled, payload, access, source, data);

    netdata_mutex_lock(&stdout_mutex);
    pluginsd_function_result_to_stdout(transaction, wb);
    netdata_mutex_unlock(&stdout_mutex);

    buffer_free(wb);
}

// ----------------------------------------------------------------------------
// --test command-line interface (mirrors systemd-journal.plugin --test):
//   macos-logs.plugin --test macos-logs [--timeout <seconds>] < payload.json
// OSLog reads the live system unified-log store, so (unlike systemd-journal)
// there is no --dir backend to pin. The request payload (same JSON the dashboard
// sends, incl. facet selections) is read from stdin and the raw JSON result is
// written to stdout -- letting us reproduce/verify filtering without netdata.

#define MACOS_LOGS_TEST_TIMEOUT_DISABLED_SECONDS (100ULL * 365ULL * 24ULL * 60ULL * 60ULL)
#define MACOS_LOGS_TEST_MAX_REQUEST_BYTES (16ULL * 1024ULL * 1024ULL)

struct macos_logs_test_command {
    bool enabled;
    const char *function_name;
    uint64_t timeout_seconds;
    bool timeout_seconds_set;
};

static void macos_logs_test_usage(FILE *stream)
{
    fprintf(
        stream,
        "usage: macos-logs.plugin --test macos-logs [--timeout <seconds>] < payload.json\n");
}

static bool macos_logs_test_option_present(int argc, char **argv)
{
    for (int i = 1; i < argc; i++) {
        if (strcmp(argv[i], "--test") == 0 || strncmp(argv[i], "--test=", strlen("--test=")) == 0)
            return true;
    }
    return false;
}

static int macos_logs_test_set_required_option_once(const char **slot, const char *value, const char *option)
{
    if (*slot) {
        fprintf(stderr, "duplicate %s\n", option);
        macos_logs_test_usage(stderr);
        return 2;
    }
    if (!value || !*value) {
        fprintf(stderr, "missing value for %s\n", option);
        macos_logs_test_usage(stderr);
        return 2;
    }
    *slot = value;
    return 0;
}

static int macos_logs_test_set_timeout_option_once(uint64_t *slot, bool *slot_set, const char *value)
{
    if (*slot_set) {
        fprintf(stderr, "duplicate --timeout\n");
        macos_logs_test_usage(stderr);
        return 2;
    }
    if (!value || !*value) {
        fprintf(stderr, "missing value for --timeout\n");
        macos_logs_test_usage(stderr);
        return 2;
    }
    for (const char *s = value; *s; s++) {
        if (*s < '0' || *s > '9') {
            fprintf(stderr, "invalid value for --timeout '%s'; expected seconds\n", value);
            macos_logs_test_usage(stderr);
            return 2;
        }
    }
    errno = 0;
    unsigned long long parsed = strtoull(value, NULL, 10);
    if (errno == ERANGE) {
        fprintf(stderr, "invalid value for --timeout '%s'; expected seconds\n", value);
        macos_logs_test_usage(stderr);
        return 2;
    }
#if ULLONG_MAX > UINT64_MAX
    if (parsed > UINT64_MAX) {
        fprintf(stderr, "invalid value for --timeout '%s'; expected seconds\n", value);
        macos_logs_test_usage(stderr);
        return 2;
    }
#endif
    *slot = (uint64_t)parsed;
    *slot_set = true;
    return 0;
}

static int parse_macos_logs_test_command(int argc, char **argv, struct macos_logs_test_command *cmd)
{
    *cmd = (struct macos_logs_test_command){0};
    if (!macos_logs_test_option_present(argc, argv))
        return 0;

    cmd->enabled = true;

    for (int i = 1; i < argc; i++) {
        const char *arg = argv[i];
        if (strcmp(arg, "--test") == 0) {
            if (++i >= argc)
                return macos_logs_test_set_required_option_once(&cmd->function_name, NULL, "--test");
            int rc = macos_logs_test_set_required_option_once(&cmd->function_name, argv[i], "--test");
            if (rc)
                return rc;
        }
        else if (strncmp(arg, "--test=", strlen("--test=")) == 0) {
            int rc = macos_logs_test_set_required_option_once(&cmd->function_name, arg + strlen("--test="), "--test");
            if (rc)
                return rc;
        }
        else if (strcmp(arg, "--timeout") == 0) {
            if (++i >= argc)
                return macos_logs_test_set_timeout_option_once(&cmd->timeout_seconds, &cmd->timeout_seconds_set, NULL);
            int rc = macos_logs_test_set_timeout_option_once(&cmd->timeout_seconds, &cmd->timeout_seconds_set, argv[i]);
            if (rc)
                return rc;
        }
        else if (strncmp(arg, "--timeout=", strlen("--timeout=")) == 0) {
            int rc = macos_logs_test_set_timeout_option_once(
                &cmd->timeout_seconds,
                &cmd->timeout_seconds_set,
                arg + strlen("--timeout="));
            if (rc)
                return rc;
        }
        else if (strcmp(arg, "-h") == 0 || strcmp(arg, "--help") == 0) {
            macos_logs_test_usage(stderr);
            return 2;
        }
        else {
            fprintf(stderr, "unsupported macos-logs test option '%s'\n", arg);
            macos_logs_test_usage(stderr);
            return 2;
        }
    }

    if (!cmd->function_name) {
        fprintf(stderr, "missing required --test\n");
        macos_logs_test_usage(stderr);
        return 2;
    }

    if (!cmd->timeout_seconds_set)
        cmd->timeout_seconds = MACOS_LOGS_DEFAULT_TIMEOUT;

    return 0;
}

static usec_t macos_logs_test_stop_monotonic_usec(uint64_t timeout_seconds)
{
    usec_t now_ut = now_monotonic_usec();
    uint64_t effective_timeout_seconds =
        timeout_seconds ? timeout_seconds : MACOS_LOGS_TEST_TIMEOUT_DISABLED_SECONDS;
    uint64_t max_timeout_seconds = (UINT64_MAX - now_ut) / USEC_PER_SEC;
    if (effective_timeout_seconds > max_timeout_seconds)
        return UINT64_MAX;
    return now_ut + effective_timeout_seconds * USEC_PER_SEC;
}

static BUFFER *macos_logs_test_read_request_payload_from_stdin(void)
{
    BUFFER *payload = buffer_create(8192, NULL);
    size_t total = 0;
    while (true) {
        char buf[8192];
        ssize_t bytes_read = read(STDIN_FILENO, buf, sizeof(buf));
        if (bytes_read == -1) {
            if (errno == EINTR)
                continue;
            fprintf(stderr, "failed to read request payload from stdin: %s\n", strerror(errno));
            buffer_free(payload);
            return NULL;
        }
        if (bytes_read == 0)
            break;
        if ((uint64_t)total + (uint64_t)bytes_read > MACOS_LOGS_TEST_MAX_REQUEST_BYTES) {
            fprintf(
                stderr,
                "request payload from stdin is too large: max %llu bytes\n",
                (unsigned long long)MACOS_LOGS_TEST_MAX_REQUEST_BYTES);
            buffer_free(payload);
            return NULL;
        }
        buffer_memcat(payload, buf, (size_t)bytes_read);
        total += (size_t)bytes_read;
    }
    if (total == 0) {
        fprintf(stderr, "request payload from stdin is empty\n");
        buffer_free(payload);
        return NULL;
    }
    payload->content_type = CT_APPLICATION_JSON;
    return payload;
}

static int run_macos_logs_test_command(const struct macos_logs_test_command *cmd)
{
    if (strcmp(cmd->function_name, MACOS_LOGS_FUNCTION_NAME) != 0) {
        fprintf(
            stderr,
            "unsupported macos-logs test function '%s' (expected '%s')\n",
            cmd->function_name,
            MACOS_LOGS_FUNCTION_NAME);
        return 2;
    }

    CLEAN_BUFFER *payload = macos_logs_test_read_request_payload_from_stdin();
    if (!payload)
        return 1;

    bool cancelled = false;
    usec_t stop_monotonic_ut = macos_logs_test_stop_monotonic_usec(cmd->timeout_seconds);

    char *function = strdupz(cmd->function_name);
    BUFFER *result = function_macos_logs_result(
        "test", function, &stop_monotonic_ut, &cancelled, payload, HTTP_ACCESS_ALL, "test-cli", NULL);
    freez(function);

    int rc = 1;
    if (result) {
        if (buffer_strlen(result))
            fwrite(buffer_tostring(result), buffer_strlen(result), 1, stdout);
        fprintf(stdout, "\n");
        fflush(stdout);
        if (result->response_code >= HTTP_RESP_OK && result->response_code < 300)
            rc = 0;
        buffer_free(result);
    }
    else {
        fprintf(stderr, "macos-logs test function returned no result\n");
    }
    return rc;
}

int main(int argc, char **argv) {
    struct macos_logs_test_command test_command = {0};
    int test_parse_rc = parse_macos_logs_test_command(argc, argv, &test_command);
    if (test_parse_rc)
        exit(test_parse_rc);

    nd_thread_tag_set("macos-logs.plugin");
    nd_log_initialize_for_external_plugins("macos-logs.plugin");
    if(nd_environment_freeze_process() != 0)
        fatal("Cannot freeze the process environment: %s", strerror(errno));
    netdata_threads_init_for_external_plugins(0);

    used_hashes_registry = dictionary_create(DICT_OPTION_DONT_OVERWRITE_VALUE);

    if (test_command.enabled) {
        int rc = run_macos_logs_test_command(&test_command);
        macos_logs_facet_value_cache_cleanup();
        dictionary_destroy(used_hashes_registry);
        exit(rc);
    }

    if(argc >= 2 && strcmp(argv[argc - 1], "debug") == 0) {
        bool cancelled = false;
        usec_t stop_monotonic_ut = now_monotonic_usec() + MACOS_LOGS_DEFAULT_TIMEOUT * USEC_PER_SEC;
        char buf[] = MACOS_LOGS_FUNCTION_NAME " info";
        function_macos_logs("debug-transaction", buf, &stop_monotonic_ut, &cancelled, NULL, HTTP_ACCESS_ALL, NULL, NULL);
        macos_logs_facet_value_cache_cleanup();
        dictionary_destroy(used_hashes_registry);
        return 0;
    }

    struct functions_evloop_globals *wg =
        functions_evloop_init(MACOS_LOGS_WORKER_THREADS, "MLOG", &stdout_mutex, &plugin_should_exit, NULL);

    functions_evloop_add_function(wg, MACOS_LOGS_FUNCTION_NAME, function_macos_logs, MACOS_LOGS_DEFAULT_TIMEOUT, NULL);

    netdata_mutex_lock(&stdout_mutex);
    fprintf(stdout,
            PLUGINSD_KEYWORD_FUNCTION " GLOBAL \"%s\" %d \"%s\" \"logs\" " HTTP_ACCESS_FORMAT " %d\n",
            MACOS_LOGS_FUNCTION_NAME,
            MACOS_LOGS_DEFAULT_TIMEOUT,
            MACOS_LOGS_FUNCTION_DESCRIPTION,
            (HTTP_ACCESS_FORMAT_CAST)(HTTP_ACCESS_SIGNED_ID | HTTP_ACCESS_SAME_SPACE | HTTP_ACCESS_SENSITIVE_DATA),
            RRDFUNCTIONS_PRIORITY_DEFAULT);
    fflush(stdout);
    netdata_mutex_unlock(&stdout_mutex);

    usec_t send_newline_ut = 0;
    const bool tty = isatty(fileno(stdout)) == 1;

    heartbeat_t hb;
    heartbeat_init(&hb, USEC_PER_SEC);
    while(!__atomic_load_n(&plugin_should_exit, __ATOMIC_ACQUIRE)) {
        usec_t dt_ut = heartbeat_next(&hb);

        send_newline_ut += dt_ut;
        if(!tty && send_newline_ut >= USEC_PER_SEC) {
            send_newline_and_flush(&stdout_mutex);
            send_newline_ut = 0;
        }
    }

    functions_evloop_cancel_threads(wg);
    // Wait for function worker threads to fully exit before freeing the registry:
    // cancel only requests cancellation, and workers access used_hashes_registry via
    // facets_report(), so destroying it first would be a use-after-free.
    functions_evloop_join_threads(wg);
    macos_logs_facet_value_cache_cleanup();
    dictionary_destroy(used_hashes_registry);
    return 0;
}
