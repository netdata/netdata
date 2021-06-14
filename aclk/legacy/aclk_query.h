// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_ACLK_QUERY_H
#define NETDATA_ACLK_QUERY_H

#include "libnetdata/libnetdata.h"
#include "web/server/web_client.h"

#define ACLK_STABLE_TIMEOUT 3 // Minimum delay to mark AGENT as stable

#define MAX_GETRUSAGE_CALLS_PER_TICK 5 // Maximum number of times getrusage can be called per tick, per thread.

extern pthread_cond_t legacy_query_cond_wait;
extern pthread_mutex_t legacy_query_lock_wait;
extern uint8_t *getrusage_called_this_tick;
#define LEGACY_QUERY_THREAD_WAKEUP pthread_cond_signal(&legacy_query_cond_wait)
#define LEGACY_QUERY_THREAD_WAKEUP_ALL pthread_cond_broadcast(&legacy_query_cond_wait)
struct aclk_query_thread {
    netdata_thread_t thread;
    int idx;
};

struct aclk_query_threads {
    struct aclk_query_thread *thread_list;
    int count;
};

struct aclk_cloud_req_v2 {
    char *data;
    RRDHOST *host;
    char *query_endpoint;
};

void *legacy_aclk_query_main_thread(void *ptr);
int legacy_aclk_queue_query(char *token, void *data, char *msg_type, char *query, int run_after, int internal, ACLK_CMD cmd);

void legacy_aclk_query_threads_start(struct aclk_query_threads *query_threads);
void legacy_aclk_query_threads_cleanup(struct aclk_query_threads *query_threads);
unsigned int aclk_query_size();

#endif //NETDATA_AGENT_CLOUD_LINK_H
