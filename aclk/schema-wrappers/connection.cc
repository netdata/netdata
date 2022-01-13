// SPDX-License-Identifier: GPL-3.0-or-later

#include "proto/agent/v1/connection.pb.h"
#include "proto/agent/v1/disconnect.pb.h"
#include "connection.h"

#include "schema_wrapper_utils.h"

#include <sys/time.h>
#include <stdlib.h>

using namespace agent::v1;

char *generate_update_agent_connection(size_t *len, const update_agent_connection_t *data)
{
    UpdateAgentConnection connupd;

    connupd.set_claim_id(data->claim_id);
    connupd.set_reachable(data->reachable);
    connupd.set_session_id(data->session_id);

    connupd.set_update_source((data->lwt) ? CONNECTION_UPDATE_SOURCE_LWT : CONNECTION_UPDATE_SOURCE_AGENT);

    struct timeval tv;
    gettimeofday(&tv, NULL);

    google::protobuf::Timestamp *timestamp = connupd.mutable_updated_at();
    timestamp->set_seconds(tv.tv_sec);
    timestamp->set_nanos(tv.tv_usec * 1000);

    *len = PROTO_COMPAT_MSG_SIZE(connupd);
    char *msg = (char*)malloc(*len);
    if (msg)
        connupd.SerializeToArray(msg, *len);

    return msg;
}

struct disconnect_cmd *parse_disconnect_cmd(const char *data, size_t len) {
    DisconnectReq req;
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
