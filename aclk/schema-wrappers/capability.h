// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef ACLK_SCHEMA_CAPABILITY_H
#define ACLK_SCHEMA_CAPABILITY_H

#ifdef __cplusplus
extern "C" {
#endif

struct capability {
    const char *name;
    uint32_t version;
    int enabled;
};

#ifdef __cplusplus
}

#include "proto/aclk/v1/lib.pb.h"

void capability_set(aclk_lib::v1::Capability *proto_capa, const struct capability *c_capa);
#endif

#endif /* ACLK_SCHEMA_CAPABILITY_H */
