// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_ACLK_QUERY_QUEUE_H
#define NETDATA_ACLK_QUERY_QUEUE_H

#include "libnetdata/libnetdata.h"
#include "daemon/common.h"

typedef enum {
    UNKNOWN,
    METADATA_INFO,
    METADATA_ALARMS,
    HTTP_API_V2,
    CHART_NEW,
    CHART_DEL,
    ALARM_STATE_UPDATE
} aclk_query_type_t;

struct aclk_query_metadata {
    RRDHOST *host;
    int initial_on_connect;
};

struct aclk_query_chart_add_del {
    RRDHOST *host;
    char* chart_name;
};

struct aclk_query_http_api_v2 {
    char *payload;
    char *query;
};

typedef struct aclk_query *aclk_query_t;
struct aclk_query {
    aclk_query_type_t type;

    // dedup_id is used to deduplicate queries in the list
    // if type and dedup_id is the same message is deduplicated
    // set dedup_id to NULL to never deduplicate the message
    // set dedup_id to constant (e.g. empty string "") to make
    // message of this type ever exist only once in the list
    char *dedup_id;
    char *callback_topic;
    char *msg_id;

    usec_t created;

    aclk_query_t next;

    // TODO maybe remove?
    int version;
    union {
        struct aclk_query_metadata metadata_info;
        struct aclk_query_metadata metadata_alarms;
        struct aclk_query_http_api_v2 http_api_v2;
        struct aclk_query_chart_add_del chart_add_del;
        json_object *alarm_update;
    } data;
};

aclk_query_t aclk_query_new(aclk_query_type_t type);
void aclk_query_free(aclk_query_t query);

int aclk_queue_query(aclk_query_t query);
aclk_query_t aclk_queue_pop(void);
void aclk_queue_flush(void);

void aclk_queue_lock(void);

#endif /* NETDATA_ACLK_QUERY_QUEUE_H */
