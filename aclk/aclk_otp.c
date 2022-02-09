
// SPDX-License-Identifier: GPL-3.0-or-later

#include "aclk_otp.h"
#include "aclk_util.h"
#include "aclk.h"

#include "daemon/common.h"

#include "mqtt_websockets/c-rbuf/include/ringbuffer.h"

struct dictionary_singleton {
    char *key;
    char *result;
};

static int json_extract_singleton(JSON_ENTRY *e)
{
    struct dictionary_singleton *data = e->callback_data;

    switch (e->type) {
        case JSON_OBJECT:
        case JSON_ARRAY:
            break;
        case JSON_STRING:
            if (!strcmp(e->name, data->key)) {
                data->result = strdupz(e->data.string);
                break;
            }
            break;
        case JSON_NUMBER:
        case JSON_BOOLEAN:
        case JSON_NULL:
            break;
    }
    return 0;
}

// Base-64 decoder.
// Note: This is non-validating, invalid input will be decoded without an error.
//       Challenges are packed into json strings so we don't skip newlines.
//       Size errors (i.e. invalid input size or insufficient output space) are caught.
static size_t base64_decode(unsigned char *input, size_t input_size, unsigned char *output, size_t output_size)
{
    static char lookup[256];
    static int first_time=1;
    if (first_time)
    {
        first_time = 0;
        for(int i=0; i<256; i++)
            lookup[i] = -1;
        for(int i='A'; i<='Z'; i++)
            lookup[i] = i-'A';
        for(int i='a'; i<='z'; i++)
            lookup[i] = i-'a' + 26;
        for(int i='0'; i<='9'; i++)
            lookup[i] = i-'0' + 52;
        lookup['+'] = 62;
        lookup['/'] = 63;
    }
    if ((input_size & 3) != 0)
    {
        error("Can't decode base-64 input length %zu", input_size);
        return 0;
    }
    size_t unpadded_size = (input_size/4) * 3;
    if ( unpadded_size > output_size )
    {
        error("Output buffer size %zu is too small to decode %zu into", output_size, input_size);
        return 0;
    }
    // Don't check padding within full quantums
    for (size_t i = 0 ; i < input_size-4 ; i+=4 )
    {
        uint32_t value = (lookup[input[0]] << 18) + (lookup[input[1]] << 12) + (lookup[input[2]] << 6) + lookup[input[3]];
        output[0] = value >> 16;
        output[1] = value >> 8;
        output[2] = value;
        //error("Decoded %c %c %c %c -> %02x %02x %02x", input[0], input[1], input[2], input[3], output[0], output[1], output[2]);
        output += 3;
        input += 4;
    }
    // Handle padding only in last quantum
    if (input[2] == '=') {
        uint32_t value = (lookup[input[0]] << 6) + lookup[input[1]];
        output[0] = value >> 4;
        //error("Decoded %c %c %c %c -> %02x", input[0], input[1], input[2], input[3], output[0]);
        return unpadded_size-2;
    }
    else if (input[3] == '=') {
        uint32_t value = (lookup[input[0]] << 12) + (lookup[input[1]] << 6) + lookup[input[2]];
        output[0] = value >> 10;
        output[1] = value >> 2;
        //error("Decoded %c %c %c %c -> %02x %02x", input[0], input[1], input[2], input[3], output[0], output[1]);
        return unpadded_size-1;
    }
    else
    {
        uint32_t value = (input[0] << 18) + (input[1] << 12) + (input[2]<<6) + input[3];
        output[0] = value >> 16;
        output[1] = value >> 8;
        output[2] = value;
        //error("Decoded %c %c %c %c -> %02x %02x %02x", input[0], input[1], input[2], input[3], output[0], output[1], output[2]);
        return unpadded_size;
    }
}

static size_t base64_encode(unsigned char *input, size_t input_size, char *output, size_t output_size)
{
    uint32_t value;
    static char lookup[] = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
                           "abcdefghijklmnopqrstuvwxyz"
                           "0123456789+/";
    if ((input_size/3+1)*4 >= output_size)
    {
        error("Output buffer for encoding size=%zu is not large enough for %zu-bytes input", output_size, input_size);
        return 0;
    }
    size_t count = 0;
    while (input_size>3)
    {
        value = ((input[0] << 16) + (input[1] << 8) + input[2]) & 0xffffff;
        output[0] = lookup[value >> 18];
        output[1] = lookup[(value >> 12) & 0x3f];
        output[2] = lookup[(value >> 6) & 0x3f];
        output[3] = lookup[value & 0x3f];
        //error("Base-64 encode (%04x) -> %c %c %c %c\n", value, output[0], output[1], output[2], output[3]);
        output += 4;
        input += 3;
        input_size -= 3;
        count += 4;
    }
    switch (input_size)
    {
        case 2:
            value = (input[0] << 10) + (input[1] << 2);
            output[0] = lookup[(value >> 12) & 0x3f];
            output[1] = lookup[(value >> 6) & 0x3f];
            output[2] = lookup[value & 0x3f];
            output[3] = '=';
            //error("Base-64 encode (%06x) -> %c %c %c %c\n", (value>>2)&0xffff, output[0], output[1], output[2], output[3]); 
            count += 4;
            break;
        case 1:
            value = input[0] << 4;
            output[0] = lookup[(value >> 6) & 0x3f];
            output[1] = lookup[value & 0x3f];
            output[2] = '=';
            output[3] = '=';
            //error("Base-64 encode (%06x) -> %c %c %c %c\n", value, output[0], output[1], output[2], output[3]); 
            count += 4;
            break;
        case 0:
            break;
    }
    return count;
}

static int private_decrypt(RSA *p_key, unsigned char * enc_data, int data_len, unsigned char *decrypted)
{
    int  result = RSA_private_decrypt( data_len, enc_data, decrypted, p_key, RSA_PKCS1_OAEP_PADDING);
    if (result == -1) {
        char err[512];
        ERR_error_string_n(ERR_get_error(), err, sizeof(err));
        error("Decryption of the challenge failed: %s", err);
    }
    return result;
}

static int aclk_https_request(https_req_t *request, https_req_response_t *response) {
    int rc;
    // wrapper for ACLK only which loads ACLK specific proxy settings
    // then only calls https_request
    struct mqtt_wss_proxy proxy_conf = { .host = NULL, .port = 0, .type = MQTT_WSS_DIRECT };
    aclk_set_proxy((char**)&proxy_conf.host, &proxy_conf.port, &proxy_conf.type);

    if (proxy_conf.type == MQTT_WSS_PROXY_HTTP) {
        request->proxy_host = (char*)proxy_conf.host; // TODO make it const as well
        request->proxy_port = proxy_conf.port;
    }

    rc = https_request(request, response);
    freez((char*)proxy_conf.host);
    return rc;
}

struct auth_data {
    char *client_id;
    char *username;
    char *passwd;
};

#define PARSE_ENV_JSON_CHK_TYPE(it, type, name)                                                                        \
    if (json_object_get_type(json_object_iter_peek_value(it)) != type) {                                               \
        error("value of key \"%s\" should be %s", name, #type);                                                        \
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
        error("JSON-C failed to parse the payload of http response of /env endpoint");
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
                error("Failed to generate topic cache!");
                goto exit;
            }
            json_object_iter_next(&it);
            continue;
        }
        error("Unknown key \"%s\" in passwd response payload. Ignoring", json_object_iter_peek_name(&it));
        json_object_iter_next(&it);
    }

    if (!auth->client_id) {
        error(JSON_KEY_CLIENTID " is compulsory key in /password response");
        goto exit;
    }
    if (!auth->passwd) {
        error(JSON_KEY_PASS " is compulsory in /password response");
        goto exit;
    }
    if (!auth->username) {
        error(JSON_KEY_USER " is compulsory in /password response");
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
        error("Missing compulsory key \"%s\" in error response", path);
        return NULL;
    }
    if (json_object_get_type(ptr) != json_type_string) {
        error("Value of Key \"%s\" in error response should be string", path);
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
        error("JSON-C failed to parse the payload of http response of /env endpoint");
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
            error("Error response Key " "/" JSON_KEY_ERTRY " should be of boolean type");
            goto exit;
        }
        block_retry = json_object_get_boolean(ptr);
    }

    // optional field
    if (!json_pointer_get(json, "/" JSON_KEY_EDELAY, &ptr)) {
        if (json_object_get_type(ptr) != json_type_int) {
            error("Error response Key " "/" JSON_KEY_EDELAY " should be of integer type");
            goto exit;
        }
        backoff = json_object_get_int(ptr);
    }

    if (block_retry > 0)
        aclk_disable_runtime = 1;

    if (backoff > 0)
        aclk_block_until = now_monotonic_sec() + backoff;

    error("Cloud returned EC=\"%s\", Msg-Key:\"%s\", Msg:\"%s\", BlockRetry:%s, Backoff:%ds (-1 unset by cloud)", ec, ek, emsg, block_retry > 0 ? "true" : "false", backoff);
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
        error("JSON-C failed to parse the payload of http response of /env endpoint");
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
                error("value of key " JSON_KEY_EDELAY " should be integer");
                goto exit;
            }

            backoff = json_object_get_int(json_object_iter_peek_value(&it));
            json_object_iter_next(&it);
            continue;
        }
        if (!strcmp(json_object_iter_peek_name(&it), JSON_KEY_ERTRY)) {
            if (json_object_get_type(json_object_iter_peek_value(&it)) != json_type_boolean) {
                error("value of key " JSON_KEY_ERTRY " should be integer");
                goto exit;
            }

            block_retry = json_object_get_boolean(json_object_iter_peek_value(&it));
            json_object_iter_next(&it);
            continue;
        }
        error("Unknown key \"%s\" in error response payload. Ignoring", json_object_iter_peek_name(&it));
        json_object_iter_next(&it);
    }

    if (block_retry > 0)
        aclk_disable_runtime = 1;

    if (backoff > 0)
        aclk_block_until = now_monotonic_sec() + backoff;

    error("Cloud returned EC=\"%s\", Msg-Key:\"%s\", Msg:\"%s\", BlockRetry:%s, Backoff:%ds (-1 unset by cloud)", ec, ek, emsg, block_retry > 0 ? "true" : "false", backoff);
    rc = 0;
exit:
    json_object_put(json);
    return rc;
}
#endif

#define OTP_URL_PREFIX "/api/v1/auth/node/"
int aclk_get_mqtt_otp(RSA *p_key, char **mqtt_id, char **mqtt_usr, char **mqtt_pass, url_t *target) {
    // TODO this fnc will be rewritten and simplified in following PRs
    // still carries lot of baggage from ACLK Legacy
    int rc = 1;
    BUFFER *url = buffer_create(strlen(OTP_URL_PREFIX) + UUID_STR_LEN + 20);

    https_req_t req = HTTPS_REQ_T_INITIALIZER;
    https_req_response_t resp = HTTPS_REQ_RESPONSE_T_INITIALIZER;

    char *agent_id = is_agent_claimed();
    if (agent_id == NULL)
    {
        error("Agent was not claimed - cannot perform challenge/response");
        goto cleanup;
    }

    // GET Challenge
    req.host = target->host;
    req.port = target->port;
    buffer_sprintf(url, "%s/node/%s/challenge", target->path, agent_id);
    req.url = url->buffer;

    if (aclk_https_request(&req, &resp)) {
        error ("ACLK_OTP Challenge failed");
        goto cleanup;
    }
    if (resp.http_code != 200) {
        error ("ACLK_OTP Challenge HTTP code not 200 OK (got %d)", resp.http_code);
        if (resp.payload_size)
            aclk_parse_otp_error(resp.payload);
        goto cleanup_resp;
    }
    info ("ACLK_OTP Got Challenge from Cloud");

    struct dictionary_singleton challenge = { .key = "challenge", .result = NULL };

    if (json_parse(resp.payload, &challenge, json_extract_singleton) != JSON_OK)
    {
        freez(challenge.result);
        error("Could not parse the the challenge");
        goto cleanup_resp;
    }
    if (challenge.result == NULL) {
        error("Could not retrieve challenge JSON key from challenge response");
        goto cleanup_resp;
    }

    // Decrypt the Challenge and Calculate Response
    size_t challenge_len = strlen(challenge.result);
    unsigned char decoded[512];
    size_t decoded_len = base64_decode((unsigned char*)challenge.result, challenge_len, decoded, sizeof(decoded));
    freez(challenge.result);

    unsigned char plaintext[4096]={};
    int decrypted_length = private_decrypt(p_key, decoded, decoded_len, plaintext);
    char encoded[512];
    size_t encoded_len = base64_encode(plaintext, decrypted_length, encoded, sizeof(encoded));
    encoded[encoded_len] = 0;
    debug(D_ACLK, "Encoded len=%zu Decryption len=%d: '%s'", encoded_len, decrypted_length, encoded);

    char response_json[4096]={};
    sprintf(response_json, "{\"response\":\"%s\"}", encoded);
    debug(D_ACLK, "Password phase: %s",response_json);

    https_req_response_free(&resp);
    https_req_response_init(&resp);

    // POST password
    req.request_type = HTTP_REQ_POST;
    buffer_flush(url);
    buffer_sprintf(url, "%s/node/%s/password", target->path, agent_id);
    req.url = url->buffer;
    req.payload = response_json;
    req.payload_size = strlen(response_json);

    if (aclk_https_request(&req, &resp)) {
        error ("ACLK_OTP Password error trying to post result to password");
        goto cleanup;
    }
    if (resp.http_code != 201) {
        error ("ACLK_OTP Password HTTP code not 201 Created (got %d)", resp.http_code);
        if (resp.payload_size)
            aclk_parse_otp_error(resp.payload);
        goto cleanup_resp;
    }
    info ("ACLK_OTP Got Password from Cloud");

    struct auth_data data = { .client_id = NULL, .passwd = NULL, .username = NULL };
    
    if (parse_passwd_response(resp.payload, &data)){
        error("Error parsing response of password endpoint");
        goto cleanup_resp;
    }

    *mqtt_pass = data.passwd;
    *mqtt_usr = data.username;
    *mqtt_id = data.client_id;

    rc = 0;
cleanup_resp:
    https_req_response_free(&resp);
cleanup:
    freez(agent_id);
    buffer_free(url);
    return rc;
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
                error(JSON_KEY_TRP_TYPE " set already");
                goto exit;
            }
            trp->type = aclk_transport_type_t_from_str(json_object_get_string(json_object_iter_peek_value(&it)));
            if (trp->type == ACLK_TRP_UNKNOWN) {
                error(JSON_KEY_TRP_TYPE " unknown type \"%s\"", json_object_get_string(json_object_iter_peek_value(&it)));
                goto exit;
            }
            json_object_iter_next(&it);
            continue;
        }

        if (!strcmp(json_object_iter_peek_name(&it), JSON_KEY_TRP_ENDPOINT)) {
            PARSE_ENV_JSON_CHK_TYPE(&it, json_type_string, JSON_KEY_TRP_ENDPOINT)
            if (trp->endpoint) {
                error(JSON_KEY_TRP_ENDPOINT " set already");
                goto exit;
            }
            trp->endpoint = strdupz(json_object_get_string(json_object_iter_peek_value(&it)));
            json_object_iter_next(&it);
            continue;
        }
        
        error ("unknown JSON key in dictionary (\"%s\")", json_object_iter_peek_name(&it));
        json_object_iter_next(&it);
    }

    if (!trp->endpoint) {
        error (JSON_KEY_TRP_ENDPOINT " is missing from JSON dictionary");
        goto exit;
    }

    if (trp->type == ACLK_TRP_UNKNOWN) {
        error ("transport type not set");
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
        error("transports have been set already");
        return 1;
    }

    env->transport_count = json_object_array_length(json_array);

    env->transports = callocz(env->transport_count , sizeof(aclk_transport_desc_t *));

    for (size_t i = 0; i < env->transport_count; i++) {
        trp = callocz(1, sizeof(aclk_transport_desc_t));
        obj = json_object_array_get_idx(json_array, i);
        if (parse_json_env_transport(obj, trp)) {
            error("error parsing transport idx %d", (int)i);
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
            error("Could not parse \"%s\". Not an integer as expected.", name);
            return MATCHED_ERROR;
        }

        *out = json_object_get_int(json_object_iter_peek_value(it));

        if (*out < min || *out > max) {
            error("Value of \"%s\"=%d out of range (%d-%d).", name, *out, min, max);
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

        error ("unknown JSON key in dictionary (\"%s\")", json_object_iter_peek_name(&it));
        json_object_iter_next(&it);
    }

    return 0;
}

static int parse_json_env_caps(json_object *json, aclk_env_t *env) {
    json_object *obj;
    const char *str;

    if (env->capabilities) {
        error("transports have been set already");
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
            error("Capability at index %d not a string!", (int)i);
            return 1;
        }
        str = json_object_get_string(obj);
        if (!str) {
            error("Error parsing capabilities");
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
        error("JSON-C failed to parse the payload of http response of /env endpoint");
        return 1;
    }

    it = json_object_iter_begin(json);
    itEnd = json_object_iter_end(json);

    while (!json_object_iter_equal(&it, &itEnd)) {
        if (!strcmp(json_object_iter_peek_name(&it), JSON_KEY_AUTH_ENDPOINT)) {
            PARSE_ENV_JSON_CHK_TYPE(&it, json_type_string, JSON_KEY_AUTH_ENDPOINT)
            if (env->auth_endpoint) {
                error("authEndpoint set already");
                goto exit;
            }
            env->auth_endpoint = strdupz(json_object_get_string(json_object_iter_peek_value(&it)));
            json_object_iter_next(&it);
            continue;
        }

        if (!strcmp(json_object_iter_peek_name(&it), JSON_KEY_ENC)) {
            PARSE_ENV_JSON_CHK_TYPE(&it, json_type_string, JSON_KEY_ENC)
            if (env->encoding != ACLK_ENC_UNKNOWN) {
                error(JSON_KEY_ENC " set already");
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
                error("Error parsing Backoff parameters in env");
                goto exit;
            }

            json_object_iter_next(&it);
            continue;
        }

        if (!strcmp(json_object_iter_peek_name(&it), JSON_KEY_CAPS)) {
            PARSE_ENV_JSON_CHK_TYPE(&it, json_type_array, JSON_KEY_CAPS)

            if (parse_json_env_caps(json_object_iter_peek_value(&it), env)) {
                error("Error parsing capabilities list");
                goto exit;
            }

            json_object_iter_next(&it);
            continue;
        }

        error ("unknown JSON key in dictionary (\"%s\")", json_object_iter_peek_name(&it));
        json_object_iter_next(&it);
    }

    // Check all compulsory keys have been set
    if (env->transport_count < 1) {
        error("env has to return at least one transport");
        goto exit;
    }
    if (!env->auth_endpoint) {
        error(JSON_KEY_AUTH_ENDPOINT " is compulsory");
        goto exit;
    }
    if (env->encoding == ACLK_ENC_UNKNOWN) {
        error(JSON_KEY_ENC " is compulsory");
        goto exit;
    }
    if (!env->backoff.base) {
        error(JSON_KEY_BACKOFF " is compulsory");
        goto exit;
    }

    json_object_put(json);
    return 0;

exit:
    aclk_env_t_destroy(env);
    json_object_put(json);
    return 1;
}

int aclk_get_env(aclk_env_t *env, const char* aclk_hostname, int aclk_port) {
    BUFFER *buf = buffer_create(1024);

    https_req_t req = HTTPS_REQ_T_INITIALIZER;
    https_req_response_t resp = HTTPS_REQ_RESPONSE_T_INITIALIZER;

    req.request_type = HTTP_REQ_GET;

    char *agent_id = is_agent_claimed();
    if (agent_id == NULL)
    {
        error("Agent was not claimed - cannot perform challenge/response");
        buffer_free(buf);
        return 1;
    }

    buffer_sprintf(buf, "/api/v1/env?v=%s&cap=proto&claim_id=%s", &(VERSION[1]) /* skip 'v' at beginning */, agent_id);
    freez(agent_id);

    req.host = (char*)aclk_hostname;
    req.port = aclk_port;
    req.url = buf->buffer;
    if (aclk_https_request(&req, &resp)) {
        error("Error trying to contact env endpoint");
        https_req_response_free(&resp);
        buffer_free(buf);
        return 1;
    }
    if (resp.http_code != 200) {
        error("The HTTP code not 200 OK (Got %d)", resp.http_code);
        https_req_response_free(&resp);
        buffer_free(buf);
        return 1;
    }

    if (!resp.payload || !resp.payload_size) {
        error("Unexpected empty payload as response to /env call");
        https_req_response_free(&resp);
        buffer_free(buf);
        return 1;
    }

    if (parse_json_env(resp.payload, env)) {
        error ("error parsing /env message");
        https_req_response_free(&resp);
        buffer_free(buf);
        return 1;
    }

    info("Getting Cloud /env successful");

    https_req_response_free(&resp);
    buffer_free(buf);
    return 0;
}
