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

enum mrg_lmdb_mode {
    MRG_LMDB_MODE_SAVE,
    MRG_LMDB_MODE_LOAD,
};

struct mrg_lmdb {
    enum mrg_lmdb_mode mode;
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

static inline bool mrg_lmdb_init(struct mrg_lmdb *lmdb, enum mrg_lmdb_mode mode, time_t base_time, uint32_t metrics_per_transaction, uint32_t tiers, bool reopen) {
    if(!reopen)
        memset(lmdb, 0, sizeof(*lmdb));

    lmdb->mode = mode;
    lmdb->tiers = tiers;
    lmdb->base_time = base_time;
    lmdb->metrics_per_transaction = metrics_per_transaction;

    // open the environment
    int flags;
    char filename[FILENAME_MAX + 1];
    if(lmdb->mode == MRG_LMDB_MODE_SAVE) {
        snprintfz(filename, FILENAME_MAX, "%s/" MRG_LMDB_TMP_FILE, netdata_configured_cache_dir);
        flags = MDB_WRITEMAP | MDB_NOSYNC | MDB_NOMETASYNC | MDB_NORDAHEAD | MDB_NOSUBDIR | MDB_NOLOCK;
    }
    else {
        snprintfz(filename, FILENAME_MAX, "%s/" MRG_LMDB_FILE, netdata_configured_cache_dir);
        if (access(filename, F_OK) != 0) {
            nd_log(NDLS_DAEMON, NDLP_INFO, "MRG LMDB: Database file %s does not exist", filename);
            return false;
        }
        flags = MDB_RDONLY | MDB_NOLOCK | MDB_NOSUBDIR;
    }

    // create the LMDB environment
    int rc = mdb_env_create(&lmdb->env);
    if(rc != MDB_SUCCESS) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "MRG LMDB: mdb_env_create() failed: %s", mdb_strerror(rc));
        return false;
    }

    if(lmdb->mode == MRG_LMDB_MODE_SAVE) {
        // set the map size
        if (lmdb->memory)
            lmdb->memory *= 2;
        else
            lmdb->memory = 4 * 1024 * 1024;

        rc = mdb_env_set_mapsize(lmdb->env, lmdb->memory);
        if (rc != MDB_SUCCESS) {
            nd_log(NDLS_DAEMON, NDLP_ERR, "MRG LMDB: mdb_env_set_mapsize() failed: %s", mdb_strerror(rc));
            mdb_env_close(lmdb->env);
            return false;
        }
    }

    // set up the number of databases
    // section + 1 for the UUIDs + 1 for the metadata
    rc = mdb_env_set_maxdbs(lmdb->env, tiers + MRG_LMDB_DBI_TIERS_BASE);
    if(rc != MDB_SUCCESS) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "MRG LMDB: mdb_env_set_maxdbs() failed: %s", mdb_strerror(rc));
        mdb_env_close(lmdb->env);
        return false;
    }

    rc = mdb_env_open(lmdb->env, filename, flags, 0660);
    if(rc != MDB_SUCCESS) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "MRG LMDB: mdb_env_open() failed: %s", mdb_strerror(rc));
        mdb_env_close(lmdb->env);
        return false;
    }

    // open the first transaction
    rc = mdb_txn_begin(lmdb->env, NULL, lmdb->mode == MRG_LMDB_MODE_LOAD ? MDB_RDONLY : 0, &lmdb->txn);
    if(rc != MDB_SUCCESS) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "MRG LMDB: mdb_txn_begin() failed: %s", mdb_strerror(rc));
        return false;
    }

    // open the databases - these remain valid for the lifetime of the environment
    for(size_t i = 0; i < lmdb->tiers + MRG_LMDB_DBI_TIERS_BASE; i++) {
        char db_name[32];

        if(i == MRG_LMDB_DBI_METADATA)
            snprintfz(db_name, 32, MRG_LMDB_DBI_METADATA_NAME);
        else if(i == MRG_LMDB_DBI_FILES)
            snprintfz(db_name, 32, MRG_LMDB_DBI_FILES_NAME);
        else if(i == MRG_LMDB_DBI_UUIDS)
            snprintfz(db_name, 32, MRG_LMDB_DBI_UUIDS_NAME);
        else
            snprintfz(db_name, 32, MRG_LMDB_DBI_TIERS_FORMAT, (size_t)(i - MRG_LMDB_DBI_TIERS_BASE));

        rc = mdb_dbi_open(lmdb->txn, db_name, lmdb->mode == MRG_LMDB_MODE_LOAD ? 0 : MDB_CREATE, &lmdb->dbi[i]);
        if(rc != MDB_SUCCESS) {
            nd_log(NDLS_DAEMON, NDLP_ERR, "MRG LMDB: mdb_dbi_open() failed: %s", mdb_strerror(rc));
            mdb_txn_abort(lmdb->txn);
            lmdb->txn = NULL;
            return false;
        }
    }

    return true;
}

static inline void mrg_lmdb_finalize(struct mrg_lmdb *lmdb, bool sync) {
    if(!lmdb->env)
        return;

    if(lmdb->txn) {
        if(lmdb->mode == MRG_LMDB_MODE_SAVE)
            mdb_txn_commit(lmdb->txn);
        else
            mdb_txn_abort(lmdb->txn);
        lmdb->txn = NULL;
    }

    for(size_t i = 0; i < lmdb->tiers + MRG_LMDB_DBI_TIERS_BASE; i++) {
        mdb_dbi_close(lmdb->env, lmdb->dbi[i]);
        lmdb->dbi[i] = 0;
    }

    if(lmdb->mode == MRG_LMDB_MODE_SAVE && sync)
        mdb_env_sync(lmdb->env, 1);

    mdb_env_close(lmdb->env);
    lmdb->env = NULL;
}

static inline void mrg_lmdb_unlink_all(void) {
    char old_filename[FILENAME_MAX + 1];

    snprintfz(old_filename, FILENAME_MAX, "%s/" MRG_LMDB_FILE, netdata_configured_cache_dir);
    unlink(old_filename);

    snprintfz(old_filename, FILENAME_MAX, "%s/" MRG_LMDB_LOCK_FILE, netdata_configured_cache_dir);
    unlink(old_filename);

    snprintfz(old_filename, FILENAME_MAX, "%s/" MRG_LMDB_TMP_FILE, netdata_configured_cache_dir);
    unlink(old_filename);

    snprintfz(old_filename, FILENAME_MAX, "%s/" MRG_LMDB_TMP_LOCK_FILE, netdata_configured_cache_dir);
    unlink(old_filename);

    errno_clear();
}

static inline bool mrg_lmdb_rename_completed(void) {
    char old_filename[FILENAME_MAX + 1];
    char new_filename[FILENAME_MAX + 1];

    snprintfz(old_filename, FILENAME_MAX, "%s/" MRG_LMDB_TMP_FILE, netdata_configured_cache_dir);
    snprintfz(new_filename, FILENAME_MAX, "%s/" MRG_LMDB_FILE, netdata_configured_cache_dir);
    if(rename(old_filename, new_filename) != 0) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "MRG LMDB: rename from '%s' to '%s' failed", old_filename, new_filename);
        return false;
    }

    snprintfz(old_filename, FILENAME_MAX, "%s/" MRG_LMDB_TMP_LOCK_FILE, netdata_configured_cache_dir);
    snprintfz(new_filename, FILENAME_MAX, "%s/" MRG_LMDB_LOCK_FILE, netdata_configured_cache_dir);
    if(rename(old_filename, new_filename) != 0) {
        // we don't need a lock file by default
        // nd_log(NDLS_DAEMON, NDLP_ERR, "MRG LMDB: rename from '%s' to '%s' failed", old_filename, new_filename);
        // return false;
    }

    return true;
}

#endif //NETDATA_MRG_LMDB_H
