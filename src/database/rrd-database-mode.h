// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_RRD_DATABASE_MODE_H
#define NETDATA_RRD_DATABASE_MODE_H

typedef enum __attribute__ ((__packed__)) rrd_memory_mode {
    RRD_DB_MODE_NONE = 0,
    RRD_DB_MODE_RAM = 1,
    RRD_DB_MODE_ALLOC = 4,
    RRD_DB_MODE_DBENGINE = 5,

    // this is 8-bit
} RRD_DB_MODE;

#define RRD_DB_MODE_NONE_NAME "none"
#define RRD_DB_MODE_RAM_NAME "ram"
#define RRD_DB_MODE_ALLOC_NAME "alloc"
#define RRD_DB_MODE_DBENGINE_NAME "dbengine"

extern RRD_DB_MODE default_rrd_memory_mode;

const char *rrd_memory_mode_name(RRD_DB_MODE id);
RRD_DB_MODE rrd_memory_mode_id(const char *name);

#endif //NETDATA_RRD_DATABASE_MODE_H
