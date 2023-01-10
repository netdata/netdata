// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_COMPLETION_H
#define NETDATA_COMPLETION_H

#include "../libnetdata.h"

struct completion {
    uv_mutex_t mutex;
    uv_cond_t cond;
    volatile unsigned completed;
    volatile unsigned completed_jobs;
};

void completion_init(struct completion *p);

void completion_destroy(struct completion *p);

void completion_wait_for(struct completion *p);

void completion_mark_complete(struct completion *p);

unsigned completion_wait_for_a_job(struct completion *p, unsigned completed_jobs);
void completion_mark_complete_a_job(struct completion *p);
bool completion_is_done(struct completion *p);

#endif /* NETDATA_COMPLETION_H */
