// SPDX-License-Identifier: GPL-3.0-or-later
/*
 *  netdata systemd-journal.plugin
 *  Copyright (C) 2023 Netdata Inc.
 *  GPL v3+
 */

#include "libnetdata/libnetdata.h"
#include "libnetdata/required_dummies.h"

#include <systemd/sd-journal.h>

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
