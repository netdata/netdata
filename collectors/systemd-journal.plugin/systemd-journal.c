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

#define SYSTEMD_JOURNAL_FUNCTION_DESCRIPTION "View, search and analyze systemd journal entries."
#define SYSTEMD_JOURNAL_DEFAULT_TIMEOUT 30

#define SYSTEMD_ALWAYS_VISIBLE_KEYS         \
    "timestamp"                             \
    "|MESSAGE"                              \
    "|UNIT"                                 \
    "|_PID"                                 \
    "|_COMM"                                \
    "|_SYSTEMD_UNIT"                        \
    "|_SYSTEMD_SLICE"                       \
    ""

#define SYSTEMD_KEYS_INCLUDED_IN_FACETS     \
    "_TRANSPORT"                            \
    "|SYSLOG_IDENTIFIER"                    \
    "|_HOSTNAME"                            \
    "|_RUNTIME_SCOPE"                       \
    "|_UID"                                 \
    "|_GID"                                 \
    "|_SYSTEMD_UNIT"                        \
    "|_SYSTEMD_SLICE"                       \
    "|_SYSTEMD_USER_SLICE"                  \
    "|_COMM"                                \
    "|_EXE"                                 \
    "|_SYSTEMD_CGROUP"                      \
    "|_SYSTEMD_USER_UNIT"                   \
    "|USER_UNIT"                            \
    "|UNIT"                                 \
    ""

//#define SYSTEMD_KEYS_EXCLUDED_FROM_FACETS \
//    "*MESSAGE*"                           \
//    "|*CMDLINE*"                          \
//    "|*TIMESTAMP*"                        \
//    "|*MONOTONIC*"                        \
//    "|*REALTIME*"                         \
//    "|*BOOTIME*"                          \
//    "|*_USEC*"                            \
//    "|*INVOCATION*"                       \
//    "|*USAGE*"                            \
//    "|*RLIMIT*"                           \
//    "|*_ID"                               \
//    "|!*COREDUMP_SIGNAL_NAME|*COREDUMP*"  \
//    "|*CODE_LINE*"                        \
//    "|_STREAM_ID"                         \
//    "|SYSLOG_RAW"                         \
//    "|_PID"                               \
//    "|_CAP_EFFECTIVE"                     \
//    "|_AUDIT_SESSION"                     \
//    "|_AUDIT_LOGINUID"                    \
//    "|_SYSTEMD_OWNER_UID"                 \
//    "|GLIB_OLD_LOG_API"                   \
//    "|SYSLOG_PID"                         \
//    "|TID"                                \
//    "|JOB_ID"                             \
//    "|_SYSTEMD_SESSION"                   \
//    ""

#define FACET_MAX_VALUE_LENGTH 8192

static netdata_mutex_t mutex = NETDATA_MUTEX_INITIALIZER;
static bool plugin_should_exit = false;

// ----------------------------------------------------------------------------

struct systemd_journal_request {
    usec_t after_ut;
    usec_t before_ut;
    usec_t anchor;

    size_t items;

    const char *filters;

    usec_t stop_monotonic_ut;

    BUFFER *wb;

    int response;
};

int systemd_journal_query(struct systemd_journal_request *c) {
    if(!c->wb)
        return HTTP_RESP_BAD_REQUEST;

    FACETS *facets = facets_create(c->items, c->anchor,
                                   SYSTEMD_ALWAYS_VISIBLE_KEYS,
                                   SYSTEMD_KEYS_INCLUDED_IN_FACETS,
                                   NULL);

    // FIXME initialize facets filters here
    facets_rows_begin(facets);

    sd_journal *j;
    int r;

    // Open the system journal for reading
    r = sd_journal_open(&j, SD_JOURNAL_ALL_NAMESPACES);
    if (r < 0) {
        c->wb->content_type = CT_TEXT_PLAIN;
        buffer_flush(c->wb);
        buffer_sprintf(c->wb, "failed to open journal: %s", strerror(-r));
        return HTTP_RESP_INTERNAL_SERVER_ERROR;
    }

    bool timed_out = false;
    size_t row_counter = 0;
    sd_journal_seek_realtime_usec(j, c->before_ut);
    SD_JOURNAL_FOREACH_BACKWARDS(j) {
            row_counter++;

            uint64_t msg_ut;
            sd_journal_get_realtime_usec(j, &msg_ut);
            if (msg_ut < c->after_ut)
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

            if((row_counter % 100) == 0 && now_monotonic_usec() > c->stop_monotonic_ut) {
                timed_out = true;
                break;
            }
        }

    sd_journal_close(j);

    buffer_flush(c->wb);
    buffer_json_initialize(c->wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_DEFAULT | BUFFER_JSON_OPTIONS_NEWLINE_ON_ARRAYS);
    buffer_json_member_add_uint64(c->wb, "status", HTTP_RESP_OK);
    buffer_json_member_add_boolean(c->wb, "timed_out", timed_out);
    buffer_json_member_add_string(c->wb, "type", "table");
    buffer_json_member_add_time_t(c->wb, "update_every", 1);
    buffer_json_member_add_string(c->wb, "help", SYSTEMD_JOURNAL_FUNCTION_DESCRIPTION);

    facets_report(facets, c->wb);

    buffer_json_member_add_time_t(c->wb, "expires", now_realtime_sec());
    buffer_json_finalize(c->wb);

    facets_destroy(facets);

    return HTTP_RESP_OK;
}

#define JOURNAL_PARAMETER_AFTER "after:"
#define JOURNAL_PARAMETER_BEFORE "before:"

static void function_systemd_journal(const char *transaction, char *function __maybe_unused, char *line_buffer __maybe_unused, int line_max __maybe_unused, int timeout __maybe_unused) {
    char *words[PLUGINSD_MAX_WORDS] = { NULL };
    size_t num_words = quoted_strings_splitter_pluginsd(function, words, PLUGINSD_MAX_WORDS);

    time_t after = 0, before = 0;

    for(int i = 1; i < PLUGINSD_MAX_WORDS ;i++) {
        const char *keyword = get_word(words, num_words, i);
        if(!keyword) break;

        if(strncmp(keyword, JOURNAL_PARAMETER_AFTER, strlen(JOURNAL_PARAMETER_AFTER)) == 0) {
            after = str2l(&keyword[strlen(JOURNAL_PARAMETER_AFTER)]);
        }
        else if(strncmp(keyword, JOURNAL_PARAMETER_BEFORE, strlen(JOURNAL_PARAMETER_BEFORE)) == 0) {
            before = str2l(&keyword[strlen(JOURNAL_PARAMETER_BEFORE)]);
        }
    }

    time_t expires = now_realtime_sec() + 1;

    struct systemd_journal_request c = {
            .wb = buffer_create(0, NULL),
            .before_ut = now_realtime_usec(),
            .after_ut = now_realtime_usec() - 86400 * USEC_PER_SEC,
            .anchor = 0,
            .items = 50,
            .filters = NULL,
            .stop_monotonic_ut = now_monotonic_usec() + (timeout - 1) * USEC_PER_SEC,
            .response = HTTP_RESP_INTERNAL_SERVER_ERROR,
    };

    c.response = systemd_journal_query(&c);

    pluginsd_function_result_begin_to_stdout(transaction, HTTP_RESP_OK, "application/json", expires);
    fwrite(buffer_tostring(c.wb), buffer_strlen(c.wb), 1, stdout);
    buffer_free(c.wb);

    pluginsd_function_result_end_to_stdout();
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

                if(strncmp(function, "systemd-journal", strlen("systemd-journal")) == 0)
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


//    // debug
//    function_systemd_journal("123", "", "", 0, 30);
//    exit(1);

    // ------------------------------------------------------------------------

    netdata_thread_t reader_thread;
    netdata_thread_create(&reader_thread, "SDJ_READER", NETDATA_THREAD_OPTION_DONT_LOG, reader_main, NULL);

    // ------------------------------------------------------------------------

    time_t started_t = now_monotonic_sec();

    size_t iteration;
    usec_t step = 1000 * USEC_PER_MS;
    bool tty = isatty(fileno(stderr)) == 1;

    netdata_mutex_lock(&mutex);
    fprintf(stdout, PLUGINSD_KEYWORD_FUNCTION " \"systemd-journal\" %d \"%s\"\n", SYSTEMD_JOURNAL_DEFAULT_TIMEOUT, SYSTEMD_JOURNAL_FUNCTION_DESCRIPTION);

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
