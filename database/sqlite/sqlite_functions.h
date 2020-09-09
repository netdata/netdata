// SPDX-License-Identifier: GPL-3.0-or-later


#ifndef NETDATA_SQLITE_FUNCTIONS_H
#define NETDATA_SQLITE_FUNCTIONS_H

#include "../../daemon/common.h"
//#include "../rrd.h"
#ifdef ENABLE_DBENGINE
#include "../engine/global_uuid_map//global_uuid_map.h"
#endif

#define NETDATA_PLUGIN_HOOK_SQLITE \
    { \
        .name = "SQLITE", \
        .config_section = CONFIG_SECTION_GLOBAL , \
        .config_name = "sqlite_stats", \
        .enabled = 1, \
        .thread = NULL, \
        .init_routine = NULL, \
        .start_routine = sqlite_stats_main \
    },

#define NETDATA_PLUGIN_HOOK_SQLITE_ROTATION \
    { \
        .name = "SQLITE", \
        .config_section = CONFIG_SECTION_GLOBAL, \
        .config_name = "sqlite_rotation", \
        .enabled = 1, \
        .thread = NULL, \
        .init_routine = NULL, \
        .start_routine = sqlite_rotation_main \
    },

extern void *sqlite_stats_main(void *ptr);
extern void *sqlite_rotation_main(void *ptr);

typedef struct dimension {
    uuid_t  dim_uuid;
    char dim_str[37];
    char *id;
    char *name;
    struct dimension *next;
} DIMENSION;

typedef struct dimension_list {
    uuid_t  dim_uuid;
    char dim_str[37];
    char *id;
    char *name;
} DIMENSION_LIST;


extern int sql_init_database();
extern int sql_close_database();
extern int sql_store_dimension(uuid_t *dim_uuid, uuid_t *chart_uuid, const char *id, const char *name, collected_number multiplier,
                        collected_number divisor, int algorithm);
    extern int sql_select_dimension(uuid_t *chart_uuid, struct dimension_list **dimension_list, int *, int *);
extern int sql_dimension_archive(uuid_t *dim_uuid, int archive);
extern int sql_dimension_options(uuid_t *dim_uuid, char *options);
extern RRDDIM *sql_create_dimension(char *dim_str, RRDSET *st, int temp);
extern RRDDIM *sql_load_chart_dimensions(RRDSET *st, int temp);
extern void sql_add_metric(uuid_t *dim_uuid, usec_t point_in_time, storage_number number);
extern void sql_add_metric_page(uuid_t *dim_uuid, storage_number *metric, size_t entries, time_t start_time, time_t end_time);

extern int sql_load_one_chart_dimension(uuid_t *chart_uuid, BUFFER *wb, int *dimensions);
extern char *sql_find_dim_uuid(RRDSET *st, char *id, char *name, collected_number multiplier, collected_number divisor, int algorithm);
extern uuid_t *sql_find_chart_uuid(RRDHOST *host, char *id, char *name, const char *type, const char *family,
                                 const char *context, const char *title, const char *units, const char *plugin, const char *module, long priority,
                                 int update_every, int chart_type, int memory_mode, long history_entries);
extern int sql_store_chart(
    uuid_t *chart_uuid, uuid_t *host_uuid, const char *type, const char *id, const char *name, const char *family,
    const char *context, const char *title, const char *units, const char *plugin, const char *module, long priority,
    int update_every, int chart_type, int memory_mode, long history_entries);

extern void sql_sync_ram_db();
extern void sql_backup_database();
extern void sql_compact_database();
extern void sql_store_datafile_info(char *path, int fileno, size_t file_size);
extern void sql_store_page_info(uuid_t temp_id, int valid_page, int page_length, usec_t  start_time, usec_t end_time, int , size_t offset, size_t size);
//extern void sql_add_metric_page_from_extent(struct rrdeng_page_descr *descr);
extern struct sqlite3_blob *sql_open_metric_blob(uuid_t *dim_uuid);

#ifdef ENABLE_DBENGINE
GUID_TYPE sql_find_object_by_guid(uuid_t *uuid, char *object, int max_size);
#endif
extern int sql_store_host(char *guid, char *hostname, char *registry_hostname, int update_every, char *os, char *timezone, char *tags);
extern void sql_rrdset2json(RRDHOST *host, BUFFER *wb, size_t *dimensions_count, size_t *memory_used);

extern time_t sql_rrdeng_metric_latest_time(RRDDIM *rd);
extern time_t sql_rrdeng_metric_oldest_time(RRDDIM *rd);

// Metric support
extern void rrddim_sql_collect_init(RRDDIM *rd);
extern void rrddim_sql_collect_store_metric(RRDDIM *rd, usec_t point_in_time, storage_number number);
extern int rrddim_sql_collect_finalize(RRDDIM *rd);
extern void rrddim_sql_query_init(RRDDIM *rd, struct rrddim_query_handle *handle, time_t start_time, time_t end_time);
extern storage_number rrddim_sql_query_next_metric(struct rrddim_query_handle *handle, time_t *current_time);
extern int rrddim_sql_query_is_finished(struct rrddim_query_handle *handle);
extern void rrddim_sql_query_finalize(struct rrddim_query_handle *handle);
extern time_t rrddim_sql_query_latest_time(RRDDIM *rd);
extern time_t rrddim_sql_query_oldest_time(RRDDIM *rd);
extern time_t sql_rrdset_first_entry_t(RRDSET *st, time_t *first, time_t *last);
extern time_t sql_rrdset_last_entry_t(RRDSET *st);
extern time_t sql_rrddim_first_last_entry_t(RRDDIM *rd, time_t *first, time_t *last);

extern struct rrddim_metric_page *rrddim_init_metric_page(RRDDIM *rd);


#endif //NETDATA_SQLITE_FUNCTIONS_H
