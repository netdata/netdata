// SPDX-License-Identifier: GPL-3.0-or-later

#include "proto/agent/v1/cmds.pb.h"

#include "agent_cmds.h"

#include "schema_wrapper_utils.h"

using namespace agent::v1;

int parse_cancel_pending_req(const char *msg, size_t msg_len, struct aclk_cancel_pending_req *req)
{
    CancelPendingRequest msg_parsed;

    if (!msg_parsed.ParseFromArray(msg, msg_len)) {
        error_report("Failed to parse CancelPendingRequest message");
        return 1;
    }

    if (msg_parsed.request_id().c_str() == NULL) {
        error_report("CancelPendingRequest message missing request_id");
        return 1;
    }
    req->request_id = strdupz(msg_parsed.request_id().c_str());

    if (msg_parsed.trace_id().c_str())
            req->trace_id = strdupz(msg_parsed.trace_id().c_str());

    set_timeval_from_google_timestamp(msg_parsed.timestamp(), &req->timestamp);

    return 0;
}

void free_cancel_pending_req(struct aclk_cancel_pending_req *req)
{
    freez(req->request_id);
    freez(req->trace_id);
}
