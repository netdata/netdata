// SPDX-License-Identifier: GPL-3.0-or-later
#ifndef ACLK_H
#define ACLK_H

#include "daemon/common.h"
#include "aclk_util.h"
#include "aclk_rrdhost_state.h"

// How many MQTT PUBACKs we need to get to consider connection
// stable for the purposes of TBEB (truncated binary exponential backoff)
#define ACLK_PUBACKS_CONN_STABLE 3

extern time_t aclk_block_until;

extern int disconnect_req;

void *aclk_main(void *ptr);

extern netdata_mutex_t aclk_shared_state_mutex;
#define ACLK_SHARED_STATE_LOCK netdata_mutex_lock(&aclk_shared_state_mutex)
#define ACLK_SHARED_STATE_UNLOCK netdata_mutex_unlock(&aclk_shared_state_mutex)

extern struct aclk_shared_state {
    ACLK_AGENT_STATE agent_state;
    time_t last_popcorn_interrupt;

    // To wait for `disconnect` message PUBACK
    // when shutting down
    // at the same time if > 0 we know link is
    // shutting down
    int mqtt_shutdown_msg_id;
    int mqtt_shutdown_msg_rcvd;
} aclk_shared_state;

void aclk_alarm_reload(void);
int aclk_update_alarm(RRDHOST *host, ALARM_ENTRY *ae);

/* Informs ACLK about created/deleted chart
 * @param create 0 - if chart was deleted, other if chart created
 */
int aclk_update_chart(RRDHOST *host, char *chart_name, int create);

void aclk_add_collector(RRDHOST *host, const char *plugin_name, const char *module_name);
void aclk_del_collector(RRDHOST *host, const char *plugin_name, const char *module_name);

void aclk_host_state_update(RRDHOST *host, int cmd);

void aclk_send_node_instances(void);

void aclk_send_bin_msg(char *msg, size_t msg_len, enum aclk_topics subtopic, const char *msgname);

char *ng_aclk_state(void);
char *ng_aclk_state_json(void);

#endif /* ACLK_H */
