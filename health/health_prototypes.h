// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_HEALTH_PROTOTYPES_H
#define NETDATA_HEALTH_PROTOTYPES_H

#include "health.h"

void health_init_prototypes(void);

bool health_plugin_enabled(void);
void health_plugin_disable(void);

void health_reload_prototypes(void);
void health_apply_prototypes_to_host(RRDHOST *host);
void health_apply_prototypes_to_all_hosts(void);

void health_prototype_alerts_for_rrdset_incrementally(RRDSET *st);

struct rrd_alert_config;
struct rrd_alert_match;
void health_prototype_copy_config(struct rrd_alert_config *dst, struct rrd_alert_config *src);
void health_prototype_copy_match_without_patterns(struct rrd_alert_match *dst, struct rrd_alert_match *src);
void health_prototype_reset_alerts_for_rrdset(RRDSET *st);

#endif //NETDATA_HEALTH_PROTOTYPES_H
