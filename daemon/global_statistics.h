// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_GLOBAL_STATISTICS_H
#define NETDATA_GLOBAL_STATISTICS_H 1

#include "database/rrd.h"

extern struct netdata_buffers_statistics {
    size_t rrdhost_allocations_size;
    size_t rrdhost_senders;
    size_t rrdhost_receivers;
    size_t query_targets_size;
    size_t rrdset_done_rda_size;
    size_t buffers_aclk;
    size_t buffers_api;
    size_t buffers_functions;
    size_t buffers_sqlite;
    size_t buffers_exporters;
    size_t buffers_health;
    size_t buffers_streaming;
    size_t cbuffers_streaming;
    size_t buffers_web;
} netdata_buffers_statistics;

extern struct dictionary_stats dictionary_stats_category_collectors;
extern struct dictionary_stats dictionary_stats_category_rrdhost;
extern struct dictionary_stats dictionary_stats_category_rrdset_rrddim;
extern struct dictionary_stats dictionary_stats_category_rrdcontext;
extern struct dictionary_stats dictionary_stats_category_rrdlabels;
extern struct dictionary_stats dictionary_stats_category_rrdhealth;
extern struct dictionary_stats dictionary_stats_category_functions;
extern struct dictionary_stats dictionary_stats_category_replication;

extern size_t rrddim_db_memory_size;

// ----------------------------------------------------------------------------
// global statistics

void global_statistics_ml_query_completed(size_t points_read);
void global_statistics_ml_models_consulted(size_t models_consulted);
void global_statistics_exporters_query_completed(size_t points_read);
void global_statistics_backfill_query_completed(size_t points_read);
void global_statistics_rrdr_query_completed(size_t queries, uint64_t db_points_read, uint64_t result_points_generated, QUERY_SOURCE query_source);
void global_statistics_sqlite3_query_completed(bool success, bool busy, bool locked);
void global_statistics_sqlite3_row_completed(void);
void global_statistics_rrdset_done_chart_collection_completed(size_t *points_read_per_tier_array);

void global_statistics_web_request_completed(uint64_t dt,
                                             uint64_t bytes_received,
                                             uint64_t bytes_sent,
                                             uint64_t content_size,
                                             uint64_t compressed_content_size);

uint64_t global_statistics_web_client_connected(void);
void global_statistics_web_client_disconnected(void);

extern bool global_statistics_enabled;

#endif /* NETDATA_GLOBAL_STATISTICS_H */
