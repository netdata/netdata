#include "node_info.h"

#include "proto/nodeinstance/info/v1/info.pb.h"

#include "schema_wrapper_utils.h"

static int generate_node_info(nodeinstance::info::v1::NodeInfo *info, struct aclk_node_info *data)
{
    google::protobuf::Map<std::string, std::string> *map;

    if (data->name)
        info->set_name(data->name);

    if (data->os)
        info->set_os(data->os);
    if (data->os_name)
        info->set_os_name(data->os_name);
    if (data->os_version)
        info->set_os_version(data->os_version);

    if (data->kernel_name)
        info->set_kernel_name(data->kernel_name);
    if (data->kernel_version)
        info->set_kernel_version(data->kernel_version);

    if (data->architecture)
        info->set_architecture(data->architecture);

    info->set_cpus(data->cpus);

    if (data->cpu_frequency)
        info->set_cpu_frequency(data->cpu_frequency);

    if (data->memory)
        info->set_memory(data->memory);

    if (data->disk_space)
        info->set_disk_space(data->disk_space);

    if (data->version)
        info->set_version(data->version);

    if (data->release_channel)
        info->set_release_channel(data->release_channel);

    if (data->timezone)
        info->set_timezone(data->timezone);

    if (data->virtualization_type)
        info->set_virtualization_type(data->virtualization_type);

    if (data->container_type)
        info->set_container_type(data->container_type);

    if (data->custom_info)
        info->set_custom_info(data->custom_info);

    if (data->machine_guid)
        info->set_machine_guid(data->machine_guid);

    nodeinstance::info::v1::MachineLearningInfo *ml_info = info->mutable_ml_info();
    ml_info->set_ml_capable(data->ml_info.ml_capable);
    ml_info->set_ml_enabled(data->ml_info.ml_enabled);

    map = info->mutable_host_labels();
    rrdlabels_walkthrough_read(data->host_labels_ptr, label_add_to_map_callback, map);
    return 0;
}

char *generate_update_node_info_message(size_t *len, struct update_node_info *info)
{
    nodeinstance::info::v1::UpdateNodeInfo msg;

    msg.set_node_id(info->node_id);
    msg.set_claim_id(info->claim_id);

    if (generate_node_info(msg.mutable_data(), &info->data))
        return NULL;

    set_google_timestamp_from_timeval(info->updated_at, msg.mutable_updated_at());
    msg.set_machine_guid(info->machine_guid);
    msg.set_child(info->child);

    nodeinstance::info::v1::MachineLearningInfo *ml_info = msg.mutable_ml_info();
    ml_info->set_ml_capable(info->ml_info.ml_capable);
    ml_info->set_ml_enabled(info->ml_info.ml_enabled);

    struct capability *capa;
    if (info->node_capabilities) {
        capa = info->node_capabilities;
        while (capa->name) {
            aclk_lib::v1::Capability *proto_capa = msg.mutable_node_info()->add_capabilities();
            capability_set(proto_capa, capa);
            capa++;
        }
    }
    if (info->node_instance_capabilities) {
        capa = info->node_instance_capabilities;
        while (capa->name) {
            aclk_lib::v1::Capability *proto_capa = msg.mutable_node_instance_info()->add_capabilities();
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

char *generate_update_node_collectors_message(size_t *len, struct update_node_collectors *upd_node_collectors)
{
    nodeinstance::info::v1::UpdateNodeCollectors msg;

    msg.set_node_id(upd_node_collectors->node_id);
    msg.set_claim_id(upd_node_collectors->claim_id);

    void *colls;
    dfe_start_read(upd_node_collectors->node_collectors, colls) {
        struct collector_info *c =(struct collector_info *)colls;
        nodeinstance::info::v1::CollectorInfo *col = msg.add_collectors();
        col->set_plugin(c->plugin);
        col->set_module(c->module);
    }
    dfe_done(colls);

    *len = PROTO_COMPAT_MSG_SIZE(msg);
    char *bin = (char*)mallocz(*len);
    if (bin)
        msg.SerializeToArray(bin, *len);

    return bin;
}
