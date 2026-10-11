// SPDX-License-Identifier: GPL-3.0-or-later

#include "aclk_tx_msgs.h"
#include "aclk_util.h"
#include "aclk.h"
#include "aclk_capas.h"

#include "schema-wrappers/proto_2_json.h"

#ifndef __GNUC__
#pragma region aclk_tx_msgs helper functions
#endif

static void freez_aclk_publish_msg(void *ptr) {
    freez(ptr);
}

static bool aclk_size_add_overflow(size_t *total, size_t add)
{
    if (add > SIZE_MAX - *total)
        return true;

    *total += add;
    return false;
}

#define ACLK_HEADER_VERSION (2)

// Returns MQTT_WSS_OK (0) when the message was handed to the mqtt layer, non-zero otherwise -
// mqtt_wss_publish5()'s error code where it produced one, and 1 for a message that never reached
// it. Either way the message has been freed. `packet_id` is optional and reports the assigned id
// (0 when none was assigned).
//
// Callers that must know whether the message really went out MUST test the return code: the send
// can fail for reasons that carry no packet id at all - an offline or disconnecting client, a full
// mqtt buffer, a message the server rejects as too big - and a caller that does not want the id
// passes NULL. The id is informational, for matching a later PUBACK.
int aclk_send_bin_message_subtopic_pid(mqtt_wss_client client, char *msg, size_t msg_len, enum aclk_topics subtopic, const char *msgname, uint16_t *packet_id)
{
#ifndef ACLK_LOG_CONVERSATION_DIR
    UNUSED(msgname);
#endif
    uint16_t pid = 0;
    const char *topic = aclk_get_topic(subtopic);

    if (packet_id)
        *packet_id = 0;

    if (unlikely(!topic)) {
        netdata_log_error("Couldn't get topic. Aborting message send.");
        freez(msg);
        return 1;
    }

    if (aclklog_enabled) {
        char *json = protomsg_to_json(msg, msg_len, msgname);
        log_aclk_message_bin(json, strlen(json), 1, topic, msgname);
        freez(json);
    }

    int rc = mqtt_wss_publish5(client, (char *)topic, NULL, msg, &freez_aclk_publish_msg, msg_len, MQTT_WSS_PUB_QOS1, &pid);
    if (rc != MQTT_WSS_OK)
        return rc;

    if (packet_id)
        *packet_id = pid;

    return MQTT_WSS_OK;
}

#define V2_BIN_PAYLOAD_SEPARATOR "\x0D\x0A\x0D\x0A"

/*
 * Starts the JSON header common to all ACLK http messages. The caller adds the members of its own
 * message and closes it with buffer_json_finalize(). A NULL msg_id gets a random one.
 */
static BUFFER *aclk_http_msg_hdr(const char *msg_id)
{
    char uuid_str[UUID_STR_LEN];

    if (unlikely(!msg_id)) {
        nd_uuid_t uuid;
        uuid_generate(uuid);
        uuid_unparse(uuid, uuid_str);
        msg_id = uuid_str;
    }

    usec_t now_ut = now_realtime_usec();
    usec_t session = aclk_session_load();

    BUFFER *wb = buffer_create(512, &netdata_buffers_statistics.buffers_aclk);
    buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_MINIFY);
    buffer_json_member_add_string(wb, "type", "http");
    buffer_json_member_add_string(wb, "msg-id", msg_id);
    buffer_json_member_add_int64(wb, "timestamp", (int64_t)(now_ut / USEC_PER_SEC));
    buffer_json_member_add_int64(wb, "timestamp-offset-usec", (int64_t)(now_ut % USEC_PER_SEC));
    buffer_json_member_add_int64(wb, "connect", (int64_t)(session / USEC_PER_SEC));
    buffer_json_member_add_int64(wb, "connect-offset-usec", (int64_t)(session % USEC_PER_SEC));
    buffer_json_member_add_int64(wb, "version", ACLK_HEADER_VERSION);

    return wb;
}

// the finalized header of a reply to a cloud http request
static BUFFER *aclk_http_msg_reply_hdr(const char *msg_id, usec_t t_exec, usec_t created, short http_code)
{
    BUFFER *wb = aclk_http_msg_hdr(msg_id);
    buffer_json_member_add_int64(wb, "t-exec", (int64_t)t_exec);
    buffer_json_member_add_int64(wb, "t-rx", (int64_t)created);
    buffer_json_member_add_int64(wb, "http-code", http_code);
    buffer_json_finalize(wb);
    return wb;
}

// the finalized header of an error reply to a cloud http request
static BUFFER *aclk_http_msg_err_hdr(const char *msg_id, int http_code, int ec, const char *emsg)
{
    BUFFER *wb = aclk_http_msg_hdr(msg_id);
    buffer_json_member_add_int64(wb, "http-code", http_code);
    buffer_json_member_add_int64(wb, "error-code", ec);
    buffer_json_member_add_string(wb, "error-description", emsg);
    buffer_json_finalize(wb);
    return wb;
}

// Lays out the finalized header, then - only when there is a payload - the separator and the two payload
// parts, in one allocation. Returns NULL when the total size overflows.
static char *aclk_http_msg_assemble(BUFFER *hdr, const void *p1, size_t p1_len, const void *p2, size_t p2_len, size_t *msg_len)
{
    if (!p1)
        p1_len = 0;
    if (!p2)
        p2_len = 0;

    const size_t sep_len = sizeof(V2_BIN_PAYLOAD_SEPARATOR) - 1;
    const bool has_payload = (p1_len != 0 || p2_len != 0);

    size_t total = buffer_strlen(hdr);
    if (unlikely(
            (has_payload && aclk_size_add_overflow(&total, sep_len)) ||
            aclk_size_add_overflow(&total, p1_len) ||
            aclk_size_add_overflow(&total, p2_len)))
        return NULL;

    char *msg = mallocz(total);

    size_t pos = buffer_strlen(hdr);
    memcpy(msg, buffer_tostring(hdr), pos);

    if (has_payload) {
        memcpy(msg + pos, V2_BIN_PAYLOAD_SEPARATOR, sep_len);
        pos += sep_len;
    }

    if (p1_len) {
        memcpy(msg + pos, p1, p1_len);
        pos += p1_len;
    }

    if (p2_len)
        memcpy(msg + pos, p2, p2_len);

    *msg_len = total;
    return msg;
}

// Publishes the finalized header and its payload, and frees the header.
// Returns 0, HTTP_RESP_CONTENT_TOO_LONG or HTTP_RESP_INTERNAL_SERVER_ERROR.
static short aclk_http_msg_send(mqtt_wss_client client, const char *topic, BUFFER *hdr,
                                const void *p1, size_t p1_len, const void *p2, size_t p2_len)
{
    if (unlikely(!topic || topic[0] != '/')) {
        netdata_log_error("Full topic required!");
        buffer_free(hdr);
        return HTTP_RESP_INTERNAL_SERVER_ERROR;
    }

    size_t msg_len = 0;
    char *msg = aclk_http_msg_assemble(hdr, p1, p1_len, p2, p2_len, &msg_len);
    buffer_free(hdr);

    if (unlikely(!msg))
        return HTTP_RESP_CONTENT_TOO_LONG;

    uint16_t packet_id = 0;
    int rc = mqtt_wss_publish5(client, (char *)topic, NULL, msg, &freez_aclk_publish_msg, msg_len, MQTT_WSS_PUB_QOS1, &packet_id);

    if (rc == MQTT_WSS_ERR_MSG_TOO_BIG)
        return HTTP_RESP_CONTENT_TOO_LONG;
    if (rc != MQTT_WSS_OK)
        return HTTP_RESP_INTERNAL_SERVER_ERROR;

    return 0;
}

#ifndef __GNUC__
#pragma endregion
#endif

#ifndef __GNUC__
#pragma region aclk_tx_msgs message generators
#endif

void aclk_http_msg_v2_err(mqtt_wss_client client, const char *topic, const char *msg_id, int http_code, int ec, const char* emsg, const char *payload, size_t payload_len)
{
    BUFFER *hdr = aclk_http_msg_err_hdr(msg_id, http_code, ec, emsg);

    if (aclk_http_msg_send(client, topic, hdr, payload, payload_len, NULL, 0)) {
        netdata_log_error("Failed to send cancellation message for http reply %zu %s", payload_len, payload);
    }
}

short aclk_http_msg_v2(mqtt_wss_client client, const char *topic, const char *msg_id, usec_t t_exec, usec_t created,
    short http_code, const char *payload, size_t payload_len)
{
    BUFFER *hdr = aclk_http_msg_reply_hdr(msg_id, t_exec, created, http_code);

    short rc = aclk_http_msg_send(client, topic, hdr, payload, payload_len, NULL, 0);

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

short aclk_http_msg_v2_direct(mqtt_wss_client client, const char *topic, const char *msg_id,
                               usec_t t_exec, usec_t created, short http_code,
                               const char *http_headers, size_t http_headers_len,
                               const char *body, size_t body_len)
{
    if (unlikely(!topic || topic[0] != '/')) {
        netdata_log_error("Full topic required!");
        return HTTP_RESP_INTERNAL_SERVER_ERROR;
    }

    BUFFER *hdr = aclk_http_msg_reply_hdr(msg_id, t_exec, created, http_code);

    short rc = aclk_http_msg_send(client, topic, hdr, http_headers, http_headers_len, body, body_len);

    if (rc == HTTP_RESP_CONTENT_TOO_LONG) {
        aclk_http_msg_v2_err(client, topic, msg_id, HTTP_RESP_CONTENT_TOO_LONG, CLOUD_EC_REQ_REPLY_TOO_BIG, CLOUD_EMSG_REQ_REPLY_TOO_BIG, NULL, 0);
        return HTTP_RESP_CONTENT_TOO_LONG;
    }

    if (rc == HTTP_RESP_INTERNAL_SERVER_ERROR) {
        aclk_http_msg_v2_err(client, topic, msg_id, HTTP_RESP_INTERNAL_SERVER_ERROR, CLOUD_EC_SND_TIMEOUT, CLOUD_EMSG_SND_TIMEOUT, NULL, 0);
        return HTTP_RESP_INTERNAL_SERVER_ERROR;
    }

    return http_code;
}

uint16_t aclk_send_agent_connection_update(mqtt_wss_client client, int reachable) {
    size_t len;
    uint16_t pid;

    update_agent_connection_t conn = {
        .reachable = (reachable ? 1 : 0),
        .lwt = 0,
        .session_id = aclk_session_load(),
        .capabilities = aclk_get_agent_capas(),
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

    // a failed publish leaves pid at 0, which is what this returned before it reported the code
    (void)aclk_send_bin_message_subtopic_pid(client, msg, len, ACLK_TOPICID_AGENT_CONN, "UpdateAgentConnection", &pid);
    if (claim_id_is_set(previous_claim_id))
        claim_id_clear_previous_working();

    return pid;
}

char *aclk_generate_lwt(size_t *size) {
    update_agent_connection_t conn = {
        .reachable = 0,
        .lwt = 1,
        .session_id = aclk_session_load(),
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

#define ACLK_HTTP_MSG_TEST(condition, msg, id) do {                              \
        if(!(condition)) {                                                       \
            fprintf(stderr, "aclk http msg unittest FAILED: %s, msg-id case %zu (%s:%d)\n", \
                    (msg), (size_t)(id), __FUNCTION__, __LINE__);                \
            errors++;                                                            \
        }                                                                        \
    } while(0)

// Cloud parses the header as JSON: check every message shape parses back to the same members, in the
// same order, with the same values - including msg-ids that need escaping.
int aclk_http_msg_unittest(void)
{
    int errors = 0;

    fprintf(stderr, "\nrunning aclk http msg unittest\n");

    char long_id[4096];
    memset(long_id, 'a', sizeof(long_id) - 1);
    long_id[sizeof(long_id) - 1] = '\0';

    const char *ids[] = {
        "3f2b1c4d-5e6f-4a7b-8c9d-0e1f2a3b4c5d",
        "with/slash",
        "with\"quote",
        "with\\backslash",
        "ctl\x01\x1f\t\n\r\b\f",
        "utf8-\xce\xb1\xce\xb2-\xe2\x82\xac",
        "",
        long_id,
        NULL,
    };

    static const char *reply_keys[] = {
        "type", "msg-id", "timestamp", "timestamp-offset-usec", "connect", "connect-offset-usec", "version",
        "t-exec", "t-rx", "http-code", NULL,
    };
    static const char *err_keys[] = {
        "type", "msg-id", "timestamp", "timestamp-offset-usec", "connect", "connect-offset-usec", "version",
        "http-code", "error-code", "error-description", NULL,
    };

    const usec_t t_exec = 12345, t_rx = 1760000000000001ULL;

    for (size_t i = 0; i < sizeof(ids) / sizeof(ids[0]); i++) {
        for (int shape = 0; shape < 2; shape++) {
            BUFFER *hdr = shape == 0 ? aclk_http_msg_reply_hdr(ids[i], t_exec, t_rx, 200)
                                     : aclk_http_msg_err_hdr(ids[i], 504, CLOUD_EC_SND_TIMEOUT, CLOUD_EMSG_SND_TIMEOUT);

            json_object *o = json_tokener_parse(buffer_tostring(hdr));
            ACLK_HTTP_MSG_TEST(o && json_object_is_type(o, json_type_object), "header is not a JSON object", i);
            if (!o) {
                buffer_free(hdr);
                continue;
            }

            const char **keys = shape == 0 ? reply_keys : err_keys;
            size_t k = 0;
            json_object_object_foreach(o, key, val) {
                (void)val;
                ACLK_HTTP_MSG_TEST(keys[k] && strcmp(key, keys[k]) == 0, "unexpected member or member order", i);
                if (keys[k])
                    k++;
            }
            ACLK_HTTP_MSG_TEST(keys[k] == NULL, "missing members", i);

            // every member except the two strings (and error-description) is an integer
            json_object_object_foreach(o, nkey, nval) {
                bool is_string = !strcmp(nkey, "type") || !strcmp(nkey, "msg-id") || !strcmp(nkey, "error-description");
                ACLK_HTTP_MSG_TEST(json_object_is_type(nval, is_string ? json_type_string : json_type_int),
                                   "member has the wrong JSON type", i);
            }

            json_object *v;
            ACLK_HTTP_MSG_TEST(json_object_object_get_ex(o, "type", &v) &&
                                   strcmp(json_object_get_string(v), "http") == 0, "type is not http", i);

            ACLK_HTTP_MSG_TEST(json_object_object_get_ex(o, "msg-id", &v) && json_object_is_type(v, json_type_string),
                               "msg-id is not a string", i);
            if (ids[i])
                ACLK_HTTP_MSG_TEST(strcmp(json_object_get_string(v), ids[i]) == 0, "msg-id did not round-trip", i);
            else {
                nd_uuid_t uuid;
                ACLK_HTTP_MSG_TEST(uuid_parse(json_object_get_string(v), uuid) == 0, "generated msg-id is not a uuid", i);
            }

            ACLK_HTTP_MSG_TEST(json_object_object_get_ex(o, "version", &v) &&
                                   json_object_get_int(v) == ACLK_HEADER_VERSION, "wrong version", i);

            int64_t ts = 0, ts_us = 0;
            if (json_object_object_get_ex(o, "timestamp", &v))
                ts = json_object_get_int64(v);
            if (json_object_object_get_ex(o, "timestamp-offset-usec", &v))
                ts_us = json_object_get_int64(v);
            // loose on purpose: the wall clock may be adjusted while the test runs
            int64_t now_s = (int64_t)(now_realtime_usec() / USEC_PER_SEC);
            ACLK_HTTP_MSG_TEST(ts_us >= 0 && ts_us < (int64_t)USEC_PER_SEC && ts > now_s - 86400 && ts < now_s + 86400,
                               "timestamp is not the current time", i);

            usec_t session = aclk_session_load();
            ACLK_HTTP_MSG_TEST(json_object_object_get_ex(o, "connect", &v) &&
                                   json_object_get_int64(v) == (int64_t)(session / USEC_PER_SEC), "wrong connect", i);
            ACLK_HTTP_MSG_TEST(json_object_object_get_ex(o, "connect-offset-usec", &v) &&
                                   json_object_get_int64(v) == (int64_t)(session % USEC_PER_SEC), "wrong connect offset", i);

            if (shape == 0) {
                ACLK_HTTP_MSG_TEST(json_object_object_get_ex(o, "t-exec", &v) &&
                                       json_object_get_int64(v) == (int64_t)t_exec, "wrong t-exec", i);
                ACLK_HTTP_MSG_TEST(json_object_object_get_ex(o, "t-rx", &v) &&
                                       json_object_get_int64(v) == (int64_t)t_rx, "wrong t-rx", i);
                ACLK_HTTP_MSG_TEST(json_object_object_get_ex(o, "http-code", &v) &&
                                       json_object_get_int(v) == 200, "wrong http-code", i);
            }
            else {
                ACLK_HTTP_MSG_TEST(json_object_object_get_ex(o, "http-code", &v) &&
                                       json_object_get_int(v) == 504, "wrong error http-code", i);
                ACLK_HTTP_MSG_TEST(json_object_object_get_ex(o, "error-code", &v) &&
                                       json_object_get_int(v) == CLOUD_EC_SND_TIMEOUT, "wrong error-code", i);
                ACLK_HTTP_MSG_TEST(json_object_object_get_ex(o, "error-description", &v) &&
                                       strcmp(json_object_get_string(v), CLOUD_EMSG_SND_TIMEOUT) == 0,
                                   "wrong error-description", i);
            }

            json_object_put(o);
            buffer_free(hdr);
        }
    }

    // message layout: the separator is there only when there is a payload, and NULL parts count as empty
    {
        BUFFER *hdr = aclk_http_msg_reply_hdr("layout", 1, 2, 200);
        size_t hdr_len = buffer_strlen(hdr);
        const size_t sep_len = sizeof(V2_BIN_PAYLOAD_SEPARATOR) - 1;
        size_t len = 0;

        char *msg = aclk_http_msg_assemble(hdr, NULL, 0, NULL, 0, &len);
        ACLK_HTTP_MSG_TEST(msg && len == hdr_len && memcmp(msg, buffer_tostring(hdr), hdr_len) == 0,
                           "header-only message is not the header alone", 0);
        freez(msg);

        msg = aclk_http_msg_assemble(hdr, NULL, 10, NULL, 20, &len);
        ACLK_HTTP_MSG_TEST(msg && len == hdr_len, "NULL payload parts were not treated as empty", 0);
        freez(msg);

        msg = aclk_http_msg_assemble(hdr, "HDRS", 4, "body", 4, &len);
        ACLK_HTTP_MSG_TEST(msg && len == hdr_len + sep_len + 8 &&
                               memcmp(msg, buffer_tostring(hdr), hdr_len) == 0 &&
                               memcmp(msg + hdr_len, V2_BIN_PAYLOAD_SEPARATOR, sep_len) == 0 &&
                               memcmp(msg + hdr_len + sep_len, "HDRSbody", 8) == 0,
                           "header + separator + payload parts are not laid out in order", 0);
        freez(msg);

        msg = aclk_http_msg_assemble(hdr, NULL, 0, "body", 4, &len);
        ACLK_HTTP_MSG_TEST(msg && len == hdr_len + sep_len + 4 && memcmp(msg + hdr_len + sep_len, "body", 4) == 0,
                           "body without headers is not laid out after the separator", 0);
        freez(msg);

        msg = aclk_http_msg_assemble(hdr, "x", 1, "y", SIZE_MAX, &len);
        ACLK_HTTP_MSG_TEST(msg == NULL, "an overflowing size was not refused", 0);
        freez(msg);

        buffer_free(hdr);
    }

    if (errors)
        fprintf(stderr, "aclk http msg unittest: %d ERROR(S)\n", errors);
    else
        fprintf(stderr, "aclk http msg unittest: OK\n");

    return errors;
}
