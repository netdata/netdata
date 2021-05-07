// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef ACLK_SCHEMA_WRAPPER_CHART_STREAM_H
#define ACLK_SCHEMA_WRAPPER_CHART_STREAM_H

#ifdef __cplusplus
extern "C" {
#endif

typedef struct {
    char* claim_id;
    char* node_id;

    uint64_t seq_id;
    uint64_t batch_id;

    struct timeval seq_id_created_at;
} stream_charts_and_dims_t;

stream_charts_and_dims_t parse_stream_charts_and_dims(const char *data, size_t len);

#ifdef __cplusplus
}
#endif

#endif /* ACLK_SCHEMA_WRAPPER_CHART_STREAM_H */
