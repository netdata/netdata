// SPDX-License-Identifier: GPL-3.0-or-later

#include "capability.h"

#include "proto/aclk/v1/lib.pb.h"

void capability_set(aclk_lib::v1::Capability *proto_capa, struct capability *c_capa) {
    proto_capa->set_name(c_capa->name);
    proto_capa->set_enabled(c_capa->enabled);
    proto_capa->set_version(c_capa->version);
}
