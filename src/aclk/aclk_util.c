// SPDX-License-Identifier: GPL-3.0-or-later

#include "aclk_util.h"

#include "aclk_proxy.h"

#include "database/rrd.h"

usec_t aclk_session_newarch = 0;

aclk_env_t *aclk_env = NULL;

void aclk_sensitive_memzero(void *ptr, size_t len)
{
    if (!ptr || len == 0)
        return;

    volatile unsigned char *p = (volatile unsigned char *)ptr;
    while (len--)
        *p++ = 0;
}

void aclk_sensitive_free(char **ptr)
{
    if (!ptr || !*ptr)
        return;

    size_t len = strlen(*ptr);
    aclk_sensitive_memzero(*ptr, len);
    freez(*ptr);
    *ptr = NULL;
}

aclk_encoding_type_t aclk_encoding_type_t_from_str(const char *str) {
    if (!strcmp(str, "json")) {
        return ACLK_ENC_JSON;
    }
    if (!strcmp(str, "proto")) {
        return ACLK_ENC_PROTO;
    }
    return ACLK_ENC_UNKNOWN;
}

aclk_transport_type_t aclk_transport_type_t_from_str(const char *str) {
    if (!strcmp(str, "MQTTv3")) {
        return ACLK_TRP_MQTT_3_1_1;
    }
    if (!strcmp(str, "MQTTv5")) {
        return ACLK_TRP_MQTT_5;
    }
    return ACLK_TRP_UNKNOWN;
}

void aclk_transport_desc_t_destroy(aclk_transport_desc_t *trp_desc) {
    freez(trp_desc->endpoint);
}

void aclk_env_t_destroy(aclk_env_t *env) {
    freez(env->auth_endpoint);
    if (env->transports) {
        for (size_t i = 0; i < env->transport_count; i++) {
            if(env->transports[i]) {
                aclk_transport_desc_t_destroy(env->transports[i]);
                freez(env->transports[i]);
                env->transports[i] = NULL;
            }
        }
        freez(env->transports);
    }
    if (env->capabilities) {
        for (size_t i = 0; i < env->capability_count; i++)
            freez(env->capabilities[i]);
        freez(env->capabilities);
    }
}

int aclk_env_has_capa(const char *capa)
{
    for (int i = 0; i < (int) aclk_env->capability_count; i++) {
        if (!strcasecmp(capa, aclk_env->capabilities[i]))
            return 1;
    }
    return 0;
}

#ifdef ACLK_LOG_CONVERSATION_DIR
volatile int aclk_conversation_log_counter = 0;
#endif

#define ACLK_TOPIC_PREFIX "/agent/"

struct aclk_topic {
    enum aclk_topics topic_id;
    // as received from cloud - we keep this for
    // eventual topic list update when claim_id changes
    char *topic_recvd;
    // constructed topic
    char *topic;
};

// This helps to cache finalized topics (assembled with claim_id)
// to not have to alloc or create buffer and construct topic every
// time message is sent as in old ACLK
static struct aclk_topic **aclk_topic_cache = NULL;
static size_t aclk_topic_cache_items = 0;

void free_topic_cache(void)
{
    if (aclk_topic_cache) {
        for (size_t i = 0; i < aclk_topic_cache_items; i++) {
            freez(aclk_topic_cache[i]->topic);
            freez(aclk_topic_cache[i]->topic_recvd);
            freez(aclk_topic_cache[i]);
        }
        freez(aclk_topic_cache);
        aclk_topic_cache = NULL;
        aclk_topic_cache_items = 0;
    }
}

#define JSON_TOPIC_KEY_TOPIC "topic"
#define JSON_TOPIC_KEY_NAME "name"

struct topic_name {
    enum aclk_topics id;
    // cloud name - how is it called
    // in answer to /password endpoint
    const char *name;
} topic_names[] = {
    { .id = ACLK_TOPICID_CHART,                 .name = "chart"                    },
    { .id = ACLK_TOPICID_ALARMS,                .name = "alarms"                   },
    { .id = ACLK_TOPICID_METADATA,              .name = "meta"                     },
    { .id = ACLK_TOPICID_COMMAND,               .name = "inbox-cmd"                },
    { .id = ACLK_TOPICID_AGENT_CONN,            .name = "agent-connection"         },
    { .id = ACLK_TOPICID_CMD_NG_V1,             .name = "inbox-cmd-v1"             },
    { .id = ACLK_TOPICID_CREATE_NODE,           .name = "create-node-instance"     },
    { .id = ACLK_TOPICID_NODE_CONN,             .name = "node-instance-connection" },
    { .id = ACLK_TOPICID_CHART_DIMS,            .name = "chart-and-dims-updated"   },
    { .id = ACLK_TOPICID_CHART_CONFIGS_UPDATED, .name = "chart-configs-updated"    },
    { .id = ACLK_TOPICID_CHART_RESET,           .name = "reset-charts"             },
    { .id = ACLK_TOPICID_RETENTION_UPDATED,     .name = "chart-retention-updated"  },
    { .id = ACLK_TOPICID_NODE_INFO,             .name = "node-instance-info"       },
    { .id = ACLK_TOPICID_ALARM_LOG,             .name = "alarm-log-v2"             },
    { .id = ACLK_TOPICID_ALARM_CHECKPOINT,      .name = "alarm-checkpoint"         },
    { .id = ACLK_TOPICID_ALARM_CONFIG,          .name = "alarm-config"             },
    { .id = ACLK_TOPICID_ALARM_SNAPSHOT,        .name = "alarm-snapshot-v2"        },
    { .id = ACLK_TOPICID_NODE_COLLECTORS,       .name = "node-instance-collectors" },
    { .id = ACLK_TOPICID_CTXS_SNAPSHOT,         .name = "contexts-snapshot"        },
    { .id = ACLK_TOPICID_CTXS_UPDATED,          .name = "contexts-updated"         },
    { .id = ACLK_TOPICID_UNKNOWN,               .name = NULL                       }
};

enum aclk_topics compulsory_topics[] = {
// TODO remove old topics once not needed anymore
    ACLK_TOPICID_CHART, //TODO from legacy
    ACLK_TOPICID_ALARMS, //TODO from legacy
    ACLK_TOPICID_METADATA, //TODO from legacy
    ACLK_TOPICID_COMMAND,
    ACLK_TOPICID_AGENT_CONN,
    ACLK_TOPICID_CMD_NG_V1,
    ACLK_TOPICID_CREATE_NODE,
    ACLK_TOPICID_NODE_CONN,
    ACLK_TOPICID_CHART_DIMS,
    ACLK_TOPICID_CHART_CONFIGS_UPDATED,
    ACLK_TOPICID_CHART_RESET,
    ACLK_TOPICID_RETENTION_UPDATED,
    ACLK_TOPICID_NODE_INFO,
    ACLK_TOPICID_ALARM_LOG,
    ACLK_TOPICID_ALARM_CHECKPOINT,
    ACLK_TOPICID_ALARM_CONFIG,
    ACLK_TOPICID_ALARM_SNAPSHOT,
    ACLK_TOPICID_NODE_COLLECTORS,
    ACLK_TOPICID_CTXS_SNAPSHOT,
    ACLK_TOPICID_CTXS_UPDATED,
    ACLK_TOPICID_UNKNOWN
};

static enum aclk_topics topic_name_to_id(const char *name) {
    struct topic_name *topic = topic_names;
    while (topic->name) {
        if (!strcmp(topic->name, name)) {
            return topic->id;
        }
        topic++;
    }
    return ACLK_TOPICID_UNKNOWN;
}

static const char *topic_id_to_name(enum aclk_topics tid) {
    struct topic_name *topic = topic_names;
    while (topic->name) {
        if (topic->id == tid)
            return topic->name;
        topic++;
    }
    return "unknown";
}

#define CLAIM_ID_REPLACE_TAG "#{claim_id}"
static void topic_generate_final(struct aclk_topic *t) {
    char *dest;
    char *replace_tag = strstr(t->topic_recvd, CLAIM_ID_REPLACE_TAG);
    if (!replace_tag)
        return;

    CLAIM_ID claim_id = claim_id_get();
    if (unlikely(!claim_id_is_set(claim_id))) {
        netdata_log_error("This should never be called if agent not claimed");
        return;
    }

    t->topic = mallocz(strlen(t->topic_recvd) + 1 - strlen(CLAIM_ID_REPLACE_TAG) + strlen(claim_id.str));
    memcpy(t->topic, t->topic_recvd, replace_tag - t->topic_recvd);
    dest = t->topic + (replace_tag - t->topic_recvd);

    memcpy(dest, claim_id.str, strlen(claim_id.str));
    dest += strlen(claim_id.str);
    replace_tag += strlen(CLAIM_ID_REPLACE_TAG);
    strcpy(dest, replace_tag);
    dest += strlen(replace_tag);
    *dest = 0;
}

static int topic_cache_add_topic(struct json_object *json, struct aclk_topic *topic)
{
    struct json_object_iterator it;
    struct json_object_iterator itEnd;

    it = json_object_iter_begin(json);
    itEnd = json_object_iter_end(json);

    while (!json_object_iter_equal(&it, &itEnd)) {
        if (!strcmp(json_object_iter_peek_name(&it), JSON_TOPIC_KEY_NAME)) {
            if (json_object_get_type(json_object_iter_peek_value(&it)) != json_type_string) {
                netdata_log_error("topic dictionary key \"" JSON_TOPIC_KEY_NAME "\" is expected to be json_type_string");
                return 1;
            }
            topic->topic_id = topic_name_to_id(json_object_get_string(json_object_iter_peek_value(&it)));
            if (topic->topic_id == ACLK_TOPICID_UNKNOWN) {
                netdata_log_debug(D_ACLK, "topic dictionary has unknown topic name \"%s\"", json_object_get_string(json_object_iter_peek_value(&it)));
            }
            json_object_iter_next(&it);
            continue;
        }
        if (!strcmp(json_object_iter_peek_name(&it), JSON_TOPIC_KEY_TOPIC)) {
            if (json_object_get_type(json_object_iter_peek_value(&it)) != json_type_string) {
                netdata_log_error("topic dictionary key \"" JSON_TOPIC_KEY_TOPIC "\" is expected to be json_type_string");
                return 1;
            }
            topic->topic_recvd = strdupz(json_object_get_string(json_object_iter_peek_value(&it)));
            json_object_iter_next(&it);
            continue;
        }

        netdata_log_error("topic dictionary has Unknown/Unexpected key \"%s\" in topic description. Ignoring!", json_object_iter_peek_name(&it));
        json_object_iter_next(&it);
    }

    if (!topic->topic_recvd) {
        netdata_log_error("topic dictionary Missig compulsory key %s", JSON_TOPIC_KEY_TOPIC);
        return 1;
    }

    topic_generate_final(topic);
    aclk_topic_cache_items++;

    return 0;
}

int aclk_generate_topic_cache(struct json_object *json)
{
    json_object *obj;

    size_t array_size = json_object_array_length(json);
    if (!array_size) {
        netdata_log_error("Empty topic list!");
        return 1;
    }

    if (aclk_topic_cache)
        free_topic_cache();

    aclk_topic_cache = callocz(array_size, sizeof(struct aclk_topic *));

    for (size_t i = 0; i < array_size; i++) {
        obj = json_object_array_get_idx(json, i);
        if (json_object_get_type(obj) != json_type_object) {
            netdata_log_error("expected json_type_object");
            return 1;
        }
        aclk_topic_cache[i] = callocz(1, sizeof(struct aclk_topic));
        if (topic_cache_add_topic(obj, aclk_topic_cache[i])) {
            netdata_log_error("failed to parse topic @idx=%d", (int)i);
            return 1;
        }
    }

    for (int i = 0; compulsory_topics[i] != ACLK_TOPICID_UNKNOWN; i++) {
        if (!aclk_get_topic(compulsory_topics[i])) {
            netdata_log_error("missing compulsory topic \"%s\" in password response from cloud", topic_id_to_name(compulsory_topics[i]));
            return 1;
        }
    }

    return 0;
}

/*
 * Build a topic based on sub_topic and final_topic
 * if the sub topic starts with / assume that is an absolute topic
 *
 */
const char *aclk_get_topic(enum aclk_topics topic)
{
    if (!aclk_topic_cache) {
        netdata_log_error("Topic cache not initialized");
        return NULL;
    }

    for (size_t i = 0; i < aclk_topic_cache_items; i++) {
        if (aclk_topic_cache[i]->topic_id == topic)
            return aclk_topic_cache[i]->topic;
    }
    netdata_log_error("Unknown topic");
    return NULL;
}

/*
 * Allows iterating all topics in topic cache without
 * having to resort to callbacks. 
 */

const char *aclk_topic_cache_iterate(size_t *iter)
{
    if (!aclk_topic_cache) {
        netdata_log_error("Topic cache not initialized when %s was called.", __FUNCTION__);
        return NULL;
    }

    if (*iter >= aclk_topic_cache_items)
        return NULL;

    return aclk_topic_cache[(*iter)++]->topic;
}

/*
 * TBEB with randomness
 *
 * @param reset 1 - to reset the delay,
 *              0 - to advance a step and calculate sleep time in ms
 * @param min, max in seconds
 * @returns delay in ms
 *
 */

unsigned long int aclk_tbeb_delay(int reset, int base, unsigned long int mins_ms, unsigned long int min_ms) {
    static int attempt = -1;

    if (reset) {
        attempt = -1;
        return 0;
    }

    attempt++;

    if (attempt == 0)
        return 0;

    unsigned long int delay = pow(base, attempt - 1);
    delay *= MSEC_PER_SEC;

    delay += (os_random32() % (MAX(1000, delay/2)));

    // Note: this is a bug, the value expected from the env backoff payload should be in seconds
    // but the code here is in milliseconds. To avoid confusion the cloud will be sending the value
    // in milliseconds so that the code will work as expected.
    if (delay <= mins_ms * MSEC_PER_SEC)
        return mins_ms;

    if (delay >= min_ms * MSEC_PER_SEC)
        return min_ms;

    return delay;
}

static inline int aclk_parse_userpass_pair(const char *src, const char c, char **a, char **b)
{
    const char *ptr = strchr(src, c);
    if (ptr == NULL)
        return 1;

    char *tmp_a = callocz(1, ptr - src + 1);
    memcpy(tmp_a, src, ptr - src);

    char *decoded_a = callocz(1, ptr - src + 1);
    url_decode_r(decoded_a, tmp_a, ptr - src + 1);
    freez(tmp_a);
    *a = decoded_a;

    char *tmp_b = strdupz(ptr+1);
    char *decoded_b = callocz(1, strlen(tmp_b) + 1);
    url_decode_r(decoded_b, tmp_b, strlen(tmp_b) + 1);
    freez(tmp_b);
    *b = decoded_b;

    return 0;
}

#define HTTP_PROXY_PREFIX "http://"
#define SOCKS5_PROXY_PREFIX "socks5://"
#define SOCKS5H_PROXY_PREFIX "socks5h://"
void aclk_set_proxy(char **ohost, int *port, char **uname, char **pwd,
    char **log_proxy, enum mqtt_wss_proxy_type *type)
{
    ACLK_PROXY_TYPE pt;
    const char *ptr = aclk_get_proxy(&pt, false);
    *log_proxy = (char *) aclk_get_proxy(&pt, true);
    char *tmp;
    const char *prefix = NULL;
    int default_port = 0;

    if (pt != PROXY_TYPE_HTTP && pt != PROXY_TYPE_SOCKS5 && pt != PROXY_TYPE_SOCKS5H)
        return;

    *uname = NULL;
    *pwd = NULL;
    *port = 0;

    char *proxy = strdupz(ptr);
    ptr = proxy;

    switch (pt) {
        case PROXY_TYPE_HTTP:
            prefix = HTTP_PROXY_PREFIX;
            default_port = 8080;
            if (type)
                *type = MQTT_WSS_PROXY_HTTP;
            break;
        case PROXY_TYPE_SOCKS5:
            prefix = SOCKS5_PROXY_PREFIX;
            default_port = 1080;
            if (type)
                *type = MQTT_WSS_PROXY_SOCKS5;
            break;
        case PROXY_TYPE_SOCKS5H:
            prefix = SOCKS5H_PROXY_PREFIX;
            default_port = 1080;
            if (type)
                *type = MQTT_WSS_PROXY_SOCKS5H;
            break;
        default:
            aclk_sensitive_free(&proxy);
            return;
    }

    if (!strncmp(ptr, prefix, strlen(prefix)))
        ptr += strlen(prefix);

    if ((tmp = strchr(ptr, '@'))) {
        *tmp = 0;
        if(aclk_parse_userpass_pair(ptr, ':', uname, pwd)) {
            error_report("Failed to get username and password for proxy. Will attempt connection without authentication");
        }
        ptr = tmp+1;
    }

    if (!*ptr) {
        aclk_sensitive_free(&proxy);
        freez(*uname);
        aclk_sensitive_free(pwd);
        return;
    }

    if ((tmp = strchr(ptr, ':'))) {
        *tmp = 0;
        tmp++;
        if(*tmp)
            *port = atoi(tmp);
    }
    *ohost = strdupz(ptr);

    if (*port <= 0 || *port > 65535)
        *port = default_port;

    if (!type) {
        freez(*uname);
        aclk_sensitive_free(pwd);
    }

    aclk_sensitive_free(&proxy);
}

enum mqtt_wss_proxy_type aclk_proxy_type_from_scheme(const char *proxy_url)
{
    if (!proxy_url || !*proxy_url)
        return MQTT_WSS_DIRECT;

    if (!strncmp(proxy_url, SOCKS5H_PROXY_PREFIX, strlen(SOCKS5H_PROXY_PREFIX)))
        return MQTT_WSS_PROXY_SOCKS5H;

    if (!strncmp(proxy_url, SOCKS5_PROXY_PREFIX, strlen(SOCKS5_PROXY_PREFIX)))
        return MQTT_WSS_PROXY_SOCKS5;

    if (!strncmp(proxy_url, HTTP_PROXY_PREFIX, strlen(HTTP_PROXY_PREFIX)))
        return MQTT_WSS_PROXY_HTTP;

    return MQTT_WSS_DIRECT;
}

const char *aclk_mqtt_proxy_type_to_scheme(enum mqtt_wss_proxy_type type)
{
    switch (type) {
        case MQTT_WSS_PROXY_HTTP:
            return HTTP_PROXY_PREFIX;
        case MQTT_WSS_PROXY_SOCKS5:
            return SOCKS5_PROXY_PREFIX;
        case MQTT_WSS_PROXY_SOCKS5H:
            return SOCKS5H_PROXY_PREFIX;
        default:
            return "";
    }
}

static int aclk_poll_for_io(int fd, short events, int timeout_ms)
{
    struct pollfd pfd = {
        .fd = fd,
        .events = events,
        .revents = 0
    };

    int rc;
    do {
        rc = poll(&pfd, 1, timeout_ms);
    } while (rc < 0 && errno == EINTR);

    if (rc <= 0)
        return rc;

    if ((pfd.revents & (POLLERR | POLLHUP | POLLNVAL)) != 0)
        return -1;

    return 1;
}

static int aclk_timeout_remaining_ms(usec_t start, int timeout_ms)
{
    if (timeout_ms <= 0)
        return 0;

    usec_t elapsed_ms = (now_monotonic_usec() - start) / USEC_PER_MS;
    if (elapsed_ms >= (usec_t)timeout_ms)
        return 0;

    return timeout_ms - (int)elapsed_ms;
}

static int aclk_write_all_timeout(int fd, const void *buf, size_t len, int timeout_ms)
{
    size_t written = 0;
    usec_t start = now_monotonic_usec();

    while (written < len) {
        int remaining_ms = aclk_timeout_remaining_ms(start, timeout_ms);
        if (remaining_ms <= 0)
            return 1;

        int rc = aclk_poll_for_io(fd, POLLOUT, remaining_ms);
        if (rc <= 0)
            return 1;

        ssize_t n = write(fd, ((const uint8_t *)buf) + written, len - written);
        if (n < 0) {
            if (errno == EAGAIN || errno == EWOULDBLOCK || errno == EINTR) {
                if ((now_monotonic_usec() - start) / USEC_PER_MS > (usec_t)timeout_ms)
                    return 1;
                continue;
            }
            return 1;
        }
        written += (size_t)n;
    }

    return 0;
}

static int aclk_read_exact_timeout(int fd, void *buf, size_t len, int timeout_ms)
{
    size_t got = 0;
    usec_t start = now_monotonic_usec();

    while (got < len) {
        int remaining_ms = aclk_timeout_remaining_ms(start, timeout_ms);
        if (remaining_ms <= 0)
            return 1;

        int rc = aclk_poll_for_io(fd, POLLIN, remaining_ms);
        if (rc <= 0)
            return 1;

        ssize_t n = read(fd, ((uint8_t *)buf) + got, len - got);
        if (n == 0)
            return 1;
        if (n < 0) {
            if (errno == EAGAIN || errno == EWOULDBLOCK || errno == EINTR) {
                if ((now_monotonic_usec() - start) / USEC_PER_MS > (usec_t)timeout_ms)
                    return 1;
                continue;
            }
            return 1;
        }
        got += (size_t)n;
    }

    return 0;
}

static int aclk_http_proxy_negotiate(int sockfd, const char *proxy_username, const char *proxy_password,
                                     const char *target_host, int target_port, int timeout_ms)
{
    int result = 1;
    bool has_creds = (proxy_username && *proxy_username);
    char req[4096];
    size_t off = 0;
    int rc = snprintf(req + off, sizeof(req) - off, "CONNECT %s:%d HTTP/1.1\r\nHost: %s\r\n",
                      target_host, target_port, target_host);
    if (rc < 0 || (size_t)rc >= sizeof(req) - off)
        goto cleanup;
    off += (size_t)rc;

    if (has_creds) {
        size_t pass_len = proxy_password ? strlen(proxy_password) : 0;
        size_t creds_plain_len = strlen(proxy_username) + pass_len + 1;
        char *creds_plain = callocz(1, creds_plain_len + 1);
        snprintfz(creds_plain, creds_plain_len + 1, "%s:%s", proxy_username, proxy_password ? proxy_password : "");

        int creds_base64_len = (((4 * (int)creds_plain_len / 3) + 3) & ~3);
        creds_base64_len += (1 + (creds_base64_len / 64)) * (int)strlen("\n");
        char *creds_base64 = callocz(1, (size_t)creds_base64_len + 1);
        (void)netdata_base64_encode((unsigned char *)creds_base64, (unsigned char *)creds_plain, creds_plain_len);

        rc = snprintf(req + off, sizeof(req) - off, "Proxy-Authorization: Basic %s\r\n", creds_base64);
        aclk_sensitive_free(&creds_plain);
        aclk_sensitive_free(&creds_base64);
        if (rc < 0 || (size_t)rc >= sizeof(req) - off)
            goto cleanup;
        off += (size_t)rc;
    }

    if (off + 2 >= sizeof(req))
        goto cleanup;
    req[off++] = '\r';
    req[off++] = '\n';

    if (aclk_write_all_timeout(sockfd, req, off, timeout_ms))
        goto cleanup;

    char resp[8192];
    size_t used = 0;
    usec_t start = now_monotonic_usec();
    while (used < sizeof(resp) - 1) {
        int remaining_ms = aclk_timeout_remaining_ms(start, timeout_ms);
        if (remaining_ms <= 0)
            goto cleanup;

        int prc = aclk_poll_for_io(sockfd, POLLIN, remaining_ms);
        if (prc <= 0)
            goto cleanup;

        ssize_t n = read(sockfd, resp + used, sizeof(resp) - 1 - used);
        if (n == 0)
            goto cleanup;
        if (n < 0) {
            if (errno == EAGAIN || errno == EWOULDBLOCK || errno == EINTR)
                continue;
            goto cleanup;
        }
        used += (size_t)n;
        resp[used] = '\0';
        if (strstr(resp, "\r\n\r\n"))
            break;
    }

    if (!strstr(resp, "\r\n\r\n"))
        goto cleanup;

    if (strncmp(resp, "HTTP/1.1 ", 9) != 0 && strncmp(resp, "HTTP/1.0 ", 9) != 0)
        goto cleanup;

    int status = atoi(resp + 9);
    result = (status == 200) ? 0 : 1;

cleanup:
    if (has_creds)
        aclk_sensitive_memzero(req, sizeof(req));
    return result;
}

static int aclk_socks5_resolve_local(const char *host, uint8_t *atype, uint8_t *addr, size_t *addr_len)
{
    struct in_addr ipv4;
    struct in6_addr ipv6;
    if (inet_pton(AF_INET, host, &ipv4) == 1) {
        *atype = 0x01;
        memcpy(addr, &ipv4, sizeof(ipv4));
        *addr_len = sizeof(ipv4);
        return 0;
    }
    if (inet_pton(AF_INET6, host, &ipv6) == 1) {
        *atype = 0x04;
        memcpy(addr, &ipv6, sizeof(ipv6));
        *addr_len = sizeof(ipv6);
        return 0;
    }

    struct addrinfo hints = {
        .ai_family = AF_UNSPEC,
        .ai_socktype = SOCK_STREAM,
        .ai_flags = AI_ADDRCONFIG
    };
    struct addrinfo *res = NULL;
    if (getaddrinfo(host, NULL, &hints, &res) != 0 || !res)
        return 1;

    int rc = 1;

    // Prefer IPv4 for compatibility with SOCKS proxies that don't accept ATYP=0x04 (IPv6).
    for (struct addrinfo *it = res; it; it = it->ai_next) {
        if (it->ai_family == AF_INET && it->ai_addrlen >= sizeof(struct sockaddr_in)) {
            struct sockaddr_in *sa = (struct sockaddr_in *)it->ai_addr;
            *atype = 0x01;
            memcpy(addr, &sa->sin_addr, sizeof(sa->sin_addr));
            *addr_len = sizeof(sa->sin_addr);
            rc = 0;
            break;
        }
    }

    if (rc != 0) {
        for (struct addrinfo *it = res; it; it = it->ai_next) {
            if (it->ai_family == AF_INET6 && it->ai_addrlen >= sizeof(struct sockaddr_in6)) {
                struct sockaddr_in6 *sa = (struct sockaddr_in6 *)it->ai_addr;
                *atype = 0x04;
                memcpy(addr, &sa->sin6_addr, sizeof(sa->sin6_addr));
                *addr_len = sizeof(sa->sin6_addr);
                rc = 0;
                break;
            }
        }
    }

    freeaddrinfo(res);
    return rc;
}

static int aclk_socks5_proxy_negotiate(int sockfd, enum mqtt_wss_proxy_type proxy_type,
                                       const char *proxy_username, const char *proxy_password,
                                       const char *target_host, int target_port, int timeout_ms)
{
    usec_t start = now_monotonic_usec();

    uint8_t greeting[4] = { 0x05, 0x01, 0x00, 0x00 };
    size_t greeting_len = 3;
    if (proxy_username && *proxy_username) {
        greeting[1] = 0x02;
        greeting[2] = 0x00;
        greeting[3] = 0x02;
        greeting_len = 4;
    }

    if (aclk_write_all_timeout(sockfd, greeting, greeting_len, aclk_timeout_remaining_ms(start, timeout_ms))) {
        return 1;
    }

    uint8_t greeting_reply[2];
    if (aclk_read_exact_timeout(sockfd, greeting_reply, sizeof(greeting_reply), aclk_timeout_remaining_ms(start, timeout_ms))) {
        return 1;
    }
    if (greeting_reply[0] != 0x05 || greeting_reply[1] == 0xFF) {
        netdata_log_error("ACLK: SOCKS5 proxy rejected methods (ver=0x%02x method=0x%02x)",
                          greeting_reply[0], greeting_reply[1]);
        return 1;
    }

    if (greeting_reply[1] == 0x02) {
        if (!proxy_username || !*proxy_username) {
            netdata_log_error("ACLK: SOCKS5 proxy requires username/password auth but credentials are missing");
            return 1;
        }
        size_t user_len = strlen(proxy_username);
        size_t pass_len = proxy_password ? strlen(proxy_password) : 0;
        if (user_len > UINT8_MAX || pass_len > UINT8_MAX) {
            netdata_log_error("ACLK: SOCKS5 credentials exceed protocol limits");
            return 1;
        }

        uint8_t auth_req[513];
        size_t pos = 0;
        auth_req[pos++] = 0x01;
        auth_req[pos++] = (uint8_t)user_len;
        memcpy(auth_req + pos, proxy_username, user_len);
        pos += user_len;
        auth_req[pos++] = (uint8_t)pass_len;
        if (pass_len) {
            memcpy(auth_req + pos, proxy_password, pass_len);
            pos += pass_len;
        }

        int auth_rc = aclk_write_all_timeout(sockfd, auth_req, pos, aclk_timeout_remaining_ms(start, timeout_ms));
        aclk_sensitive_memzero(auth_req, sizeof(auth_req));
        if (auth_rc) {
            return 1;
        }

        uint8_t auth_reply[2];
        if (aclk_read_exact_timeout(sockfd, auth_reply, sizeof(auth_reply), aclk_timeout_remaining_ms(start, timeout_ms))) {
            return 1;
        }
        if (auth_reply[0] != 0x01 || auth_reply[1] != 0x00) {
            netdata_log_error("ACLK: SOCKS5 auth failed (ver=0x%02x status=0x%02x)",
                              auth_reply[0], auth_reply[1]);
            return 1;
        }
    } else if (greeting_reply[1] != 0x00) {
        netdata_log_error("ACLK: SOCKS5 proxy selected unsupported method 0x%02x", greeting_reply[1]);
        return 1;
    }

    uint8_t connect_req[300];
    size_t pos = 0;
    connect_req[pos++] = 0x05;
    connect_req[pos++] = 0x01;
    connect_req[pos++] = 0x00;

    if (proxy_type == MQTT_WSS_PROXY_SOCKS5H) {
        size_t host_len = strlen(target_host);
        if (host_len == 0 || host_len > UINT8_MAX) {
            netdata_log_error("ACLK: SOCKS5H target hostname length invalid (%zu)", host_len);
            return 1;
        }
        connect_req[pos++] = 0x03;
        connect_req[pos++] = (uint8_t)host_len;
        memcpy(connect_req + pos, target_host, host_len);
        pos += host_len;
    } else {
        uint8_t atyp = 0;
        uint8_t addr[16];
        size_t addr_len = 0;
        if (aclk_socks5_resolve_local(target_host, &atyp, addr, &addr_len)) {
            netdata_log_error("ACLK: SOCKS5 local DNS resolution failed for target host '%s'", target_host);
            return 1;
        }
        connect_req[pos++] = atyp;
        memcpy(connect_req + pos, addr, addr_len);
        pos += addr_len;
    }

    connect_req[pos++] = (uint8_t)((target_port >> 8) & 0xFF);
    connect_req[pos++] = (uint8_t)(target_port & 0xFF);

    if (aclk_write_all_timeout(sockfd, connect_req, pos, aclk_timeout_remaining_ms(start, timeout_ms))) {
        return 1;
    }

    uint8_t reply_hdr[4];
    if (aclk_read_exact_timeout(sockfd, reply_hdr, sizeof(reply_hdr), aclk_timeout_remaining_ms(start, timeout_ms))) {
        return 1;
    }
    if (reply_hdr[0] != 0x05 || reply_hdr[1] != 0x00) {
        netdata_log_error("ACLK: SOCKS5 CONNECT failed (ver=0x%02x rep=0x%02x atyp=0x%02x)",
                          reply_hdr[0], reply_hdr[1], reply_hdr[3]);
        return 1;
    }

    size_t to_read = 0;
    switch (reply_hdr[3]) {
        case 0x01:
            to_read = 4 + 2;
            break;
        case 0x03: {
            uint8_t domain_len = 0;
            if (aclk_read_exact_timeout(sockfd, &domain_len, 1, aclk_timeout_remaining_ms(start, timeout_ms))) {
                return 1;
            }
            to_read = (size_t)domain_len + 2;
            break;
        }
        case 0x04:
            to_read = 16 + 2;
            break;
        default:
            netdata_log_error("ACLK: SOCKS5 CONNECT reply has invalid ATYP 0x%02x", reply_hdr[3]);
            return 1;
    }

    uint8_t discard[260];
    if (to_read > sizeof(discard)) {
        return 1;
    }

    if (aclk_read_exact_timeout(sockfd, discard, to_read, aclk_timeout_remaining_ms(start, timeout_ms))) {
        return 1;
    }

    return 0;
}

int aclk_proxy_negotiation_connect(int sockfd, enum mqtt_wss_proxy_type proxy_type,
                                   const char *proxy_username, const char *proxy_password,
                                   const char *target_host, int target_port, int timeout_ms)
{
    if (proxy_type == MQTT_WSS_DIRECT)
        return 0;

    if (proxy_type == MQTT_WSS_PROXY_HTTP)
        return aclk_http_proxy_negotiate(sockfd, proxy_username, proxy_password, target_host, target_port, timeout_ms);

    if (proxy_type == MQTT_WSS_PROXY_SOCKS5 || proxy_type == MQTT_WSS_PROXY_SOCKS5H)
        return aclk_socks5_proxy_negotiate(sockfd, proxy_type, proxy_username, proxy_password, target_host, target_port, timeout_ms);

    return 1;
}
