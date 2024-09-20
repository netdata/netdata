// SPDX-License-Identifier: GPL-3.0-or-later

#include "aclk_tx_msgs.h"
#include "aclk_util.h"
#include "aclk.h"
#include "aclk_capas.h"

#include "schema-wrappers/proto_2_json.h"

#ifndef __GNUC__
#pragma region aclk_tx_msgs helper functions
#endif

static void freez_aclk_publish5a(void *ptr) {
    freez(ptr);
}
static void freez_aclk_publish5b(void *ptr) {
    freez(ptr);
}

#define ACLK_HEADER_VERSION (2)

uint16_t aclk_send_bin_message_subtopic_pid(mqtt_wss_client client, char *msg, size_t msg_len, enum aclk_topics subtopic, const char *msgname)
{
#ifndef ACLK_LOG_CONVERSATION_DIR
    UNUSED(msgname);
#endif
    uint16_t packet_id;
    const char *topic = aclk_get_topic(subtopic);

    if (unlikely(!topic)) {
        netdata_log_error("Couldn't get topic. Aborting message send.");
        return 0;
    }

    mqtt_wss_publish5(client, (char*)topic, NULL, msg, &freez_aclk_publish5a, msg_len, MQTT_WSS_PUB_QOS1, &packet_id);

    if (aclklog_enabled) {
        char *json = protomsg_to_json(msg, msg_len, msgname);
        log_aclk_message_bin(json, strlen(json), 1, topic, msgname);
        freez(json);
    }

    return packet_id;
}

#define V2_BIN_PAYLOAD_SEPARATOR "\x0D\x0A\x0D\x0A"
static short aclk_send_message_with_bin_payload(mqtt_wss_client client, json_object *msg, const char *topic, const void *payload, size_t payload_len)
{
    uint16_t packet_id;
    const char *str;
    char *full_msg = NULL;
    size_t len;

    if (unlikely(!topic || topic[0] != '/')) {
        netdata_log_error("Full topic required!");
        json_object_put(msg);
        return HTTP_RESP_INTERNAL_SERVER_ERROR;
    }

    str = json_object_to_json_string_ext(msg, JSON_C_TO_STRING_PLAIN);
    len = strlen(str);

    size_t full_msg_len = len;
    if (payload_len)
        full_msg_len += strlen(V2_BIN_PAYLOAD_SEPARATOR) + payload_len;

    full_msg = mallocz(full_msg_len);
    memcpy(full_msg, str, len);
    json_object_put(msg);

    if (payload_len) {
        memcpy(&full_msg[len], V2_BIN_PAYLOAD_SEPARATOR, sizeof(V2_BIN_PAYLOAD_SEPARATOR) - 1);
        len += strlen(V2_BIN_PAYLOAD_SEPARATOR);
        memcpy(&full_msg[len], payload, payload_len);
    }

    int rc = mqtt_wss_publish5(client, (char*)topic, NULL, full_msg, &freez_aclk_publish5b, full_msg_len, MQTT_WSS_PUB_QOS1, &packet_id);

    if (rc == MQTT_WSS_ERR_MSG_TOO_BIG)
        return HTTP_RESP_CONTENT_TOO_LONG;

    return 0;
}

/*
 * Creates universal header common for all ACLK messages. User gets ownership of json object created.
 * Usually this is freed by send function after message has been sent.
 */
static struct json_object *create_hdr(const char *type, const char *msg_id)
{
    nd_uuid_t uuid;
    char uuid_str[UUID_STR_LEN];
    json_object *tmp;
    json_object *obj = json_object_new_object();
    time_t ts_secs;
    usec_t ts_us;

    tmp = json_object_new_string(type);
    json_object_object_add(obj, "type", tmp);

    if (unlikely(!msg_id)) {
        uuid_generate(uuid);
        uuid_unparse(uuid, uuid_str);
        msg_id = uuid_str;
    }

    ts_us = now_realtime_usec();
    ts_secs = ts_us / USEC_PER_SEC;
    ts_us = ts_us % USEC_PER_SEC;

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

    tmp = json_object_new_int(ACLK_HEADER_VERSION);
    json_object_object_add(obj, "version", tmp);

    return obj;
}

#ifndef __GNUC__
#pragma endregion
#endif

#ifndef __GNUC__
#pragma region aclk_tx_msgs message generators
#endif

void aclk_http_msg_v2_err(mqtt_wss_client client, const char *topic, const char *msg_id, int http_code, int ec, const char* emsg, const char *payload, size_t payload_len)
{
    json_object *tmp, *msg;
    msg = create_hdr("http", msg_id);
    tmp = json_object_new_int(http_code);
    json_object_object_add(msg, "http-code", tmp);

    tmp = json_object_new_int(ec);
    json_object_object_add(msg, "error-code", tmp);

    tmp = json_object_new_string(emsg);
    json_object_object_add(msg, "error-description", tmp);

    if (aclk_send_message_with_bin_payload(client, msg, topic, payload, payload_len)) {
        netdata_log_error("Failed to send cancellation message for http reply %zu %s", payload_len, payload);
    }
}

short aclk_http_msg_v2(mqtt_wss_client client, const char *topic, const char *msg_id, usec_t t_exec, usec_t created,
    short http_code, const char *payload, size_t payload_len)
{
    json_object *tmp, *msg;

    msg = create_hdr("http", msg_id);

    tmp = json_object_new_int64(t_exec);
    json_object_object_add(msg, "t-exec", tmp);

    tmp = json_object_new_int64(created);
    json_object_object_add(msg, "t-rx", tmp);

    tmp = json_object_new_int(http_code);
    json_object_object_add(msg, "http-code", tmp);

    short rc = aclk_send_message_with_bin_payload(client, msg, topic, payload, payload_len);

    switch (rc) {
        case HTTP_RESP_CONTENT_TOO_LONG:
            aclk_http_msg_v2_err(client, topic, msg_id, rc, CLOUD_EC_REQ_REPLY_TOO_BIG, CLOUD_EMSG_REQ_REPLY_TOO_BIG, NULL, 0);
            break;
        case HTTP_RESP_INTERNAL_SERVER_ERROR:
            aclk_http_msg_v2_err(client, topic, msg_id, rc, CLOUD_EC_FAIL_TOPIC, CLOUD_EMSG_FAIL_TOPIC, payload, payload_len);
            break;
        default:
            rc = http_code;
            break;
    }
    return rc;
}

uint16_t aclk_send_agent_connection_update(mqtt_wss_client client, int reachable) {
    size_t len;
    uint16_t pid;

    update_agent_connection_t conn = {
        .reachable = (reachable ? 1 : 0),
        .lwt = 0,
        .session_id = aclk_session_newarch,
        .capabilities = aclk_get_agent_capas()
    };

    CLAIM_ID claim_id = claim_id_get();
    if (unlikely(!claim_id_is_set(claim_id))) {
        netdata_log_error("Internal error. Should not come here if not claimed");
        return 0;
    }

    CLAIM_ID previous_claim_id = claim_id_get_last_working();
    if (claim_id_is_set(previous_claim_id))
        conn.claim_id = previous_claim_id.str;
    else
        conn.claim_id = claim_id.str;

    char *msg = generate_update_agent_connection(&len, &conn);

    if (!msg) {
        netdata_log_error("Error generating agent::v1::UpdateAgentConnection payload");
        return 0;
    }

    pid = aclk_send_bin_message_subtopic_pid(client, msg, len, ACLK_TOPICID_AGENT_CONN, "UpdateAgentConnection");
    if (claim_id_is_set(previous_claim_id))
        claim_id_clear_previous_working();

    return pid;
}

char *aclk_generate_lwt(size_t *size) {
    update_agent_connection_t conn = {
        .reachable = 0,
        .lwt = 1,
        .session_id = aclk_session_newarch,
        .capabilities = NULL
    };

    CLAIM_ID claim_id = claim_id_get();
    if(!claim_id_is_set(claim_id)) {
        netdata_log_error("Internal error. Should not come here if not claimed");
        return NULL;
    }
    conn.claim_id = claim_id.str;

    char *msg = generate_update_agent_connection(size, &conn);

    if (!msg)
        netdata_log_error("Error generating agent::v1::UpdateAgentConnection payload for LWT");

    return msg;
}

#ifndef __GNUC__
#pragma endregion
#endif
