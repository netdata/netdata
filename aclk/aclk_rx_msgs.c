// SPDX-License-Identifier: GPL-3.0-or-later

#include "aclk_rx_msgs.h"

#include "aclk_stats.h"
#include "aclk_query_queue.h"
#include "aclk.h"

#define ACLK_V2_PAYLOAD_SEPARATOR "\x0D\x0A\x0D\x0A"
#define ACLK_CLOUD_REQ_V2_PREFIX "GET /api/v1/"

#define ACLK_V_COMPRESSION 2

struct aclk_request {
    char *type_id;
    char *msg_id;
    char *callback_topic;
    char *payload;
    int version;
    int min_version;
    int max_version;
};

static int cloud_to_agent_parse(JSON_ENTRY *e)
{
    struct aclk_request *data = e->callback_data;

    switch (e->type) {
        case JSON_OBJECT:
        case JSON_ARRAY:
            break;
        case JSON_STRING:
            if (!strcmp(e->name, "msg-id")) {
                data->msg_id = strdupz(e->data.string);
                break;
            }
            if (!strcmp(e->name, "type")) {
                data->type_id = strdupz(e->data.string);
                break;
            }
            if (!strcmp(e->name, "callback-topic")) {
                data->callback_topic = strdupz(e->data.string);
                break;
            }
            if (!strcmp(e->name, "payload")) {
                if (likely(e->data.string)) {
                    size_t len = strlen(e->data.string);
                    data->payload = mallocz(len+1);
                    if (!url_decode_r(data->payload, e->data.string, len + 1))
                        strcpy(data->payload, e->data.string);
                }
                break;
            }
            break;
        case JSON_NUMBER:
            if (!strcmp(e->name, "version")) {
                data->version = e->data.number;
                break;
            }
            if (!strcmp(e->name, "min-version")) {
                data->min_version = e->data.number;
                break;
            }
            if (!strcmp(e->name, "max-version")) {
                data->max_version = e->data.number;
                break;
            }

            break;

        case JSON_BOOLEAN:
            break;

        case JSON_NULL:
            break;
    }
    return 0;
}

static inline int aclk_extract_v2_data(char *payload, char **data)
{
    char* ptr = strstr(payload, ACLK_V2_PAYLOAD_SEPARATOR);
    if(!ptr)
        return 1;
    ptr += strlen(ACLK_V2_PAYLOAD_SEPARATOR);
    *data = strdupz(ptr);
    return 0;
}

static inline int aclk_v2_payload_get_query(const char *payload, char **query_url)
{
    const char *start, *end;

    if(strncmp(payload, ACLK_CLOUD_REQ_V2_PREFIX, strlen(ACLK_CLOUD_REQ_V2_PREFIX))) {
        errno = 0;
        error("Only accepting requests that start with \"%s\" from CLOUD.", ACLK_CLOUD_REQ_V2_PREFIX);
        return 1;
    }
    start = payload + 4;

    if(!(end = strstr(payload, " HTTP/1.1\x0D\x0A"))) {
        errno = 0;
        error("Doesn't look like HTTP GET request.");
        return 1;
    }

    *query_url = mallocz((end - start) + 1);
    strncpyz(*query_url, start, end - start);

    return 0;
}

#define HTTP_CHECK_AGENT_INITIALIZED() ACLK_SHARED_STATE_LOCK;\
    if (unlikely(aclk_shared_state.agent_state == ACLK_HOST_INITIALIZING)) {\
        debug(D_ACLK, "Ignoring \"http\" cloud request; agent not in stable state");\
        ACLK_SHARED_STATE_UNLOCK;\
        return 1;\
    }\
    ACLK_SHARED_STATE_UNLOCK;

static int aclk_handle_cloud_request_v2(struct aclk_request *cloud_to_agent, char *raw_payload)
{
    HTTP_CHECK_AGENT_INITIALIZED();

    aclk_query_t query;

    errno = 0;
    if (cloud_to_agent->version < ACLK_V_COMPRESSION) {
        error(
            "This handler cannot reply to request with version older than %d, received %d.",
            ACLK_V_COMPRESSION,
            cloud_to_agent->version);
        return 1;
    }

    query = aclk_query_new(HTTP_API_V2);

    if (unlikely(aclk_extract_v2_data(raw_payload, &query->data.http_api_v2.payload))) {
        error("Error extracting payload expected after the JSON dictionary.");
        goto error;
    }

    if (unlikely(aclk_v2_payload_get_query(query->data.http_api_v2.payload, &query->dedup_id))) {
        error("Could not extract payload from query");
        goto error;
    }

    if (unlikely(!cloud_to_agent->callback_topic)) {
        error("Missing callback_topic");
        goto error;
    }

    if (unlikely(!cloud_to_agent->msg_id)) {
        error("Missing msg_id");
        goto error;
    }

    // aclk_queue_query takes ownership of data pointer
    query->callback_topic = cloud_to_agent->callback_topic;
    // for clarity and code readability as when we process the request
    // it would be strange to get URL from `dedup_id`
    query->data.http_api_v2.query = query->dedup_id;
    query->msg_id = cloud_to_agent->msg_id;
    aclk_queue_query(query);
    return 0;

error:
    aclk_query_free(query);
    return 1;
}

typedef struct aclk_incoming_msg_type{
    char *name;
    int(*fnc)(struct aclk_request *, char *);
}aclk_incoming_msg_type;

aclk_incoming_msg_type aclk_incoming_msg_types_compression[] = {
    { .name = "http",    .fnc = aclk_handle_cloud_request_v2 },
    { .name = NULL,      .fnc = NULL                         }
};

struct aclk_incoming_msg_type *aclk_incoming_msg_types = aclk_incoming_msg_types_compression;

int aclk_handle_cloud_message(char *payload)
{
    struct aclk_request cloud_to_agent;
    memset(&cloud_to_agent, 0, sizeof(struct aclk_request));

    if (aclk_stats_enabled) {
        ACLK_STATS_LOCK;
        aclk_metrics_per_sample.cloud_req_recvd++;
        ACLK_STATS_UNLOCK;
    }

    if (unlikely(!payload)) {
        errno = 0;
        error("ACLK incoming message is empty");
        goto err_cleanup_nojson;
    }

    debug(D_ACLK, "ACLK incoming message (%s)", payload);

    int rc = json_parse(payload, &cloud_to_agent, cloud_to_agent_parse);

    if (unlikely(rc != JSON_OK)) {
        errno = 0;
        error("Malformed json request (%s)", payload);
        goto err_cleanup;
    }

    if (!cloud_to_agent.type_id) {
        errno = 0;
        error("Cloud message is missing compulsory key \"type\"");
        goto err_cleanup;
    }


    for (int i = 0; aclk_incoming_msg_types[i].name; i++) {
        if (strcmp(cloud_to_agent.type_id, aclk_incoming_msg_types[i].name) == 0) {
            if (likely(!aclk_incoming_msg_types[i].fnc(&cloud_to_agent, payload))) {
                // in case of success handler is supposed to clean up after itself
                // or as in the case of aclk_handle_cloud_request take
                // ownership of the pointers (done to avoid copying)
                // see what `aclk_queue_query` parameter `internal` does

                // NEVER CONTINUE THIS LOOP AFTER CALLING FUNCTION!!!
                // msg handlers (namely aclk_handle_version_responce)
                // can freely change what aclk_incoming_msg_types points to
                // so either exit or restart this for loop
                freez(cloud_to_agent.type_id);
                return 0;
            }
            goto err_cleanup;
        }
    }

    errno = 0;
    error("Unknown message type from Cloud \"%s\"", cloud_to_agent.type_id);

err_cleanup:
    if (cloud_to_agent.payload)
        freez(cloud_to_agent.payload);
    if (cloud_to_agent.type_id)
        freez(cloud_to_agent.type_id);
    if (cloud_to_agent.msg_id)
        freez(cloud_to_agent.msg_id);
    if (cloud_to_agent.callback_topic)
        freez(cloud_to_agent.callback_topic);

err_cleanup_nojson:
    if (aclk_stats_enabled) {
        ACLK_STATS_LOCK;
        aclk_metrics_per_sample.cloud_req_err++;
        ACLK_STATS_UNLOCK;
    }

    return 1;
}
