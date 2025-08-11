// SPDX-License-Identifier: GPL-3.0-or-later

#include "aclk_otp.h"
#include "aclk_util.h"
#include "aclk.h"

static https_client_resp_t aclk_https_request(https_req_t *request, https_req_response_t *response, bool *fallback_ipv4) {
    https_client_resp_t rc;
    // wrapper for ACLK only which loads ACLK specific proxy settings
    // then only calls https_request
    struct mqtt_wss_proxy proxy_conf = { .host = NULL, .port = 0, .username = NULL, .password = NULL, .proxy_destination = NULL, .type = MQTT_WSS_DIRECT };
    aclk_set_proxy(
        (char **)&proxy_conf.host,
        &proxy_conf.port,
        (char **)&proxy_conf.username,
        (char **)&proxy_conf.password,
        (char **)&proxy_conf.proxy_destination,
        &proxy_conf.type);

    if (proxy_conf.type == MQTT_WSS_PROXY_HTTP) {
        request->proxy_host = (char *)proxy_conf.host;
        request->proxy_port = proxy_conf.port;
        request->proxy_username = proxy_conf.username;
        request->proxy_password = proxy_conf.password;
        request->proxy = proxy_conf.proxy_destination;
    }

    rc = https_request(request, response, fallback_ipv4);
    freez((char*)proxy_conf.host);
    freez((char*)proxy_conf.username);
    freez((char*)proxy_conf.password);
    return rc;
}

struct auth_data {
    char *client_id;
    char *username;
    char *passwd;
};

#define PARSE_ENV_JSON_CHK_TYPE(it, type, name)                                                                        \
    if (json_object_get_type(json_object_iter_peek_value(it)) != type) {                                               \
        netdata_log_error("ACLK: value of key \"%s\" should be %s", name, #type);                                      \
        goto exit;                                                                                                     \
    }

#define JSON_KEY_CLIENTID "clientID"
#define JSON_KEY_USER     "username"
#define JSON_KEY_PASS     "password"
#define JSON_KEY_TOPICS   "topics"

static int parse_passwd_response(const char *json_str, struct auth_data *auth) {
    int rc = 1;
    json_object *json;
    struct json_object_iterator it;
    struct json_object_iterator itEnd;

    json = json_tokener_parse(json_str);
    if (!json) {
        netdata_log_error("ACLK: JSON-C failed to parse the payload of http response of /env endpoint");
        return 1;
    }

    it = json_object_iter_begin(json);
    itEnd = json_object_iter_end(json);

    while (!json_object_iter_equal(&it, &itEnd)) {
        if (!strcmp(json_object_iter_peek_name(&it), JSON_KEY_CLIENTID)) {
            PARSE_ENV_JSON_CHK_TYPE(&it, json_type_string, JSON_KEY_CLIENTID)

            auth->client_id = strdupz(json_object_get_string(json_object_iter_peek_value(&it)));
            json_object_iter_next(&it);
            continue;
        }
        if (!strcmp(json_object_iter_peek_name(&it), JSON_KEY_USER)) {
            PARSE_ENV_JSON_CHK_TYPE(&it, json_type_string, JSON_KEY_USER)

            auth->username = strdupz(json_object_get_string(json_object_iter_peek_value(&it)));
            json_object_iter_next(&it);
            continue;
        }
        if (!strcmp(json_object_iter_peek_name(&it), JSON_KEY_PASS)) {
            PARSE_ENV_JSON_CHK_TYPE(&it, json_type_string, JSON_KEY_PASS)

            auth->passwd = strdupz(json_object_get_string(json_object_iter_peek_value(&it)));
            json_object_iter_next(&it);
            continue;
        }
        if (!strcmp(json_object_iter_peek_name(&it), JSON_KEY_TOPICS)) {
            PARSE_ENV_JSON_CHK_TYPE(&it, json_type_array, JSON_KEY_TOPICS)

            if (aclk_generate_topic_cache(json_object_iter_peek_value(&it))) {
                netdata_log_error("ACLK: Failed to generate topic cache!");
                goto exit;
            }
            json_object_iter_next(&it);
            continue;
        }
        netdata_log_error("ACLK: Unknown key \"%s\" in passwd response payload. Ignoring", json_object_iter_peek_name(&it));
        json_object_iter_next(&it);
    }

    if (!auth->client_id) {
        netdata_log_error("ACLK: " JSON_KEY_CLIENTID " is compulsory key in /password response");
        goto exit;
    }
    if (!auth->passwd) {
        netdata_log_error("ACLK: " JSON_KEY_PASS " is compulsory in /password response");
        goto exit;
    }
    if (!auth->username) {
        netdata_log_error("ACLK: " JSON_KEY_USER " is compulsory in /password response");
        goto exit;
    }

    rc = 0;
exit:
    json_object_put(json);
    return rc;
}

#define JSON_KEY_ERTRY   "errorNonRetryable"
#define JSON_KEY_EDELAY  "errorRetryDelaySeconds"
#define JSON_KEY_EEC     "errorCode"
#define JSON_KEY_EMSGKEY "errorMsgKey"
#define JSON_KEY_EMSG    "errorMessage"
#if JSON_C_MINOR_VERSION >= 13
static const char *get_json_str_by_path(json_object *json, const char *path) {
    json_object *ptr;
    if (json_pointer_get(json, path, &ptr)) {
        netdata_log_error("ACLK: Missing compulsory key \"%s\" in error response", path);
        return NULL;
    }
    if (json_object_get_type(ptr) != json_type_string) {
        netdata_log_error("ACLK: Value of Key \"%s\" in error response should be string", path);
        return NULL;
    }
    return json_object_get_string(ptr);
}

static int aclk_parse_otp_error(const char *json_str) {
    int rc = 1;
    json_object *json, *ptr;
    const char *ec;
    const char *ek;
    const char *emsg;
    int block_retry = -1, backoff = -1;


    json = json_tokener_parse(json_str);
    if (!json) {
        netdata_log_error("ACLK: JSON-C failed to parse the payload of http response of /env endpoint");
        return 1;
    }

    if ((ec = get_json_str_by_path(json, "/" JSON_KEY_EEC)) == NULL)
        goto exit;

    if ((ek = get_json_str_by_path(json, "/" JSON_KEY_EMSGKEY)) == NULL)
        goto exit;

    if ((emsg = get_json_str_by_path(json, "/" JSON_KEY_EMSG)) == NULL)
        goto exit;

    // optional field
    if (!json_pointer_get(json, "/" JSON_KEY_ERTRY, &ptr)) {
        if (json_object_get_type(ptr) != json_type_boolean) {
            netdata_log_error("ACLK: Error response Key " "/" JSON_KEY_ERTRY " should be of boolean type");
            goto exit;
        }
        block_retry = json_object_get_boolean(ptr);
    }

    // optional field
    if (!json_pointer_get(json, "/" JSON_KEY_EDELAY, &ptr)) {
        if (json_object_get_type(ptr) != json_type_int) {
            netdata_log_error("ACLK: Error response Key " "/" JSON_KEY_EDELAY " should be of integer type");
            goto exit;
        }
        backoff = json_object_get_int(ptr);
    }

    if (block_retry > 0)
        aclk_disable_runtime = 1;

    if (backoff > 0)
        aclk_block_until = now_monotonic_sec() + backoff;

    netdata_log_error("ACLK: Cloud returned EC=\"%s\", Msg-Key:\"%s\", Msg:\"%s\", BlockRetry:%s, Backoff:%ds (-1 unset by cloud)", ec, ek, emsg, block_retry > 0 ? "true" : "false", backoff);
    rc = 0;
exit:
    json_object_put(json);
    return rc;
}
#else
static int aclk_parse_otp_error(const char *json_str) {
    int rc = 1;
    int block_retry = -1, backoff = -1;

    const char *ec = NULL;
    const char *ek = NULL;
    const char *emsg = NULL;

    json_object *json;
    struct json_object_iterator it;
    struct json_object_iterator itEnd;

    json = json_tokener_parse(json_str);
    if (!json) {
        netdata_log_error("ACLK: JSON-C failed to parse the payload of http response of /env endpoint");
        return 1;
    }

    it = json_object_iter_begin(json);
    itEnd = json_object_iter_end(json);

    while (!json_object_iter_equal(&it, &itEnd)) {
        if (!strcmp(json_object_iter_peek_name(&it), JSON_KEY_EMSG)) {
            PARSE_ENV_JSON_CHK_TYPE(&it, json_type_string, JSON_KEY_EMSG)

            emsg = json_object_get_string(json_object_iter_peek_value(&it));
            json_object_iter_next(&it);
            continue;
        }
        if (!strcmp(json_object_iter_peek_name(&it), JSON_KEY_EMSGKEY)) {
            PARSE_ENV_JSON_CHK_TYPE(&it, json_type_string, JSON_KEY_EMSGKEY)

            ek = json_object_get_string(json_object_iter_peek_value(&it));
            json_object_iter_next(&it);
            continue;
        }
        if (!strcmp(json_object_iter_peek_name(&it), JSON_KEY_EEC)) {
            PARSE_ENV_JSON_CHK_TYPE(&it, json_type_string, JSON_KEY_EEC)

            ec = strdupz(json_object_get_string(json_object_iter_peek_value(&it)));
            json_object_iter_next(&it);
            continue;
        }
        if (!strcmp(json_object_iter_peek_name(&it), JSON_KEY_EDELAY)) {
            if (json_object_get_type(json_object_iter_peek_value(&it)) != json_type_int) {
                netdata_log_error("ACLK: value of key " JSON_KEY_EDELAY " should be integer");
                goto exit;
            }

            backoff = json_object_get_int(json_object_iter_peek_value(&it));
            json_object_iter_next(&it);
            continue;
        }
        if (!strcmp(json_object_iter_peek_name(&it), JSON_KEY_ERTRY)) {
            if (json_object_get_type(json_object_iter_peek_value(&it)) != json_type_boolean) {
                netdata_log_error("ACLK: value of key " JSON_KEY_ERTRY " should be integer");
                goto exit;
            }

            block_retry = json_object_get_boolean(json_object_iter_peek_value(&it));
            json_object_iter_next(&it);
            continue;
        }
        netdata_log_error("ACLK: Unknown key \"%s\" in error response payload. Ignoring", json_object_iter_peek_name(&it));
        json_object_iter_next(&it);
    }

    if (block_retry > 0)
        aclk_disable_runtime = 1;

    if (backoff > 0)
        aclk_block_until = now_monotonic_sec() + backoff;

    netdata_log_error("ACLK: Cloud returned EC=\"%s\", Msg-Key:\"%s\", Msg:\"%s\", BlockRetry:%s, Backoff:%ds (-1 unset by cloud)", ec, ek, emsg, block_retry > 0 ? "true" : "false", backoff);
    rc = 0;
exit:
    json_object_put(json);
    return rc;
}
#endif

#define CHALLENGE_LEN 256
#define CHALLENGE_LEN_BASE64 344

#define OTP_URL_PREFIX "/api/v1/auth/node/"
static https_client_resp_t aclk_get_otp_challenge(url_t *target, const char *agent_id, unsigned char **challenge, int *challenge_bytes, bool *fallback_ipv4)
{
    https_client_resp_t rc;
    https_req_t req = HTTPS_REQ_T_INITIALIZER;
    https_req_response_t resp = HTTPS_REQ_RESPONSE_T_INITIALIZER;

    BUFFER *url = buffer_create(strlen(OTP_URL_PREFIX) + UUID_STR_LEN + 20, &netdata_buffers_statistics.buffers_aclk);

    req.host = target->host;
    req.port = target->port;
    buffer_sprintf(url, "%s/node/%s/challenge", target->path, agent_id);
    req.url = (char *)buffer_tostring(url);

    rc = aclk_https_request(&req, &resp, fallback_ipv4);
    if (rc != HTTPS_CLIENT_RESP_OK) {
        netdata_log_error("ACLK: OTP Challenge failed");
        buffer_free(url);
        return rc;
    }
    if (resp.http_code != 200) {
        rc = HTTPS_CLIENT_RESP_OTP_CHALLENGE_NOT_200;
        netdata_log_error("ACLK: OTP Challenge HTTP code not 200 OK (got %d)", resp.http_code);
        buffer_free(url);
        if (resp.payload_size)
            aclk_parse_otp_error(resp.payload);
        goto cleanup_resp;
    }
    buffer_free(url);

    json_object *json = json_tokener_parse(resp.payload);
    if (!json) {
        rc = HTTPS_CLIENT_RESP_OTP_CHALLENGE_INVALID;
        netdata_log_error("ACLK: couldn't parse HTTP GET challenge payload");
        goto cleanup_resp;
    }
    json_object *challenge_json;
    if (!json_object_object_get_ex(json, "challenge", &challenge_json)) {
        rc = HTTPS_CLIENT_RESP_OTP_CHALLENGE_INVALID;
        netdata_log_error("ACLK: No key named \"challenge\" in the returned JSON");
        goto cleanup_json;
    }
    if (!json_object_is_type(challenge_json, json_type_string)) {
        rc = HTTPS_CLIENT_RESP_OTP_CHALLENGE_INVALID;
        netdata_log_error("ACLK: \"challenge\" is not a string JSON type");
        goto cleanup_json;
    }
    const char *challenge_base64;
    if (!((challenge_base64 = json_object_get_string(challenge_json)))) {
        rc = HTTPS_CLIENT_RESP_OTP_CHALLENGE_INVALID;
        netdata_log_error("ACLK: Failed to extract challenge from JSON object");
        goto cleanup_json;
    }
    if (strlen(challenge_base64) != CHALLENGE_LEN_BASE64) {
        rc = HTTPS_CLIENT_RESP_OTP_CHALLENGE_INVALID;
        netdata_log_error("ACLK: Received Challenge has unexpected length of %zu (expected %d)", strlen(challenge_base64), CHALLENGE_LEN_BASE64);
        goto cleanup_json;
    }

    *challenge = mallocz((CHALLENGE_LEN_BASE64 / 4) * 3 + 1);
    *challenge_bytes = netdata_base64_decode(*challenge, (const unsigned char *) challenge_base64, CHALLENGE_LEN_BASE64);

    if (*challenge_bytes != CHALLENGE_LEN) {
        rc = HTTPS_CLIENT_RESP_OTP_CHALLENGE_INVALID;
        netdata_log_error("ACLK: Unexpected challenge length of %d instead of %d", *challenge_bytes, CHALLENGE_LEN);
        freez(*challenge);
        *challenge = NULL;
        goto cleanup_json;
    }

    rc = HTTPS_CLIENT_RESP_OK;

cleanup_json:
    json_object_put(json);
cleanup_resp:
    https_req_response_free(&resp);
    return rc;
}

static https_client_resp_t aclk_send_otp_response(const char *agent_id, const unsigned char *response, int response_bytes, url_t *target, struct auth_data *mqtt_auth, bool *fallback_ipv4)
{
    https_client_resp_t rc;
    https_req_t req = HTTPS_REQ_T_INITIALIZER;
    https_req_response_t resp = HTTPS_REQ_RESPONSE_T_INITIALIZER;

    req.host = target->host;
    req.port = target->port;
    req.request_type = HTTP_REQ_POST;

    unsigned char base64[CHALLENGE_LEN_BASE64 + 1];
    memset(base64, 0, CHALLENGE_LEN_BASE64 + 1);

    (void) netdata_base64_encode(base64, response, response_bytes);

    BUFFER *url = buffer_create(strlen(OTP_URL_PREFIX) + UUID_STR_LEN + 20, &netdata_buffers_statistics.buffers_aclk);
    BUFFER *resp_json = buffer_create(strlen(OTP_URL_PREFIX) + UUID_STR_LEN + 20, &netdata_buffers_statistics.buffers_aclk);

    buffer_sprintf(url, "%s/node/%s/password", target->path, agent_id);
    buffer_sprintf(resp_json, "{\"agent_version\":\"%s\", \"response\":\"%s\"}", NETDATA_VERSION, base64);

    req.url = (char *)buffer_tostring(url);
    req.payload = (char *)buffer_tostring(resp_json);
    req.payload_size = strlen(req.payload);

    rc = aclk_https_request(&req, &resp, fallback_ipv4);
    if (rc != HTTPS_CLIENT_RESP_OK) {
        netdata_log_error("ACLK: OTP Password error trying to post result to password");
        goto cleanup_buffers;
    }
    if (resp.http_code != 201) {
        rc = HTTPS_CLIENT_RESP_OTP_PASSWORD_NOT_201;
        netdata_log_error("ACLK: OTP Password HTTP code not 201 Created (got %d)", resp.http_code);
        if (resp.payload_size)
            aclk_parse_otp_error(resp.payload);
        goto cleanup_response;
    }
    if (resp.payload_size == 0 || resp.payload == NULL) {
        rc = HTTPS_CLIENT_RESP_OTP_PASSWORD_EMPTY;
        netdata_log_error("ACLK: OTP Password response payload is empty despite returning 201 Created!");
        goto cleanup_response;
    }

    if (parse_passwd_response(resp.payload, mqtt_auth)){
        rc = HTTPS_CLIENT_RESP_OTP_PASSWORD_NOT_JSON;
        netdata_log_error("ACLK: Error parsing response of password endpoint");
        goto cleanup_response;
    }

    rc = HTTPS_CLIENT_RESP_OK;

cleanup_response:
    https_req_response_free(&resp);
cleanup_buffers:
    buffer_free(resp_json);
    buffer_free(url);
    return rc;
}

#if OPENSSL_VERSION_NUMBER >= OPENSSL_VERSION_300
static int private_decrypt(EVP_PKEY *p_key, unsigned char * enc_data, int data_len, unsigned char **decrypted)
#else
static int private_decrypt(RSA *p_key, unsigned char * enc_data, int data_len, unsigned char **decrypted)
#endif
{
    int result;
#if OPENSSL_VERSION_NUMBER >= OPENSSL_VERSION_300
    size_t outlen = EVP_PKEY_size(p_key);
    EVP_PKEY_CTX *ctx = EVP_PKEY_CTX_new(p_key, NULL);
    if (!ctx)
        return 1;

    if (EVP_PKEY_decrypt_init(ctx) <= 0) {
        EVP_PKEY_CTX_free(ctx);
        return 1;
    }

    if (EVP_PKEY_CTX_set_rsa_padding(ctx, RSA_PKCS1_OAEP_PADDING) <= 0) {
        EVP_PKEY_CTX_free(ctx);
        return 1;
    }

    *decrypted = mallocz(outlen);

    if (EVP_PKEY_decrypt(ctx, *decrypted, &outlen, enc_data, data_len) == 1)
        result = (int) outlen;
    else
        result = -1;

    EVP_PKEY_CTX_free(ctx);
#else
    *decrypted = mallocz(RSA_size(p_key));
    result = RSA_private_decrypt(data_len, enc_data, *decrypted, p_key, RSA_PKCS1_OAEP_PADDING);
#endif
    if (result == -1)
    {
        char err[512];
        ERR_error_string_n(ERR_get_error(), err, sizeof(err));
        netdata_log_error("ACLK: Decryption of the challenge failed: %s", err);
    }
    return result;
}

#if OPENSSL_VERSION_NUMBER >= OPENSSL_VERSION_300
https_client_resp_t aclk_get_mqtt_otp(EVP_PKEY *p_key, char **mqtt_id, char **mqtt_usr, char **mqtt_pass, url_t *target, bool *fallback_ipv4)
#else
https_client_resp_t aclk_get_mqtt_otp(RSA *p_key, char **mqtt_id, char **mqtt_usr, char **mqtt_pass, url_t *target, bool *fallback_ipv4)
#endif
{
    unsigned char *challenge = NULL;
    int challenge_bytes;
    https_client_resp_t rc;

    CLAIM_ID claim_id = claim_id_get();
    if (!claim_id_is_set(claim_id)) {
        netdata_log_error("ACLK: Agent was not claimed - cannot perform challenge/response");
        return HTTPS_CLIENT_RESP_OTP_AGENT_NOT_CLAIMED;
    }

    // Get Challenge
    rc = aclk_get_otp_challenge(target, claim_id.str, &challenge, &challenge_bytes, fallback_ipv4);
    if (rc != HTTPS_CLIENT_RESP_OK) {
        netdata_log_error("ACLK: error getting challenge");
        return rc;
    }

    // Decrypt Challenge / Get response
    unsigned char *response_plaintext = NULL;
    int response_plaintext_bytes = private_decrypt(p_key, challenge, challenge_bytes, &response_plaintext);
    if (response_plaintext_bytes < 0) {
        netdata_log_error("ACLK: Couldn't decrypt the challenge received");
        freez(response_plaintext);
        freez(challenge);
        return HTTPS_CLIENT_RESP_OTP_CHALLENGE_DECRYPTION_FAILED;
    }
    freez(challenge);

    // Encode and Send Challenge
    struct auth_data data = { .client_id = NULL, .passwd = NULL, .username = NULL };
    rc = aclk_send_otp_response(claim_id.str, response_plaintext, response_plaintext_bytes, target, &data, fallback_ipv4);
    if (rc != HTTPS_CLIENT_RESP_OK) {
        netdata_log_error("ACLK: Error getting response");
        freez(response_plaintext);
        return rc;
    }

    *mqtt_pass = data.passwd;
    *mqtt_usr = data.username;
    *mqtt_id = data.client_id;

    freez(response_plaintext);
    return HTTPS_CLIENT_RESP_OK;
}

#define JSON_KEY_ENC "encoding"
#define JSON_KEY_AUTH_ENDPOINT "authEndpoint"
#define JSON_KEY_TRP "transports"
#define JSON_KEY_TRP_TYPE "type"
#define JSON_KEY_TRP_ENDPOINT "endpoint"
#define JSON_KEY_BACKOFF "backoff"
#define JSON_KEY_BACKOFF_BASE "base"
#define JSON_KEY_BACKOFF_MAX "maxSeconds"
#define JSON_KEY_BACKOFF_MIN "minSeconds"
#define JSON_KEY_CAPS "capabilities"

static int parse_json_env_transport(json_object *json, aclk_transport_desc_t *trp) {
    struct json_object_iterator it;
    struct json_object_iterator itEnd;

    it = json_object_iter_begin(json);
    itEnd = json_object_iter_end(json);

    while (!json_object_iter_equal(&it, &itEnd)) {
        if (!strcmp(json_object_iter_peek_name(&it), JSON_KEY_TRP_TYPE)) {
            PARSE_ENV_JSON_CHK_TYPE(&it, json_type_string, JSON_KEY_TRP_TYPE)
            if (trp->type != ACLK_TRP_UNKNOWN) {
                netdata_log_error("ACLK: " JSON_KEY_TRP_TYPE " set already");
                goto exit;
            }
            trp->type = aclk_transport_type_t_from_str(json_object_get_string(json_object_iter_peek_value(&it)));
            if (trp->type == ACLK_TRP_UNKNOWN) {
                netdata_log_error("ACLK: " JSON_KEY_TRP_TYPE " unknown type \"%s\"", json_object_get_string(json_object_iter_peek_value(&it)));
                goto exit;
            }
            json_object_iter_next(&it);
            continue;
        }

        if (!strcmp(json_object_iter_peek_name(&it), JSON_KEY_TRP_ENDPOINT)) {
            PARSE_ENV_JSON_CHK_TYPE(&it, json_type_string, JSON_KEY_TRP_ENDPOINT)
            if (trp->endpoint) {
                netdata_log_error("ACLK: " JSON_KEY_TRP_ENDPOINT " set already");
                goto exit;
            }
            trp->endpoint = strdupz(json_object_get_string(json_object_iter_peek_value(&it)));
            json_object_iter_next(&it);
            continue;
        }

        netdata_log_error("ACLK: unknown JSON key in dictionary (\"%s\")", json_object_iter_peek_name(&it));
        json_object_iter_next(&it);
    }

    if (!trp->endpoint) {
        netdata_log_error("ACLK: " JSON_KEY_TRP_ENDPOINT " is missing from JSON dictionary");
        goto exit;
    }

    if (trp->type == ACLK_TRP_UNKNOWN) {
        netdata_log_error("ACLK: transport type not set");
        goto exit;
    }

    return 0;

exit:
    aclk_transport_desc_t_destroy(trp);
    return 1;
}

static int parse_json_env_transports(json_object *json_array, aclk_env_t *env) {
    aclk_transport_desc_t *trp;
    json_object *obj;

    if (env->transports) {
        netdata_log_error("ACLK: transports have been set already");
        return 1;
    }

    env->transport_count = json_object_array_length(json_array);

    env->transports = callocz(env->transport_count , sizeof(aclk_transport_desc_t *));

    for (size_t i = 0; i < env->transport_count; i++) {
        trp = callocz(1, sizeof(aclk_transport_desc_t));
        obj = json_object_array_get_idx(json_array, i);
        if (parse_json_env_transport(obj, trp)) {
            netdata_log_error("ACLK: error parsing transport idx %d", (int)i);
            freez(trp);
            return 1;
        }
        env->transports[i] = trp;
    }

    return 0;
}

#define MATCHED_CORRECT  1
#define MATCHED_ERROR   -1
#define NOT_MATCHED      0
static int parse_json_backoff_int(struct json_object_iterator *it, int *out, const char* name, int min, int max) {
    if (!strcmp(json_object_iter_peek_name(it), name)) {
        if (json_object_get_type(json_object_iter_peek_value(it)) != json_type_int) {
            netdata_log_error("ACLK: Could not parse \"%s\". Not an integer as expected.", name);
            return MATCHED_ERROR;
        }

        *out = json_object_get_int(json_object_iter_peek_value(it));

        if (*out < min || *out > max) {
            netdata_log_error("ACLK: Value of \"%s\"=%d out of range (%d-%d).", name, *out, min, max);
            return MATCHED_ERROR;
        }

        return MATCHED_CORRECT;
    }
    return NOT_MATCHED;
}

static int parse_json_backoff(json_object *json, aclk_backoff_t *backoff) {
    struct json_object_iterator it;
    struct json_object_iterator itEnd;
    int ret;

    it = json_object_iter_begin(json);
    itEnd = json_object_iter_end(json);

    while (!json_object_iter_equal(&it, &itEnd)) {
        if ( (ret = parse_json_backoff_int(&it, &backoff->base, JSON_KEY_BACKOFF_BASE, 1, 10)) ) {
            if (ret == MATCHED_ERROR) {
                return 1;
            }
            json_object_iter_next(&it);
            continue;
        }

        if ( (ret = parse_json_backoff_int(&it, &backoff->max_s, JSON_KEY_BACKOFF_MAX, 500, INT_MAX)) ) {
            if (ret == MATCHED_ERROR) {
                return 1;
            }
            json_object_iter_next(&it);
            continue;
        }

        if ( (ret = parse_json_backoff_int(&it, &backoff->min_s, JSON_KEY_BACKOFF_MIN, 0, INT_MAX)) ) {
            if (ret == MATCHED_ERROR) {
                return 1;
            }
            json_object_iter_next(&it);
            continue;
        }

        netdata_log_error("ACLK: unknown JSON key in dictionary (\"%s\")", json_object_iter_peek_name(&it));
        json_object_iter_next(&it);
    }

    return 0;
}

static int parse_json_env_caps(json_object *json, aclk_env_t *env) {
    json_object *obj;
    const char *str;

    if (env->capabilities) {
        netdata_log_error("ACLK: transports have been set already");
        return 1;
    }

    env->capability_count = json_object_array_length(json);

    // empty capabilities list is allowed
    if (!env->capability_count)
        return 0;

    env->capabilities = callocz(env->capability_count , sizeof(char *));

    for (size_t i = 0; i < env->capability_count; i++) {
        obj = json_object_array_get_idx(json, i);
        if (json_object_get_type(obj) != json_type_string) {
            netdata_log_error("ACLK: Capability at index %d not a string!", (int)i);
            return 1;
        }
        str = json_object_get_string(obj);
        if (!str) {
            netdata_log_error("ACLK: Error parsing capabilities");
            return 1;
        }
        env->capabilities[i] = strdupz(str);
    }

    return 0;
}

static int parse_json_env(const char *json_str, aclk_env_t *env) {
    json_object *json;
    struct json_object_iterator it;
    struct json_object_iterator itEnd;

    json = json_tokener_parse(json_str);
    if (!json) {
        netdata_log_error("ACLK: JSON-C failed to parse the payload of http response of /env endpoint");
        return 1;
    }

    it = json_object_iter_begin(json);
    itEnd = json_object_iter_end(json);

    while (!json_object_iter_equal(&it, &itEnd)) {
        if (!strcmp(json_object_iter_peek_name(&it), JSON_KEY_AUTH_ENDPOINT)) {
            PARSE_ENV_JSON_CHK_TYPE(&it, json_type_string, JSON_KEY_AUTH_ENDPOINT)
            if (env->auth_endpoint) {
                netdata_log_error("ACLK: authEndpoint set already");
                goto exit;
            }
            env->auth_endpoint = strdupz(json_object_get_string(json_object_iter_peek_value(&it)));
            json_object_iter_next(&it);
            continue;
        }

        if (!strcmp(json_object_iter_peek_name(&it), JSON_KEY_ENC)) {
            PARSE_ENV_JSON_CHK_TYPE(&it, json_type_string, JSON_KEY_ENC)
            if (env->encoding != ACLK_ENC_UNKNOWN) {
                netdata_log_error("ACLK: " JSON_KEY_ENC " set already");
                goto exit;
            }
            env->encoding = aclk_encoding_type_t_from_str(json_object_get_string(json_object_iter_peek_value(&it)));
            json_object_iter_next(&it);
            continue;
        }

        if (!strcmp(json_object_iter_peek_name(&it), JSON_KEY_TRP)) {
            PARSE_ENV_JSON_CHK_TYPE(&it, json_type_array, JSON_KEY_TRP)

            json_object *now = json_object_iter_peek_value(&it);
            parse_json_env_transports(now, env);

            json_object_iter_next(&it);
            continue;
        }

        if (!strcmp(json_object_iter_peek_name(&it), JSON_KEY_BACKOFF)) {
            PARSE_ENV_JSON_CHK_TYPE(&it, json_type_object, JSON_KEY_BACKOFF)

            if (parse_json_backoff(json_object_iter_peek_value(&it), &env->backoff)) {
                env->backoff.base = 0;
                netdata_log_error("ACLK: Error parsing Backoff parameters in env");
                goto exit;
            }

            json_object_iter_next(&it);
            continue;
        }

        if (!strcmp(json_object_iter_peek_name(&it), JSON_KEY_CAPS)) {
            PARSE_ENV_JSON_CHK_TYPE(&it, json_type_array, JSON_KEY_CAPS)

            if (parse_json_env_caps(json_object_iter_peek_value(&it), env)) {
                netdata_log_error("ACLK: Error parsing capabilities list");
                goto exit;
            }

            json_object_iter_next(&it);
            continue;
        }

        netdata_log_error("ACLK: unknown JSON key in dictionary (\"%s\")", json_object_iter_peek_name(&it));
        json_object_iter_next(&it);
    }

    // Check all compulsory keys have been set
    if (env->transport_count < 1) {
        netdata_log_error("ACLK: env has to return at least one transport");
        goto exit;
    }
    if (!env->auth_endpoint) {
        netdata_log_error("ACLK: " JSON_KEY_AUTH_ENDPOINT " is compulsory");
        goto exit;
    }
    if (env->encoding == ACLK_ENC_UNKNOWN) {
        netdata_log_error("ACLK: " JSON_KEY_ENC " is compulsory");
        goto exit;
    }
    if (!env->backoff.base) {
        netdata_log_error("ACLK: " JSON_KEY_BACKOFF " is compulsory");
        goto exit;
    }

    json_object_put(json);
    return 0;

exit:
    json_object_put(json);
    return 1;
}

https_client_resp_t aclk_get_env(aclk_env_t *env, const char* aclk_hostname, int aclk_port, bool *fallback_ipv4) {
    BUFFER *buf = buffer_create(1024, &netdata_buffers_statistics.buffers_aclk);

    https_client_resp_t rc;
    https_req_t req = HTTPS_REQ_T_INITIALIZER;
    https_req_response_t resp = HTTPS_REQ_RESPONSE_T_INITIALIZER;

    req.request_type = HTTP_REQ_GET;

    CLAIM_ID claim_id = claim_id_get();
    if (!claim_id_is_set(claim_id)) {
        netdata_log_error("ACLK: failed to get ACLK environment (agent is not claimed)");
        buffer_free(buf);
        return HTTPS_CLIENT_RESP_ENV_AGENT_NOT_CLAIMED;
    }

    buffer_sprintf(buf, "/api/v1/env?v=%s&cap=proto,ctx&claim_id=%s", &(NETDATA_VERSION[1]) /* skip 'v' at beginning */, claim_id.str);

    req.host = (char*)aclk_hostname;
    req.port = aclk_port;
    req.url = buf->buffer;
    rc = aclk_https_request(&req, &resp, fallback_ipv4);
    if (rc != HTTPS_CLIENT_RESP_OK) {
        netdata_log_error("ACLK: failed to get ACLK environment (cannot contact ENV endpoint)");
        https_req_response_free(&resp);
        buffer_free(buf);
        return rc;
    }
    if (resp.http_code != 200) {
        netdata_log_error("ACLK: failed to get ACLK environment (ENV response code is not 200) (got %d)", resp.http_code);
        if (resp.payload_size)
            aclk_parse_otp_error(resp.payload);
        https_req_response_free(&resp);
        buffer_free(buf);
        return HTTPS_CLIENT_RESP_ENV_NOT_200;
    }

    if (!resp.payload || !resp.payload_size) {
        netdata_log_error("ACLK: failed to get ACLK environment (ENV response is empty)");
        https_req_response_free(&resp);
        buffer_free(buf);
        return HTTPS_CLIENT_RESP_ENV_EMPTY;
    }

    if (parse_json_env(resp.payload, env)) {
        netdata_log_error("ACLK: failed to get ACLK environment (ENV response is not JSON)");
        https_req_response_free(&resp);
        buffer_free(buf);
        return HTTPS_CLIENT_RESP_ENV_NOT_JSON;
    }

    https_req_response_free(&resp);
    buffer_free(buf);
    return HTTPS_CLIENT_RESP_OK;
}
