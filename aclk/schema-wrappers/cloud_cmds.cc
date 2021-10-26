// SPDX-License-Identifier: GPL-3.0-or-later

#include "proto/aclk/v1/cmd.pb.h"
#include "cloud_cmds.h"

#include "schema_wrapper_utils.h"

using namespace aclk_cmd::v1;

struct disconnect_cmd *parse_disconnect_cmd(const char *data, size_t len) {
    ACLKDisconnectReq req;
    struct disconnect_cmd *res;

    if (!req.ParseFromArray(data, len))
        return NULL;

    res = (struct disconnect_cmd *)calloc(1, sizeof(struct disconnect_cmd));

    if (!res)
        return NULL;

    res->reconnect_after_s = req.reconnect_after_seconds();
    res->permaban = req.permaban();
    res->error_code = req.error_code();
    if (req.error_description().c_str()) {
        res->error_description = strdup(req.error_description().c_str());
        if (!res->error_description) {
            free(res);
            return NULL;
        }
    }

    return res;
}
