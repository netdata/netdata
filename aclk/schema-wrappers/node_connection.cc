// SPDX-License-Identifier: GPL-3.0-or-later

#include "proto/nodeinstance/connection/v1/connection.pb.h"
#include "node_connection.h"

#include "schema_wrapper_utils.h"

#include <sys/time.h>
#include <stdlib.h>

char *generate_node_instance_connection(size_t *len, const node_instance_connection_t *data) {
    nodeinstance::v1::UpdateNodeInstanceConnection msg;

    if(data->claim_id)
        msg.set_claim_id(data->claim_id);
    msg.set_node_id(data->node_id);

    msg.set_liveness(data->live);
    msg.set_queryable(data->queryable);

    msg.set_session_id(data->session_id);
    msg.set_hops(data->hops);

    struct timeval tv;
    gettimeofday(&tv, NULL);

    google::protobuf::Timestamp *timestamp = msg.mutable_updated_at();
    timestamp->set_seconds(tv.tv_sec);
    timestamp->set_nanos(tv.tv_usec * 1000);

    if (data->capabilities) {
        const struct capability *capa = data->capabilities;
        while (capa->name) {
            aclk_lib::v1::Capability *proto_capa = msg.add_capabilities();
            capability_set(proto_capa, capa);
            capa++;
        }
    }

    *len = PROTO_COMPAT_MSG_SIZE(msg);
    char *bin = (char*)mallocz(*len);
    if (bin)
        msg.SerializeToArray(bin, *len);

    return bin;
}
