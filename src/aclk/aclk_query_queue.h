// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_ACLK_QUERY_QUEUE_H
#define NETDATA_ACLK_QUERY_QUEUE_H

#include "libnetdata/libnetdata.h"
#include "daemon/common.h"
#include "schema-wrappers/schema_wrappers.h"

#include "aclk_util.h"

typedef enum {
    UNKNOWN = 0,
    HTTP_API_V2,
    REGISTER_NODE,
    NODE_STATE_UPDATE,
    UPDATE_NODE_INFO,
    ALARM_PROVIDE_CFG,
    ALARM_SNAPSHOT,
    UPDATE_NODE_COLLECTORS,
    PROTO_BIN_MESSAGE,
    ACLK_QUERY_TYPE_COUNT // always keep this as last
} aclk_query_type_t;

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
    aclk_query_type_t type;

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

    // TODO maybe remove?
    int version;
    union {
        struct aclk_query_http_api_v2 http_api_v2;
        struct aclk_bin_payload bin_payload;
    } data;
};

aclk_query_t aclk_query_new(aclk_query_type_t type);
void aclk_query_free(aclk_query_t query);

void aclk_execute_query(aclk_query_t query);

#define QUEUE_IF_PAYLOAD_PRESENT(query)                                                                                \
    do {                                                                                                               \
        if (likely((query)->data.bin_payload.payload)) {                                                               \
            aclk_execute_query(query);                                                                                 \
        } else {                                                                                                       \
            nd_log(NDLS_DAEMON, NDLP_ERR, "Failed to generate payload");                                               \
            aclk_query_free(query);                                                                                    \
        }                                                                                                              \
    } while (0)

#endif /* NETDATA_ACLK_QUERY_QUEUE_H */
