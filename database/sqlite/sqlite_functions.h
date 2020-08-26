// SPDX-License-Identifier: GPL-3.0-or-later


#ifndef NETDATA_SQLITE_FUNCTIONS_H
#define NETDATA_SQLITE_FUNCTIONS_H

#include "../../daemon/common.h"
#include "../rrd.h"

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
extern void sql_add_metric_page(uuid_t *dim_uuid, struct rrdeng_page_descr *descr);
//extern int sql_load_one_chart_dimension(uuid_t *chart_uuid, struct dimension **dimension_list);
extern int sql_load_one_chart_dimension(uuid_t *chart_uuid, BUFFER *wb, int *dimensions);
extern char *sql_find_dim_uuid(RRDSET *st, char *id, char *name);

extern void sql_sync_ram_db();
extern void sql_backup_database();
extern void sql_compact_database();
extern void sql_store_datafile_info(char *path, int fileno, size_t file_size);
extern void sql_store_page_info(uuid_t temp_id, int valid_page, int page_length, usec_t  start_time, usec_t end_time, int , size_t offset, size_t size);
extern void sql_add_metric_page_from_extent(struct rrdeng_page_descr *descr);
extern struct sqlite3_blob *sql_open_metric_blob(uuid_t *dim_uuid);



#endif //NETDATA_SQLITE_FUNCTIONS_H
