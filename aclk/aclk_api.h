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

extern int use_mqtt_5;
extern int aclk_ctx_based;

#ifdef ENABLE_ACLK

void aclk_host_state_update(RRDHOST *host, int connect);
#endif

void add_aclk_host_labels(void);
char *aclk_state(void);
char *aclk_state_json(void);

#endif /* ACLK_API_H */
