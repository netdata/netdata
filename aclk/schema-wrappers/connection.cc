// SPDX-License-Identifier: GPL-3.0-or-later

#include "proto/agent/v1/connection.pb.h"
#include "connection.h"

#include "schema_wrapper_utils.h"

#include <sys/time.h>
#include <stdlib.h>

char *generate_update_agent_connection(size_t *len, const update_agent_connection_t *data)
{
    agent::v1::UpdateAgentConnection connupd;

    connupd.set_claim_id(data->claim_id);
    connupd.set_reachable(data->reachable);
    connupd.set_session_id(data->session_id);

    connupd.set_update_source((data->lwt) ? agent::v1::CONNECTION_UPDATE_SOURCE_LWT : agent::v1::CONNECTION_UPDATE_SOURCE_AGENT);

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
