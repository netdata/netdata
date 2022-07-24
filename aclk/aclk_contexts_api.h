// SPDX-License-Identifier: GPL-3.0-or-later
#ifndef ACLK_CONTEXTS_API_H
#define ACLK_CONTEXTS_API_H

#include "schema-wrappers/schema_wrappers.h"


void aclk_send_contexts_snapshot(contexts_snapshot_t data);
void aclk_send_contexts_updated(contexts_updated_t data);

#endif /* ACLK_CONTEXTS_API_H */

