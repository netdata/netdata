// SPDX-License-Identifier: GPL-3.0-or-later
/*
 *  netdata systemd-journal.plugin
 *  Copyright (C) 2023 Netdata Inc.
 *  GPL v3+
 */

#include "collectors/all.h"
#include "libnetdata/libnetdata.h"
#include "libnetdata/required_dummies.h"

#include <systemd/sd-journal.h>

#define FACET_MAX_VALUE_LENGTH                  8192

#define SYSTEMD_JOURNAL_FUNCTION_DESCRIPTION    "View, search and analyze systemd journal entries."
#define SYSTEMD_JOURNAL_FUNCTION_NAME           "systemd-journal"
#define SYSTEMD_JOURNAL_DEFAULT_TIMEOUT         30
#define SYSTEMD_JOURNAL_MAX_PARAMS              100
#define SYSTEMD_JOURNAL_DEFAULT_QUERY_DURATION  (3 * 3600)
#define SYSTEMD_JOURNAL_DEFAULT_ITEMS_PER_QUERY 200

#define JOURNAL_PARAMETER_HELP                  "help"
#define JOURNAL_PARAMETER_AFTER                 "after"
#define JOURNAL_PARAMETER_BEFORE                "before"
#define JOURNAL_PARAMETER_ANCHOR                "anchor"
#define JOURNAL_PARAMETER_LAST                  "last"

#define SYSTEMD_ALWAYS_VISIBLE_KEYS             NULL
#define SYSTEMD_KEYS_EXCLUDED_FROM_FACETS       NULL
#define SYSTEMD_KEYS_INCLUDED_IN_FACETS         \
    "_TRANSPORT"                                \
    "|SYSLOG_IDENTIFIER"                        \
    "|SYSLOG_FACILITY"                          \
    "|PRIORITY"                                 \
    "|_HOSTNAME"                                \
    "|_RUNTIME_SCOPE"                           \
    "|_PID"                                     \
    "|_UID"                                     \
    "|_GID"                                     \
    "|_SYSTEMD_UNIT"                            \
    "|_SYSTEMD_SLICE"                           \
    "|_SYSTEMD_USER_SLICE"                      \
    "|_COMM"                                    \
    "|_EXE"                                     \
    "|_SYSTEMD_CGROUP"                          \
    "|_SYSTEMD_USER_UNIT"                       \
    "|USER_UNIT"                                \
    "|UNIT"                                     \
    ""

static netdata_mutex_t mutex = NETDATA_MUTEX_INITIALIZER;
static bool plugin_should_exit = false;

// ----------------------------------------------------------------------------

int systemd_journal_query(BUFFER *wb, FACETS *facets, usec_t after_ut, usec_t before_ut, usec_t stop_monotonic_ut) {
    sd_journal *j;
    int r;

    // Open the system journal for reading
    r = sd_journal_open(&j, SD_JOURNAL_ALL_NAMESPACES);
    if (r < 0)
        return HTTP_RESP_INTERNAL_SERVER_ERROR;

    facets_rows_begin(facets);

    bool timed_out = false;
    size_t row_counter = 0;
    sd_journal_seek_realtime_usec(j, before_ut);
    SD_JOURNAL_FOREACH_BACKWARDS(j) {
            row_counter++;

            uint64_t msg_ut;
            sd_journal_get_realtime_usec(j, &msg_ut);
            if (msg_ut < after_ut)
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
                facets_add_key_value_length(facets, key_copy, value, value_length <= FACET_MAX_VALUE_LENGTH ? value_length : FACET_MAX_VALUE_LENGTH);
            }

            facets_row_finished(facets, msg_ut);

            if((row_counter % 100) == 0 && now_monotonic_usec() > stop_monotonic_ut) {
                timed_out = true;
                break;
            }
        }

    sd_journal_close(j);

    buffer_json_member_add_uint64(wb, "status", HTTP_RESP_OK);
    buffer_json_member_add_boolean(wb, "partial", timed_out);
    buffer_json_member_add_string(wb, "type", "table");
    buffer_json_member_add_time_t(wb, "update_every", 1);
    buffer_json_member_add_string(wb, "help", SYSTEMD_JOURNAL_FUNCTION_DESCRIPTION);

    facets_report(facets, wb);

    buffer_json_member_add_time_t(wb, "expires", now_realtime_sec());
    buffer_json_finalize(wb);

    return HTTP_RESP_OK;
}

static void systemd_journal_function_help(const char *transaction) {
    pluginsd_function_result_begin_to_stdout(transaction, HTTP_RESP_OK, "text/plain", now_realtime_sec() + 3600);
    fprintf(stdout,
            "%s / %s\n"
            "\n"
            "%s\n"
            "\n"
            "The following filters are supported:\n"
            "\n"
            "   help\n"
            "      Shows this help message.\n"
            "\n"
            "   before:TIMESTAMP\n"
            "      Absolute or relative (to now) timestamp in seconds, to start the query.\n"
            "      The query is always executed from the most recent to the oldest log entry.\n"
            "      If not given the default is: now.\n"
            "\n"
            "   after:TIMESTAMP\n"
            "      Absolute or relative (to `before`) timestamp in seconds, to end the query.\n"
            "      If not given, the default is %d.\n"
            "\n"
            "   last:ITEMS\n"
            "      The number of items to return.\n"
            "      The default is %d.\n"
            "\n"
            "   anchor:NUMBER\n"
            "      The `timestamp` of the item last received, to return log entries after that.\n"
            "      If not given, the query will return the top `ITEMS` from the most recent.\n"
            "\n"
            "   facet_id:value_id1,value_id2,value_id3,...\n"
            "      Apply filters to the query, based on the facet IDs returned.\n"
            "      Each `facet_id` can be given once, but multiple `facet_ids` can be given.\n"
            "\n"
            "Filters can be combined. Each filter can be given only one time.\n"
            , program_name
            , SYSTEMD_JOURNAL_FUNCTION_NAME
            , SYSTEMD_JOURNAL_FUNCTION_DESCRIPTION
            , -SYSTEMD_JOURNAL_DEFAULT_QUERY_DURATION
            , SYSTEMD_JOURNAL_DEFAULT_ITEMS_PER_QUERY
    );
    pluginsd_function_result_end_to_stdout();
}

static void systemd_journal_dynamic_row_id(FACETS *facets __maybe_unused, BUFFER *wb, FACET_ROW_KEY_VALUE *rkv, FACET_ROW *row, void *data __maybe_unused) {
    FACET_ROW_KEY_VALUE *syslog_identifier_rkv = dictionary_get(row->dict, "SYSLOG_IDENTIFIER");
    FACET_ROW_KEY_VALUE *pid_rkv = dictionary_get(row->dict, "_PID");

    const char *identifier = syslog_identifier_rkv ? buffer_tostring(syslog_identifier_rkv->wb) : "UNKNOWN";
    const char *pid = pid_rkv ? buffer_tostring(pid_rkv->wb) : "UNKNOWN";

    buffer_flush(rkv->wb);
    buffer_sprintf(rkv->wb, "%s[%s]", identifier, pid);

    buffer_json_add_array_item_string(wb, buffer_tostring(rkv->wb));
}

static void function_systemd_journal(const char *transaction, char *function __maybe_unused, char *line_buffer __maybe_unused, int line_max __maybe_unused, int timeout __maybe_unused) {
    char *words[SYSTEMD_JOURNAL_MAX_PARAMS] = { NULL };
    size_t num_words = quoted_strings_splitter_pluginsd(function, words, SYSTEMD_JOURNAL_MAX_PARAMS);

    BUFFER *wb = buffer_create(0, NULL);
    buffer_flush(wb);
    buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_DEFAULT | BUFFER_JSON_OPTIONS_NEWLINE_ON_ARRAYS);

    FACETS *facets = facets_create(50, 0, 0,
                                   SYSTEMD_ALWAYS_VISIBLE_KEYS,
                                   SYSTEMD_KEYS_INCLUDED_IN_FACETS,
                                   SYSTEMD_KEYS_EXCLUDED_FROM_FACETS);

    facets_accepted_param(facets, JOURNAL_PARAMETER_AFTER);
    facets_accepted_param(facets, JOURNAL_PARAMETER_BEFORE);
    facets_accepted_param(facets, JOURNAL_PARAMETER_ANCHOR);
    facets_accepted_param(facets, JOURNAL_PARAMETER_LAST);

    // register the fields in the order you want them on the dashboard

    facets_register_dynamic_key(facets, "ND_JOURNAL_PROCESS", FACET_KEY_OPTION_VISIBLE,
                                systemd_journal_dynamic_row_id, NULL);

    facets_register_key(facets, "MESSAGE",
                        FACET_KEY_OPTION_NO_FACET|FACET_KEY_OPTION_VISIBLE);

    time_t after_s = 0, before_s = 0;
    usec_t anchor = 0;
    size_t last = 0;

    buffer_json_member_add_object(wb, "request");
    buffer_json_member_add_object(wb, "filters");

    for(int i = 1; i < SYSTEMD_JOURNAL_MAX_PARAMS ;i++) {
        const char *keyword = get_word(words, num_words, i);
        if(!keyword) break;

        if(strcmp(keyword, JOURNAL_PARAMETER_HELP) == 0) {
            systemd_journal_function_help(transaction);
            goto cleanup;
        }
        else if(strncmp(keyword, JOURNAL_PARAMETER_AFTER ":", strlen(JOURNAL_PARAMETER_AFTER ":")) == 0) {
            after_s = str2l(&keyword[strlen(JOURNAL_PARAMETER_AFTER ":")]);
        }
        else if(strncmp(keyword, JOURNAL_PARAMETER_BEFORE ":", strlen(JOURNAL_PARAMETER_BEFORE ":")) == 0) {
            before_s = str2l(&keyword[strlen(JOURNAL_PARAMETER_BEFORE ":")]);
        }
        else if(strncmp(keyword, JOURNAL_PARAMETER_ANCHOR ":", strlen(JOURNAL_PARAMETER_ANCHOR ":")) == 0) {
            anchor = str2ull(&keyword[strlen(JOURNAL_PARAMETER_ANCHOR ":")], NULL);
        }
        else if(strncmp(keyword, JOURNAL_PARAMETER_LAST ":", strlen(JOURNAL_PARAMETER_LAST ":")) == 0) {
            last = str2ul(&keyword[strlen(JOURNAL_PARAMETER_LAST ":")]);
        }
        else {
            char *value = strchr(keyword, ':');
            if(value) {
                *value++ = '\0';

                buffer_json_member_add_array(wb, keyword);

                while(value) {
                    char *sep = strchr(value, ',');
                    if(sep)
                        *sep++ = '\0';

                    facets_register_facet_filter(facets, keyword, value, 0);
                    buffer_json_add_array_item_string(wb, value);

                    value = sep;
                }

                buffer_json_array_close(wb); // keyword
            }
        }
    }

    buffer_json_object_close(wb); // filters

    time_t expires = now_realtime_sec() + 1;
    time_t now_s;

    if(!after_s && !before_s) {
        now_s = now_realtime_sec();
        before_s = now_s;
        after_s = before_s - SYSTEMD_JOURNAL_DEFAULT_QUERY_DURATION;
    }
    else
        rrdr_relative_window_to_absolute(&after_s, &before_s, &now_s);

    if(after_s > before_s) {
        time_t tmp = after_s;
        after_s = before_s;
        before_s = tmp;
    }

    if(after_s == before_s)
        after_s = before_s - SYSTEMD_JOURNAL_DEFAULT_QUERY_DURATION;

    if(!last)
        last = SYSTEMD_JOURNAL_DEFAULT_ITEMS_PER_QUERY;

    buffer_json_member_add_time_t(wb, "after", after_s);
    buffer_json_member_add_time_t(wb, "before", before_s);
    buffer_json_member_add_uint64(wb, "anchor", anchor);
    buffer_json_member_add_uint64(wb, "last", last);
    buffer_json_member_add_time_t(wb, "timeout", timeout);
    buffer_json_object_close(wb); // request

    facets_set_items(facets, last);
    facets_set_anchor(facets, anchor);
    int response = systemd_journal_query(wb, facets, after_s * USEC_PER_SEC, before_s * USEC_PER_SEC,
                                       now_monotonic_usec() + (timeout - 1) * USEC_PER_SEC);

    if(response != HTTP_RESP_OK) {
        pluginsd_function_json_error(transaction, response, "failed");
        goto cleanup;
    }

    pluginsd_function_result_begin_to_stdout(transaction, HTTP_RESP_OK, "application/json", expires);
    fwrite(buffer_tostring(wb), buffer_strlen(wb), 1, stdout);

    pluginsd_function_result_end_to_stdout();

cleanup:
    facets_destroy(facets);
    buffer_free(wb);
}

static void *reader_main(void *arg __maybe_unused) {
    char buffer[PLUGINSD_LINE_MAX + 1];

    char *s = NULL;
    while(!plugin_should_exit && (s = fgets(buffer, PLUGINSD_LINE_MAX, stdin))) {

        char *words[PLUGINSD_MAX_WORDS] = { NULL };
        size_t num_words = quoted_strings_splitter_pluginsd(buffer, words, PLUGINSD_MAX_WORDS);

        const char *keyword = get_word(words, num_words, 0);

        if(keyword && strcmp(keyword, PLUGINSD_KEYWORD_FUNCTION) == 0) {
            char *transaction = get_word(words, num_words, 1);
            char *timeout_s = get_word(words, num_words, 2);
            char *function = get_word(words, num_words, 3);

            if(!transaction || !*transaction || !timeout_s || !*timeout_s || !function || !*function) {
                netdata_log_error("Received incomplete %s (transaction = '%s', timeout = '%s', function = '%s'). Ignoring it.",
                                  keyword,
                                  transaction?transaction:"(unset)",
                                  timeout_s?timeout_s:"(unset)",
                                  function?function:"(unset)");
            }
            else {
                int timeout = str2i(timeout_s);
                if(timeout <= 0) timeout = SYSTEMD_JOURNAL_DEFAULT_TIMEOUT;

                netdata_mutex_lock(&mutex);

                if(strncmp(function, SYSTEMD_JOURNAL_FUNCTION_NAME, strlen(SYSTEMD_JOURNAL_FUNCTION_NAME)) == 0)
                    function_systemd_journal(transaction, function, buffer, PLUGINSD_LINE_MAX + 1, timeout);
                else
                    pluginsd_function_json_error(transaction, HTTP_RESP_NOT_FOUND, "No function with this name found in systemd-journal.plugin.");

                fflush(stdout);
                netdata_mutex_unlock(&mutex);
            }
        }
        else
            netdata_log_error("Received unknown command: %s", keyword?keyword:"(unset)");
    }

    if(!s || feof(stdin) || ferror(stdin)) {
        plugin_should_exit = true;
        netdata_log_error("Received error on stdin.");
    }

    exit(1);
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

    // ------------------------------------------------------------------------
    // debug

    if(argc == 2 && strcmp(argv[1], "debug") == 0) {
        function_systemd_journal("123", "", "", 0, 30);
        exit(1);
    }

    // ------------------------------------------------------------------------

    netdata_thread_t reader_thread;
    netdata_thread_create(&reader_thread, "SDJ_READER", NETDATA_THREAD_OPTION_DONT_LOG, reader_main, NULL);

    // ------------------------------------------------------------------------

    time_t started_t = now_monotonic_sec();

    size_t iteration;
    usec_t step = 1000 * USEC_PER_MS;
    bool tty = isatty(fileno(stderr)) == 1;

    netdata_mutex_lock(&mutex);
    fprintf(stdout, PLUGINSD_KEYWORD_FUNCTION " \"%s\" %d \"%s\"\n",
            SYSTEMD_JOURNAL_FUNCTION_NAME, SYSTEMD_JOURNAL_DEFAULT_TIMEOUT, SYSTEMD_JOURNAL_FUNCTION_DESCRIPTION);

    heartbeat_t hb;
    heartbeat_init(&hb);
    for(iteration = 0; 1 ; iteration++) {
        netdata_mutex_unlock(&mutex);
        heartbeat_next(&hb, step);
        netdata_mutex_lock(&mutex);

        if(!tty)
            fprintf(stdout, "\n");

        fflush(stdout);

        time_t now = now_monotonic_sec();
        if(now - started_t > 86400)
            break;
    }

    exit(0);
}
