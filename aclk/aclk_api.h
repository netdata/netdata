// SPDX-License-Identifier: GPL-3.0-or-later
#ifndef ACLK_API_H
#define ACLK_API_H

#include "libnetdata/libnetdata.h"

#include "aclk_proxy.h"

// TODO get rid global vars as soon as
// ACLK Legacy is removed
extern int aclk_connected;
extern int aclk_kill_link;

extern usec_t aclk_session_us;
extern time_t aclk_session_sec;

extern int aclk_disable_runtime;

extern int aclk_stats_enabled;
extern int aclk_alert_reloaded;

extern int aclk_ng;
extern int use_mqtt_5;

#ifdef ENABLE_ACLK
void *aclk_starter(void *ptr);

#ifdef ENABLE_NEW_CLOUD_PROTOCOL
void aclk_host_state_update(RRDHOST *host, int connect);
#endif

#define NETDATA_ACLK_HOOK                                                                                              \
    { .name = "ACLK_Main",                                                                                             \
      .config_section = NULL,                                                                                          \
      .config_name = NULL,                                                                                             \
      .enabled = 1,                                                                                                    \
      .thread = NULL,                                                                                                  \
      .init_routine = NULL,                                                                                            \
      .start_routine = aclk_starter },

#endif

void add_aclk_host_labels(void);
char *aclk_state(void);
char *aclk_state_json(void);

#endif /* ACLK_API_H */
