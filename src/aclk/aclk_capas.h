// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef ACLK_CAPAS_H
#define ACLK_CAPAS_H

#include "daemon/common.h"
#include "libnetdata/libnetdata.h"

#include "schema-wrappers/capability.h"

const struct capability *aclk_get_agent_capas();
struct capability *aclk_get_node_instance_capas(RRDHOST *host);

#endif /* ACLK_CAPAS_H */
