// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef ACLK_SCHEMA_WRAPPER_NODE_CONNECTION_H
#define ACLK_SCHEMA_WRAPPER_NODE_CONNECTION_H

#include "capability.h"

#ifdef __cplusplus
extern "C" {
#endif

typedef struct {
    const char* claim_id;
    const char* node_id;

    unsigned int live:1;
    unsigned int queryable:1;

    int64_t session_id;

    int32_t hops;
    const struct capability *capabilities;
} node_instance_connection_t;

char *generate_node_instance_connection(size_t *len, const node_instance_connection_t *data);


#ifdef __cplusplus
}
#endif

#endif /* ACLK_SCHEMA_WRAPPER_NODE_CONNECTION_H */
