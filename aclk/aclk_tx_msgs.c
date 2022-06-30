// SPDX-License-Identifier: GPL-3.0-or-later

#include "aclk_tx_msgs.h"
#include "daemon/common.h"
#include "aclk_util.h"
#include "aclk_stats.h"
#include "aclk.h"

#include "schema-wrappers/proto_2_json.h"

#ifndef __GNUC__
#pragma region aclk_tx_msgs helper functions
#endif

// version for aclk legacy (old cloud arch)
#define ACLK_VERSION 2

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

    if (use_mqtt_5)
        mqtt_wss_publish5(client, (char*)topic, NULL, msg, &freez, msg_len, MQTT_WSS_PUB_QOS1, &packet_id);
    else
        mqtt_wss_publish_pid(client, topic, msg, msg_len,  MQTT_WSS_PUB_QOS1, &packet_id);

#ifdef NETDATA_INTERNAL_CHECKS
    aclk_stats_msg_published(packet_id);
    char *json = protomsg_to_json(msg, msg_len, msgname);
    if (json) {
        log_aclk_message_bin(json, strlen(json), 1, topic, msgname);
        freez(json);
    }
#endif

    return packet_id;
}

/* UNUSED now but can be used soon MVP1?
static void aclk_send_message_topic(mqtt_wss_client client, json_object *msg, const char *topic)
{
    if (unlikely(!topic || topic[0] != '/')) {
        error ("Full topic required!");
        return;
    }

    const char *str = json_object_to_json_string_ext(msg, JSON_C_TO_STRING_PLAIN);

    mqtt_wss_publish(client, topic, str, strlen(str),  MQTT_WSS_PUB_QOS1);
#ifdef NETDATA_INTERNAL_CHECKS
    aclk_stats_msg_published();
#endif
#ifdef ACLK_LOG_CONVERSATION_DIR
#define FN_MAX_LEN 1024
    char filename[FN_MAX_LEN];
    snprintf(filename, FN_MAX_LEN, ACLK_LOG_CONVERSATION_DIR "/%010d-tx.json", ACLK_GET_CONV_LOG_NEXT());
    json_object_to_file_ext(filename, msg, JSON_C_TO_STRING_PRETTY);
#endif
}
*/

#define TOPIC_MAX_LEN 512
#define V2_BIN_PAYLOAD_SEPARATOR "\x0D\x0A\x0D\x0A"
static int aclk_send_message_with_bin_payload(mqtt_wss_client client, json_object *msg, const char *topic, const void *payload, size_t payload_len)
{
    uint16_t packet_id;
    const char *str;
    char *full_msg = NULL;
    int len, rc;

    if (unlikely(!topic || topic[0] != '/')) {
        error ("Full topic required!");
        return HTTP_RESP_INTERNAL_SERVER_ERROR;
    }

    str = json_object_to_json_string_ext(msg, JSON_C_TO_STRING_PLAIN);
    len = strlen(str);

    if (payload_len) {
        full_msg = mallocz(len + strlen(V2_BIN_PAYLOAD_SEPARATOR) + payload_len);

        memcpy(full_msg, str, len);
        memcpy(&full_msg[len], V2_BIN_PAYLOAD_SEPARATOR, strlen(V2_BIN_PAYLOAD_SEPARATOR));
        len += strlen(V2_BIN_PAYLOAD_SEPARATOR);
        memcpy(&full_msg[len], payload, payload_len);
        len += payload_len;
    }

/* TODO
#ifdef ACLK_LOG_CONVERSATION_DIR
#define FN_MAX_LEN 1024
    char filename[FN_MAX_LEN];
    snprintf(filename, FN_MAX_LEN, ACLK_LOG_CONVERSATION_DIR "/%010d-tx.json", ACLK_GET_CONV_LOG_NEXT());
    json_object_to_file_ext(filename, msg, JSON_C_TO_STRING_PRETTY);
#endif */

    if (use_mqtt_5)
        mqtt_wss_publish5(client, (char*)topic, NULL, (char*)(payload_len ? full_msg : str), NULL, len, MQTT_WSS_PUB_QOS1, &packet_id);
    else {
        rc = mqtt_wss_publish_pid_block(client, topic, payload_len ? full_msg : str, len,  MQTT_WSS_PUB_QOS1, &packet_id, 5000);
        if (rc == MQTT_WSS_ERR_BLOCK_TIMEOUT) {
            error("Timeout sending binpacked message");
            freez(full_msg);
            return HTTP_RESP_BACKEND_FETCH_FAILED;
        }
        if (rc == MQTT_WSS_ERR_TX_BUF_TOO_SMALL) {
            error("Message is bigger than allowed maximum");
            freez(full_msg);
            return HTTP_RESP_FORBIDDEN;
        }
    }

#ifdef NETDATA_INTERNAL_CHECKS
    aclk_stats_msg_published(packet_id);
#endif
    freez(full_msg);
    return 0;
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

#ifndef __GNUC__
#pragma endregion
#endif

#ifndef __GNUC__
#pragma region aclk_tx_msgs message generators
#endif

void aclk_http_msg_v2_err(mqtt_wss_client client, const char *topic, const char *msg_id, int http_code, int ec, const char* emsg, const char *payload, size_t payload_len)
{
    json_object *tmp, *msg;
    msg = create_hdr("http", msg_id, 0, 0, 2);
    tmp = json_object_new_int(http_code);
    json_object_object_add(msg, "http-code", tmp);

    tmp = json_object_new_int(ec);
    json_object_object_add(msg, "error-code", tmp);

    tmp = json_object_new_string(emsg);
    json_object_object_add(msg, "error-description", tmp);

    if (aclk_send_message_with_bin_payload(client, msg, topic, payload, payload_len)) {
        error("Failed to send cancelation message for http reply");
    }
    json_object_put(msg);
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

    int rc = aclk_send_message_with_bin_payload(client, msg, topic, payload, payload_len);
    json_object_put(msg);

    switch (rc) {
    case HTTP_RESP_FORBIDDEN:
        aclk_http_msg_v2_err(client, topic, msg_id, rc, CLOUD_EC_REQ_REPLY_TOO_BIG, CLOUD_EMSG_REQ_REPLY_TOO_BIG, payload, payload_len);
        break;
    case HTTP_RESP_INTERNAL_SERVER_ERROR:
        aclk_http_msg_v2_err(client, topic, msg_id, rc, CLOUD_EC_FAIL_TOPIC, CLOUD_EMSG_FAIL_TOPIC, payload, payload_len);
        break;
    case HTTP_RESP_BACKEND_FETCH_FAILED:
        aclk_http_msg_v2_err(client, topic, msg_id, rc, CLOUD_EC_SND_TIMEOUT, CLOUD_EMSG_SND_TIMEOUT, payload, payload_len);
        break;
    }
}

uint16_t aclk_send_agent_connection_update(mqtt_wss_client client, int reachable) {
    size_t len;
    uint16_t pid;

    struct capability agent_capabilities[] = {
        { .name = "json",  .version = 2, .enabled = 0 },
        { .name = "proto", .version = 1, .enabled = 1 },
#ifdef ENABLE_ML
        { .name = "ml",    .version = 1, .enabled = ml_enabled(localhost) },
#endif
        { .name = "mc",    .version = enable_metric_correlations ? metric_correlations_version : 0, .enabled = enable_metric_correlations },
        { .name = NULL,    .version = 0, .enabled = 0 }
    };

    update_agent_connection_t conn = {
        .reachable = (reachable ? 1 : 0),
        .lwt = 0,
        .session_id = aclk_session_newarch,
        .capabilities = agent_capabilities
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
    if (!use_mqtt_5)
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
        .session_id = aclk_session_newarch,
        .capabilities = NULL
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

#ifndef __GNUC__
#pragma endregion
#endif
