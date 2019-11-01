// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_EXPORTING_GRAPHITE_H
#define NETDATA_EXPORTING_GRAPHITE_H

#include "exporting/exporting_engine.h"

int format_dimension_collected_graphite_plaintext(struct instance *instance, RRDDIM *rd);
int init_graphite_connector(struct connector *connector);
int init_graphite_instance(struct instance *instance);
void graphite_connector_worker(void *instance_p);

#endif //NETDATA_EXPORTING_GRAPHITE_H
