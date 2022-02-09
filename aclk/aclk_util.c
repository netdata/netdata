// SPDX-License-Identifier: GPL-3.0-or-later

#include "aclk_util.h"

#include "daemon/common.h"

usec_t aclk_session_newarch = 0;

aclk_env_t *aclk_env = NULL;

int chart_batch_id;

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
#if !defined(HAVE_C___ATOMIC) || defined(NETDATA_NO_ATOMIC_INSTRUCTIONS)
netdata_mutex_t aclk_conversation_log_mutex = NETDATA_MUTEX_INITIALIZER;
int aclk_get_conv_log_next()
{
    int ret;
    netdata_mutex_lock(&aclk_conversation_log_mutex);
    ret = aclk_conversation_log_counter++;
    netdata_mutex_unlock(&aclk_conversation_log_mutex);
    return ret;
}
#endif
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
    { .id = ACLK_TOPICID_ALARM_LOG,             .name = "alarm-log"                },
    { .id = ACLK_TOPICID_ALARM_HEALTH,          .name = "alarm-health"             },
    { .id = ACLK_TOPICID_ALARM_CONFIG,          .name = "alarm-config"             },
    { .id = ACLK_TOPICID_ALARM_SNAPSHOT,        .name = "alarm-snapshot"           },
    { .id = ACLK_TOPICID_UNKNOWN,               .name = NULL                       }
};

enum aclk_topics compulsory_topics_new_cloud_arch[] = {
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
    ACLK_TOPICID_ALARM_HEALTH,
    ACLK_TOPICID_ALARM_CONFIG,
    ACLK_TOPICID_ALARM_SNAPSHOT,
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

    rrdhost_aclk_state_lock(localhost);
    if (unlikely(!localhost->aclk_state.claimed_id)) {
        error("This should never be called if agent not claimed");
        rrdhost_aclk_state_unlock(localhost);
        return;
    }

    t->topic = mallocz(strlen(t->topic_recvd) + 1 - strlen(CLAIM_ID_REPLACE_TAG) + strlen(localhost->aclk_state.claimed_id));
    memcpy(t->topic, t->topic_recvd, replace_tag - t->topic_recvd);
    dest = t->topic + (replace_tag - t->topic_recvd);

    memcpy(dest, localhost->aclk_state.claimed_id, strlen(localhost->aclk_state.claimed_id));
    dest += strlen(localhost->aclk_state.claimed_id);
    rrdhost_aclk_state_unlock(localhost);
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
                error("topic dictionary key \"" JSON_TOPIC_KEY_NAME "\" is expected to be json_type_string");
                return 1;
            }
            topic->topic_id = topic_name_to_id(json_object_get_string(json_object_iter_peek_value(&it)));
            if (topic->topic_id == ACLK_TOPICID_UNKNOWN) {
                debug(D_ACLK, "topic dictionary has unknown topic name \"%s\"", json_object_get_string(json_object_iter_peek_value(&it)));
            }
            json_object_iter_next(&it);
            continue;
        }
        if (!strcmp(json_object_iter_peek_name(&it), JSON_TOPIC_KEY_TOPIC)) {
            if (json_object_get_type(json_object_iter_peek_value(&it)) != json_type_string) {
                error("topic dictionary key \"" JSON_TOPIC_KEY_TOPIC "\" is expected to be json_type_string");
                return 1;
            }
            topic->topic_recvd = strdupz(json_object_get_string(json_object_iter_peek_value(&it)));
            json_object_iter_next(&it);
            continue;
        }

        error("topic dictionary has Unknown/Unexpected key \"%s\" in topic description. Ignoring!", json_object_iter_peek_name(&it));
        json_object_iter_next(&it);
    }

    if (!topic->topic_recvd) {
        error("topic dictionary Missig compulsory key %s", JSON_TOPIC_KEY_TOPIC);
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
        error("Empty topic list!");
        return 1;
    }

    if (aclk_topic_cache)
        free_topic_cache();

    aclk_topic_cache = callocz(array_size, sizeof(struct aclk_topic *));

    for (size_t i = 0; i < array_size; i++) {
        obj = json_object_array_get_idx(json, i);
        if (json_object_get_type(obj) != json_type_object) {
            error("expected json_type_object");
            return 1;
        }
        aclk_topic_cache[i] = callocz(1, sizeof(struct aclk_topic));
        if (topic_cache_add_topic(obj, aclk_topic_cache[i])) {
            error("failed to parse topic @idx=%d", (int)i);
            return 1;
        }
    }

    enum aclk_topics *compulsory_topics = compulsory_topics_new_cloud_arch;

    for (int i = 0; compulsory_topics[i] != ACLK_TOPICID_UNKNOWN; i++) {
        if (!aclk_get_topic(compulsory_topics[i])) {
            error("missing compulsory topic \"%s\" in password response from cloud", topic_id_to_name(compulsory_topics[i]));
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
        error("Topic cache not initialized");
        return NULL;
    }

    for (size_t i = 0; i < aclk_topic_cache_items; i++) {
        if (aclk_topic_cache[i]->topic_id == topic)
            return aclk_topic_cache[i]->topic;
    }
    error("Unknown topic");
    return NULL;
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

unsigned long int aclk_tbeb_delay(int reset, int base, unsigned long int min, unsigned long int max) {
    static int attempt = -1;

    if (reset) {
        attempt = -1;
        return 0;
    }

    attempt++;

    if (attempt == 0) {
        srandom(time(NULL));
        return 0;
    }

    unsigned long int delay = pow(base, attempt - 1);
    delay *= MSEC_PER_SEC;

    delay += (random() % (MAX(1000, delay/2)));

    if (delay <= min * MSEC_PER_SEC)
        return min;

    if (delay >= max * MSEC_PER_SEC)
        return max;

    return delay;
}


#define HTTP_PROXY_PREFIX "http://"
void aclk_set_proxy(char **ohost, int *port, enum mqtt_wss_proxy_type *type)
{
    ACLK_PROXY_TYPE pt;
    const char *ptr = aclk_get_proxy(&pt);
    char *tmp;
    char *host;
    if (pt != PROXY_TYPE_HTTP)
        return;

    *port = 0;

    if (!strncmp(ptr, HTTP_PROXY_PREFIX, strlen(HTTP_PROXY_PREFIX)))
        ptr += strlen(HTTP_PROXY_PREFIX);

    if ((tmp = strchr(ptr, '@')))
        ptr = tmp;

    if ((tmp = strchr(ptr, '/'))) {
        host = mallocz((tmp - ptr) + 1);
        memcpy(host, ptr, (tmp - ptr));
        host[tmp - ptr] = 0;
    } else
        host = strdupz(ptr);

    if ((tmp = strchr(host, ':'))) {
        *tmp = 0;
        tmp++;
        *port = atoi(tmp);
    }

    if (*port <= 0 || *port > 65535)
        *port = 8080;

    *ohost = host;

    if (type)
        *type = MQTT_WSS_PROXY_HTTP;
}
