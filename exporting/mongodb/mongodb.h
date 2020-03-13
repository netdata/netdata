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

extern int mongodb_init(const char *uri_string, const char *database_string, const char *collection_string, const int32_t socket_timeout);

extern int mongodb_insert(char *data, size_t n_metrics);

extern void mongodb_cleanup();

int init_mongodb_instance(struct instance *instance);
void mongodb_connector_worker(void *instance_p);

#endif //NETDATA_EXPORTING_MONGODB_H
