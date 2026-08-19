#ifndef NETIPC_PROTOCOL_CGROUPS_SNAPSHOT_INTERNAL_H
#define NETIPC_PROTOCOL_CGROUPS_SNAPSHOT_INTERNAL_H

#include "netipc_protocol_internal.h"

/* Cgroups item wire header (internal, 32 bytes) */
typedef struct {
    uint16_t layout_version;
    uint16_t flags;
    uint32_t hash;
    uint32_t options;
    uint32_t enabled;
    uint32_t name_offset;
    uint32_t name_length;
    uint32_t path_offset;
    uint32_t path_length;
} nipc_cgroups_item_wire_t;

#endif /* NETIPC_PROTOCOL_CGROUPS_SNAPSHOT_INTERNAL_H */
