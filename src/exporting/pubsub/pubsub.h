// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_EXPORTING_PUBSUB_H
#define NETDATA_EXPORTING_PUBSUB_H

#include "exporting/exporting_engine.h"
#include "exporting/json/json.h"
#include "pubsub_publish.h"

int init_pubsub_instance(struct instance *instance);
void clean_pubsub_instance(struct instance *instance);
void pubsub_connector_worker(void *instance_p);

#endif //NETDATA_EXPORTING_PUBSUB_H
