
// SPDX-License-Identifier: GPL-3.0-or-later

#include "aclk_otp.h"

#include "https_client.h"

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
