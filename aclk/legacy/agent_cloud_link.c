// SPDX-License-Identifier: GPL-3.0-or-later

#include "libnetdata/libnetdata.h"
#include "agent_cloud_link.h"
#include "aclk_lws_https_client.h"
#include "aclk_query.h"
#include "aclk_common.h"
#include "aclk_stats.h"
#include "../aclk_collector_list.h"

#ifdef ENABLE_ACLK
#include <libwebsockets.h>
#endif

int aclk_shutting_down = 0;

// Other global state
static int aclk_subscribed = 0;
static char *aclk_username = NULL;
static char *aclk_password = NULL;

static char *global_base_topic = NULL;
static int aclk_connecting = 0;
int aclk_force_reconnect = 0;       // Indication from lower layers

static netdata_mutex_t aclk_mutex = NETDATA_MUTEX_INITIALIZER;

#define ACLK_LOCK netdata_mutex_lock(&aclk_mutex)
#define ACLK_UNLOCK netdata_mutex_unlock(&aclk_mutex)

void lws_wss_check_queues(size_t *write_len, size_t *write_len_bytes, size_t *read_len);
void aclk_lws_wss_destroy_context();

char *create_uuid()
{
    uuid_t uuid;
    char *uuid_str = mallocz(36 + 1);

    uuid_generate(uuid);
    uuid_unparse(uuid, uuid_str);

    return uuid_str;
}

int legacy_cloud_to_agent_parse(JSON_ENTRY *e)
{
    struct aclk_request *data = e->callback_data;

    switch (e->type) {
        case JSON_OBJECT:
        case JSON_ARRAY:
            break;
        case JSON_STRING:
            if (!strcmp(e->name, "msg-id")) {
                data->msg_id = strdupz(e->data.string);
                break;
            }
            if (!strcmp(e->name, "type")) {
                data->type_id = strdupz(e->data.string);
                break;
            }
            if (!strcmp(e->name, "callback-topic")) {
                data->callback_topic = strdupz(e->data.string);
                break;
            }
            if (!strcmp(e->name, "payload")) {
                if (likely(e->data.string)) {
                    size_t len = strlen(e->data.string);
                    data->payload = mallocz(len+1);
                    if (!url_decode_r(data->payload, e->data.string, len + 1))
                        strcpy(data->payload, e->data.string);
                }
                break;
            }
            break;
        case JSON_NUMBER:
            if (!strcmp(e->name, "version")) {
                data->version = e->data.number;
                break;
            }
            if (!strcmp(e->name, "min-version")) {
                data->min_version = e->data.number;
                break;
            }
            if (!strcmp(e->name, "max-version")) {
                data->max_version = e->data.number;
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


static RSA *aclk_private_key = NULL;
static int create_private_key()
{
    if (aclk_private_key != NULL)
        RSA_free(aclk_private_key);
    aclk_private_key = NULL;
    char filename[FILENAME_MAX + 1];
    snprintfz(filename, FILENAME_MAX, "%s/cloud.d/private.pem", netdata_configured_varlib_dir);

    long bytes_read;
    char *private_key = read_by_filename(filename, &bytes_read);
    if (!private_key) {
        error("Claimed agent cannot establish ACLK - unable to load private key '%s' failed.", filename);
        return 1;
    }
    debug(D_ACLK, "Claimed agent loaded private key len=%ld bytes", bytes_read);

    BIO *key_bio = BIO_new_mem_buf(private_key, -1);
    if (key_bio==NULL) {
        error("Claimed agent cannot establish ACLK - failed to create BIO for key");
        goto biofailed;
    }

    aclk_private_key = PEM_read_bio_RSAPrivateKey(key_bio, NULL, NULL, NULL);
    BIO_free(key_bio);
    if (aclk_private_key!=NULL)
    {
        freez(private_key);
        return 0;
    }
    char err[512];
    ERR_error_string_n(ERR_get_error(), err, sizeof(err));
    error("Claimed agent cannot establish ACLK - cannot create private key: %s", err);

biofailed:
    freez(private_key);
    return 1;
}

/*
 * After a connection failure -- delay in milliseconds
 * When a connection is established, the delay function
 * should be called with
 *
 * mode 0 to reset the delay
 * mode 1 to calculate sleep time [0 .. ACLK_MAX_BACKOFF_DELAY * 1000] ms
 *
 */
unsigned long int aclk_reconnect_delay(int mode)
{
    static int fail = -1;
    unsigned long int delay;

    if (!mode || fail == -1) {
        srandom(time(NULL));
        fail = mode - 1;
        return 0;
    }

    delay = (1 << fail);

    if (delay >= ACLK_MAX_BACKOFF_DELAY) {
        delay = ACLK_MAX_BACKOFF_DELAY * 1000;
    } else {
        fail++;
        delay *= 1000;
        delay += (random() % (MAX(1000, delay/2)));
    }

    return delay;
}

// This will give the base topic that the agent will publish messages.
// subtopics will be sent under the base topic e.g.  base_topic/subtopic
// This is called during the connection, we delete any previous topic
// in-case the user has changed the agent id and reclaimed.

char *create_publish_base_topic()
{
    char *agent_id = is_agent_claimed();
    if (unlikely(!agent_id))
        return NULL;

    ACLK_LOCK;

    if (global_base_topic)
        freez(global_base_topic);
    char tmp_topic[ACLK_MAX_TOPIC + 1], *tmp;

    snprintf(tmp_topic, ACLK_MAX_TOPIC, ACLK_TOPIC_STRUCTURE, agent_id);
    tmp = strchr(tmp_topic, '\n');
    if (unlikely(tmp))
        *tmp = '\0';
    global_base_topic = strdupz(tmp_topic);

    ACLK_UNLOCK;
    freez(agent_id);
    return global_base_topic;
}

/*
 * Build a topic based on sub_topic and final_topic
 * if the sub topic starts with / assume that is an absolute topic
 *
 */

char *get_topic(char *sub_topic, char *final_topic, int max_size)
{
    int rc;

    if (likely(sub_topic && sub_topic[0] == '/'))
        return sub_topic;

    if (unlikely(!global_base_topic))
        return sub_topic;

    rc = snprintf(final_topic, max_size, "%s/%s", global_base_topic, sub_topic);
    if (unlikely(rc >= max_size))
        debug(D_ACLK, "Topic has been truncated to [%s] instead of [%s/%s]", final_topic, global_base_topic, sub_topic);

    return final_topic;
}

/* Avoids the need to scan through all RRDHOSTS
 * every time any Query Thread Wakes Up
 * (every time we need to check child popcorn expiry)
 * call with legacy_aclk_shared_state_LOCK held
 */
void aclk_update_next_child_to_popcorn(void)
{
    RRDHOST *host;
    int any = 0;

    rrd_rdlock();
    rrdhost_foreach_read(host) {
        if (unlikely(host == localhost || rrdhost_flag_check(host, RRDHOST_FLAG_ARCHIVED)))
            continue;

        rrdhost_aclk_state_lock(host);
        if (!ACLK_IS_HOST_POPCORNING(host)) {
            rrdhost_aclk_state_unlock(host);
            continue;
        }

        any = 1;

        if (unlikely(!legacy_aclk_shared_state.next_popcorn_host)) {
            legacy_aclk_shared_state.next_popcorn_host = host;
            rrdhost_aclk_state_unlock(host);
            continue;
        }

        if (legacy_aclk_shared_state.next_popcorn_host->aclk_state.t_last_popcorn_update > host->aclk_state.t_last_popcorn_update)
            legacy_aclk_shared_state.next_popcorn_host = host;

        rrdhost_aclk_state_unlock(host);
    }
    if(!any)
        legacy_aclk_shared_state.next_popcorn_host = NULL;

    rrd_unlock();
}

/* If popcorning bump timer.
 * If popcorning or initializing (host not stable) return 1
 * Otherwise return 0
 */
static int aclk_popcorn_check_bump(RRDHOST *host)
{
    time_t now = now_monotonic_sec();
    int updated = 0, ret;
    legacy_aclk_shared_state_LOCK;
    rrdhost_aclk_state_lock(host);

    ret = ACLK_IS_HOST_INITIALIZING(host);
    if (unlikely(ACLK_IS_HOST_POPCORNING(host))) {
        if(now != host->aclk_state.t_last_popcorn_update) {
            updated = 1;
            info("Restarting ACLK popcorn timer for host \"%s\" with GUID \"%s\"", host->hostname, host->machine_guid);
        }
        host->aclk_state.t_last_popcorn_update = now;
        rrdhost_aclk_state_unlock(host);

        if (host != localhost && updated)
            aclk_update_next_child_to_popcorn();

        legacy_aclk_shared_state_UNLOCK;
        return ret;
    }

    rrdhost_aclk_state_unlock(host);
    legacy_aclk_shared_state_UNLOCK;
    return ret;
}

inline static int aclk_host_initializing(RRDHOST *host)
{
    rrdhost_aclk_state_lock(host);
    int ret = ACLK_IS_HOST_INITIALIZING(host);
    rrdhost_aclk_state_unlock(host);
    return ret;
}

static void aclk_start_host_popcorning(RRDHOST *host)
{
    usec_t now = now_monotonic_sec();
    info("Starting ACLK popcorn timer for host \"%s\" with GUID \"%s\"", host->hostname, host->machine_guid);
    legacy_aclk_shared_state_LOCK;
    rrdhost_aclk_state_lock(host);
    if (host == localhost && !ACLK_IS_HOST_INITIALIZING(host)) {
        errno = 0;
        error("Localhost is allowed to do popcorning only once after startup!");
        rrdhost_aclk_state_unlock(host);
        legacy_aclk_shared_state_UNLOCK;
        return;
    }

    host->aclk_state.state = ACLK_HOST_INITIALIZING;
    host->aclk_state.metadata = ACLK_METADATA_REQUIRED;
    host->aclk_state.t_last_popcorn_update = now;
    rrdhost_aclk_state_unlock(host);
    if (host != localhost)
        aclk_update_next_child_to_popcorn();
    legacy_aclk_shared_state_UNLOCK;
}

static void aclk_stop_host_popcorning(RRDHOST *host)
{
    legacy_aclk_shared_state_LOCK;
    rrdhost_aclk_state_lock(host);
    if (!ACLK_IS_HOST_POPCORNING(host)) {
        rrdhost_aclk_state_unlock(host);
        legacy_aclk_shared_state_UNLOCK;
        return;
    }
    
    info("Host Disconnected before ACLK popcorning finished. Canceling. Host \"%s\" GUID:\"%s\"", host->hostname, host->machine_guid);
    host->aclk_state.t_last_popcorn_update = 0;
    host->aclk_state.metadata = ACLK_METADATA_REQUIRED;
    rrdhost_aclk_state_unlock(host);

    if(host == legacy_aclk_shared_state.next_popcorn_host) {
        legacy_aclk_shared_state.next_popcorn_host = NULL;
        aclk_update_next_child_to_popcorn();
    }
    legacy_aclk_shared_state_UNLOCK;
}

/*
 * Add a new collector to the list
 * If it exists, update the chart count
 */
void legacy_aclk_add_collector(RRDHOST *host, const char *plugin_name, const char *module_name)
{
    struct _collector *tmp_collector;
    if (unlikely(!netdata_ready)) {
        return;
    }

    COLLECTOR_LOCK;

    tmp_collector = _add_collector(host->machine_guid, plugin_name, module_name);

    if (unlikely(tmp_collector->count != 1)) {
        COLLECTOR_UNLOCK;
        return;
    }

    COLLECTOR_UNLOCK;

    if(aclk_popcorn_check_bump(host))
        return;

    if (unlikely(legacy_aclk_queue_query("collector", host, NULL, NULL, 0, 1, ACLK_CMD_ONCONNECT)))
        debug(D_ACLK, "ACLK failed to queue on_connect command on collector addition");
}

/*
 * Delete a collector from the list
 * If the chart count reaches zero the collector will be removed
 * from the list by calling del_collector.
 *
 * This function will release the memory used and schedule
 * a cloud update
 */
void legacy_aclk_del_collector(RRDHOST *host, const char *plugin_name, const char *module_name)
{
    struct _collector *tmp_collector;
    if (unlikely(!netdata_ready)) {
        return;
    }

    COLLECTOR_LOCK;

    tmp_collector = _del_collector(host->machine_guid, plugin_name, module_name);

    if (unlikely(!tmp_collector || tmp_collector->count)) {
        COLLECTOR_UNLOCK;
        return;
    }

    debug(
        D_ACLK, "DEL COLLECTOR [%s:%s] -- charts %u", plugin_name ? plugin_name : "*", module_name ? module_name : "*",
        tmp_collector->count);

    COLLECTOR_UNLOCK;

    _free_collector(tmp_collector);

    if (aclk_popcorn_check_bump(host))
        return;

    if (unlikely(legacy_aclk_queue_query("collector", host, NULL, NULL, 0, 1, ACLK_CMD_ONCONNECT)))
        debug(D_ACLK, "ACLK failed to queue on_connect command on collector deletion");
}

static void aclk_graceful_disconnect()
{
    size_t write_q, write_q_bytes, read_q;
    time_t event_loop_timeout;

    // Send a graceful disconnect message
    BUFFER *b = buffer_create(512);
    aclk_create_header(b, "disconnect", NULL, 0, 0, legacy_aclk_shared_state.version_neg);
    buffer_strcat(b, ",\n\t\"payload\": \"graceful\"}");
    aclk_send_message(ACLK_METADATA_TOPIC, (char*)buffer_tostring(b), NULL);
    buffer_free(b);

    event_loop_timeout = now_realtime_sec() + 5;
    write_q = 1;
    while (write_q && event_loop_timeout > now_realtime_sec()) {
        _link_event_loop();
        lws_wss_check_queues(&write_q, &write_q_bytes, &read_q);
    }

    aclk_shutting_down = 1;
    _link_shutdown();
    aclk_lws_wss_mqtt_layer_disconnect_notif();

    write_q = 1;
    event_loop_timeout = now_realtime_sec() + 5;
    while (write_q && event_loop_timeout > now_realtime_sec()) {
        _link_event_loop();
        lws_wss_check_queues(&write_q, &write_q_bytes, &read_q);
    }
    aclk_shutting_down = 0;
}

#ifndef __GNUC__
#pragma region Incoming Msg Parsing
#endif

struct dictionary_singleton {
    char *key;
    char *result;
};

int json_extract_singleton(JSON_ENTRY *e)
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

#ifndef __GNUC__
#pragma endregion
#endif


#ifndef __GNUC__
#pragma region Challenge Response
#endif

// Base-64 decoder.
// Note: This is non-validating, invalid input will be decoded without an error.
//       Challenges are packed into json strings so we don't skip newlines.
//       Size errors (i.e. invalid input size or insufficient output space) are caught.
size_t base64_decode(unsigned char *input, size_t input_size, unsigned char *output, size_t output_size)
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

size_t base64_encode(unsigned char *input, size_t input_size, char *output, size_t output_size)
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



int private_decrypt(unsigned char * enc_data, int data_len, unsigned char *decrypted)
{
    int  result = RSA_private_decrypt( data_len, enc_data, decrypted, aclk_private_key, RSA_PKCS1_OAEP_PADDING);
    if (result == -1) {
        char err[512];
        ERR_error_string_n(ERR_get_error(), err, sizeof(err));
        error("Decryption of the challenge failed: %s", err);
    }
    return result;
}

void aclk_get_challenge(char *aclk_hostname, int port)
{
    char *data_buffer = mallocz(NETDATA_WEB_RESPONSE_INITIAL_SIZE);
    debug(D_ACLK, "Performing challenge-response sequence");
    if (aclk_password != NULL)
    {
        freez(aclk_password);
        aclk_password = NULL;
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
    if(aclk_send_https_request("GET", aclk_hostname, port, url, data_buffer, NETDATA_WEB_RESPONSE_INITIAL_SIZE, NULL))
    {
        error("Challenge failed: %s", data_buffer);
        goto CLEANUP;
    }
    struct dictionary_singleton challenge = { .key = "challenge", .result = NULL };

    debug(D_ACLK, "Challenge response from cloud: %s", data_buffer);
    if ( json_parse(data_buffer, &challenge, json_extract_singleton) != JSON_OK)
    {
        freez(challenge.result);
        error("Could not parse the json response with the challenge: %s", data_buffer);
        goto CLEANUP;
    }
    if (challenge.result == NULL ) {
        error("Could not retrieve challenge from auth response: %s", data_buffer);
        goto CLEANUP;
    }


    size_t challenge_len = strlen(challenge.result);
    unsigned char decoded[512];
    size_t decoded_len = base64_decode((unsigned char*)challenge.result, challenge_len, decoded, sizeof(decoded));

    unsigned char plaintext[4096]={};
    int decrypted_length = private_decrypt(decoded, decoded_len, plaintext);
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
    if(aclk_send_https_request("POST", aclk_hostname, port, url, data_buffer, NETDATA_WEB_RESPONSE_INITIAL_SIZE, response_json))
    {
        error("Challenge-response failed: %s", data_buffer);
        goto CLEANUP;
    }

    debug(D_ACLK, "Password response from cloud: %s", data_buffer);

    struct dictionary_singleton password = { .key = "password", .result = NULL };
    if ( json_parse(data_buffer, &password, json_extract_singleton) != JSON_OK)
    {
        freez(password.result);
        error("Could not parse the json response with the password: %s", data_buffer);
        goto CLEANUP;
    }

    if (password.result == NULL ) {
        error("Could not retrieve password from auth response");
        goto CLEANUP;
    }
    if (aclk_password != NULL )
        freez(aclk_password);
    aclk_password = password.result;
    if (aclk_username != NULL)
        freez(aclk_username);
    aclk_username = agent_id;
    agent_id = NULL;

CLEANUP:
    if (agent_id != NULL)
        freez(agent_id);
    freez(data_buffer);
    return;
}

#ifndef __GNUC__
#pragma endregion
#endif

static void aclk_try_to_connect(char *hostname, int port)
{
    int rc;

// this is useful for developers working on ACLK
// allows connecting agent to any MQTT broker
// for debugging, development and testing purposes
#ifndef ACLK_DISABLE_CHALLENGE
    if (!aclk_private_key) {
        error("Cannot try to establish the agent cloud link - no private key available!");
        return;
    }
#endif

    info("Attempting to establish the agent cloud link");
#ifdef ACLK_DISABLE_CHALLENGE
    error("Agent built with ACLK_DISABLE_CHALLENGE. This is for testing "
          "and development purposes only. Warranty void. Won't be able "
          "to connect to Netdata Cloud.");
    if (aclk_password == NULL)
        aclk_password = strdupz("anon");
#else
    aclk_get_challenge(hostname, port);
    if (aclk_password == NULL)
        return;
#endif

    aclk_connecting = 1;
    create_publish_base_topic();

    legacy_aclk_shared_state_LOCK;
    legacy_aclk_shared_state.version_neg = 0;
    legacy_aclk_shared_state.version_neg_wait_till = 0;
    legacy_aclk_shared_state_UNLOCK;

    rc = mqtt_attempt_connection(hostname, port, aclk_username, aclk_password);
    if (unlikely(rc)) {
        error("Failed to initialize the agent cloud link library");
    }
}

// Sends "hello" message to negotiate ACLK version with cloud
static inline void aclk_hello_msg()
{
    BUFFER *buf = buffer_create(NETDATA_WEB_RESPONSE_HEADER_SIZE);

    char *msg_id = create_uuid();

    legacy_aclk_shared_state_LOCK;
    legacy_aclk_shared_state.version_neg = 0;
    legacy_aclk_shared_state.version_neg_wait_till = now_monotonic_usec() + USEC_PER_SEC * VERSION_NEG_TIMEOUT;
    legacy_aclk_shared_state_UNLOCK;

    //Hello message is versioned separately from the rest of the protocol
    aclk_create_header(buf, "hello", msg_id, 0, 0, ACLK_VERSION_NEG_VERSION);
    buffer_sprintf(buf, ",\"min-version\":%d,\"max-version\":%d}", ACLK_VERSION_MIN, ACLK_VERSION_MAX);
    aclk_send_message(ACLK_METADATA_TOPIC, buf->buffer, msg_id);
    freez(msg_id);
    buffer_free(buf);
}

/**
 * Main agent cloud link thread
 *
 * This thread will simply call the main event loop that handles
 * pending requests - both inbound and outbound
 *
 * @param ptr is a pointer to the netdata_static_thread structure.
 *
 * @return It always returns NULL
 */
void *legacy_aclk_main(void *ptr)
{
    struct netdata_static_thread *static_thread = (struct netdata_static_thread *)ptr;
    struct aclk_query_threads query_threads;
    struct aclk_stats_thread *stats_thread = NULL;
    time_t last_periodic_query_wakeup = 0;

    query_threads.thread_list = NULL;

    // This thread is unusual in that it cannot be cancelled by cancel_main_threads()
    // as it must notify the far end that it shutdown gracefully and avoid the LWT.
    netdata_thread_disable_cancelability();

#if defined( DISABLE_CLOUD ) || !defined( ENABLE_ACLK)
    info("Killing ACLK thread -> cloud functionality has been disabled");
    static_thread->enabled = NETDATA_MAIN_THREAD_EXITED;
    return NULL;
#endif

#ifndef LWS_WITH_SOCKS5
    ACLK_PROXY_TYPE proxy_type;
    aclk_get_proxy(&proxy_type);
    if(proxy_type == PROXY_TYPE_SOCKS5) {
        error("Disabling ACLK due to requested SOCKS5 proxy.");
        static_thread->enabled = NETDATA_MAIN_THREAD_EXITED;
        return NULL;
    }
#endif

    info("Waiting for netdata to be ready");
    while (!netdata_ready) {
        sleep_usec(USEC_PER_MS * 300);
    }

    info("Waiting for Cloud to be enabled");
    while (!netdata_cloud_setting) {
        sleep_usec(USEC_PER_SEC * 1);
        if (netdata_exit) {
            static_thread->enabled = NETDATA_MAIN_THREAD_EXITED;
            return NULL;
        }
    }

    query_threads.count = MIN(processors/2, 6);
    query_threads.count = MAX(query_threads.count, 2);
    query_threads.count = config_get_number(CONFIG_SECTION_CLOUD, "query thread count", query_threads.count);
    if(query_threads.count < 1) {
        error("You need at least one query thread. Overriding configured setting of \"%d\"", query_threads.count);
        query_threads.count = 1;
        config_set_number(CONFIG_SECTION_CLOUD, "query thread count", query_threads.count);
    }

    //start localhost popcorning
    aclk_start_host_popcorning(localhost);

    aclk_stats_enabled = config_get_boolean(CONFIG_SECTION_CLOUD, "statistics", CONFIG_BOOLEAN_YES);
    if (aclk_stats_enabled) {
        stats_thread = callocz(1, sizeof(struct aclk_stats_thread));
        stats_thread->thread = mallocz(sizeof(netdata_thread_t));
        stats_thread->query_thread_count = query_threads.count;
        netdata_thread_create(
            stats_thread->thread, ACLK_STATS_THREAD_NAME, NETDATA_THREAD_OPTION_JOINABLE, legacy_aclk_stats_main_thread,
            stats_thread);
    }

    char *aclk_hostname = NULL; // Initializers are over-written but prevent gcc complaining about clobbering.
    int port_num = 0;
    info("Waiting for netdata to be claimed");
    while(1) {
        char *agent_id = is_agent_claimed();
        while (likely(!agent_id)) {
            sleep_usec(USEC_PER_SEC * 1);
            if (netdata_exit)
                goto exited;
            agent_id = is_agent_claimed();
        }
        freez(agent_id);
        // The NULL return means the value was never initialised, but this value has been initialized in post_conf_load.
        // We trap the impossible NULL here to keep the linter happy without using a fatal() in the code.
        char *cloud_base_url = appconfig_get(&cloud_config, CONFIG_SECTION_GLOBAL, "cloud base url", NULL);
        if (cloud_base_url == NULL) {
            error("Do not move the cloud base url out of post_conf_load!!");
            goto exited;
        }
        if (aclk_decode_base_url(cloud_base_url, &aclk_hostname, &port_num))
            error("Agent is claimed but the configuration is invalid, please fix");
        else if (!create_private_key() && !_mqtt_lib_init())
                break;

        for (int i=0; i<60; i++) {
            if (netdata_exit)
                goto exited;

            sleep_usec(USEC_PER_SEC * 1);
        }
    }

    usec_t reconnect_expiry = 0; // In usecs

    while (!netdata_exit) {
        static int first_init = 0;
 /*       size_t write_q, write_q_bytes, read_q;
        lws_wss_check_queues(&write_q, &write_q_bytes, &read_q);*/

        if (aclk_disable_runtime && !aclk_connected) {
            sleep(1);
            continue;
        }

        if (aclk_kill_link) {                       // User has reloaded the claiming state
            aclk_kill_link = 0;
            aclk_graceful_disconnect();
            create_private_key();
            continue;
        }

        if (aclk_force_reconnect) {
            aclk_lws_wss_destroy_context();
            aclk_force_reconnect = 0;
        }
        if (unlikely(!netdata_exit && !aclk_connected && !aclk_force_reconnect)) {
            if (unlikely(!first_init)) {
                aclk_try_to_connect(aclk_hostname, port_num);
                first_init = 1;
            } else {
                if (aclk_connecting == 0) {
                    if (reconnect_expiry == 0) {
                        unsigned long int delay = aclk_reconnect_delay(1);
                        reconnect_expiry = now_realtime_usec() + delay * 1000;
                        info("Retrying to establish the ACLK connection in %.3f seconds", delay / 1000.0);
                    }
                    if (now_realtime_usec() >= reconnect_expiry) {
                        reconnect_expiry = 0;
                        aclk_try_to_connect(aclk_hostname, port_num);
                    }
                    sleep_usec(USEC_PER_MS * 100);
                }
            }
            if (aclk_connecting) {
                _link_event_loop();
                sleep_usec(USEC_PER_MS * 100);
            }
            continue;
        }

        _link_event_loop();
        if (unlikely(!aclk_connected || aclk_force_reconnect))
            continue;
        /*static int stress_counter = 0;
        if (write_q_bytes==0 && stress_counter ++ >5)
        {
            aclk_send_stress_test(8000000);
            stress_counter = 0;
        }*/

        if (unlikely(!aclk_subscribed)) {
            aclk_subscribed = !aclk_subscribe(ACLK_COMMAND_TOPIC, 1);
            aclk_hello_msg();
        }

        if (unlikely(!query_threads.thread_list)) {
            legacy_aclk_query_threads_start(&query_threads);
        }

        time_t now = now_monotonic_sec();
        if(aclk_connected && last_periodic_query_wakeup < now) {
            // to make `legacy_aclk_queue_query()` param `run_after` work
            // also makes per child popcorning work
            last_periodic_query_wakeup = now;
            LEGACY_QUERY_THREAD_WAKEUP;
        }
    } // forever
exited:
    // Wakeup query thread to cleanup
    LEGACY_QUERY_THREAD_WAKEUP_ALL;

    freez(aclk_username);
    freez(aclk_password);
    freez(aclk_hostname);
    if (aclk_private_key != NULL)
        RSA_free(aclk_private_key);

    static_thread->enabled = NETDATA_MAIN_THREAD_EXITING;

    char *agent_id = is_agent_claimed();
    if (agent_id && aclk_connected) {
        freez(agent_id);
        // Wakeup thread to cleanup
        LEGACY_QUERY_THREAD_WAKEUP;
        aclk_graceful_disconnect();
    }

    legacy_aclk_query_threads_cleanup(&query_threads);

    _reset_collector_list();
    freez(collector_list);

    if(aclk_stats_enabled) {
        netdata_thread_join(*stats_thread->thread, NULL);
        legacy_aclk_stats_thread_cleanup();
        freez(stats_thread->thread);
        freez(stats_thread);
    }

    /*
     * this must be last -> if all static threads signal
     * THREAD_EXITED rrdengine will dealloc the RRDSETs
     * and RRDDIMs that are used by still running stat thread.
     * see netdata_cleanup_and_exit() for reference
     */
    static_thread->enabled = NETDATA_MAIN_THREAD_EXITED;
    return NULL;
}

/*
 * Send a message to the cloud, using a base topic and sib_topic
 * The final topic will be in the form <base_topic>/<sub_topic>
 * If base_topic is missing then the global_base_topic will be used (if available)
 *
 */
int aclk_send_message_bin(char *sub_topic, const void *message, size_t len, char *msg_id)
{
    int rc;
    int mid;
    char topic[ACLK_MAX_TOPIC + 1];
    char *final_topic;

    UNUSED(msg_id);

    if (!aclk_connected)
        return 0;

    if (unlikely(!message))
        return 0;

    final_topic = get_topic(sub_topic, topic, ACLK_MAX_TOPIC);

    if (unlikely(!final_topic)) {
        errno = 0;
        error("Unable to build outgoing topic; truncated?");
        return 1;
    }

    ACLK_LOCK;
    rc = _link_send_message(final_topic, message, len, &mid);
    // TODO: link the msg_id with the mid so we can trace it
    ACLK_UNLOCK;

    if (unlikely(rc)) {
        errno = 0;
        error("Failed to send message, error code %d (%s)", rc, _link_strerror(rc));
    }

    return rc;
}

int aclk_send_message(char *sub_topic, char *message, char *msg_id)
{
    return aclk_send_message_bin(sub_topic, message, strlen(message), msg_id);
}

/*
 * Subscribe to a topic in the cloud
 * The final subscription will be in the form
 * /agent/claim_id/<sub_topic>
 */
int aclk_subscribe(char *sub_topic, int qos)
{
    int rc;
    char topic[ACLK_MAX_TOPIC + 1];
    char *final_topic;

    final_topic = get_topic(sub_topic, topic, ACLK_MAX_TOPIC);
    if (unlikely(!final_topic)) {
        errno = 0;
        error("Unable to build outgoing topic; truncated?");
        return 1;
    }

    if (!aclk_connected) {
        error("Cannot subscribe to %s - not connected!", topic);
        return 1;
    }

    ACLK_LOCK;
    rc = _link_subscribe(final_topic, qos);
    ACLK_UNLOCK;

    // TODO: Add better handling -- error will flood the logfile here
    if (unlikely(rc)) {
        errno = 0;
        error("Failed subscribe to command topic %d (%s)", rc, _link_strerror(rc));
    }

    return rc;
}

// This is called from a callback when the link goes up
void aclk_connect()
{
    info("Connection detected (%u queued queries)", aclk_query_size());

    legacy_aclk_stats_upd_online(1);

    aclk_connected = 1;
    aclk_reconnect_delay(0);

    LEGACY_QUERY_THREAD_WAKEUP;
    return;
}

// This is called from a callback when the link goes down
void aclk_disconnect()
{
    if (likely(aclk_connected))
        info("Disconnect detected (%u queued queries)", aclk_query_size());

    legacy_aclk_stats_upd_online(0);

    aclk_subscribed = 0;
    rrdhost_aclk_state_lock(localhost);
    localhost->aclk_state.metadata = ACLK_METADATA_REQUIRED;
    rrdhost_aclk_state_unlock(localhost);
    aclk_connected = 0;
    aclk_connecting = 0;
    aclk_force_reconnect = 1;
}

inline void aclk_create_header(BUFFER *dest, char *type, char *msg_id, time_t ts_secs, usec_t ts_us, int version)
{
    uuid_t uuid;
    char uuid_str[36 + 1];

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

    buffer_sprintf(
        dest,
        "{\t\"type\": \"%s\",\n"
        "\t\"msg-id\": \"%s\",\n"
        "\t\"timestamp\": %ld,\n"
        "\t\"timestamp-offset-usec\": %llu,\n"
        "\t\"connect\": %ld,\n"
        "\t\"connect-offset-usec\": %llu,\n"
        "\t\"version\": %d",
        type, msg_id, ts_secs, ts_us, aclk_session_sec, aclk_session_us, version);

    debug(D_ACLK, "Sending v%d msgid [%s] type [%s] time [%ld]", version, msg_id, type, ts_secs);
}


/*
 * This will send alarm information which includes
 *    configured alarms
 *    alarm_log
 *    active alarms
 */
void health_active_log_alarms_2json(RRDHOST *host, BUFFER *wb);

void legacy_aclk_send_alarm_metadata(ACLK_METADATA_STATE metadata_submitted)
{
    BUFFER *local_buffer = buffer_create(NETDATA_WEB_RESPONSE_INITIAL_SIZE);

    char *msg_id = create_uuid();
    buffer_flush(local_buffer);
    local_buffer->contenttype = CT_APPLICATION_JSON;

    debug(D_ACLK, "Metadata alarms start");

    // on_connect messages are sent on a health reload, if the on_connect message is real then we
    // use the session time as the fake timestamp to indicate that it starts the session. If it is
    // a fake on_connect message then use the real timestamp to indicate it is within the existing
    // session.

    if (metadata_submitted == ACLK_METADATA_SENT)
        aclk_create_header(local_buffer, "connect_alarms", msg_id, 0, 0, legacy_aclk_shared_state.version_neg);
    else
        aclk_create_header(local_buffer, "connect_alarms", msg_id, aclk_session_sec, aclk_session_us, legacy_aclk_shared_state.version_neg);
    buffer_strcat(local_buffer, ",\n\t\"payload\": ");


    buffer_sprintf(local_buffer, "{\n\t \"configured-alarms\" : ");
    health_alarms2json(localhost, local_buffer, 1);
    debug(D_ACLK, "Metadata %s with configured alarms has %zu bytes", msg_id, local_buffer->len);
    //    buffer_sprintf(local_buffer, ",\n\t \"alarm-log\" : ");
    //   health_alarm_log2json(localhost, local_buffer, 0);
    //   debug(D_ACLK, "Metadata %s with alarm_log has %zu bytes", msg_id, local_buffer->len);
    buffer_sprintf(local_buffer, ",\n\t \"alarms-active\" : ");
    health_active_log_alarms_2json(localhost, local_buffer);
    //debug(D_ACLK, "Metadata message %s", local_buffer->buffer);



    buffer_sprintf(local_buffer, "\n}\n}");
    aclk_send_message(ACLK_ALARMS_TOPIC, local_buffer->buffer, msg_id);

    freez(msg_id);
    buffer_free(local_buffer);
}

/*
 * This will send the agent metadata
 *    /api/v1/info
 *    charts
 */
int legacy_aclk_send_info_metadata(ACLK_METADATA_STATE metadata_submitted, RRDHOST *host)
{
    BUFFER *local_buffer = buffer_create(NETDATA_WEB_RESPONSE_INITIAL_SIZE);

    debug(D_ACLK, "Metadata /info start");

    char *msg_id = create_uuid();
    buffer_flush(local_buffer);
    local_buffer->contenttype = CT_APPLICATION_JSON;

    // on_connect messages are sent on a health reload, if the on_connect message is real then we
    // use the session time as the fake timestamp to indicate that it starts the session. If it is
    // a fake on_connect message then use the real timestamp to indicate it is within the existing
    // session.
    if (metadata_submitted == ACLK_METADATA_SENT)
        aclk_create_header(local_buffer, "update", msg_id, 0, 0, legacy_aclk_shared_state.version_neg);
    else
        aclk_create_header(local_buffer, "connect", msg_id, aclk_session_sec, aclk_session_us, legacy_aclk_shared_state.version_neg);
    buffer_strcat(local_buffer, ",\n\t\"payload\": ");

    buffer_sprintf(local_buffer, "{\n\t \"info\" : ");
    web_client_api_request_v1_info_fill_buffer(host, local_buffer);
    debug(D_ACLK, "Metadata %s with info has %zu bytes", msg_id, local_buffer->len);

    buffer_sprintf(local_buffer, ", \n\t \"charts\" : ");
    charts2json(host, local_buffer, 1, 0);
    buffer_sprintf(local_buffer, "\n}\n}");
    debug(D_ACLK, "Metadata %s with chart has %zu bytes", msg_id, local_buffer->len);

    aclk_send_message(ACLK_METADATA_TOPIC, local_buffer->buffer, msg_id);

    freez(msg_id);
    buffer_free(local_buffer);
    return 0;
}

int aclk_send_info_child_connection(RRDHOST *host, ACLK_CMD cmd)
{
    BUFFER *local_buffer = buffer_create(NETDATA_WEB_RESPONSE_INITIAL_SIZE);
    local_buffer->contenttype = CT_APPLICATION_JSON;

    if(legacy_aclk_shared_state.version_neg < ACLK_V_CHILDRENSTATE)
        fatal("This function should not be called if ACLK version is less than %d (current %d)", ACLK_V_CHILDRENSTATE, legacy_aclk_shared_state.version_neg);

    debug(D_ACLK, "Sending Child Disconnect");

    char *msg_id = create_uuid();

    aclk_create_header(local_buffer, cmd == ACLK_CMD_CHILD_CONNECT ? "child_connect" : "child_disconnect", msg_id, 0, 0, legacy_aclk_shared_state.version_neg);

    buffer_strcat(local_buffer, ",\"payload\":");

    buffer_sprintf(local_buffer, "{\"guid\":\"%s\",\"claim_id\":", host->machine_guid);
    rrdhost_aclk_state_lock(host);
    if(host->aclk_state.claimed_id)
        buffer_sprintf(local_buffer, "\"%s\"}}", host->aclk_state.claimed_id);
    else
        buffer_strcat(local_buffer, "null}}");

    rrdhost_aclk_state_unlock(host);

    aclk_send_message(ACLK_METADATA_TOPIC, local_buffer->buffer, msg_id);

    freez(msg_id);
    buffer_free(local_buffer);
    return 0;
}

void legacy_aclk_host_state_update(RRDHOST *host, int connect)
{
#if ACLK_VERSION_MIN < ACLK_V_CHILDRENSTATE
    if (legacy_aclk_shared_state.version_neg < ACLK_V_CHILDRENSTATE)
        return;
#else
#warning "This check became unnecessary. Remove"
#endif

    if (unlikely(aclk_host_initializing(localhost)))
        return;

    if (connect) {
        debug(D_ACLK, "Child Connected %s %s.", host->hostname, host->machine_guid);
        aclk_start_host_popcorning(host);
        legacy_aclk_queue_query("add_child", host, NULL, NULL, 0, 1, ACLK_CMD_CHILD_CONNECT);
    } else {
        debug(D_ACLK, "Child Disconnected %s %s.", host->hostname, host->machine_guid);
        aclk_stop_host_popcorning(host);
        legacy_aclk_queue_query("del_child", host, NULL, NULL, 0, 1, ACLK_CMD_CHILD_DISCONNECT);
    }
}

void aclk_send_stress_test(size_t size)
{
    char *buffer = mallocz(size);
    if (buffer != NULL)
    {
        for(size_t i=0; i<size; i++)
            buffer[i] = 'x';
        buffer[size-1] = 0;
        time_t time_created = now_realtime_sec();
        sprintf(buffer,"{\"type\":\"stress\", \"timestamp\":%ld,\"payload\":", time_created);
        buffer[strlen(buffer)] = '"';
        buffer[size-2] = '}';
        buffer[size-3] = '"';
        aclk_send_message(ACLK_METADATA_TOPIC, buffer, NULL);
        error("Sending stress of size %zu at time %ld", size, time_created);
    }
    free(buffer);
}

// Send info metadata message to the cloud if the link is established
// or on request
int aclk_send_metadata(ACLK_METADATA_STATE state, RRDHOST *host)
{
    legacy_aclk_send_info_metadata(state, host);

    if(host == localhost)
        legacy_aclk_send_alarm_metadata(state);

    return 0;
}

// Triggered by a health reload, sends the alarm metadata
void legacy_aclk_alarm_reload()
{
    if (unlikely(aclk_host_initializing(localhost)))
        return;

    if (unlikely(legacy_aclk_queue_query("on_connect", localhost, NULL, NULL, 0, 1, ACLK_CMD_ONCONNECT))) {
        if (likely(aclk_connected)) {
            errno = 0;
            error("ACLK failed to queue on_connect command on alarm reload");
        }
    }
}
//rrd_stats_api_v1_chart(RRDSET *st, BUFFER *buf)

int aclk_send_single_chart(RRDHOST *host, char *chart)
{
    RRDSET *st = rrdset_find(host, chart);
    if (!st)
        st = rrdset_find_byname(host, chart);
    if (!st) {
        info("FAILED to find chart %s", chart);
        return 1;
    }

    BUFFER *local_buffer = buffer_create(NETDATA_WEB_RESPONSE_INITIAL_SIZE);
    char *msg_id = create_uuid();
    buffer_flush(local_buffer);
    local_buffer->contenttype = CT_APPLICATION_JSON;

    aclk_create_header(local_buffer, "chart", msg_id, 0, 0, legacy_aclk_shared_state.version_neg);
    buffer_strcat(local_buffer, ",\n\t\"payload\": ");

    rrdset2json(st, local_buffer, NULL, NULL, 1);
    buffer_sprintf(local_buffer, "\t\n}");

    aclk_send_message(ACLK_CHART_TOPIC, local_buffer->buffer, msg_id);

    freez(msg_id);
    buffer_free(local_buffer);
    return 0;
}

int legacy_aclk_update_chart(RRDHOST *host, char *chart_name, int create)
{
#ifndef ENABLE_ACLK
    UNUSED(host);
    UNUSED(chart_name);
    return 0;
#else
    if (unlikely(!netdata_ready))
        return 0;

    if (!netdata_cloud_setting)
        return 0;

    if (legacy_aclk_shared_state.version_neg < ACLK_V_CHILDRENSTATE && host != localhost)
        return 0;

    if (aclk_host_initializing(localhost))
        return 0;

    if (unlikely(aclk_disable_single_updates))
        return 0;

    if (aclk_popcorn_check_bump(host))
        return 0;

    if (unlikely(legacy_aclk_queue_query("_chart", host, NULL, chart_name, 0, 1, create ? ACLK_CMD_CHART : ACLK_CMD_CHARTDEL))) {
        if (likely(aclk_connected)) {
            errno = 0;
            error("ACLK failed to queue chart_update command");
        }
    }

    return 0;
#endif
}

int legacy_aclk_update_alarm(RRDHOST *host, ALARM_ENTRY *ae)
{
    BUFFER *local_buffer = NULL;

    if (unlikely(!netdata_ready))
        return 0;

    if (host != localhost)
        return 0;

    if(unlikely(aclk_host_initializing(localhost)))
        return 0;

    /*
     * Check if individual updates have been disabled
     * This will be the case when we do health reload
     * and all the alarms will be dropped and recreated.
     * At the end of the health reload the complete alarm metadata
     * info will be sent
     */
    if (unlikely(aclk_disable_single_updates))
        return 0;

    local_buffer = buffer_create(NETDATA_WEB_RESPONSE_INITIAL_SIZE);
    char *msg_id = create_uuid();

    buffer_flush(local_buffer);
    aclk_create_header(local_buffer, "status-change", msg_id, 0, 0, legacy_aclk_shared_state.version_neg);
    buffer_strcat(local_buffer, ",\n\t\"payload\": ");

    netdata_rwlock_rdlock(&host->health_log.alarm_log_rwlock);
    health_alarm_entry2json_nolock(local_buffer, ae, host);
    netdata_rwlock_unlock(&host->health_log.alarm_log_rwlock);

    buffer_sprintf(local_buffer, "\n}");

    if (unlikely(legacy_aclk_queue_query(ACLK_ALARMS_TOPIC, NULL, msg_id, local_buffer->buffer, 0, 1, ACLK_CMD_ALARM))) {
        if (likely(aclk_connected)) {
            errno = 0;
            error("ACLK failed to queue alarm_command on alarm_update");
        }
    }

    freez(msg_id);
    buffer_free(local_buffer);

    return 0;
}

char *legacy_aclk_state(void)
{
    BUFFER *wb = buffer_create(1024);
    char *ret;

    buffer_strcat(wb,
        "ACLK Available: Yes\n"
        "ACLK Implementation: Legacy\n"
        "Claimed: "
    );

    char *agent_id = is_agent_claimed();
    if (agent_id == NULL)
        buffer_strcat(wb, "No\n");
    else {
        buffer_sprintf(wb, "Yes\nClaimed Id: %s\n", agent_id);
        freez(agent_id);
    }

    buffer_sprintf(wb, "Online: %s", aclk_connected ? "Yes" : "No");

    ret = strdupz(buffer_tostring(wb));
    buffer_free(wb);
    return ret;
}

char *legacy_aclk_state_json(void)
{
    BUFFER *wb = buffer_create(1024);
    char *agent_id = is_agent_claimed();

    buffer_sprintf(wb,
        "{\"aclk-available\":true,"
        "\"aclk-implementation\":\"Legacy\","
        "\"agent-claimed\":%s,"
        "\"claimed-id\":",
        agent_id ? "true" : "false"
    );

    if (agent_id) {
        buffer_sprintf(wb, "\"%s\"", agent_id);
        freez(agent_id);
    } else
        buffer_strcat(wb, "null");

    buffer_sprintf(wb, ",\"online\":%s}", aclk_connected ? "true" : "false");

    return strdupz(buffer_tostring(wb));
}
