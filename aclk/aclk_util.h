// SPDX-License-Identifier: GPL-3.0-or-later
#ifndef ACLK_UTIL_H
#define ACLK_UTIL_H

#include "libnetdata/libnetdata.h"

// Helper stuff which should not have any further inside ACLK dependency
// and are supposed not to be needed outside of ACLK

int aclk_decode_base_url(char *url, char **aclk_hostname, int *aclk_port);

enum aclk_topics {
    ACLK_TOPICID_CHART    = 0,
    ACLK_TOPICID_ALARMS   = 1,
    ACLK_TOPICID_METADATA = 2,
    ACLK_TOPICID_COMMAND  = 3,
};

const char *aclk_get_topic(enum aclk_topics topic);
void free_topic_cache(void);
// TODO
// aclk_topics_reload //when claim id changes

#ifdef ACLK_LOG_CONVERSATION_DIR
extern volatile int aclk_conversation_log_counter;
#if defined(HAVE_C___ATOMIC) && !defined(NETDATA_NO_ATOMIC_INSTRUCTIONS)
#define ACLK_GET_CONV_LOG_NEXT() __atomic_fetch_add(&aclk_conversation_log_counter, 1, __ATOMIC_SEQ_CST)
#else
extern netdata_mutex_t aclk_conversation_log_mutex;
int aclk_get_conv_log_next();
#define ACLK_GET_CONV_LOG_NEXT() aclk_get_conv_log_next()
#endif
#endif

#endif /* ACLK_UTIL_H */
