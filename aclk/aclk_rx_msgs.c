// SPDX-License-Identifier: GPL-3.0-or-later

#include "aclk_rx_msgs.h"

#include "aclk_stats.h"
#include "aclk_query_queue.h"
#include "aclk.h"

#define ACLK_V2_PAYLOAD_SEPARATOR "\x0D\x0A\x0D\x0A"
#define ACLK_CLOUD_REQ_V2_PREFIX "GET /"

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

    // TODO better check of URL
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

static int aclk_handle_cloud_http_request_v2(struct aclk_request *cloud_to_agent, char *raw_payload)
{
    if (!aclk_use_new_cloud_arch) {
        HTTP_CHECK_AGENT_INITIALIZED();
    }

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

int aclk_handle_cloud_cmd_message(char *payload)
{
    struct aclk_request cloud_to_agent;
    memset(&cloud_to_agent, 0, sizeof(struct aclk_request));

    if (unlikely(!payload)) {
        error_report("ACLK incoming 'cmd' message is empty");
        return 1;
    }

    debug(D_ACLK, "ACLK incoming 'cmd' message (%s)", payload);

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
    if (strcmp(cloud_to_agent.type_id, "http")) {
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

#ifdef ENABLE_NEW_CLOUD_PROTOCOL
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
    if (aclk_handle_cloud_cmd_message(str)) {
        freez(str);
        return 1;
    }
    freez(str);
    return 0;
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

    debug(D_ACLK, "CreateNodeInstanceResult: guid:%s nodeid:%s", res.machine_guid, res.node_id);
#ifdef NETDATA_INTERNAL_CHECKS
    sql_log_message("CreateNodeInstanceResult", 1, 0, res.node_id, res.machine_guid);
#endif

    uuid_t host_id, node_id;
    if (uuid_parse(res.machine_guid, host_id)) {
        error("Error parsing machine_guid provided by CreateNodeInstanceResult");
        freez(res.machine_guid);
        freez(res.node_id);
        return 1;
    }
    if (uuid_parse(res.node_id, node_id)) {
        error("Error parsing node_id provided by CreateNodeInstanceResult");
        freez(res.machine_guid);
        freez(res.node_id);
        return 1;
    }
    update_node_id(&host_id, &node_id);

    aclk_query_t query = aclk_query_new(NODE_STATE_UPDATE);
    query->data.node_update.hops = 1; //TODO - real hop count instead of hardcoded
    rrdhost_aclk_state_lock(localhost);
    query->data.node_update.claim_id = strdupz(localhost->aclk_state.claimed_id);
    rrdhost_aclk_state_unlock(localhost);

    RRDHOST *host = rrdhost_find_by_guid(res.machine_guid, 0);
    query->data.node_update.live = 0;

    if (host) {
        // not all host must have RRDHOST struct created for them
        // if they never connected during runtime of agent
        if (host == localhost) {
            query->data.node_update.live = 1;
            query->data.node_update.hops = 0;
        } else {
            netdata_mutex_lock(&host->receiver_lock);
            query->data.node_update.live = (host->receiver != NULL);
            netdata_mutex_unlock(&host->receiver_lock);
            query->data.node_update.hops = host->system_info->hops;
        }
    }

    query->data.node_update.node_id = res.node_id; // aclk_query_free will free it
    query->data.node_update.queryable = 1;
    query->data.node_update.session_id = aclk_session_newarch;
    query->node_id = res.node_id;
    aclk_queue_query(query);
    freez(res.machine_guid);
    return 0;
}

int send_node_instances(const char *msg, size_t msg_len)
{
    UNUSED(msg);
    UNUSED(msg_len);
    aclk_send_node_instances();
#ifdef NETDATA_INTERNAL_CHECKS
    sql_log_message("SendNodeInstances", 1, 0, NULL, NULL);
#endif
    return 0;
}

int stream_charts_and_dimensions(const char *msg, size_t msg_len)
{
    stream_charts_and_dims_t res = parse_stream_charts_and_dims(msg, msg_len);
    if (!res.claim_id || !res.node_id) {
        error("Error parsing StreamChartsAndDimensions msg");
        freez(res.claim_id);
        freez(res.node_id);
        return 1;
    }
    chart_batch_id = res.batch_id;
    aclk_start_streaming(res.node_id, res.seq_id, res.seq_id_created_at.tv_sec, res.batch_id);
#ifdef NETDATA_INTERNAL_CHECKS
    char context[512];
    sprintf(context, "%lu - %lu", res.batch_id, res.seq_id);
    sql_log_message("StreamChartsAndDimensions", 1, 0, res.node_id, context);
#endif
    freez(res.claim_id);
    freez(res.node_id);
    return 0;
}

int charts_and_dimensions_ack(const char *msg, size_t msg_len)
{
    chart_and_dim_ack_t res = parse_chart_and_dimensions_ack(msg, msg_len);
    if (!res.claim_id || !res.node_id) {
        error("Error parsing ChartsAnddimensionsAck msg");
        freez(res.claim_id);
        freez(res.node_id);
        return 1;
    }
    aclk_ack_chart_sequence_id(res.node_id, res.last_seq_id);
#ifdef NETDATA_INTERNAL_CHECKS
    sql_log_message("ChartsAnddimensionsAck", 1, 0, res.node_id, NULL);
#endif
    freez(res.claim_id);
    freez(res.node_id);
    return 0;
}

int update_chart_configs(const char *msg, size_t msg_len)
{
    struct update_chart_config res = parse_update_chart_config(msg, msg_len);
    if (!res.claim_id || !res.node_id || !res.hashes)
        error("Error parsing UpdateChartConfigs msg");
    else
        aclk_get_chart_config(res.hashes);
#ifdef NETDATA_INTERNAL_CHECKS
    sql_log_message("UpdateChartConfigs", 1, 0, res.node_id, NULL);
#endif
    destroy_update_chart_config(&res);
    return 0;
}

int start_alarm_streaming(const char *msg, size_t msg_len)
{
    struct start_alarm_streaming res = parse_start_alarm_streaming(msg, msg_len);
    if (!res.node_id || !res.batch_id) {
        error("Error parsing StartAlarmStreaming");
        freez(res.node_id);
        return 1;
    }
    aclk_start_alert_streaming(res.node_id, res.batch_id, res.start_seq_id);
#ifdef NETDATA_INTERNAL_CHECKS
    char context[512];
    sprintf(context, "%lu - %lu", res.batch_id, res.start_seq_id);
    sql_log_message("StartAlarmStreaming", 1, 0, res.node_id, context);
#endif
    freez(res.node_id);
    return 0;
}

int send_alarm_log_health(const char *msg, size_t msg_len)
{
    char *node_id = parse_send_alarm_log_health(msg, msg_len);
    if (!node_id) {
        error("Error parsing SendAlarmLogHealth");
        return 1;
    }
    aclk_send_alarm_health_log(node_id);

#ifdef NETDATA_INTERNAL_CHECKS
    sql_log_message("SendAlarmLogHealth", 1, 0, node_id, NULL);
#endif
    freez(node_id);
    return 0;
}

int send_alarm_configuration(const char *msg, size_t msg_len)
{
    char *config_hash = parse_send_alarm_configuration(msg, msg_len);
    if (!config_hash || !*config_hash) {
        error("Error parsing SendAlarmConfiguration");
        freez(config_hash);
        return 1;
    }
    aclk_send_alarm_configuration(config_hash);
#ifdef NETDATA_INTERNAL_CHECKS
    sql_log_message("SendAlarmConfiguration", 1, 0, NULL, config_hash);
#endif
    freez(config_hash);
    return 0;
}

int send_alarm_snapshot(const char *msg, size_t msg_len)
{
    struct send_alarm_snapshot *sas = parse_send_alarm_snapshot(msg, msg_len);
    if (!sas->node_id || !sas->claim_id) {
        error("Error parsing SendAlarmSnapshot");
        destroy_send_alarm_snapshot(sas);
        return 1;
    }
    aclk_process_send_alarm_snapshot(sas->node_id, sas->claim_id, sas->snapshot_id, sas->sequence_id);
#ifdef NETDATA_INTERNAL_CHECKS
    char context[512];
    sprintf(context, "%lu - %lu", sas->snapshot_id, sas->sequence_id);
    sql_log_message("SendAlarmSnapshot", 1, 0, sas->node_id, context);
#endif
    destroy_send_alarm_snapshot(sas);
    return 0;
}

int handle_disconnect_req(const char *msg, size_t msg_len)
{
    struct disconnect_cmd *cmd = parse_disconnect_cmd(msg, msg_len);
    if (!cmd)
        return 1;
    if (cmd->permaban) {
        error("Cloud Banned This Agent!");
        aclk_disable_runtime = 1;
    }
    info("Cloud requested disconnect (EC=%u, \"%s\")", (unsigned int)cmd->error_code, cmd->error_description);
    if (cmd->reconnect_after_s > 0) {
        aclk_block_until = now_monotonic_sec() + cmd->reconnect_after_s;
        info(
            "Cloud asks not to reconnect for %u seconds. We shall honor that request",
            (unsigned int)cmd->reconnect_after_s);
    }
    disconnect_req = 1;
    freez(cmd->error_description);
    freez(cmd);
    return 0;
}

typedef struct {
    const char *name;
    simple_hash_t name_hash;
    rx_msg_handler fnc;
} new_cloud_rx_msg_t;

new_cloud_rx_msg_t rx_msgs[] = {
    { .name = "cmd",                       .name_hash = 0, .fnc = handle_old_proto_cmd         },
    { .name = "CreateNodeInstanceResult",  .name_hash = 0, .fnc = create_node_instance_result  },
    { .name = "SendNodeInstances",         .name_hash = 0, .fnc = send_node_instances          },
    { .name = "StreamChartsAndDimensions", .name_hash = 0, .fnc = stream_charts_and_dimensions },
    { .name = "ChartsAndDimensionsAck",    .name_hash = 0, .fnc = charts_and_dimensions_ack    },
    { .name = "UpdateChartConfigs",        .name_hash = 0, .fnc = update_chart_configs         },
    { .name = "StartAlarmStreaming",       .name_hash = 0, .fnc = start_alarm_streaming        },
    { .name = "SendAlarmLogHealth",        .name_hash = 0, .fnc = send_alarm_log_health        },
    { .name = "SendAlarmConfiguration",    .name_hash = 0, .fnc = send_alarm_configuration     },
    { .name = "SendAlarmSnapshot",         .name_hash = 0, .fnc = send_alarm_snapshot          },
    { .name = "DisconnectReq",             .name_hash = 0, .fnc = handle_disconnect_req        },
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
    for (int i = 0; rx_msgs[i].fnc; i++) {
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

void aclk_handle_new_cloud_msg(const char *message_type, const char *msg, size_t msg_len)
{
    if (aclk_stats_enabled) {
        ACLK_STATS_LOCK;
        aclk_metrics_per_sample.cloud_req_recvd++;
        ACLK_STATS_UNLOCK;
    }
    new_cloud_rx_msg_t *msg_descriptor = find_rx_handler_by_hash(simple_hash(message_type));
    debug(D_ACLK, "Got message named '%s' from cloud", message_type);
    if (unlikely(!msg_descriptor)) {
        error("Do not know how to handle message of type '%s'. Ignoring", message_type);
        if (aclk_stats_enabled) {
            ACLK_STATS_LOCK;
            aclk_metrics_per_sample.cloud_req_err++;
            ACLK_STATS_UNLOCK;
        }
        return;
    }
    if (msg_descriptor->fnc(msg, msg_len)) {
        error("Error processing message of type '%s'", message_type);
        if (aclk_stats_enabled) {
            ACLK_STATS_LOCK;
            aclk_metrics_per_sample.cloud_req_err++;
            ACLK_STATS_UNLOCK;
        }
        return;
    }
}
#endif
