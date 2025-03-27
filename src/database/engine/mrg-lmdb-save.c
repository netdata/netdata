// SPDX-License-Identifier: GPL-3.0-or-later

#include "mrg-lmdb.h"

#define MRG_LMDB_BASE_TIMESTAMP 1262304000

void mrg_lmdb_unlink_all(void) {
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

static bool mrg_lmdb_rename_completed(void) {
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

static bool mrg_lmdb_save_init(struct mrg_lmdb *lmdb, time_t base_time, uint32_t metrics_per_transaction, uint32_t tiers, bool grow) {
    if(!grow)
        memset(lmdb, 0, sizeof(*lmdb));

    lmdb->tiers = tiers;
    lmdb->base_time = base_time;
    lmdb->metrics_per_transaction = metrics_per_transaction;

    if(lmdb->memory)
        lmdb->memory *= 2;
    else
        lmdb->memory = 4 * 1024 * 1024;

    // create the LMDB environment
    int rc = mdb_env_create(&lmdb->env);
    if(rc != MDB_SUCCESS) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "MRG LMDB: mdb_env_create() failed: %s", mdb_strerror(rc));
        return false;
    }

    // set the map size
    rc = mdb_env_set_mapsize(lmdb->env, lmdb->memory);
    if(rc != MDB_SUCCESS) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "MRG LMDB: mdb_env_set_mapsize() failed: %s", mdb_strerror(rc));
        mdb_env_close(lmdb->env);
        return false;
    }

    // set up the number of databases
    // section + 1 for the UUIDs + 1 for the metadata
    rc = mdb_env_set_maxdbs(lmdb->env, tiers + MRG_LMDB_DBI_TIERS_BASE);
    if(rc != MDB_SUCCESS) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "MRG LMDB: mdb_env_set_maxdbs() failed: %s", mdb_strerror(rc));
        mdb_env_close(lmdb->env);
        return false;
    }

    // open the environment
    {
        char filename[FILENAME_MAX + 1];
        snprintfz(filename, FILENAME_MAX, "%s/" MRG_LMDB_TMP_FILE, netdata_configured_cache_dir);
        rc = mdb_env_open(lmdb->env, filename, MDB_WRITEMAP | MDB_NOSYNC | MDB_NOMETASYNC | MDB_NORDAHEAD | MDB_NOSUBDIR | MDB_NOLOCK, 0660);
        if(rc != MDB_SUCCESS) {
            nd_log(NDLS_DAEMON, NDLP_ERR, "MRG LMDB: mdb_env_open() failed: %s", mdb_strerror(rc));
            mdb_env_close(lmdb->env);
            return false;
        }
    }

    // open the first transaction
    rc = mdb_txn_begin(lmdb->env, NULL, 0, &lmdb->txn);
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

        rc = mdb_dbi_open(lmdb->txn, db_name, MDB_CREATE, &lmdb->dbi[i]);
        if(rc != MDB_SUCCESS) {
            nd_log(NDLS_DAEMON, NDLP_ERR, "MRG LMDB: mdb_dbi_open() failed: %s", mdb_strerror(rc));
            mdb_txn_abort(lmdb->txn);
            lmdb->txn = NULL;
            return false;
        }
    }

    return true;
}

static void mrg_lmdb_save_finalize(struct mrg_lmdb *lmdb, bool sync) {
    if(!lmdb->env)
        return;

    if(lmdb->txn) {
        mdb_txn_commit(lmdb->txn);
        lmdb->txn = NULL;
    }

    for(size_t i = 0; i < lmdb->tiers + MRG_LMDB_DBI_TIERS_BASE; i++) {
        mdb_dbi_close(lmdb->env, lmdb->dbi[i]);
        lmdb->dbi[i] = 0;
    }

    if(sync)
        mdb_env_sync(lmdb->env, 1);

    mdb_env_close(lmdb->env);
    lmdb->env = NULL;
}

static bool mrg_lmdb_save_reopen_transaction(struct mrg_lmdb *lmdb, bool grow) {
    if(lmdb->txn) {
        mdb_txn_commit(lmdb->txn);
        lmdb->txn = NULL;
    }

    if(grow) {
        mrg_lmdb_save_finalize(lmdb, false);
        if(!mrg_lmdb_save_init(lmdb, lmdb->base_time, lmdb->metrics_per_transaction, lmdb->tiers, grow)) {
            nd_log(NDLS_DAEMON, NDLP_ERR, "MRG LMDB: failed to grow the LMDB environment");
            return false;
        }
    }

    if(!lmdb->txn) {
        // open the transaction if not open already
        // keep in mind mrg_lmdb_init() opens a transaction too
        int rc = mdb_txn_begin(lmdb->env, NULL, 0, &lmdb->txn);
        if (rc != MDB_SUCCESS) {
            nd_log(NDLS_DAEMON, NDLP_ERR, "MRG LMDB: mdb_txn_begin() failed: %s", mdb_strerror(rc));
            return false;
        }
    }

    lmdb->metrics_in_this_transaction = 0;
    return true;
}

static int mrg_lmdb_put_auto(struct mrg_lmdb *lmdb, MDB_dbi dbi, MDB_val *key, MDB_val *data) {
    int flags = 0;
    int rc = mdb_put(lmdb->txn, dbi, key, data, flags);
    if(rc == MDB_MAP_FULL) {
        if(!mrg_lmdb_save_reopen_transaction(lmdb, true)) {
            nd_log(NDLS_DAEMON, NDLP_ERR, "MRG LMDB: failed to grow the LMDB environment");
            return rc;
        }

        rc = mdb_put(lmdb->txn, dbi, key, data, flags);
    }

    if(rc != MDB_SUCCESS) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "MRG LMDB: mdb_put() failed: %s", mdb_strerror(rc));
        return rc;
    }

    return rc;
}

static bool mrg_lmdb_put_uuid(struct mrg_lmdb *lmdb, UUIDMAP_ID uid) {
    ND_UUID uuid = uuidmap_get(uid);
    if(UUIDiszero(uuid)) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "MRG LMDB: not saving, invalid UUID found");
        return false;
    }

    MDB_val key, data;
    key.mv_size = sizeof(lmdb->metrics_added);
    key.mv_data = &lmdb->metrics_added;
    data.mv_size = sizeof(ND_UUID);
    data.mv_data = &uuid;

    int rc = mrg_lmdb_put_auto(lmdb, lmdb->dbi[MRG_LMDB_DBI_UUIDS], &key, &data);
    if(rc != MDB_SUCCESS) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "MRG LMDB: mdb_put() failed: %s", mdb_strerror(rc));
        return false;
    }

    lmdb->metrics_added++;
    lmdb->metrics_in_this_transaction++;

    if(!lmdb->txn || lmdb->metrics_in_this_transaction >= lmdb->metrics_per_transaction) {
        if(!mrg_lmdb_save_reopen_transaction(lmdb, false))
            return false;
    }

    return true;
}

static bool mrg_lmdb_put_metric_at_tier(struct mrg_lmdb *lmdb, size_t tier, size_t id, uint32_t update_every, time_t first_time_s, time_t last_time_s) {
    MDB_val key, data;
    key.mv_size = sizeof(uint32_t);
    key.mv_data = &id;

    struct mrg_lmdb_metric_value value;
    value.first_time = first_time_s - lmdb->base_time;
    value.last_time = last_time_s - lmdb->base_time;
    value.update_every = update_every;

    data.mv_size = sizeof(value);
    data.mv_data = &value;

    int rc = mrg_lmdb_put_auto(lmdb, lmdb->dbi[tier + MRG_LMDB_DBI_TIERS_BASE], &key, &data);
    if(rc != MDB_SUCCESS) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "MRG LMDB: mdb_put() failed: %s", mdb_strerror(rc));
        return false;
    }

    return true;
}

static bool mrg_lmdb_put_meta_uint64(struct mrg_lmdb *lmdb, MDB_dbi dbi, const char *key, uint64_t value) {
    MDB_val k, v;
    k.mv_size = strlen(key);
    k.mv_data = (void *)key;
    v.mv_size = sizeof(uint64_t);
    v.mv_data = &value;

    int rc = mrg_lmdb_put_auto(lmdb, dbi, &k, &v);
    if(rc != MDB_SUCCESS) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "MRG LMDB: mdb_put() failed: %s", mdb_strerror(rc));
        return false;
    }

    return true;
}

static bool mrg_lmdb_put_file(struct mrg_lmdb *lmdb, size_t tier, size_t fileno, size_t size, usec_t mtime) {
    lmdb->files_added++;

    MDB_val key, data;
    key.mv_size = sizeof(lmdb->files_added);
    key.mv_data = &lmdb->files_added;

    struct mrg_lmdb_file_value value = {
        .tier = tier,
        .fileno = fileno,
        .size = size,
        .mtime = mtime,
    };

    data.mv_size = sizeof(value);
    data.mv_data = &value;

    int rc = mrg_lmdb_put_auto(lmdb, lmdb->dbi[MRG_LMDB_DBI_FILES], &key, &data);
    if(rc != MDB_SUCCESS) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "MRG LMDB: mdb_put() failed: %s", mdb_strerror(rc));
        return false;
    }

    return true;
}

bool mrg_lmdb_save(MRG *mrg) {
    mrg_lmdb_unlink_all();

    struct mrg_lmdb lmdb;
    if(!mrg_lmdb_save_init(&lmdb, MRG_LMDB_BASE_TIMESTAMP, 100000, nd_profile.storage_tiers, false)) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "MRG LMDB: not saving, failed to initialize LMDB");
        return false;
    }

    for (size_t i = 0; i < UUIDMAP_PARTITIONS; i++) {
        mrg_index_read_lock(mrg, i);

        Word_t uuid_index = 0;

        // Traverse all UUIDs in this partition
        for(Pvoid_t *uuid_pvalue = JudyLFirst(mrg->index[i].uuid_judy, &uuid_index, PJE0);
             uuid_pvalue != NULL && uuid_pvalue != PJERR;
             uuid_pvalue = JudyLNext(mrg->index[i].uuid_judy, &uuid_index, PJE0)) {

            size_t added = 0;
            Pvoid_t sections_judy = *uuid_pvalue;
            Word_t section_index = 0;
            for(Pvoid_t *section_pvalue = JudyLFirst(sections_judy, &section_index, PJE0);
                 section_pvalue != NULL && section_pvalue != PJERR;
                 section_pvalue = JudyLNext(sections_judy, &section_index, PJE0)) {

                METRIC *m = *section_pvalue;

                if(unlikely(!m->first_time_s || !m->latest_time_s_clean)) {
                    lmdb.metrics_on_tiers_invalid++;
                    continue;
                }

                struct rrdengine_instance *ctx = (struct rrdengine_instance *)m->section;

                size_t tier = SIZE_MAX;
                for(size_t t = 0; t < lmdb.tiers; t++) {
                    if(ctx == multidb_ctx[t]) {
                        tier = t;
                        break;
                    }
                }

                if(unlikely(tier == SIZE_MAX)) {
                    nd_log(NDLS_DAEMON, NDLP_ERR, "MRG LMDB: not saving, invalid tier");
                    mrg_index_read_unlock(mrg, i);
                    goto failed;
                }

                if(unlikely(!mrg_lmdb_put_metric_at_tier(
                        &lmdb,
                        tier,
                        lmdb.metrics_added,
                        m->latest_update_every_s,
                        m->first_time_s,
                        m->latest_time_s_clean))) {
                    nd_log(NDLS_DAEMON, NDLP_ERR, "MRG LMDB: not saving, failed to add metric");
                    mrg_index_read_unlock(mrg, i);
                    goto failed;
                }
                added++;
                lmdb.metrics_on_tiers_ok++;
            }

            if(unlikely(added && !mrg_lmdb_put_uuid(&lmdb, uuid_index))) {
                nd_log(NDLS_DAEMON, NDLP_ERR, "MRG LMDB: not saving, failed to add UUID");
                mrg_index_read_unlock(mrg, i);
                goto failed;
            }
        }

        mrg_index_read_unlock(mrg, i);
    }

    char filename[FILENAME_MAX + 1];
    for(size_t tier = 0; tier < RRD_STORAGE_TIERS ; tier++) {
        if(!multidb_ctx[tier]) continue;

        uv_rwlock_rdlock(&multidb_ctx[tier]->datafiles.rwlock);
        for(struct rrdengine_datafile *d = multidb_ctx[tier]->datafiles.first; d ;d = d->next) {
            if(d->tier != 1) {
                nd_log(NDLS_DAEMON, NDLP_ERR, "MRG LMDB: not saving, invalid tier %u", d->tier);
                uv_rwlock_rdunlock(&multidb_ctx[tier]->datafiles.rwlock);
                goto failed;
            }

            generate_datafilepath(d, filename, sizeof(filename));

            struct stat st;
            if(stat(filename, &st) != 0) {
                nd_log(NDLS_DAEMON, NDLP_ERR, "MRG LMDB: not saving, failed to stat() %s: %s", filename, strerror(errno));
                uv_rwlock_rdunlock(&multidb_ctx[tier]->datafiles.rwlock);
                goto failed;
            }

            if(!mrg_lmdb_put_file(&lmdb, tier, d->fileno, st.st_size, st.st_mtime * USEC_PER_SEC + st.st_mtim.tv_nsec / 1000)) {
                nd_log(NDLS_DAEMON, NDLP_ERR, "MRG LMDB: not saving, failed to add file");
                uv_rwlock_rdunlock(&multidb_ctx[tier]->datafiles.rwlock);
                goto failed;
            }
        }
        uv_rwlock_rdunlock(&multidb_ctx[tier]->datafiles.rwlock);
    }

    // save the metadata
    if (!mrg_lmdb_put_meta_uint64(&lmdb, lmdb.dbi[MRG_LMDB_DBI_METADATA], "version", 1) ||
        !mrg_lmdb_put_meta_uint64(&lmdb, lmdb.dbi[MRG_LMDB_DBI_METADATA], "base_time", lmdb.base_time) ||
        !mrg_lmdb_put_meta_uint64(&lmdb, lmdb.dbi[MRG_LMDB_DBI_METADATA], "metrics", lmdb.metrics_added) ||
        !mrg_lmdb_put_meta_uint64(&lmdb, lmdb.dbi[MRG_LMDB_DBI_METADATA], "tiers", lmdb.tiers))
        goto failed;

    mrg_lmdb_save_finalize(&lmdb, true);

    bool rc = mrg_lmdb_rename_completed();
    if(rc)
        nd_log(NDLS_DAEMON, NDLP_INFO, "MRG LMDB: saved %u metrics in %u tiers (%u total, %u invalid), from %u files.",
               lmdb.metrics_added, lmdb.tiers, lmdb.metrics_on_tiers_ok, lmdb.metrics_on_tiers_invalid, lmdb.files_added);
    else
        goto failed;

    return rc;

failed:
    mrg_lmdb_save_finalize(&lmdb, false);
    mrg_lmdb_unlink_all();
    return false;
}
