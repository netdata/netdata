// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_HEALTH_PROTOTYPES_H
#define NETDATA_HEALTH_PROTOTYPES_H

#include "health.h"

bool health_plugin_enabled(void);
void health_plugin_disable(void);

void health_load_config_defaults(void);

void health_reload_prototypes(void);
void health_apply_prototypes_to_host(RRDHOST *host);
void health_apply_prototypes_to_all_hosts(void);

#endif //NETDATA_HEALTH_PROTOTYPES_H
