// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_ACLK_QUERY_QUEUE_H
#define NETDATA_ACLK_QUERY_QUEUE_H

#include "libnetdata/libnetdata.h"
#include "daemon/common.h"
#include "schema-wrappers/schema_wrappers.h"

#include "aclk_util.h"

struct aclk_query_http_api_v2 {
    char *payload;
    char *query;
};

struct aclk_bin_payload { 
    char *payload;
    size_t size;
    enum aclk_topics topic;
    const char *msg_name;
};

typedef struct aclk_query *aclk_query_t;
struct aclk_query {
    // dedup_id is used to deduplicate queries in the list
    // if type and dedup_id is the same message is deduplicated
    // set dedup_id to NULL to never deduplicate the message
    // set dedup_id to constant (e.g. empty string "") to make
    // message of this type ever exist only once in the list
    char *dedup_id;
    char *callback_topic;
    char *msg_id;

    struct timeval created_tv;
    usec_t created;
    int timeout;
    aclk_query_t next;

    // TODO maybe remove?
    int version;

    struct aclk_query_http_api_v2 http_api_v2;
};

aclk_query_t aclk_query_new();
void aclk_query_free(aclk_query_t query);

int aclk_queue_query(aclk_query_t query);
aclk_query_t aclk_queue_pop(void);
void aclk_queue_flush(void);

void aclk_queue_lock(void);
void aclk_queue_unlock(void);

#endif /* NETDATA_ACLK_QUERY_QUEUE_H */
