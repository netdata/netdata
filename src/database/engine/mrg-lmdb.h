// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_MRG_LMDB_H
#define NETDATA_MRG_LMDB_H

#include "mrg-internals.h"
#include "rrdengineapi.h"
#include <lmdb.h>

struct mrg_lmdb_metric_value {
    uint32_t first_time;
    uint32_t last_time;
    uint32_t update_every;
};

struct mrg_lmdb_file_value {
    size_t tier;
    size_t fileno;
    uint64_t size;
    usec_t mtime;
};

#define MRG_LMDB_LOCK_SUFFIX "-lock"
#define MRG_LMDB_EXTENSION ".mdb"
#define MRG_LMDB_FILE "mrg" MRG_LMDB_EXTENSION
#define MRG_LMDB_LOCK_FILE MRG_LMDB_FILE MRG_LMDB_LOCK_SUFFIX
#define MRG_LMDB_TMP_FILE "mrg-tmp" MRG_LMDB_EXTENSION
#define MRG_LMDB_TMP_LOCK_FILE MRG_LMDB_TMP_FILE MRG_LMDB_LOCK_SUFFIX

#define MRG_LMDB_DBI_METADATA 0
#define MRG_LMDB_DBI_FILES 1
#define MRG_LMDB_DBI_UUIDS 2
#define MRG_LMDB_DBI_TIERS_BASE 3

#define MRG_LMDB_DBI_METADATA_NAME "metadata"
#define MRG_LMDB_DBI_FILES_NAME "files"
#define MRG_LMDB_DBI_UUIDS_NAME "uuids"
#define MRG_LMDB_DBI_TIERS_FORMAT "tier-%zu"

struct mrg_lmdb {
    time_t base_time;
    size_t memory;
    MDB_env *env;
    MDB_dbi dbi[RRD_STORAGE_TIERS + MRG_LMDB_DBI_TIERS_BASE];
    MDB_txn *txn;
    uint32_t metrics_per_transaction;
    uint32_t metrics_in_this_transaction;
    uint32_t metrics_added;
    uint32_t files_added;
    uint32_t tiers;

    uint32_t metrics_on_tiers_ok;
    uint32_t metrics_on_tiers_invalid;
};

void mrg_lmdb_unlink_all(void);

#endif //NETDATA_MRG_LMDB_H
