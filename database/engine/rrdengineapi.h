// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_RRDENGINEAPI_H
#define NETDATA_RRDENGINEAPI_H

#include "rrdengine.h"

#define RRDENG_MIN_PAGE_CACHE_SIZE_MB (8)
#define RRDENG_MIN_DISK_SPACE_MB (64)

#define RRDENG_NR_STATS (37)

#define RRDENG_FD_BUDGET_PER_INSTANCE (50)

extern int db_engine_use_malloc;
extern int default_rrdeng_page_fetch_timeout;
extern int default_rrdeng_page_fetch_retries;
extern int default_rrdeng_page_cache_mb;
extern int default_rrdeng_disk_quota_mb;
extern int default_multidb_disk_quota_mb;
extern uint8_t rrdeng_drop_metrics_under_page_cache_pressure;
extern struct rrdengine_instance *multidb_ctx[RRD_STORAGE_TIERS];
extern size_t page_type_size[];

#define PAGE_POINT_SIZE_BYTES(x) page_type_size[(x)->type]

struct rrdeng_region_info {
    time_t start_time;
    int update_every;
    unsigned points;
};

void *rrdeng_create_page(struct rrdengine_instance *ctx, uuid_t *id, struct rrdeng_page_descr **ret_descr);
void rrdeng_commit_page(struct rrdengine_instance *ctx, struct rrdeng_page_descr *descr,
                               Word_t page_correlation_id);
void *rrdeng_get_latest_page(struct rrdengine_instance *ctx, uuid_t *id, void **handle);
void *rrdeng_get_page(struct rrdengine_instance *ctx, uuid_t *id, usec_t point_in_time, void **handle);
void rrdeng_put_page(struct rrdengine_instance *ctx, void *handle);

void rrdeng_generate_legacy_uuid(const char *dim_id, char *chart_id, uuid_t *ret_uuid);
void rrdeng_convert_legacy_uuid_to_multihost(char machine_guid[GUID_LEN + 1], uuid_t *legacy_uuid,
                                                    uuid_t *ret_uuid);


STORAGE_METRIC_HANDLE *rrdeng_metric_init(RRDDIM *rd, STORAGE_INSTANCE *db_instance);
void rrdeng_metric_free(STORAGE_METRIC_HANDLE *db_metric_handle);

STORAGE_COLLECT_HANDLE *rrdeng_store_metric_init(STORAGE_METRIC_HANDLE *db_metric_handle);
void rrdeng_store_metric_flush_current_page(STORAGE_COLLECT_HANDLE *collection_handle);
void rrdeng_store_metric_next(STORAGE_COLLECT_HANDLE *collection_handle, usec_t point_in_time, NETDATA_DOUBLE n,
                                     NETDATA_DOUBLE min_value,
                                     NETDATA_DOUBLE max_value,
                                     uint16_t count,
                                     uint16_t anomaly_count,
                                     SN_FLAGS flags);
int rrdeng_store_metric_finalize(STORAGE_COLLECT_HANDLE *collection_handle);

unsigned rrdeng_variable_step_boundaries(RRDSET *st, time_t start_time, time_t end_time,
                                    struct rrdeng_region_info **region_info_arrayp, unsigned *max_intervalp, struct context_param *context_param_list);

void rrdeng_load_metric_init(STORAGE_METRIC_HANDLE *db_metric_handle, struct rrddim_query_handle *rrdimm_handle,
                                    time_t start_time, time_t end_time, TIER_QUERY_FETCH tier_query_fetch_type);
STORAGE_POINT rrdeng_load_metric_next(struct rrddim_query_handle *rrdimm_handle);

int rrdeng_load_metric_is_finished(struct rrddim_query_handle *rrdimm_handle);
void rrdeng_load_metric_finalize(struct rrddim_query_handle *rrdimm_handle);
time_t rrdeng_metric_latest_time(STORAGE_METRIC_HANDLE *db_metric_handle);
time_t rrdeng_metric_oldest_time(STORAGE_METRIC_HANDLE *db_metric_handle);

void rrdeng_get_37_statistics(struct rrdengine_instance *ctx, unsigned long long *array);

/* must call once before using anything */
int rrdeng_init(RRDHOST *host, struct rrdengine_instance **ctxp, char *dbfiles_path, unsigned page_cache_mb,
                       unsigned disk_space_mb, int tier);

int rrdeng_exit(struct rrdengine_instance *ctx);
void rrdeng_prepare_exit(struct rrdengine_instance *ctx);
int rrdeng_metric_latest_time_by_uuid(uuid_t *dim_uuid, time_t *first_entry_t, time_t *last_entry_t, int tier);
int rrdeng_metric_retention_by_uuid(STORAGE_INSTANCE *si, uuid_t *dim_uuid, time_t *first_entry_t, time_t *last_entry_t);

typedef struct rrdengine_size_statistics {
    size_t default_granularity_secs;

    size_t sizeof_metric;
    size_t sizeof_metric_in_index;
    size_t sizeof_page;
    size_t sizeof_page_in_index;
    size_t sizeof_extent;
    size_t sizeof_page_in_extent;
    size_t sizeof_datafile;
    size_t sizeof_page_in_cache;
    size_t sizeof_point_data;
    size_t sizeof_page_data;

    size_t pages_per_extent;

    size_t datafiles;
    size_t extents;
    size_t extents_pages;
    size_t points;
    size_t metrics;
    size_t metrics_pages;

    size_t extents_compressed_bytes;
    size_t pages_uncompressed_bytes;
    time_t pages_duration_secs;

    struct {
        size_t pages;
        size_t pages_uncompressed_bytes;
        time_t pages_duration_secs;
        size_t points;
    } page_types[256];

    size_t single_point_pages;

    usec_t first_t;
    usec_t last_t;

    size_t currently_collected_metrics;
    size_t max_concurrently_collected_metrics;
    size_t estimated_concurrently_collected_metrics;

    size_t disk_space;
    size_t max_disk_space;

    time_t database_retention_secs;
    double average_compression_savings;
    double average_point_duration_secs;
    double average_metric_retention_secs;

    double ephemeral_metrics_per_day_percent;

    double average_page_size_bytes;
} RRDENG_SIZE_STATS;

RRDENG_SIZE_STATS rrdeng_size_statistics(struct rrdengine_instance *ctx);

#endif /* NETDATA_RRDENGINEAPI_H */
