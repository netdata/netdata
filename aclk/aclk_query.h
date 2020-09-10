// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_ACLK_QUERY_H
#define NETDATA_ACLK_QUERY_H

#include "libnetdata/libnetdata.h"
#include "web/server/web_client.h"

#define ACLK_STABLE_TIMEOUT 3 // Minimum delay to mark AGENT as stable

extern pthread_cond_t query_cond_wait;
extern pthread_mutex_t query_lock_wait;
#define QUERY_THREAD_WAKEUP pthread_cond_signal(&query_cond_wait)
#define QUERY_THREAD_WAKEUP_ALL pthread_cond_broadcast(&query_cond_wait)

extern volatile int aclk_connected;

struct aclk_query_thread {
    netdata_thread_t thread;
    int idx;
};

struct aclk_query_threads {
    struct aclk_query_thread *thread_list;
    int count;
};

void *aclk_query_main_thread(void *ptr);
int aclk_queue_query(char *token, void *data, char *msg_type, char *query, int run_after, int internal, ACLK_CMD cmd);

void aclk_query_threads_start(struct aclk_query_threads *query_threads);
void aclk_query_threads_cleanup(struct aclk_query_threads *query_threads);
unsigned int aclk_query_size();

#endif //NETDATA_AGENT_CLOUD_LINK_H
