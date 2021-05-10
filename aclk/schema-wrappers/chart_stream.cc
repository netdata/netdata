// SPDX-License-Identifier: GPL-3.0-or-later

#include "proto/chart/v1/stream.pb.h"
#include "chart_stream.h"

#include <sys/time.h>
#include <stdlib.h>

stream_charts_and_dims_t parse_stream_charts_and_dims(const char *data, size_t len)
{
    chart::v1::StreamChartsAndDimensions msg;
    stream_charts_and_dims_t res = { .claim_id = NULL, .node_id = NULL };

    if (!msg.ParseFromArray(data, len))
        return res;

    res.node_id = strdup(msg.node_id().c_str());
    res.claim_id = strdup(msg.claim_id().c_str());
    res.seq_id = msg.sequence_id();
    res.batch_id = msg.batch_id();
    res.seq_id_created_at.tv_usec = msg.seq_id_created_at().nanos() / 1000;
    res.seq_id_created_at.tv_sec = msg.seq_id_created_at().seconds();

    return res;
}

chart_and_dim_ack_t parse_chart_and_dimensions_ack(const char *data, size_t len)
{
    chart::v1::ChartsAndDimensionsAck msg;
    chart_and_dim_ack_t res = { .claim_id = NULL, .node_id = NULL };

    if (!msg.ParseFromArray(data, len))
        return res;

    res.node_id = strdup(msg.node_id().c_str());
    res.claim_id = strdup(msg.claim_id().c_str());
    res.last_seq_id = msg.last_sequence_id();

    return res;
}

char *generate_reset_chart_messages(size_t *len, const chart_reset_reason_t reason)
{
    chart::v1::ResetChartMessages msg;

    switch (reason) {
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

    *len = msg.ByteSizeLong();
    char *bin = (char*)malloc(*len);
    if (bin)
        msg.SerializeToArray(bin, *len);

    return bin;
}

char *generate_charts_and_dimensions_updated(size_t *len, const charts_and_dims_updated_t *updates)
{
    chart::v1::ChartsAndDimensionsUpdated msg;
    aclk_lib::v1::ACLKMessagePosition *pos;

    google::protobuf::Map<std::string, std::string> *map;
    struct label *label;

    if (!updates->chart_count && !updates->dim_count) {
        return NULL;
    }

    msg.set_batch_id(updates->batch_id);

    for (int i = 0; i < updates->chart_count; i++) {
        chart::v1::ChartInstanceUpdated *chart = msg.add_charts();
        struct chart_instance_updated *c_chart = &updates->charts[i];
        chart->set_id(c_chart->id);
        chart->set_claim_id(c_chart->claim_id);
        chart->set_node_id(c_chart->node_id);
        chart->set_name(c_chart->name);

        map = chart->mutable_chart_labels();
        label = c_chart->label_head;
        while (label) {
            map->insert({label->key, label->value});
            label = label->next;
        }

        switch (c_chart->memory_mode) {
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
            return NULL;
            break;
        }

        chart->set_update_every_interval(c_chart->update_every);
        chart->set_config_hash(c_chart->config_hash);

        pos = chart->mutable_position();
        pos->set_sequence_id(c_chart->position.sequence_id);
        pos->set_previous_sequence_id(c_chart->position.previous_sequence_id);
        pos->mutable_seq_id_created_at()->set_seconds(c_chart->position.seq_id_creation_time.tv_sec);
        pos->mutable_seq_id_created_at()->set_nanos(c_chart->position.seq_id_creation_time.tv_sec * 1000);
    }

    for (int i = 0; i < updates->dim_count; i++) {
        chart::v1::ChartDimensionUpdated *dim = msg.add_dimensions();
        struct chart_dimension_updated *c_dim = &updates->dims[i];
        dim->set_id(c_dim->id);
        dim->set_chart_id(c_dim->chart_id);
        dim->set_node_id(c_dim->node_id);
        dim->set_claim_id(c_dim->claim_id);
        dim->set_name(c_dim->name);

        google::protobuf::Timestamp *tv = dim->mutable_created_at();
        tv->set_seconds(c_dim->created_at.tv_sec);
        tv->set_nanos(c_dim->created_at.tv_usec * 1000);

        tv = dim->mutable_last_timestamp();
        tv->set_seconds(c_dim->last_timestamp.tv_sec);
        tv->set_nanos(c_dim->last_timestamp.tv_usec * 1000);

        pos = dim->mutable_position();
        pos->set_sequence_id(c_dim->position.sequence_id);
        pos->set_previous_sequence_id(c_dim->position.previous_sequence_id);
        pos->mutable_seq_id_created_at()->set_seconds(c_dim->position.seq_id_creation_time.tv_sec);
        pos->mutable_seq_id_created_at()->set_nanos(c_dim->position.seq_id_creation_time.tv_sec * 1000);
    }

    *len = msg.ByteSizeLong();
    char *bin = (char*)malloc(*len);
    if (bin)
        msg.SerializeToArray(bin, *len);

    return bin;
}
