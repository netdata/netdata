// SPDX-License-Identifier: GPL-3.0-or-later

#include "aclk/aclk_util.h"

#include "proto/chart/v1/stream.pb.h"
#include "chart_stream.h"

#include "schema_wrapper_utils.h"

#include <sys/time.h>
#include <stdlib.h>

stream_charts_and_dims_t parse_stream_charts_and_dims(const char *data, size_t len)
{
    chart::v1::StreamChartsAndDimensions msg;
    stream_charts_and_dims_t res;
    memset(&res, 0, sizeof(res));

    if (!msg.ParseFromArray(data, len))
        return res;

    res.node_id = strdup(msg.node_id().c_str());
    res.claim_id = strdup(msg.claim_id().c_str());
    res.seq_id = msg.sequence_id();
    res.batch_id = msg.batch_id();
    set_timeval_from_google_timestamp(msg.seq_id_created_at(), &res.seq_id_created_at);

    return res;
}

chart_and_dim_ack_t parse_chart_and_dimensions_ack(const char *data, size_t len)
{
    chart::v1::ChartsAndDimensionsAck msg;
    chart_and_dim_ack_t res = { .claim_id = NULL, .node_id = NULL, .last_seq_id = 0 };

    if (!msg.ParseFromArray(data, len))
        return res;

    res.node_id = strdup(msg.node_id().c_str());
    res.claim_id = strdup(msg.claim_id().c_str());
    res.last_seq_id = msg.last_sequence_id();

    return res;
}

char *generate_reset_chart_messages(size_t *len, chart_reset_t reset)
{
    chart::v1::ResetChartMessages msg;

    msg.set_claim_id(reset.claim_id);
    msg.set_node_id(reset.node_id);
    switch (reset.reason) {
        case DB_EMPTY:
            msg.set_reason(chart::v1::ResetReason::DB_EMPTY);
            break;
        case SEQ_ID_NOT_EXISTS:
            msg.set_reason(chart::v1::ResetReason::SEQ_ID_NOT_EXISTS);
            break;
        case TIMESTAMP_MISMATCH:
            msg.set_reason(chart::v1::ResetReason::TIMESTAMP_MISMATCH);
            break;
        default:
            return NULL;
    }

    *len = PROTO_COMPAT_MSG_SIZE(msg);
    char *bin = (char*)malloc(*len);
    if (bin)
        msg.SerializeToArray(bin, *len);

    return bin;
}

void chart_instance_updated_destroy(struct chart_instance_updated *instance)
{
    freez((char*)instance->id);
    freez((char*)instance->claim_id);

    free_label_list(instance->label_head);

    freez((char*)instance->config_hash);
}

static int set_chart_instance_updated(chart::v1::ChartInstanceUpdated *chart, const struct chart_instance_updated *update)
{
    google::protobuf::Map<std::string, std::string> *map;
    aclk_lib::v1::ACLKMessagePosition *pos;
    struct label *label;

    chart->set_id(update->id);
    chart->set_claim_id(update->claim_id);
    chart->set_node_id(update->node_id);
    chart->set_name(update->name);

    map = chart->mutable_chart_labels();
    label = update->label_head;
    while (label) {
        map->insert({label->key, label->value});
        label = label->next;
    }

    switch (update->memory_mode) {
    case RRD_MEMORY_MODE_NONE:
        chart->set_memory_mode(chart::v1::NONE);
        break;
    case RRD_MEMORY_MODE_RAM:
        chart->set_memory_mode(chart::v1::RAM);
        break;
    case RRD_MEMORY_MODE_MAP:
        chart->set_memory_mode(chart::v1::MAP);
        break;
    case RRD_MEMORY_MODE_SAVE:
        chart->set_memory_mode(chart::v1::SAVE);
        break;
    case RRD_MEMORY_MODE_ALLOC:
        chart->set_memory_mode(chart::v1::ALLOC);
        break;
    case RRD_MEMORY_MODE_DBENGINE:
        chart->set_memory_mode(chart::v1::DB_ENGINE);
        break;
    default:
        return 1;
        break;
    }

    chart->set_update_every_interval(update->update_every);
    chart->set_config_hash(update->config_hash);

    pos = chart->mutable_position();
    pos->set_sequence_id(update->position.sequence_id);
    pos->set_previous_sequence_id(update->position.previous_sequence_id);
    set_google_timestamp_from_timeval(update->position.seq_id_creation_time, pos->mutable_seq_id_created_at());

    return 0;
}

static int set_chart_dim_updated(chart::v1::ChartDimensionUpdated *dim, const struct chart_dimension_updated *c_dim)
{
    aclk_lib::v1::ACLKMessagePosition *pos;

    dim->set_id(c_dim->id);
    dim->set_chart_id(c_dim->chart_id);
    dim->set_node_id(c_dim->node_id);
    dim->set_claim_id(c_dim->claim_id);
    dim->set_name(c_dim->name);

    set_google_timestamp_from_timeval(c_dim->created_at, dim->mutable_created_at());
    set_google_timestamp_from_timeval(c_dim->last_timestamp, dim->mutable_last_timestamp());

    pos = dim->mutable_position();
    pos->set_sequence_id(c_dim->position.sequence_id);
    pos->set_previous_sequence_id(c_dim->position.previous_sequence_id);
    set_google_timestamp_from_timeval(c_dim->position.seq_id_creation_time, pos->mutable_seq_id_created_at());

    return 0;
}

char *generate_charts_and_dimensions_updated(size_t *len, char **payloads, size_t *payload_sizes, int *is_dim, struct aclk_message_position *new_positions, uint64_t batch_id)
{
    chart::v1::ChartsAndDimensionsUpdated msg;
    chart::v1::ChartInstanceUpdated db_chart;
    chart::v1::ChartDimensionUpdated db_dim;
    aclk_lib::v1::ACLKMessagePosition *pos;

    msg.set_batch_id(batch_id);

    for (int i = 0; payloads[i]; i++) {
        if (is_dim[i]) {
            if (!db_dim.ParseFromArray(payloads[i], payload_sizes[i])) {
                error("[ACLK] Could not parse chart::v1::chart_dimension_updated");
                return NULL;
            }

            pos = db_dim.mutable_position();
            pos->set_sequence_id(new_positions[i].sequence_id);
            pos->set_previous_sequence_id(new_positions[i].previous_sequence_id);
            set_google_timestamp_from_timeval(new_positions[i].seq_id_creation_time, pos->mutable_seq_id_created_at());

            chart::v1::ChartDimensionUpdated *dim = msg.add_dimensions();
            *dim = db_dim;
        } else {
            if (!db_chart.ParseFromArray(payloads[i], payload_sizes[i])) {
                error("[ACLK] Could not parse chart::v1::ChartInstanceUpdated");
                return NULL;
            }

            pos = db_chart.mutable_position();
            pos->set_sequence_id(new_positions[i].sequence_id);
            pos->set_previous_sequence_id(new_positions[i].previous_sequence_id);
            set_google_timestamp_from_timeval(new_positions[i].seq_id_creation_time, pos->mutable_seq_id_created_at());

            chart::v1::ChartInstanceUpdated *chart = msg.add_charts();
            *chart = db_chart;
        }
    }

    *len = PROTO_COMPAT_MSG_SIZE(msg);
    char *bin = (char*)mallocz(*len);
    msg.SerializeToArray(bin, *len);

    return bin;
}

char *generate_charts_updated(size_t *len, char **payloads, size_t *payload_sizes, struct aclk_message_position *new_positions)
{
    chart::v1::ChartsAndDimensionsUpdated msg;

    msg.set_batch_id(chart_batch_id);

    for (int i = 0; payloads[i]; i++) {
        chart::v1::ChartInstanceUpdated db_msg;
        chart::v1::ChartInstanceUpdated *chart;
        aclk_lib::v1::ACLKMessagePosition *pos;

        if (!db_msg.ParseFromArray(payloads[i], payload_sizes[i])) {
            error("[ACLK] Could not parse chart::v1::ChartInstanceUpdated");
            return NULL;
        }

        pos = db_msg.mutable_position();
        pos->set_sequence_id(new_positions[i].sequence_id);
        pos->set_previous_sequence_id(new_positions[i].previous_sequence_id);
        set_google_timestamp_from_timeval(new_positions[i].seq_id_creation_time, pos->mutable_seq_id_created_at());

        chart = msg.add_charts();
        *chart = db_msg;
    }

    *len = PROTO_COMPAT_MSG_SIZE(msg);
    char *bin = (char*)mallocz(*len);
    msg.SerializeToArray(bin, *len);

    return bin;
}

char *generate_chart_dimensions_updated(size_t *len, char **payloads, size_t *payload_sizes, struct aclk_message_position *new_positions)
{
    chart::v1::ChartsAndDimensionsUpdated msg;

    msg.set_batch_id(chart_batch_id);

    for (int i = 0; payloads[i]; i++) {
        chart::v1::ChartDimensionUpdated db_msg;
        chart::v1::ChartDimensionUpdated *dim;
        aclk_lib::v1::ACLKMessagePosition *pos;

        if (!db_msg.ParseFromArray(payloads[i], payload_sizes[i])) {
            error("[ACLK] Could not parse chart::v1::chart_dimension_updated");
            return NULL;
        }

        pos = db_msg.mutable_position();
        pos->set_sequence_id(new_positions[i].sequence_id);
        pos->set_previous_sequence_id(new_positions[i].previous_sequence_id);
        set_google_timestamp_from_timeval(new_positions[i].seq_id_creation_time, pos->mutable_seq_id_created_at());

        dim = msg.add_dimensions();
        *dim = db_msg;
    }

    *len = PROTO_COMPAT_MSG_SIZE(msg);
    char *bin = (char*)mallocz(*len);
    msg.SerializeToArray(bin, *len);

    return bin;
}

char *generate_chart_instance_updated(size_t *len, const struct chart_instance_updated *update)
{
    chart::v1::ChartInstanceUpdated *chart = new chart::v1::ChartInstanceUpdated();

    if (set_chart_instance_updated(chart, update))
        return NULL;

    *len = PROTO_COMPAT_MSG_SIZE_PTR(chart);
    char *bin = (char*)mallocz(*len);
    chart->SerializeToArray(bin, *len);

    delete chart;
    return bin;
}

char *generate_chart_dimension_updated(size_t *len, const struct chart_dimension_updated *dim)
{
    chart::v1::ChartDimensionUpdated *proto_dim = new chart::v1::ChartDimensionUpdated();

    if (set_chart_dim_updated(proto_dim, dim))
        return NULL;

    *len = PROTO_COMPAT_MSG_SIZE_PTR(proto_dim);
    char *bin = (char*)mallocz(*len);
    proto_dim->SerializeToArray(bin, *len);

    delete proto_dim;
    return bin;
}

using namespace google::protobuf;

char *generate_retention_updated(size_t *len, struct retention_updated *data)
{
    chart::v1::RetentionUpdated msg;

    msg.set_claim_id(data->claim_id);
    msg.set_node_id(data->node_id);

    switch (data->memory_mode) {
    case RRD_MEMORY_MODE_NONE:
        msg.set_memory_mode(chart::v1::NONE);
        break;
    case RRD_MEMORY_MODE_RAM:
        msg.set_memory_mode(chart::v1::RAM);
        break;
    case RRD_MEMORY_MODE_MAP:
        msg.set_memory_mode(chart::v1::MAP);
        break;
    case RRD_MEMORY_MODE_SAVE:
        msg.set_memory_mode(chart::v1::SAVE);
        break;
    case RRD_MEMORY_MODE_ALLOC:
        msg.set_memory_mode(chart::v1::ALLOC);
        break;
    case RRD_MEMORY_MODE_DBENGINE:
        msg.set_memory_mode(chart::v1::DB_ENGINE);
        break;
    default:
        return NULL;
    }

    for (int i = 0; i < data->interval_duration_count; i++) {
        Map<uint32, uint32> *map = msg.mutable_interval_durations();
        map->insert({data->interval_durations[i].update_every, data->interval_durations[i].retention});
    }

    set_google_timestamp_from_timeval(data->rotation_timestamp, msg.mutable_rotation_timestamp());

    *len = PROTO_COMPAT_MSG_SIZE(msg);
    char *bin = (char*)mallocz(*len);
    msg.SerializeToArray(bin, *len);

    return bin;
}
