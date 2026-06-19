#ifndef NETIPC_PROTOCOL_APPS_LOOKUP_INTERNAL_H
#define NETIPC_PROTOCOL_APPS_LOOKUP_INTERNAL_H

#include "netipc_protocol_lookup_common.h"

typedef struct {
    uint32_t pid;
    uint32_t reserved;
} nipc_apps_lookup_key_wire_t;

typedef struct {
    uint16_t layout_version;
    uint16_t status;
    uint16_t orchestrator;
    uint16_t cgroup_status;
    uint32_t pid;
    uint32_t ppid;
    uint32_t uid;
    uint32_t reserved0;
    uint64_t starttime;
    uint32_t comm_offset;
    uint32_t comm_length;
    uint32_t cgroup_path_offset;
    uint32_t cgroup_path_length;
    uint32_t cgroup_name_offset;
    uint32_t cgroup_name_length;
    uint16_t label_count;
    uint16_t reserved1;
} nipc_apps_lookup_item_wire_t;

#endif /* NETIPC_PROTOCOL_APPS_LOOKUP_INTERNAL_H */
