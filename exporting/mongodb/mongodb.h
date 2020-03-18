// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_EXPORTING_MONGODB_H
#define NETDATA_EXPORTING_MONGODB_H

#include "exporting/exporting_engine.h"
#include "exporting/json/json.h"
#include <mongoc.h>

struct mongodb_specific_data {
    mongoc_client_t *client;
    mongoc_collection_t *collection;
};

extern int mongodb_init(struct instance *instance);

extern int mongodb_insert(struct instance *instance, char *data, size_t n_metrics);

extern void mongodb_cleanup(struct instance *instance);

int init_mongodb_instance(struct instance *instance);
void mongodb_connector_worker(void *instance_p);

#endif //NETDATA_EXPORTING_MONGODB_H
