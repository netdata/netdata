// SPDX-License-Identifier: GPL-3.0-or-later

#include "aclk_rx_msgs.h"

#include "aclk_query_queue.h"
#include "aclk.h"
#include "aclk_capas.h"
#include "aclk_query.h"
#include "mqtt_websockets/aclk_mqtt_workers.h"
#include "web/server/web_client.h"

#include "schema-wrappers/proto_2_json.h"

#define ACLK_V2_PAYLOAD_SEPARATOR "\x0D\x0A\x0D\x0A"
#define ACLK_SEPARATOR_SEARCH_CHUNK_SIZE (64 * 1024)

#define ACLK_V_COMPRESSION 2

struct aclk_request {
    bool has_type;
    bool is_http;
    // Heap-allocated string fields below are owned by the local instance in
    // aclk_handle_cloud_cmd_message. On the v2 success path, msg_id and
    // callback_topic are transferred into the query and released by
    // aclk_query_free; payload is not consumed by v2 (the HTTP body is
    // re-derived from the raw frame) and must be freed by the caller on both
    // success and error paths. Any new owned field added here must extend
    // both cleanup paths to preserve this invariant.
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
                data->msg_id = e->data.string ? strdupz(e->data.string) : NULL;
                break;
            }
            if (!strcmp(e->name, "type")) {
                data->has_type = true;
                data->is_http = (e->data.string && (strcmp(e->data.string, "http") == 0));
                break;
            }
            if (!strcmp(e->name, "callback-topic")) {
                data->callback_topic = e->data.string ? strdupz(e->data.string) : NULL;
                break;
            }
            if (!strcmp(e->name, "payload")) {
                if (likely(e->data.string)) {
                    size_t len = strlen(e->data.string);
                    data->payload = mallocz(len+1);
                    if(url_decode_r_len(data->payload, len + 1, e->data.string, len, NULL) != URL_DECODE_OK)
                        memcpy(data->payload, e->data.string, len + 1);
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

static bool aclk_v2_payload_request_offset(const char *payload, size_t payload_length, size_t *request_offset) {
    static const char separator[] = ACLK_V2_PAYLOAD_SEPARATOR;
    const size_t separator_length = sizeof(separator) - 1;

    if(!payload || !request_offset || payload_length < separator_length)
        return false;

    size_t scratch_size = MIN(payload_length, (size_t)ACLK_SEPARATOR_SEARCH_CHUNK_SIZE);
    char *scratch = mallocz(scratch_size + 1);
    size_t offset = 0;
    bool found = false;

    while(offset < payload_length) {
        size_t remaining = payload_length - offset;
        size_t bytes = MIN(remaining, scratch_size);
        memcpy(scratch, payload + offset, bytes);
        scratch[bytes] = '\0';

        const char *separator_start = strstr(scratch, separator);
        if(separator_start) {
            *request_offset = offset + (size_t)(separator_start - scratch) + separator_length;
            found = true;
            break;
        }

        // Match master's C-string behavior: bytes after the first embedded NUL are not part of the envelope.
        if(memchr(scratch, '\0', bytes) || bytes == remaining)
            break;

        offset += bytes - (separator_length - 1);
    }

    freez(scratch);
    return found;
}

static inline int aclk_extract_v2_data(
    char *payload, size_t payload_length, size_t request_offset, size_t original_request_size,
    char **data, size_t *request_size)
{
    if(request_offset > payload_length)
        return 1;

    const char *request = payload + request_offset;
    size_t available_size = payload_length - request_offset;
    size_t bounded_size = MIN(original_request_size, (size_t)NETDATA_WEB_REQUEST_MAX_SIZE + 1);
    if(available_size < bounded_size)
        return 1;

    *request_size = original_request_size;
    size_t length = strnlen(request, bounded_size);
    *data = mallocz(length + 1);
    memcpy(*data, request, length);
    (*data)[length] = '\0';
    return 0;
}

static char *aclk_cmd_to_cstring(
    const char *msg, size_t msg_len, size_t *copied_length, size_t *request_offset, size_t *request_size)
{
    size_t offset;
    if(!aclk_v2_payload_request_offset(msg, msg_len, &offset))
        return NULL;

    size_t original_request_size = msg_len - offset;
    size_t bounded_request_size = MIN(original_request_size, (size_t)NETDATA_WEB_REQUEST_MAX_SIZE + 1);
    size_t length = offset + bounded_request_size;

    char *str = mallocz(length + 1);
    memcpy(str, msg, length);
    str[length] = '\0';

    if(copied_length)
        *copied_length = length;
    if(request_offset)
        *request_offset = offset;
    if(request_size)
        *request_size = original_request_size;

    return str;
}

int aclk_rx_msgs_unittest(void) {
    int errors = 0;
    static const char envelope[] = "{}" ACLK_V2_PAYLOAD_SEPARATOR;
    size_t envelope_length = sizeof(envelope) - 1;
    size_t oversized_request_size = NETDATA_WEB_REQUEST_MAX_SIZE + 1024;
    CLEAN_CHAR_P *raw = mallocz(envelope_length + oversized_request_size + 1);
    memcpy(raw, envelope, envelope_length);
    memset(raw + envelope_length, 'a', oversized_request_size);
    raw[envelope_length + oversized_request_size] = '\0';

    char *request = NULL;
    size_t request_size = 0;
    raw[envelope_length + NETDATA_WEB_REQUEST_MAX_SIZE] = '\0';
    if(aclk_extract_v2_data(raw, envelope_length + NETDATA_WEB_REQUEST_MAX_SIZE, envelope_length,
                            NETDATA_WEB_REQUEST_MAX_SIZE,
                            &request, &request_size) ||
       request_size != NETDATA_WEB_REQUEST_MAX_SIZE ||
       strlen(request) != NETDATA_WEB_REQUEST_MAX_SIZE)
        errors++;
    freez(request);

    raw[envelope_length + NETDATA_WEB_REQUEST_MAX_SIZE] = 'a';
    if(aclk_extract_v2_data(raw, envelope_length + NETDATA_WEB_REQUEST_MAX_SIZE + 1, envelope_length,
                            NETDATA_WEB_REQUEST_MAX_SIZE + 1,
                            &request, &request_size) ||
       request_size != NETDATA_WEB_REQUEST_MAX_SIZE + 1 ||
       strlen(request) != NETDATA_WEB_REQUEST_MAX_SIZE + 1)
        errors++;
    freez(request);

    raw[envelope_length + 16] = '\0';
    size_t copied_length = 0;
    size_t request_offset = 0;
    size_t original_request_size = 0;
    CLEAN_CHAR_P *bounded = aclk_cmd_to_cstring(
        raw, envelope_length + oversized_request_size, &copied_length, &request_offset, &original_request_size);
    if(!bounded || copied_length != envelope_length + NETDATA_WEB_REQUEST_MAX_SIZE + 1 ||
       request_offset != envelope_length || bounded[copied_length] != '\0' ||
       original_request_size != oversized_request_size ||
       aclk_extract_v2_data(bounded, copied_length, request_offset, original_request_size,
                            &request, &request_size) ||
       request_size != oversized_request_size || strlen(request) != 16)
        errors++;
    freez(request);
    raw[envelope_length + 16] = 'a';

    if(aclk_cmd_to_cstring("no separator", 12, NULL, NULL, NULL))
        errors++;

    size_t crossing_separator_offset = ACLK_SEPARATOR_SEARCH_CHUNK_SIZE - 2;
    CLEAN_CHAR_P *crossing = mallocz(crossing_separator_offset + sizeof(ACLK_V2_PAYLOAD_SEPARATOR));
    memset(crossing, 'x', crossing_separator_offset);
    memcpy(crossing + crossing_separator_offset, ACLK_V2_PAYLOAD_SEPARATOR, sizeof(ACLK_V2_PAYLOAD_SEPARATOR) - 1);
    if(!aclk_v2_payload_request_offset(
           crossing, crossing_separator_offset + sizeof(ACLK_V2_PAYLOAD_SEPARATOR) - 1, &request_offset) ||
       request_offset != crossing_separator_offset + sizeof(ACLK_V2_PAYLOAD_SEPARATOR) - 1)
        errors++;

    static const char nul_before_separator[] = "x\0x" ACLK_V2_PAYLOAD_SEPARATOR;
    if(aclk_v2_payload_request_offset(nul_before_separator, sizeof(nul_before_separator) - 1, &request_offset))
        errors++;

    if(errors)
        fprintf(stderr, "ACLK RX: %d test(s) failed\n", errors);

    return errors;
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

static int aclk_handle_cloud_http_request_v2(
    struct aclk_request *cloud_to_agent, char *raw_payload, size_t raw_payload_length, size_t request_offset,
    size_t request_size)
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
    if (unlikely(aclk_extract_v2_data(raw_payload, raw_payload_length, request_offset, request_size,
                                      &query->data.http_api_v2.payload,
                                      &query->data.http_api_v2.request_size))) {
        netdata_log_error("Error extracting payload expected after the JSON dictionary.");
        goto error;
    }

    if (unlikely(query->data.http_api_v2.request_size <= NETDATA_WEB_REQUEST_MAX_SIZE &&
                 aclk_v2_payload_get_query(query->data.http_api_v2.payload, &query->dedup_id))) {
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

static int aclk_handle_cloud_cmd_message_at(
    char *payload, size_t payload_length, size_t request_offset, size_t request_size)
{
    struct aclk_request cloud_to_agent;
    memset(&cloud_to_agent, 0, sizeof(struct aclk_request));

    if (unlikely(!payload)) {
        error_report("ACLK incoming 'cmd' message is empty");
        return 1;
    }

    netdata_log_debug(D_ACLK, "ACLK incoming 'cmd' message of %zu bytes", payload_length);

    int rc = json_parse(payload, &cloud_to_agent, cloud_to_agent_parse);

    if (unlikely(rc != JSON_OK)) {
        error_report("Malformed ACLK command message of %zu bytes", payload_length);
        goto err_cleanup;
    }

    if (!cloud_to_agent.has_type) {
        error_report("Cloud message is missing compulsory key \"type\"");
        goto err_cleanup;
    }

    // Originally we were expecting to have multiple types of 'cmd' message,
    // but after the new protocol was designed we will ever only have 'http'
    if (!cloud_to_agent.is_http) {
        error_report("Only 'http' cmd message is supported");
        goto err_cleanup;
    }

    if (likely(!aclk_handle_cloud_http_request_v2(
            &cloud_to_agent, payload, payload_length, request_offset, request_size))) {
        // aclk_handle_cloud_http_request_v2 takes ownership of msg_id and
        // callback_topic on success. The JSON-parsed payload field is not
        // consumed by v2 (the HTTP body comes from the raw frame), so free
        // it here to avoid leaking when the cmd JSON included a "payload" key.
        freez(cloud_to_agent.payload);
        return 0;
    }

err_cleanup:
    if (cloud_to_agent.payload)
        freez(cloud_to_agent.payload);
    if (cloud_to_agent.msg_id)
        freez(cloud_to_agent.msg_id);
    if (cloud_to_agent.callback_topic)
        freez(cloud_to_agent.callback_topic);

    return 1;
}

int aclk_handle_cloud_cmd_message(char *payload, size_t payload_length)
{
    size_t copied_length = 0;
    size_t request_offset = 0;
    size_t request_size = 0;
    char *str = aclk_cmd_to_cstring(payload, payload_length, &copied_length, &request_offset, &request_size);
    if(!str) {
        error_report("ACLK command message has no HTTP payload separator");
        return 1;
    }

    int rc = aclk_handle_cloud_cmd_message_at(str, copied_length, request_offset, request_size);
    freez(str);
    return rc;
}

typedef uint32_t simple_hash_t;
typedef int(*rx_msg_handler)(const char *msg, size_t msg_len);

int handle_old_proto_cmd(const char *msg, size_t msg_len)
{
    // msg is binary payload in all other cases
    // however in this message from old legacy cloud
    // we have to convert it to C string
    size_t copied_length = 0;
    size_t request_offset = 0;
    size_t request_size = 0;
    char *str = aclk_cmd_to_cstring(msg, msg_len, &copied_length, &request_offset, &request_size);
    if(!str) {
        error_report("ACLK legacy command message has no HTTP payload separator");
        return 1;
    }

    int rc = aclk_handle_cloud_cmd_message_at(str, copied_length, request_offset, request_size);
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
    if (!sas || !sas->node_id || !sas->claim_id || !sas->snapshot_uuid) {
        netdata_log_error("Error parsing SendAlarmSnapshot");
        destroy_send_alarm_snapshot(sas);
        return 1;
    }
    aclk_query_t *query = aclk_query_new(ALERT_CHECKPOINT);
    query->data.node_id = sas->node_id;     // Will be freed on query free
    query->claim_id = sas->claim_id;        // Will be freed on query free
    sas->node_id = NULL;
    sas->claim_id = NULL;
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
        __atomic_store_n(&aclk_disable_runtime, 1, __ATOMIC_RELAXED);
    }
    netdata_log_info("Cloud requested disconnect (EC=%u, \"%s\")", (unsigned int)cmd->error_code, cmd->error_description);
    if (cmd->reconnect_after_s > 0) {
        aclk_block_until = now_monotonic_sec() + cmd->reconnect_after_s;
        netdata_log_info(
            "Cloud asks not to reconnect for %u seconds. We shall honor that request",
            (unsigned int)cmd->reconnect_after_s);
    }
    __atomic_store_n(&disconnect_req, ACLK_CLOUD_DISCONNECT, __ATOMIC_RELAXED);
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
