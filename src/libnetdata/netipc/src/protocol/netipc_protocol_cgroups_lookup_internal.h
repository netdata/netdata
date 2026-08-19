#ifndef NETIPC_PROTOCOL_CGROUPS_LOOKUP_INTERNAL_H
#define NETIPC_PROTOCOL_CGROUPS_LOOKUP_INTERNAL_H

#include "netipc_protocol_lookup_common.h"

typedef struct {
    uint16_t layout_version;
    uint16_t status;
    uint16_t orchestrator;
    uint16_t reserved0;
    uint32_t path_offset;
    uint32_t path_length;
    uint32_t name_offset;
    uint32_t name_length;
    uint16_t label_count;
    uint16_t reserved1;
} nipc_cgroups_lookup_item_wire_t;

#endif /* NETIPC_PROTOCOL_CGROUPS_LOOKUP_INTERNAL_H */
