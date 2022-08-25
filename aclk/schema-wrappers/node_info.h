// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef ACLK_SCHEMA_WRAPPER_NODE_INFO_H
#define ACLK_SCHEMA_WRAPPER_NODE_INFO_H

#include <stdlib.h>
#include <stdint.h>

#include "capability.h"
#include "database/rrd.h"

#ifdef __cplusplus
extern "C" {
#endif

struct machine_learning_info {
    bool ml_capable;
    bool ml_enabled;
};

struct aclk_node_info {
    char *name;

    char *os;
    char *os_name;
    char *os_version;

    char *kernel_name;
    char *kernel_version;

    char *architecture;

    uint32_t cpus;

    char *cpu_frequency;

    char *memory;

    char *disk_space;

    char *version;

    char *release_channel;

    char *timezone;

    char *virtualization_type;

    char *container_type;

    char *custom_info;

    char *machine_guid;

    DICTIONARY *host_labels_ptr;

    struct machine_learning_info ml_info;
};

struct update_node_info {
    char *node_id;
    char *claim_id;
    struct aclk_node_info data;
    struct timeval updated_at;
    char *machine_guid;
    int child;

    struct machine_learning_info ml_info;

    struct capability *node_capabilities;
    struct capability *node_instance_capabilities;
};

struct collector_info {
    const char *module;
    const char *plugin;
};

struct update_node_collectors {
    char *claim_id;
    char *node_id;
    DICTIONARY *node_collectors;
};

char *generate_update_node_info_message(size_t *len, struct update_node_info *info);

char *generate_update_node_collectors_message(size_t *len, struct update_node_collectors *collectors);

#ifdef __cplusplus
}
#endif

#endif /* ACLK_SCHEMA_WRAPPER_NODE_INFO_H */
