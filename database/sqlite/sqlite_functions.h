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

extern int sql_init_database();
extern int sql_close_database();
extern int sql_store_dimension(uuid_t *dim_uuid, uuid_t *chart_uuid, const char *id, const char *name, collected_number multiplier,
                        collected_number divisor, int algorithm);
extern int sql_select_dimension(uuid_t *chart_uuid, struct dimension **dimension_list);
extern int sql_dimension_archive(uuid_t *dim_uuid, int archive);
extern int sql_dimension_options(uuid_t *dim_uuid, char *options);
extern int sql_load_dimension(char *dim_str, RRDSET *st);
extern int sql_load_chart_dimensions(RRDSET *st, char *dimensions);
extern void sql_add_metric(uuid_t *dim_uuid, usec_t point_in_time, storage_number number);
extern void sql_add_metric_page(uuid_t *dim_uuid, struct rrdeng_page_descr *descr);



#endif //NETDATA_SQLITE_FUNCTIONS_H
