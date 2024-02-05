// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_EXPORTING_MONGODB_H
#define NETDATA_EXPORTING_MONGODB_H

#include "exporting/exporting_engine.h"
#include "exporting/json/json.h"
#include <mongoc.h>

struct bson_buffer {
    bson_t **insert;
    size_t documents_inserted;
    size_t buffered_bytes;

    struct bson_buffer *next;
};

struct mongodb_specific_data {
    mongoc_client_t *client;
    mongoc_collection_t *collection;

    size_t total_documents_inserted;

    struct bson_buffer *first_buffer;
    struct bson_buffer *last_buffer;
};

int mongodb_init(struct instance *instance);
void mongodb_cleanup(struct instance *instance);

int init_mongodb_instance(struct instance *instance);
int format_batch_mongodb(struct instance *instance);
void mongodb_connector_worker(void *instance_p);

#endif //NETDATA_EXPORTING_MONGODB_H
