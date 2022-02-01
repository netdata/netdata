// SPDX-License-Identifier: GPL-3.0-or-later

#include "aclk_tx_msgs.h"
#include "daemon/common.h"
#include "aclk_util.h"
#include "aclk_stats.h"
#include "aclk.h"

#ifndef __GNUC__
#pragma region aclk_tx_msgs helper functions
#endif

// version for aclk legacy (old cloud arch)
#define ACLK_VERSION 2

static void aclk_send_message_subtopic(mqtt_wss_client client, json_object *msg, enum aclk_topics subtopic)
{
    uint16_t packet_id;
    const char *str = json_object_to_json_string_ext(msg, JSON_C_TO_STRING_PLAIN);
    const char *topic = aclk_get_topic(subtopic);

    if (unlikely(!topic)) {
        error("Couldn't get topic. Aborting message send");
        return;
    }

    mqtt_wss_publish_pid(client, topic, str, strlen(str),  MQTT_WSS_PUB_QOS1, &packet_id);
#ifdef NETDATA_INTERNAL_CHECKS
    aclk_stats_msg_published(packet_id);
#endif
#ifdef ACLK_LOG_CONVERSATION_DIR
#define FN_MAX_LEN 1024
    char filename[FN_MAX_LEN];
    snprintf(filename, FN_MAX_LEN, ACLK_LOG_CONVERSATION_DIR "/%010d-tx.json", ACLK_GET_CONV_LOG_NEXT());
    json_object_to_file_ext(filename, msg, JSON_C_TO_STRING_PRETTY);
#endif
}

uint16_t aclk_send_bin_message_subtopic_pid(mqtt_wss_client client, char *msg, size_t msg_len, enum aclk_topics subtopic, const char *msgname)
{
#ifndef ACLK_LOG_CONVERSATION_DIR
    UNUSED(msgname);
#endif
    uint16_t packet_id;
    const char *topic = aclk_get_topic(subtopic);

    if (unlikely(!topic)) {
        error("Couldn't get topic. Aborting message send.");
        return 0;
    }

    mqtt_wss_publish_pid(client, topic, msg, msg_len,  MQTT_WSS_PUB_QOS1, &packet_id);
#ifdef NETDATA_INTERNAL_CHECKS
    aclk_stats_msg_published(packet_id);
#endif
#ifdef ACLK_LOG_CONVERSATION_DIR
#define FN_MAX_LEN 1024
    char filename[FN_MAX_LEN];
    snprintf(filename, FN_MAX_LEN, ACLK_LOG_CONVERSATION_DIR "/%010d-tx-%s.bin", ACLK_GET_CONV_LOG_NEXT(), msgname);
    FILE *fptr;
    if (fptr = fopen(filename,"w")) {
        fwrite(msg, msg_len, 1, fptr);
        fclose(fptr);
    }
#endif

    return packet_id;
}

#define TOPIC_MAX_LEN 512
#define V2_BIN_PAYLOAD_SEPARATOR "\x0D\x0A\x0D\x0A"
static void aclk_send_message_with_bin_payload(mqtt_wss_client client, json_object *msg, const char *topic, const void *payload, size_t payload_len)
{
    uint16_t packet_id;
    const char *str;
    char *full_msg;
    int len;

    if (unlikely(!topic || topic[0] != '/')) {
        error ("Full topic required!");
        return;
    }

    str = json_object_to_json_string_ext(msg, JSON_C_TO_STRING_PLAIN);
    len = strlen(str);

    full_msg = mallocz(len + strlen(V2_BIN_PAYLOAD_SEPARATOR) + payload_len);

    memcpy(full_msg, str, len);
    memcpy(&full_msg[len], V2_BIN_PAYLOAD_SEPARATOR, strlen(V2_BIN_PAYLOAD_SEPARATOR));
    len += strlen(V2_BIN_PAYLOAD_SEPARATOR);
    memcpy(&full_msg[len], payload, payload_len);
    len += payload_len;

/* TODO
#ifdef ACLK_LOG_CONVERSATION_DIR
#define FN_MAX_LEN 1024
    char filename[FN_MAX_LEN];
    snprintf(filename, FN_MAX_LEN, ACLK_LOG_CONVERSATION_DIR "/%010d-tx.json", ACLK_GET_CONV_LOG_NEXT());
    json_object_to_file_ext(filename, msg, JSON_C_TO_STRING_PRETTY);
#endif */

    int rc = mqtt_wss_publish_pid_block(client, topic, full_msg, len,  MQTT_WSS_PUB_QOS1, &packet_id, 5000);
    if (rc == MQTT_WSS_ERR_BLOCK_TIMEOUT)
        error("Timeout sending binpacked message");
    if (rc == MQTT_WSS_ERR_TX_BUF_TOO_SMALL)
        error("Message is bigger than allowed maximum");
#ifdef NETDATA_INTERNAL_CHECKS
    aclk_stats_msg_published(packet_id);
#endif
    freez(full_msg);
}

/*
 * Creates universal header common for all ACLK messages. User gets ownership of json object created.
 * Usually this is freed by send function after message has been sent.
 */
static struct json_object *create_hdr(const char *type, const char *msg_id, time_t ts_secs, usec_t ts_us, int version)
{
    uuid_t uuid;
    char uuid_str[36 + 1];
    json_object *tmp;
    json_object *obj = json_object_new_object();

    tmp = json_object_new_string(type);
    json_object_object_add(obj, "type", tmp);

    if (unlikely(!msg_id)) {
        uuid_generate(uuid);
        uuid_unparse(uuid, uuid_str);
        msg_id = uuid_str;
    }

    if (ts_secs == 0) {
        ts_us = now_realtime_usec();
        ts_secs = ts_us / USEC_PER_SEC;
        ts_us = ts_us % USEC_PER_SEC;
    }

    tmp = json_object_new_string(msg_id);
    json_object_object_add(obj, "msg-id", tmp);

    tmp = json_object_new_int64(ts_secs);
    json_object_object_add(obj, "timestamp", tmp);

// TODO handle this somehow on older json-c
//    tmp = json_object_new_uint64(ts_us);
// probably jso->_to_json_string -> custom function
//          jso->o.c_uint64 -> map this with pointer to signed int
// commit that implements json_object_new_uint64 is 3c3b592
// between 0.14 and 0.15
    tmp = json_object_new_int64(ts_us);
    json_object_object_add(obj, "timestamp-offset-usec", tmp);

    tmp = json_object_new_int64(aclk_session_sec);
    json_object_object_add(obj, "connect", tmp);

// TODO handle this somehow see above
//    tmp = json_object_new_uint64(0 /* TODO aclk_session_us */);
    tmp = json_object_new_int64(aclk_session_us);
    json_object_object_add(obj, "connect-offset-usec", tmp);

    tmp = json_object_new_int(version);
    json_object_object_add(obj, "version", tmp);

    return obj;
}

static char *create_uuid()
{
    uuid_t uuid;
    char *uuid_str = mallocz(36 + 1);

    uuid_generate(uuid);
    uuid_unparse(uuid, uuid_str);

    return uuid_str;
}

#ifndef __GNUC__
#pragma endregion
#endif

#ifndef __GNUC__
#pragma region aclk_tx_msgs message generators
#endif

/*
 * This will send the /api/v1/info
 */
#define BUFFER_INITIAL_SIZE (1024 * 16)
void aclk_send_info_metadata(mqtt_wss_client client, int metadata_submitted, RRDHOST *host)
{
    BUFFER *local_buffer = buffer_create(BUFFER_INITIAL_SIZE);
    json_object *msg, *payload, *tmp;

    char *msg_id = create_uuid();
    buffer_flush(local_buffer);
    local_buffer->contenttype = CT_APPLICATION_JSON;

    // on_connect messages are sent on a health reload, if the on_connect message is real then we
    // use the session time as the fake timestamp to indicate that it starts the session. If it is
    // a fake on_connect message then use the real timestamp to indicate it is within the existing
    // session.
    if (metadata_submitted)
        msg = create_hdr("update", msg_id, 0, 0, ACLK_VERSION);
    else
        msg = create_hdr("connect", msg_id, aclk_session_sec, aclk_session_us, ACLK_VERSION);

    payload = json_object_new_object();
    json_object_object_add(msg, "payload", payload);

    web_client_api_request_v1_info_fill_buffer(host, local_buffer);
    tmp = json_tokener_parse(local_buffer->buffer);
    json_object_object_add(payload, "info", tmp);

    buffer_flush(local_buffer);

    charts2json(host, local_buffer, 1, 0);
    tmp = json_tokener_parse(local_buffer->buffer);
    json_object_object_add(payload, "charts", tmp);

    aclk_send_message_subtopic(client, msg, ACLK_TOPICID_METADATA);

    json_object_put(msg);
    freez(msg_id);
    buffer_free(local_buffer);
}

// TODO should include header instead
void health_active_log_alarms_2json(RRDHOST *host, BUFFER *wb);

void aclk_send_alarm_metadata(mqtt_wss_client client, int metadata_submitted)
{
    BUFFER *local_buffer = buffer_create(BUFFER_INITIAL_SIZE);
    json_object *msg, *payload, *tmp;

    char *msg_id = create_uuid();
    buffer_flush(local_buffer);
    local_buffer->contenttype = CT_APPLICATION_JSON;

    // on_connect messages are sent on a health reload, if the on_connect message is real then we
    // use the session time as the fake timestamp to indicate that it starts the session. If it is
    // a fake on_connect message then use the real timestamp to indicate it is within the existing
    // session.

    if (metadata_submitted)
        msg = create_hdr("connect_alarms", msg_id, 0, 0, ACLK_VERSION);
    else
        msg = create_hdr("connect_alarms", msg_id, aclk_session_sec, aclk_session_us, ACLK_VERSION);

    payload = json_object_new_object();
    json_object_object_add(msg, "payload", payload);

    health_alarms2json(localhost, local_buffer, 1);
    tmp = json_tokener_parse(local_buffer->buffer);
    json_object_object_add(payload, "configured-alarms", tmp);

    buffer_flush(local_buffer);

    health_active_log_alarms_2json(localhost, local_buffer);
    tmp = json_tokener_parse(local_buffer->buffer);
    json_object_object_add(payload, "alarms-active", tmp);

    aclk_send_message_subtopic(client, msg, ACLK_TOPICID_ALARMS);

    json_object_put(msg);
    freez(msg_id);
    buffer_free(local_buffer);
}

void aclk_http_msg_v2(mqtt_wss_client client, const char *topic, const char *msg_id, usec_t t_exec, usec_t created, int http_code, const char *payload, size_t payload_len)
{
    json_object *tmp, *msg;

    msg = create_hdr("http", msg_id, 0, 0, 2);

    tmp = json_object_new_int64(t_exec);
    json_object_object_add(msg, "t-exec", tmp);

    tmp = json_object_new_int64(created);
    json_object_object_add(msg, "t-rx", tmp);

    tmp = json_object_new_int(http_code);
    json_object_object_add(msg, "http-code", tmp);

    aclk_send_message_with_bin_payload(client, msg, topic, payload, payload_len);
    json_object_put(msg);
}

void aclk_chart_msg(mqtt_wss_client client, RRDHOST *host, const char *chart)
{
    json_object *msg, *payload;
    BUFFER *tmp_buffer;
    RRDSET *st;
    
    st = rrdset_find(host, chart);
    if (!st)
        st = rrdset_find_byname(host, chart);
    if (!st) {
        info("FAILED to find chart %s", chart);
        return;
    }

    tmp_buffer = buffer_create(BUFFER_INITIAL_SIZE);
    rrdset2json(st, tmp_buffer, NULL, NULL, 1);
    payload = json_tokener_parse(tmp_buffer->buffer);
    if (!payload) {
        error("Failed to parse JSON from rrdset2json");
        buffer_free(tmp_buffer);
        return;
    }

    msg = create_hdr("chart", NULL, 0, 0, ACLK_VERSION);
    json_object_object_add(msg, "payload", payload);

    aclk_send_message_subtopic(client, msg, ACLK_TOPICID_CHART);

    buffer_free(tmp_buffer);
    json_object_put(msg);
}

void aclk_alarm_state_msg(mqtt_wss_client client, json_object *msg)
{
    // we create header here on purpose (and not send message with it already as `msg` param)
    // timestamps etc. which in ACLK legacy would be wrong (because ACLK legacy
    // send message with timestamps already to Query Queue they would be incorrect at time
    // when query queue would get to send them)
    json_object *obj = create_hdr("status-change", NULL, 0, 0, ACLK_VERSION);
    json_object_object_add(obj, "payload", msg);

    aclk_send_message_subtopic(client, obj, ACLK_TOPICID_ALARMS);
    json_object_put(obj);
}

#ifdef ENABLE_NEW_CLOUD_PROTOCOL
// new protobuf msgs
uint16_t aclk_send_agent_connection_update(mqtt_wss_client client, int reachable) {
    size_t len;
    uint16_t pid;
    update_agent_connection_t conn = {
        .reachable = (reachable ? 1 : 0),
        .lwt = 0,
        .session_id = aclk_session_newarch
    };

    rrdhost_aclk_state_lock(localhost);
    if (unlikely(!localhost->aclk_state.claimed_id)) {
        error("Internal error. Should not come here if not claimed");
        rrdhost_aclk_state_unlock(localhost);
        return 0;
    }
    if (localhost->aclk_state.prev_claimed_id)
        conn.claim_id = localhost->aclk_state.prev_claimed_id;
    else
        conn.claim_id = localhost->aclk_state.claimed_id;

    char *msg = generate_update_agent_connection(&len, &conn);
    rrdhost_aclk_state_unlock(localhost);

    if (!msg) {
        error("Error generating agent::v1::UpdateAgentConnection payload");
        return 0;
    }

    pid = aclk_send_bin_message_subtopic_pid(client, msg, len, ACLK_TOPICID_AGENT_CONN, "UpdateAgentConnection");
    freez(msg);
    if (localhost->aclk_state.prev_claimed_id) {
        freez(localhost->aclk_state.prev_claimed_id);
        localhost->aclk_state.prev_claimed_id = NULL;
    }
    return pid;
}

char *aclk_generate_lwt(size_t *size) {
    update_agent_connection_t conn = {
        .reachable = 0,
        .lwt = 1,
        .session_id = aclk_session_newarch
    };

    rrdhost_aclk_state_lock(localhost);
    if (unlikely(!localhost->aclk_state.claimed_id)) {
        error("Internal error. Should not come here if not claimed");
        rrdhost_aclk_state_unlock(localhost);
        return NULL;
    }
    conn.claim_id = localhost->aclk_state.claimed_id;

    char *msg = generate_update_agent_connection(size, &conn);
    rrdhost_aclk_state_unlock(localhost);

    if (!msg)
        error("Error generating agent::v1::UpdateAgentConnection payload for LWT");

    return msg;
}

void aclk_generate_node_registration(mqtt_wss_client client, node_instance_creation_t *node_creation) {
    size_t len;
    char *msg = generate_node_instance_creation(&len, node_creation);
    if (!msg) {
        error("Error generating nodeinstance::create::v1::CreateNodeInstance");
        return;
    }

    aclk_send_bin_message_subtopic_pid(client, msg, len, ACLK_TOPICID_CREATE_NODE, "CreateNodeInstance");
    freez(msg);
}

void aclk_generate_node_state_update(mqtt_wss_client client, node_instance_connection_t *node_connection) {
    size_t len;
    char *msg = generate_node_instance_connection(&len, node_connection);
    if (!msg) {
        error("Error generating nodeinstance::v1::UpdateNodeInstanceConnection");
        return;
    }

    aclk_send_bin_message_subtopic_pid(client, msg, len, ACLK_TOPICID_NODE_CONN, "UpdateNodeInstanceConnection");
    freez(msg);
}
#endif /* ENABLE_NEW_CLOUD_PROTOCOL */

#ifndef __GNUC__
#pragma endregion
#endif
