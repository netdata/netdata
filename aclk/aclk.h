// SPDX-License-Identifier: GPL-3.0-or-later
#ifndef ACLK_H
#define ACLK_H

typedef struct aclk_rrdhost_state {
    char *claimed_id; // Claimed ID if host has one otherwise NULL
} aclk_rrdhost_state;

#include "../daemon/common.h"
#include "aclk_util.h"

// minimum and maximum supported version of ACLK
// in this version of agent
#define ACLK_VERSION_MIN 2
#define ACLK_VERSION_MAX 2

// Version negotiation messages have they own versioning
// this is also used for LWT message as we set that up
// before version negotiation
#define ACLK_VERSION_NEG_VERSION 1

// Maximum time to wait for version negotiation before aborting
// and defaulting to oldest supported version
#define VERSION_NEG_TIMEOUT 3

#if ACLK_VERSION_MIN > ACLK_VERSION_MAX
#error "ACLK_VERSION_MAX must be >= than ACLK_VERSION_MIN"
#endif

// Define ACLK Feature Version Boundaries Here
#define ACLK_V_COMPRESSION 2

// How many MQTT PUBACKs we need to get to consider connection
// stable for the purposes of TBEB (truncated binary exponential backoff)
#define ACLK_PUBACKS_CONN_STABLE 3

// TODO get rid of this shit
extern int aclk_disable_runtime;
extern int aclk_disable_single_updates;
extern int aclk_kill_link;
extern int aclk_connected;

extern usec_t aclk_session_us;
extern time_t aclk_session_sec;

void *aclk_main(void *ptr);
void aclk_single_update_disable();
void aclk_single_update_enable();

#define NETDATA_ACLK_HOOK                                                                                              \
    { .name = "ACLK_Main",                                                                                             \
      .config_section = NULL,                                                                                          \
      .config_name = NULL,                                                                                             \
      .enabled = 1,                                                                                                    \
      .thread = NULL,                                                                                                  \
      .init_routine = NULL,                                                                                            \
      .start_routine = aclk_main },

extern netdata_mutex_t aclk_shared_state_mutex;
#define ACLK_SHARED_STATE_LOCK netdata_mutex_lock(&aclk_shared_state_mutex)
#define ACLK_SHARED_STATE_UNLOCK netdata_mutex_unlock(&aclk_shared_state_mutex)

typedef enum aclk_agent_state {
    AGENT_INITIALIZING,
    AGENT_STABLE
} ACLK_AGENT_STATE;
extern struct aclk_shared_state {
    ACLK_AGENT_STATE agent_state;
    time_t last_popcorn_interrupt;

    // read only while ACLK connected
    // protect by lock otherwise
    int version_neg;
    usec_t version_neg_wait_till;

    // To wait for `disconnect` message PUBACK
    // when shuting down
    // at the same time if > 0 we know link is
    // shutting down
    int mqtt_shutdown_msg_id;
    int mqtt_shutdown_msg_rcvd;
} aclk_shared_state;

void aclk_alarm_reload(void);
int aclk_update_alarm(RRDHOST *host, ALARM_ENTRY *ae);

// TODO this is for bacward compatibility with ACLK legacy
#define ACLK_CMD_CHART 1
#define ACLK_CMD_CHARTDEL 0
/* Informs ACLK about created/deleted chart
 * @param create 0 - if chart was deleted, other if chart created
 */
int aclk_update_chart(RRDHOST *host, char *chart_name, int create);

void aclk_add_collector(RRDHOST *host, const char *plugin_name, const char *module_name);
void aclk_del_collector(RRDHOST *host, const char *plugin_name, const char *module_name);

struct label *add_aclk_host_labels(struct label *label);

#endif /* ACLK_H */
