// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_COMPLETION_H
#define NETDATA_COMPLETION_H

#include "../libnetdata.h"

struct completion {
    netdata_mutex_t mutex;
    netdata_cond_t cond;
    volatile unsigned completed;
    volatile unsigned completed_jobs;
};

void completion_reset(struct completion *p);

void completion_init(struct completion *p);

void completion_destroy(struct completion *p);

void completion_wait_for(struct completion *p);

// Wait for at most `timeout` seconds. Return true on success, false on
// error or timeout.
bool completion_timedwait_for(struct completion *p, uint64_t timeout_s);

void completion_mark_complete(struct completion *p);

unsigned completion_wait_for_a_job(struct completion *p, unsigned completed_jobs);
unsigned completion_wait_for_a_job_with_timeout(struct completion *p, unsigned completed_jobs, uint64_t timeout_ms);
void completion_mark_complete_a_job(struct completion *p);
bool completion_is_done(struct completion *p);

#endif /* NETDATA_COMPLETION_H */
