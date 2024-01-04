// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_DYNCFG_H
#define NETDATA_DYNCFG_H

#include "../common.h"

typedef enum __attribute__((packed)) {
    DYNCFG_TYPE_SINGLE = 0,
    DYNCFG_TYPE_TEMPLATE,
    DYNCFG_TYPE_JOB,
} DYNCFG_TYPE;
DYNCFG_TYPE dyncfg_type2id(const char *type);
const char *dyncfg_id2type(DYNCFG_TYPE type);

typedef enum __attribute__((packed)) {
    DYNCFG_SOURCE_TYPE_STOCK = 0,
    DYNCFG_SOURCE_TYPE_USER,
    DYNCFG_SOURCE_TYPE_DYNCFG,
    DYNCFG_SOURCE_TYPE_DISCOVERY,
} DYNCFG_SOURCE_TYPE;
DYNCFG_SOURCE_TYPE dyncfg_source_type2id(const char *source_type);
const char *dyncfg_id2source_type(DYNCFG_SOURCE_TYPE source_type);

typedef enum __attribute__((packed)) {
    DYNCFG_STATUS_NONE = 0,
    DYNCFG_STATUS_OK,
    DYNCFG_STATUS_DISABLED,
    DYNCFG_STATUS_REJECTED,
    DYNCFG_STATUS_ORPHAN,
} DYNCFG_STATUS;
DYNCFG_STATUS dyncfg_status2id(const char *status);
const char *dyncfg_id2status(DYNCFG_STATUS status);

typedef enum __attribute__((packed)) {
    DYNCFG_CMD_NONE     = 0,
    DYNCFG_CMD_GET      = (1 << 0),
    DYNCFG_CMD_SCHEMA   = (1 << 1),
    DYNCFG_CMD_UPDATE   = (1 << 2),
    DYNCFG_CMD_ADD      = (1 << 3),
    DYNCFG_CMD_REMOVE   = (1 << 4),
    DYNCFG_CMD_ENABLE   = (1 << 5),
    DYNCFG_CMD_DISABLE  = (1 << 6),
    DYNCFG_CMD_RESTART  = (1 << 7),
} DYNCFG_CMDS;
DYNCFG_CMDS dyncfg_cmds2id(const char *cmds);

#include "../../database/rrd.h"
#include "../../database/rrdfunctions.h"

bool dyncfg_add(RRDHOST *host, const char *id, const char *path, DYNCFG_STATUS status, DYNCFG_TYPE type, DYNCFG_SOURCE_TYPE source_type, const char *source, DYNCFG_CMDS cmds, usec_t created_ut, usec_t modified_ut, bool sync, rrd_function_execute_cb_t execute_cb, void *execute_cb_data);

void dyncfg_init(void);

#endif //NETDATA_DYNCFG_H
