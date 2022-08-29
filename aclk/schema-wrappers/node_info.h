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
    const char *name;

    const char *os;
    const char *os_name;
    const char *os_version;
    const char *kernel_name;
    const char *kernel_version;
    const char *architecture;
    uint32_t cpus;
    const char *cpu_frequency;
    const char *memory;
    const char *disk_space;
    const char *version;
    const char *release_channel;
    const char *timezone;
    const char *virtualization_type;
    const char *container_type;
    const char *custom_info;
    const char *machine_guid;

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
