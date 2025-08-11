// SPDX-License-Identifier: GPL-3.0-or-later

#include "aclk_rx_msgs.h"

#include "aclk_query_queue.h"
#include "aclk.h"
#include "aclk_capas.h"
#include "aclk_query.h"
#include "mqtt_websockets/aclk_mqtt_workers.h"

#include "schema-wrappers/proto_2_json.h"

#define ACLK_V2_PAYLOAD_SEPARATOR "\x0D\x0A\x0D\x0A"

#define ACLK_V_COMPRESSION 2

struct aclk_request {
    char *type_id;
    char *msg_id;
    char *callback_topic;
    char *payload;
    int version;
    int timeout;
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
                data->version = (int)e->data.number;
                break;
            }
            if (!strcmp(e->name, "timeout")) {
                data->timeout = (int)e->data.number;
                break;
            }
            if (!strcmp(e->name, "min-version")) {
                data->min_version = (int)e->data.number;
                break;
            }
            if (!strcmp(e->name, "max-version")) {
                data->max_version = (int)e->data.number;
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

    if(strncmp(payload, "GET /", 5) == 0 || strncmp(payload, "PUT /", 5) == 0)
        start = payload + 4;
    else if(strncmp(payload, "POST /", 6) == 0)
        start = payload + 5;
    else if(strncmp(payload, "DELETE /", 8) == 0)
        start = payload + 7;
    else {
        errno_clear();
        netdata_log_error("Only accepting requests that start with GET, POST, PUT, DELETE from CLOUD.");
        return 1;
    }

    if(!(end = strstr(payload, HTTP_1_1 HTTP_ENDL))) {
        errno_clear();
        netdata_log_error("Doesn't look like HTTP GET request.");
        return 1;
    }

    *query_url = mallocz((end - start) + 1);
    strncpyz(*query_url, start, end - start);

    return 0;
}

static int aclk_handle_cloud_http_request_v2(struct aclk_request *cloud_to_agent, char *raw_payload)
{
    errno_clear();
    if (cloud_to_agent->version < ACLK_V_COMPRESSION) {
        netdata_log_error(
            "This handler cannot reply to request with version older than %d, received %d.",
            ACLK_V_COMPRESSION,
            cloud_to_agent->version);
        return 1;
    }

    aclk_query_t *query = aclk_query_new(HTTP_API_V2);

    if (unlikely(aclk_extract_v2_data(raw_payload, &query->data.http_api_v2.payload))) {
        netdata_log_error("Error extracting payload expected after the JSON dictionary.");
        goto error;
    }

    if (unlikely(aclk_v2_payload_get_query(query->data.http_api_v2.payload, &query->dedup_id))) {
        netdata_log_error("Could not extract payload from query");
        goto error;
    }

    if (unlikely(!cloud_to_agent->callback_topic)) {
        netdata_log_error("Missing callback_topic");
        goto error;
    }

    if (unlikely(!cloud_to_agent->msg_id)) {
        netdata_log_error("Missing msg_id");
        goto error;
    }

    // aclk_queue_query takes ownership of data pointer
    query->callback_topic = cloud_to_agent->callback_topic;
    query->timeout = cloud_to_agent->timeout;
    // for clarity and code readability as when we process the request
    // it would be strange to get URL from `dedup_id`
    query->data.http_api_v2.query = query->dedup_id;
    query->msg_id = cloud_to_agent->msg_id;
    aclk_execute_query(query);
    return 0;

error:
    aclk_query_free(query);
    return 1;
}

int aclk_handle_cloud_cmd_message(char *payload)
{
    struct aclk_request cloud_to_agent;
    memset(&cloud_to_agent, 0, sizeof(struct aclk_request));

    if (unlikely(!payload)) {
        error_report("ACLK incoming 'cmd' message is empty");
        return 1;
    }

    netdata_log_debug(D_ACLK, "ACLK incoming 'cmd' message (%s)", payload);

    int rc = json_parse(payload, &cloud_to_agent, cloud_to_agent_parse);

    if (unlikely(rc != JSON_OK)) {
        error_report("Malformed json request (%s)", payload);
        goto err_cleanup;
    }

    if (!cloud_to_agent.type_id) {
        error_report("Cloud message is missing compulsory key \"type\"");
        goto err_cleanup;
    }

    // Originally we were expecting to have multiple types of 'cmd' message,
    // but after the new protocol was designed we will ever only have 'http'
    if (strcmp(cloud_to_agent.type_id, "http") != 0) {
        error_report("Only 'http' cmd message is supported");
        goto err_cleanup;
    }

    if (likely(!aclk_handle_cloud_http_request_v2(&cloud_to_agent, payload))) {
        // aclk_handle_cloud_request takes ownership of the pointers
        // (to avoid copying) in case of success
        freez(cloud_to_agent.type_id);
        return 0;
    }

err_cleanup:
    if (cloud_to_agent.payload)
        freez(cloud_to_agent.payload);
    if (cloud_to_agent.type_id)
        freez(cloud_to_agent.type_id);
    if (cloud_to_agent.msg_id)
        freez(cloud_to_agent.msg_id);
    if (cloud_to_agent.callback_topic)
        freez(cloud_to_agent.callback_topic);

    return 1;
}

typedef uint32_t simple_hash_t;
typedef int(*rx_msg_handler)(const char *msg, size_t msg_len);

int handle_old_proto_cmd(const char *msg, size_t msg_len)
{
    // msg is binary payload in all other cases
    // however in this message from old legacy cloud
    // we have to convert it to C string
    char *str = mallocz(msg_len+1);
    memcpy(str, msg, msg_len);
    str[msg_len] = 0;
    int rc = aclk_handle_cloud_cmd_message(str);
    freez(str);
    return rc;
}

int create_node_instance_result(const char *msg, size_t msg_len)
{
    node_instance_creation_result_t res = parse_create_node_instance_result(msg, msg_len);
    if (!res.machine_guid || !res.node_id) {
        error_report("Error parsing CreateNodeInstanceResult");
        freez(res.machine_guid);
        freez(res.node_id);
        return 1;
    }

    netdata_log_debug(D_ACLK, "CreateNodeInstanceResult: guid:%s nodeid:%s", res.machine_guid, res.node_id);

    aclk_query_t *query = aclk_query_new(CREATE_NODE_INSTANCE);

    query->data.node_id = res.node_id;          // Will be freed on query free
    query->machine_guid = res.machine_guid;     // Will be freed on query free
    aclk_add_job(query);
    return 0;
}

int send_node_instances(const char *msg, size_t msg_len)
{
    UNUSED(msg);
    UNUSED(msg_len);
    aclk_query_t *query = aclk_query_new(SEND_NODE_INSTANCES);
    aclk_add_job(query);
    return 0;
}

int stream_charts_and_dimensions(const char *msg, size_t msg_len)
{
    UNUSED(msg);
    UNUSED(msg_len);
    error_report("Received obsolete StreamChartsAndDimensions msg");
    return 0;
}

int charts_and_dimensions_ack(const char *msg, size_t msg_len)
{
    UNUSED(msg);
    UNUSED(msg_len);
    error_report("Received obsolete StreamChartsAndDimensionsAck msg");
    return 0;
}

int update_chart_configs(const char *msg, size_t msg_len)
{
    UNUSED(msg);
    UNUSED(msg_len);
    error_report("Received obsolete UpdateChartConfigs msg");
    return 0;
}

int start_alarm_streaming(const char *msg, size_t msg_len)
{
    struct start_alarm_streaming res = parse_start_alarm_streaming(msg, msg_len);
    if (!res.node_id) {
        netdata_log_error("Error parsing StartAlarmStreaming");
        return 1;
    }
    aclk_query_t *query = aclk_query_new(ALERT_START_STREAMING);
    query->data.node_id = res.node_id;      // Will be freed on query free
    query->version = res.version;
    aclk_add_job(query);
    return 0;
}

int send_alarm_checkpoint(const char *msg, size_t msg_len)
{
    struct send_alarm_checkpoint sac = parse_send_alarm_checkpoint(msg, msg_len);
    if (!sac.node_id || !sac.claim_id) {
        netdata_log_error("Error parsing SendAlarmCheckpoint");
        freez(sac.node_id);
        freez(sac.claim_id);
        return 1;
    }
    aclk_query_t *query = aclk_query_new(ALERT_CHECKPOINT);
    query->data.node_id = sac.node_id;  // Will be freed on query free
    query->claim_id = sac.claim_id;
    query->version = sac.version;
    aclk_add_job(query);
    return 0;
}

int send_alarm_configuration(const char *msg, size_t msg_len)
{
    char *config_hash = parse_send_alarm_configuration(msg, msg_len);
    if (!config_hash || !*config_hash) {
        netdata_log_error("Error parsing SendAlarmConfiguration");
        freez(config_hash);
        return 1;
    }
    aclk_send_alert_configuration(config_hash);
    freez(config_hash);
    return 0;
}

int send_alarm_snapshot(const char *msg, size_t msg_len)
{
    struct send_alarm_snapshot *sas = parse_send_alarm_snapshot(msg, msg_len);
    if (!sas->node_id || !sas->claim_id || !sas->snapshot_uuid) {
        netdata_log_error("Error parsing SendAlarmSnapshot");
        destroy_send_alarm_snapshot(sas);
        return 1;
    }
    aclk_query_t *query = aclk_query_new(ALERT_CHECKPOINT);
    query->data.node_id = sas->node_id;     // Will be freed on query free
    query->claim_id = sas->claim_id;        // Will be freed on query free
    query->version = 0; // force snapshot
    aclk_add_job(query);
    destroy_send_alarm_snapshot(sas);
    return 0;
}

int handle_disconnect_req(const char *msg, size_t msg_len)
{
    struct disconnect_cmd *cmd = parse_disconnect_cmd(msg, msg_len);
    if (!cmd)
        return 1;
    if (cmd->permaban) {
        netdata_log_error("Cloud Banned This Agent!");
        aclk_disable_runtime = 1;
    }
    netdata_log_info("Cloud requested disconnect (EC=%u, \"%s\")", (unsigned int)cmd->error_code, cmd->error_description);
    if (cmd->reconnect_after_s > 0) {
        aclk_block_until = now_monotonic_sec() + cmd->reconnect_after_s;
        netdata_log_info(
            "Cloud asks not to reconnect for %u seconds. We shall honor that request",
            (unsigned int)cmd->reconnect_after_s);
    }
    disconnect_req = ACLK_CLOUD_DISCONNECT;
    freez(cmd->error_description);
    freez(cmd);
    return 0;
}

int contexts_checkpoint(const char *msg, size_t msg_len)
{
    aclk_ctx_based = 1;

    struct ctxs_checkpoint *cmd = parse_ctxs_checkpoint(msg, msg_len);
    if (!cmd)
        return 1;

    aclk_query_t *query = aclk_query_new(CTX_CHECKPOINT);
    query->data.payload = cmd;
    aclk_add_job(query);
    return 0;
}

int stop_streaming_contexts(const char *msg, size_t msg_len)
{
    if (!aclk_ctx_based) {
        error_report("Received StopStreamingContexts message but context based communication was not enabled  (Cloud violated the protocol). Ignoring message");
        return 1;
    }

    struct stop_streaming_ctxs *cmd = parse_stop_streaming_ctxs(msg, msg_len);
    if (!cmd)
        return 1;

    aclk_query_t *query = aclk_query_new(CTX_STOP_STREAMING);
    query->data.payload = cmd;
    aclk_add_job(query);
    return 0;
}

int cancel_pending_req(const char *msg, size_t msg_len)
{
    struct aclk_cancel_pending_req cmd = {.request_id = NULL, .trace_id = NULL};
    if(parse_cancel_pending_req(msg, msg_len, &cmd)) {
        error_report("Error parsing CancelPendingReq");
        return 1;
    }

    nd_log(NDLS_ACCESS, NDLP_NOTICE, "ACLK CancelPendingRequest REQ: %s, cloud trace-id: %s", cmd.request_id, cmd.trace_id);

    if (mark_pending_req_cancelled(cmd.request_id))
        error_report("CancelPending Request for %s failed. No such pending request.", cmd.request_id);

    free_cancel_pending_req(&cmd);
    return 0;
}

typedef struct {
    const char *name;
    simple_hash_t name_hash;
    rx_msg_handler fnc;
} new_cloud_rx_msg_t;

new_cloud_rx_msg_t rx_msgs[] = {
    { .name = "cmd",                       .name_hash = 0, .fnc = handle_old_proto_cmd         },
    { .name = "CreateNodeInstanceResult",  .name_hash = 0, .fnc = create_node_instance_result  },  // async
    { .name = "SendNodeInstances",         .name_hash = 0, .fnc = send_node_instances          },  // async
    { .name = "StreamChartsAndDimensions", .name_hash = 0, .fnc = stream_charts_and_dimensions },  // unused
    { .name = "ChartsAndDimensionsAck",    .name_hash = 0, .fnc = charts_and_dimensions_ack    },  // unused
    { .name = "UpdateChartConfigs",        .name_hash = 0, .fnc = update_chart_configs         },  // unused
    { .name = "StartAlarmStreaming",       .name_hash = 0, .fnc = start_alarm_streaming        },  // async
    { .name = "SendAlarmCheckpoint",       .name_hash = 0, .fnc = send_alarm_checkpoint        },  // async
    { .name = "SendAlarmConfiguration",    .name_hash = 0, .fnc = send_alarm_configuration     },  // async
    { .name = "SendAlarmSnapshot",         .name_hash = 0, .fnc = send_alarm_snapshot          },  // shouldn't be used
    { .name = "DisconnectReq",             .name_hash = 0, .fnc = handle_disconnect_req        },
    { .name = "ContextsCheckpoint",        .name_hash = 0, .fnc = contexts_checkpoint          },  // async
    { .name = "StopStreamingContexts",     .name_hash = 0, .fnc = stop_streaming_contexts      },  // async
    { .name = "CancelPendingRequest",      .name_hash = 0, .fnc = cancel_pending_req           },
    { .name = NULL,                        .name_hash = 0, .fnc = NULL                         },
};

new_cloud_rx_msg_t *find_rx_handler_by_hash(simple_hash_t hash)
{
    // we can afford to not compare strings after hash match
    // because we check for collisions at initialization in
    // aclk_init_rx_msg_handlers()
    for (int i = 0; rx_msgs[i].fnc; i++) {
        if (rx_msgs[i].name_hash == hash)
            return &rx_msgs[i];
    }
    return NULL;
}

void aclk_init_rx_msg_handlers(void)
{
    int i;
    for (i = 0; rx_msgs[i].fnc; i++) {
        simple_hash_t hash = simple_hash(rx_msgs[i].name);
        new_cloud_rx_msg_t *hdl = find_rx_handler_by_hash(hash);
        if (unlikely(hdl)) {
            // the list of message names changes only by changing
            // the source code, therefore fatal is appropriate
            fatal("Hash collision. Choose better hash. Added '%s' clashes with existing '%s'", rx_msgs[i].name, hdl->name);
        }
        rx_msgs[i].name_hash = hash;
    }
}

void aclk_handle_new_cloud_msg(const char *message_type, const char *msg, size_t msg_len, const char *topic __maybe_unused)
{
    new_cloud_rx_msg_t *msg_descriptor = find_rx_handler_by_hash(simple_hash(message_type));
    netdata_log_debug(D_ACLK, "Got message named '%s' from cloud", message_type);
    if (unlikely(!msg_descriptor)) {
        netdata_log_error("Do not know how to handle message of type '%s'. Ignoring", message_type);
        return;
    }

    if (aclklog_enabled) {
        if (!strncmp(message_type, "cmd", strlen("cmd"))) {
            log_aclk_message_bin(msg, msg_len, 0, topic, msg_descriptor->name);
        } else {
            char *json = protomsg_to_json(msg, msg_len, msg_descriptor->name);
            log_aclk_message_bin(json, strlen(json), 0, topic, msg_descriptor->name);
            freez(json);
        }
    }

    if (msg_descriptor->fnc(msg, msg_len)) {
        netdata_log_error("Error processing message of type '%s'", message_type);
        return;
    }
}
