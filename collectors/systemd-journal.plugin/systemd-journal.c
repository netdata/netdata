// SPDX-License-Identifier: GPL-3.0-or-later
/*
 *  netdata systemd-journal.plugin
 *  Copyright (C) 2023 Netdata Inc.
 *  GPL v3+
 */

#include "libnetdata/libnetdata.h"
#include "libnetdata/required_dummies.h"

#include <systemd/sd-journal.h>

#define SYSTEMD_KEYS_EXCLUDED_FROM_FACETS \
    "*MESSAGE*"                           \
    "|*CMDLINE*"                          \
    "|*TIMESTAMP*"                        \
    "|*INVOCATION*"                       \
    "|*USAGE*"                            \
    "|*RLIMIT*"                           \
    "|*_ID"                               \
    "|!*COREDUMP_SIGNAL_NAME|*COREDUMP*"  \
    "|*CODE_LINE*"                        \
    "|_STREAM_ID"                         \
    "|SYSLOG_RAW"                         \
    "|_PID"                               \
    "|_CAP_EFFECTIVE"                     \
    "|_AUDIT_SESSION"                     \
    "|_AUDIT_LOGINUID"                    \
    "|_SYSTEMD_OWNER_UID"                 \
    "|GLIB_OLD_LOG_API"                   \
    "|SYSLOG_PID"                         \
    "|TID"                                \
    "|JOB_ID"                             \
    "|_SYSTEMD_SESSION"                   \
    ""

#define MAX_VALUE_LENGTH 4095

struct systemd_journal_request {
    usec_t after_ut;
    usec_t before_ut;
    usec_t anchor;

    size_t items;

    const char *filters;

    BUFFER *wb;

    int response;
};

int systemd_journal_query(struct systemd_journal_request *c) {
    if(!c->wb)
        return HTTP_RESP_BAD_REQUEST;

    FACETS *facets = facets_create(c->items, c->anchor, NULL, SYSTEMD_KEYS_EXCLUDED_FROM_FACETS);
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

    sd_journal_seek_realtime_usec(j, c->before_ut);
    SD_JOURNAL_FOREACH_BACKWARDS(j) {
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
                facets_add_key_value_length(facets, key_copy, value, value_length <= MAX_VALUE_LENGTH ? value_length : MAX_VALUE_LENGTH);
            }

            facets_row_finished(facets, msg_ut);
        }

    sd_journal_close(j);

    buffer_flush(c->wb);
    buffer_json_initialize(c->wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_DEFAULT | BUFFER_JSON_OPTIONS_NEWLINE_ON_ARRAYS);
    facets_report(facets, c->wb);
    buffer_json_finalize(c->wb);

    facets_destroy(facets);

    return HTTP_RESP_OK;
}

void *worker_query(void *ptr) {
    struct systemd_journal_request *c = ptr;
    c->response = systemd_journal_query(c);
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

    // ------------------------------------------------------------------------

    struct systemd_journal_request c = {
            .wb = buffer_create(0, NULL),
            .before_ut = now_realtime_usec(),
            .after_ut = now_realtime_usec() - 86400 * USEC_PER_SEC,
            .anchor = 0,
            .items = 50,
            .filters = NULL,
            .response = HTTP_RESP_INTERNAL_SERVER_ERROR,
    };

    c.response = systemd_journal_query(&c);

    fprintf(stdout, "%s\nHTTP RESPONSE CODE: %d\n", buffer_tostring(c.wb), c.response);
    buffer_free(c.wb);

    /*
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
    */
}
