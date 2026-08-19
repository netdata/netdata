// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef ACLK_SCHEMA_WRAPPER_NODE_MANIFEST_H
#define ACLK_SCHEMA_WRAPPER_NODE_MANIFEST_H

#include <stdlib.h>
#include <stdint.h>

#include "database/rrd.h"

#ifdef __cplusplus
extern "C" {
#endif

struct update_node_instance_manifest {
    char *node_id;
    char *claim_id;
    struct timeval updated_at;

    // dictionary keyed by function name -> struct rrd_function_manifest_entry
    // (see database/rrdfunctions-exporters.h, built by host_functions_to_manifest_dict())
    DICTIONARY *functions;
};

char *generate_update_node_instance_manifest_message(size_t *len, struct update_node_instance_manifest *manifest);

#ifdef __cplusplus
}
#endif

#endif /* ACLK_SCHEMA_WRAPPER_NODE_MANIFEST_H */
