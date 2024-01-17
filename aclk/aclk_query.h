// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_ACLK_QUERY_H
#define NETDATA_ACLK_QUERY_H

#include "libnetdata/libnetdata.h"

#include "mqtt_websockets/mqtt_wss_client.h"

#include "aclk_query_queue.h"

extern pthread_cond_t query_cond_wait;
extern pthread_mutex_t query_lock_wait;
#define QUERY_THREAD_WAKEUP pthread_cond_signal(&query_cond_wait)
#define QUERY_THREAD_WAKEUP_ALL pthread_cond_broadcast(&query_cond_wait)

// TODO
//extern volatile int aclk_connected;

struct aclk_query_thread {
    netdata_thread_t thread;
    int idx;
    mqtt_wss_client client;
};

struct aclk_query_threads {
    struct aclk_query_thread *thread_list;
    int count;
};

void aclk_query_threads_start(struct aclk_query_threads *query_threads, mqtt_wss_client client);
void aclk_query_threads_cleanup(struct aclk_query_threads *query_threads);

const char *aclk_query_get_name(aclk_query_type_t qt, int unknown_ok);

int mark_pending_req_cancelled(const char *msg_id);

#endif //NETDATA_AGENT_CLOUD_LINK_H
