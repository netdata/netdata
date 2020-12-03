#include "aclk_util.h"

#include <stdio.h>

#include "../daemon/common.h"

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
    const char *topic_suffix;
    char *topic;
};

// This helps to cache finalized topics (assembled with claim_id)
// to not have to alloc or create buffer and construct topic every
// time message is sent as in old ACLK
static struct aclk_topic aclk_topic_cache[] = {
    { .topic_suffix = "outbound/meta",   .topic = NULL }, // ACLK_TOPICID_CHART
    { .topic_suffix = "outbound/alarms", .topic = NULL }, // ACLK_TOPICID_ALARMS
    { .topic_suffix = "outbound/meta",   .topic = NULL }, // ACLK_TOPICID_METADATA
    { .topic_suffix = "inbound/cmd",     .topic = NULL }, // ACLK_TOPICID_COMMAND
    { .topic_suffix = NULL,              .topic = NULL }
};

void free_topic_cache(void)
{
    struct aclk_topic *tc = aclk_topic_cache;
    while (tc->topic_suffix) {
        if (tc->topic) {
            freez(tc->topic);
            tc->topic = NULL;
        }
        tc++;
    }
}

static inline void generate_topic_cache(void)
{
    struct aclk_topic *tc = aclk_topic_cache;
    char *ptr;
    if (unlikely(!tc->topic)) {
        rrdhost_aclk_state_lock(localhost);
        while(tc->topic_suffix) {
            tc->topic = mallocz(strlen(ACLK_TOPIC_PREFIX) + (UUID_STR_LEN - 1) + 2 /* '/' and \0 */ + strlen(tc->topic_suffix));
            ptr = tc->topic;
            strcpy(ptr, ACLK_TOPIC_PREFIX);
            ptr += strlen(ACLK_TOPIC_PREFIX);
            strcpy(ptr, localhost->aclk_state.claimed_id);
            ptr += (UUID_STR_LEN - 1);
            *ptr++ = '/';
            strcpy(ptr, tc->topic_suffix);
            tc++;
        }
        rrdhost_aclk_state_unlock(localhost);
    }
}

/*
 * Build a topic based on sub_topic and final_topic
 * if the sub topic starts with / assume that is an absolute topic
 *
 */
const char *aclk_get_topic(enum aclk_topics topic)
{
    generate_topic_cache();

    return aclk_topic_cache[topic].topic;
}

int aclk_decode_base_url(char *url, char **aclk_hostname, int *aclk_port)
{
    int pos = 0;
    if (!strncmp("https://", url, 8)) {
        pos = 8;
    } else if (!strncmp("http://", url, 7)) {
        error("Cannot connect ACLK over %s -> unencrypted link is not supported", url);
        return 1;
    }
    int host_end = pos;
    while (url[host_end] != 0 && url[host_end] != '/' && url[host_end] != ':')
        host_end++;
    if (url[host_end] == 0) {
        *aclk_hostname = strdupz(url + pos);
        *aclk_port = 443;
        info("Setting ACLK target host=%s port=%d from %s", *aclk_hostname, *aclk_port, url);
        return 0;
    }
    if (url[host_end] == ':') {
        *aclk_hostname = callocz(host_end - pos + 1, 1);
        strncpy(*aclk_hostname, url + pos, host_end - pos);
        int port_end = host_end + 1;
        while (url[port_end] >= '0' && url[port_end] <= '9')
            port_end++;
        if (port_end - host_end > 6) {
            error("Port specified in %s is invalid", url);
            freez(*aclk_hostname);
            *aclk_hostname = NULL;
            return 1;
        }
        *aclk_port = atoi(&url[host_end+1]);
    }
    if (url[host_end] == '/') {
        *aclk_port = 443;
        *aclk_hostname = callocz(1, host_end - pos + 1);
        strncpy(*aclk_hostname, url+pos, host_end - pos);
    }
    info("Setting ACLK target host=%s port=%d from %s", *aclk_hostname, *aclk_port, url);
    return 0;
}
