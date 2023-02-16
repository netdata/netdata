// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef ACLK_SCHEMA_WRAPPER_NODE_CREATION_H
#define ACLK_SCHEMA_WRAPPER_NODE_CREATION_H

#ifdef __cplusplus
extern "C" {
#endif

typedef struct {
    const char *claim_id;
    const char *machine_guid;
    const char *hostname;

    int32_t hops;
} node_instance_creation_t;

typedef struct {
    char *node_id;
    char *machine_guid;
} node_instance_creation_result_t;

char *generate_node_instance_creation(size_t *len, const node_instance_creation_t *data);
node_instance_creation_result_t parse_create_node_instance_result(const char *data, size_t len);


#ifdef __cplusplus
}
#endif

#endif /* ACLK_SCHEMA_WRAPPER_NODE_CREATION_H */
