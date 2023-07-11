// SPDX-License-Identifier: GPL-3.0-or-later
/*
 *  netdata systemd-journal.plugin
 *  Copyright (C) 2023 Netdata Inc.
 *  GPL v3+
 */

#include "libnetdata/libnetdata.h"
#include "libnetdata/required_dummies.h"

#include <systemd/sd-journal.h>

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
            printf("%.*s\n", (int)length, (const char *)data);
        }

        printf("\n");
    }

    sd_journal_close(j);
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
