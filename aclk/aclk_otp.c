
// SPDX-License-Identifier: GPL-3.0-or-later

#include "aclk_otp.h"

#include "libnetdata/libnetdata.h"

#include "../daemon/common.h"

#include "../mqtt_websockets/c-rbuf/include/ringbuffer.h"

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

typedef enum http_req_type {
    HTTP_REQ_GET,
    HTTP_REQ_POST
} http_req_type;

enum http_parse_state {
    HTTP_PARSE_INITIAL = 0,
    HTTP_PARSE_HEADERS,
    HTTP_PARSE_CONTENT
};

typedef struct {
    enum http_parse_state state;
    int content_length;
    int http_code;
} http_parse_ctx;

#define HTTP_PARSE_CTX_INITIALIZER { .state = HTTP_PARSE_INITIAL, .content_length = -1, .http_code = 0 }

#define NEED_MORE_DATA  0
#define PARSE_SUCCESS   1
#define PARSE_ERROR    -1
#define HTTP_LINE_TERM "\x0D\x0A"
#define RESP_PROTO "HTTP/1.1 "
#define HTTP_KEYVAL_SEPARATOR ": "
#define HTTP_HDR_BUFFER_SIZE 256
#define PORT_STR_MAX_BYTES 7

static void process_http_hdr(http_parse_ctx *parse_ctx, const char *key, const char *val)
{
    // currently we care only about content-length
    // but in future the way this is written
    // it can be extended
    if (!strcmp("content-length", key)) {
        parse_ctx->content_length = atoi(val);
    }
}

static int parse_http_hdr(rbuf_t buf, http_parse_ctx *parse_ctx)
{
    int idx, idx_end;
    char buf_key[HTTP_HDR_BUFFER_SIZE];
    char buf_val[HTTP_HDR_BUFFER_SIZE];
    char *ptr = buf_key;
    if (!rbuf_find_bytes(buf, HTTP_LINE_TERM, strlen(HTTP_LINE_TERM), &idx_end)) {
        error("CRLF expected");
        return 1;
    }

    char *separator = rbuf_find_bytes(buf, HTTP_KEYVAL_SEPARATOR, strlen(HTTP_KEYVAL_SEPARATOR), &idx);
    if (!separator) {
        error("Missing Key/Value separator");
        return 1;
    }
    if (idx >= HTTP_HDR_BUFFER_SIZE) {
        error("Key name is too long");
        return 1;
    }

    rbuf_pop(buf, buf_key, idx);
    buf_key[idx] = 0;

    rbuf_bump_tail(buf, strlen(HTTP_KEYVAL_SEPARATOR));
    idx_end -= strlen(HTTP_KEYVAL_SEPARATOR) + idx;
    if (idx_end >= HTTP_HDR_BUFFER_SIZE) {
        error("Value of key \"%s\" too long", buf_key);
        return 1;
    }

    rbuf_pop(buf, buf_val, idx_end);
    buf_val[idx_end] = 0;

    rbuf_bump_tail(buf, strlen(HTTP_KEYVAL_SEPARATOR));

    for (ptr = buf_key; *ptr; ptr++)
        *ptr = tolower(*ptr);

    process_http_hdr(parse_ctx, buf_key, buf_val);

    return 0;
}

static int parse_http_response(rbuf_t buf, http_parse_ctx *parse_ctx)
{
    int idx;
    char rc[4];

    do {
        if (parse_ctx->state != HTTP_PARSE_CONTENT && !rbuf_find_bytes(buf, HTTP_LINE_TERM, strlen(HTTP_LINE_TERM), &idx))
            return NEED_MORE_DATA;
        switch (parse_ctx->state) {
            case HTTP_PARSE_INITIAL:
                if (rbuf_memcmp_n(buf, RESP_PROTO, strlen(RESP_PROTO))) {
                    error("Expected response to start with \"%s\"", RESP_PROTO);
                    return PARSE_ERROR;
                }
                rbuf_bump_tail(buf, strlen(RESP_PROTO));
                if (rbuf_pop(buf, rc, 4) != 4) {
                    error("Expected HTTP status code");
                    return PARSE_ERROR;
                }
                if (rc[3] != ' ') {
                    error("Expected space after HTTP return code");
                    return PARSE_ERROR;
                }
                rc[3] = 0;
                parse_ctx->http_code = atoi(rc);
                if (parse_ctx->http_code < 100 || parse_ctx->http_code >= 600) {
                    error("HTTP code not in range 100 to 599");
                    return PARSE_ERROR;
                }

                rbuf_find_bytes(buf, HTTP_LINE_TERM, strlen(HTTP_LINE_TERM), &idx);

                rbuf_bump_tail(buf, idx + strlen(HTTP_LINE_TERM));

                parse_ctx->state = HTTP_PARSE_HEADERS;
                break;
            case HTTP_PARSE_HEADERS:
                if (!idx) {
                    parse_ctx->state = HTTP_PARSE_CONTENT;
                    rbuf_bump_tail(buf, strlen(HTTP_LINE_TERM));
                    break;
                }
                if (parse_http_hdr(buf, parse_ctx))
                    return PARSE_ERROR;
                rbuf_find_bytes(buf, HTTP_LINE_TERM, strlen(HTTP_LINE_TERM), &idx);
                rbuf_bump_tail(buf, idx + strlen(HTTP_LINE_TERM));
                break;
            case HTTP_PARSE_CONTENT:
                if (parse_ctx->content_length < 0) {
                    error("content-length missing and http headers ended");
                    return PARSE_ERROR;
                }
                if (rbuf_bytes_available(buf) >= (size_t)parse_ctx->content_length)
                    return PARSE_SUCCESS;
                return NEED_MORE_DATA;
        }
    } while(1);
}

static int https_request(http_req_type method, char *host, int port, char *url, char *b, size_t b_size, char *payload)
{
    struct timeval timeout = { .tv_sec = 30, .tv_usec = 0 };
    char sport[PORT_STR_MAX_BYTES];
    size_t len = 0;
    int rc = 1;
    int ret;
    char *ptr;
    http_parse_ctx parse_ctx = HTTP_PARSE_CTX_INITIALIZER;

    rbuf_t buffer = rbuf_create(b_size);
    if (!buffer)
        return 1;

    snprintf(sport, PORT_STR_MAX_BYTES, "%d", port);

    if (payload != NULL)
        len = strlen(payload);

    snprintf(
        b,
        b_size,
        "%s %s HTTP/1.1\r\nHost: %s\r\nAccept: application/json\r\nContent-length: %zu\r\nAccept-Language: en-us\r\n"
        "User-Agent: Netdata/rocks\r\n\r\n",
        (method == HTTP_REQ_GET ? "GET" : "POST"), url, host, len);

    if (payload != NULL)
        strncat(b, payload, b_size - len);

    len = strlen(b);

    debug(D_ACLK, "Sending HTTPS req (%zu bytes): '%s'", len, b);
    int sock = connect_to_this_ip46(IPPROTO_TCP, SOCK_STREAM, host, 0, sport, &timeout);

    if (unlikely(sock == -1)) {
        error("Handshake failed");
        goto exit_buf;
    }

    SSL_CTX *ctx = security_initialize_openssl_client();
    if (ctx==NULL) {
        error("Cannot allocate SSL context");
        goto exit_sock;
    }
    // Certificate chain: not updating the stores - do we need private CA roots?
    // Calls to SSL_CTX_load_verify_locations would go here.
    SSL *ssl = SSL_new(ctx);
    if (ssl==NULL) {
        error("Cannot allocate SSL");
        goto exit_CTX;
    }
    SSL_set_fd(ssl, sock);
    ret = SSL_connect(ssl);
    if (ret != 1) {
        error("SSL_connect() failed with err=%d", ret);
        goto exit_SSL;
    }

    ret = SSL_write(ssl, b, len);
    if (ret <= 0)
    {
        error("SSL_write() failed with err=%d", ret);
        goto exit_SSL;
    }

    b[0] = 0;

    do {
        ptr = rbuf_get_linear_insert_range(buffer, &len);
        ret = SSL_read(ssl, ptr, len - 1);
        if (ret)
            rbuf_bump_head(buffer, ret);
        if (ret <= 0)
        {
            error("No response available - SSL_read()=%d", ret);
            goto exit_FULL;
        }
    } while (!(ret = parse_http_response(buffer, &parse_ctx)));

    if (ret != PARSE_SUCCESS) {
        error("Error parsing HTTP response");
        goto exit_FULL;
    }

    if (parse_ctx.http_code < 200 || parse_ctx.http_code >= 300) {
        error("HTTP Response not Success (got %d)", parse_ctx.http_code);
        goto exit_FULL;
    }

    len = rbuf_pop(buffer, b, b_size);
    b[MIN(len, b_size-1)] = 0;

    rc = 0;
exit_FULL:
exit_SSL:
    SSL_free(ssl);
exit_CTX:
    SSL_CTX_free(ctx);
exit_sock:
    close(sock);
exit_buf:
    rbuf_free(buffer);
    return rc;
}

// aclk_get_mqtt_otp is slightly modified original code from @amoss
void aclk_get_mqtt_otp(RSA *p_key, char *aclk_hostname, int port, char **mqtt_usr, char **mqtt_pass)
{
    char *data_buffer = mallocz(NETDATA_WEB_RESPONSE_INITIAL_SIZE);
    debug(D_ACLK, "Performing challenge-response sequence");
    if (*mqtt_pass != NULL)
    {
        freez(*mqtt_pass);
        *mqtt_pass = NULL;
    }
    // curl http://cloud-iam-agent-service:8080/api/v1/auth/node/00000000-0000-0000-0000-000000000000/challenge
    // TODO - target host?
    char *agent_id = is_agent_claimed();
    if (agent_id == NULL)
    {
        error("Agent was not claimed - cannot perform challenge/response");
        goto CLEANUP;
    }
    char url[1024];
    sprintf(url, "/api/v1/auth/node/%s/challenge", agent_id);
    info("Retrieving challenge from cloud: %s %d %s", aclk_hostname, port, url);
    if (https_request(HTTP_REQ_GET, aclk_hostname, port, url, data_buffer, NETDATA_WEB_RESPONSE_INITIAL_SIZE, NULL))
    {
        error("Challenge failed: %s", data_buffer);
        goto CLEANUP;
    }
    struct dictionary_singleton challenge = { .key = "challenge", .result = NULL };

    debug(D_ACLK, "Challenge response from cloud: %s", data_buffer);
    if (json_parse(data_buffer, &challenge, json_extract_singleton) != JSON_OK)
    {
        freez(challenge.result);
        error("Could not parse the json response with the challenge: %s", data_buffer);
        goto CLEANUP;
    }
    if (challenge.result == NULL) {
        error("Could not retrieve challenge from auth response: %s", data_buffer);
        goto CLEANUP;
    }


    size_t challenge_len = strlen(challenge.result);
    unsigned char decoded[512];
    size_t decoded_len = base64_decode((unsigned char*)challenge.result, challenge_len, decoded, sizeof(decoded));

    unsigned char plaintext[4096]={};
    int decrypted_length = private_decrypt(p_key, decoded, decoded_len, plaintext);
    freez(challenge.result);
    char encoded[512];
    size_t encoded_len = base64_encode(plaintext, decrypted_length, encoded, sizeof(encoded));
    encoded[encoded_len] = 0;
    debug(D_ACLK, "Encoded len=%zu Decryption len=%d: '%s'", encoded_len, decrypted_length, encoded);

    char response_json[4096]={};
    sprintf(response_json, "{\"response\":\"%s\"}", encoded);
    debug(D_ACLK, "Password phase: %s",response_json);
    // TODO - host
    sprintf(url, "/api/v1/auth/node/%s/password", agent_id);
    if (https_request(HTTP_REQ_POST, aclk_hostname, port, url, data_buffer, NETDATA_WEB_RESPONSE_INITIAL_SIZE, response_json))
    {
        error("Challenge-response failed: %s", data_buffer);
        goto CLEANUP;
    }

    debug(D_ACLK, "Password response from cloud: %s", data_buffer);

    struct dictionary_singleton password = { .key = "password", .result = NULL };
    if (json_parse(data_buffer, &password, json_extract_singleton) != JSON_OK)
    {
        freez(password.result);
        error("Could not parse the json response with the password: %s", data_buffer);
        goto CLEANUP;
    }

    if (password.result == NULL ) {
        error("Could not retrieve password from auth response");
        goto CLEANUP;
    }
    if (*mqtt_pass != NULL )
        freez(*mqtt_pass);
    *mqtt_pass = password.result;
    if (*mqtt_usr != NULL)
        freez(*mqtt_usr);
    *mqtt_usr = agent_id;
    agent_id = NULL;

CLEANUP:
    if (agent_id != NULL)
        freez(agent_id);
    freez(data_buffer);
    return;
}
