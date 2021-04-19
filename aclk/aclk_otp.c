
// SPDX-License-Identifier: GPL-3.0-or-later

#include "aclk_otp.h"

#include "https_client.h"

#include "../daemon/common.h"

#include "../mqtt_websockets/c-rbuf/include/ringbuffer.h"

// CentOS 7 has older version that doesn't define this
// same goes for MacOS
#ifndef UUID_STR_LEN
#define UUID_STR_LEN 37
#endif

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

#define OTP_URL_PREFIX "/api/v1/auth/node/"
void aclk_get_mqtt_otp(RSA *p_key, char *aclk_hostname, int port, char **mqtt_usr, char **mqtt_pass) {
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
    req.host = aclk_hostname;
    req.port = port;
    buffer_sprintf(url, "%s%s/challenge", OTP_URL_PREFIX, agent_id);
    req.url = url->buffer;

    if (aclk_https_request(&req, &resp)) {
        error ("ACLK_OTP Challenge failed");
        goto cleanup;
    }
    if (resp.http_code != 200) {
        error ("ACLK_OTP Challenge HTTP code not 200 OK (got %d)", resp.http_code);
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
    buffer_sprintf(url, "%s%s/password", OTP_URL_PREFIX, agent_id);
    req.url = url->buffer;
    req.payload = response_json;
    req.payload_size = strlen(response_json);

    if (aclk_https_request(&req, &resp)) {
        error ("ACLK_OTP Password error trying to post result to password");
        goto cleanup;
    }
    if (resp.http_code != 201) {
        error ("ACLK_OTP Password HTTP code not 201 Created (got %d)", resp.http_code);
        goto cleanup_resp;
    }
    info ("ACLK_OTP Got Password from Cloud");

    struct dictionary_singleton password = { .key = "password", .result = NULL };
    if (json_parse(resp.payload, &password, json_extract_singleton) != JSON_OK)
    {
        freez(password.result);
        error("Could not parse the json response with the password");
        goto cleanup_resp;
    }

    if (password.result == NULL ) {
        error("Could not retrieve password from auth response");
        goto cleanup_resp;
    }
    if (*mqtt_pass != NULL )
        freez(*mqtt_pass);
    *mqtt_pass = password.result;
    if (*mqtt_usr != NULL)
        freez(*mqtt_usr);
    *mqtt_usr = agent_id;
    agent_id = NULL;

cleanup_resp:
    https_req_response_free(&resp);
cleanup:
    if (agent_id != NULL)
        freez(agent_id);
    buffer_free(url);
}
