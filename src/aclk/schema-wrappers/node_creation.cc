// SPDX-License-Identifier: GPL-3.0-or-later

#include "proto/nodeinstance/create/v1/creation.pb.h"
#include "node_creation.h"

#include "schema_wrapper_utils.h"

#include <stdlib.h>

char *generate_node_instance_creation(size_t *len, const node_instance_creation_t *data)
{
    nodeinstance::create::v1::CreateNodeInstance msg;

    if (data->claim_id)
        msg.set_claim_id(data->claim_id);
    msg.set_machine_guid(data->machine_guid);
    msg.set_hostname(data->hostname);
    msg.set_hops(data->hops);

    *len = PROTO_COMPAT_MSG_SIZE(msg);
    char *bin = (char*)mallocz(*len);
    if (bin)
        msg.SerializeToArray(bin, *len);

    return bin;
}

node_instance_creation_result_t parse_create_node_instance_result(const char *data, size_t len)
{
    nodeinstance::create::v1::CreateNodeInstanceResult msg;
    node_instance_creation_result_t res = { .node_id = NULL, .machine_guid = NULL };

    if (!msg.ParseFromArray(data, len))
        return res;

    res.node_id = strdupz(msg.node_id().c_str());
    res.machine_guid = strdupz(msg.machine_guid().c_str());
    return res;
}
