// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef ACLK_SCHEMA_WRAPPERS_AGENT_CMDS_H
#define ACLK_SCHEMA_WRAPPERS_AGENT_CMDS_H

#include "libnetdata/libnetdata.h"

#ifdef __cplusplus
extern "C" {
#endif

struct aclk_cancel_pending_req {
    char *request_id;

    struct timeval timestamp;

    char *trace_id;
};

int parse_cancel_pending_req(const char *msg, size_t msg_len, struct aclk_cancel_pending_req *req);
void free_cancel_pending_req(struct aclk_cancel_pending_req *req);

#ifdef __cplusplus
}
#endif

#endif /* ACLK_SCHEMA_WRAPPERS_AGENT_CMDS_H */
