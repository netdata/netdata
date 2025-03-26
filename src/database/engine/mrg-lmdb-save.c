// SPDX-License-Identifier: GPL-3.0-or-later

#include "mrg-internals.h"
#include "rrdengineapi.h"
#include <lmdb.h>

struct mrg_lmdb_value {
    uint32_t first_time;
    uint32_t last_time;
    uint16_t update_every;
} __attribute__((packed));

struct mrg_lmdb_stats {
    uint32_t max_sections;
    uint32_t metrics;
    uint32_t min_update_every;
    uint32_t max_update_every;
    time_t min_first_time_s;
    time_t max_last_time_s;
};

void mrg_lmdb_statistics(MRG *mrg, struct mrg_lmdb_stats *stats) {
    memset(stats, 0, sizeof(*stats));
    stats->min_update_every = UINT32_MAX;
    stats->min_first_time_s = now_realtime_sec();

    for (size_t i = 0; i < UUIDMAP_PARTITIONS; i++) {
        mrg_index_read_lock(mrg, i);

        Word_t uuid_index = 0;

        // Traverse all UUIDs in this partition
        for(Pvoid_t *uuid_pvalue = JudyLFirst(mrg->index[i].uuid_judy, &uuid_index, PJE0);
             uuid_pvalue != NULL && uuid_pvalue != PJERR;
                uuid_pvalue = JudyLNext(mrg->index[i].uuid_judy, &uuid_index, PJE0)) {

            size_t sections = 0;
            Pvoid_t sections_judy = *uuid_pvalue;
            Word_t section_index = 0;
            for(Pvoid_t *section_pvalue = JudyLFirst(sections_judy, &section_index, PJE0);
                section_pvalue != NULL && section_pvalue != PJERR;
                section_pvalue = JudyLNext(sections_judy, &section_index, PJE0)) {

                METRIC *m = *section_pvalue;

                if(unlikely(m->latest_update_every_s < stats->min_update_every))
                    stats->min_update_every = m->latest_update_every_s;

                if(unlikely(m->latest_update_every_s > stats->max_update_every))
                    stats->max_update_every = m->latest_update_every_s;

                if(unlikely(m->first_time_s < stats->min_first_time_s))
                    stats->min_first_time_s = m->first_time_s;

                if(unlikely(m->latest_time_s_clean > stats->max_last_time_s))
                    stats->max_last_time_s = m->latest_time_s_clean;

                if(unlikely(m->latest_time_s_hot > stats->max_last_time_s))
                    stats->max_last_time_s = m->latest_time_s_hot;

                sections++;
            }

            stats->metrics++;

            if(sections > stats->max_sections)
                stats->max_sections = sections;
        }

        mrg_index_read_unlock(mrg, i);
    }
}

static void mrg_lmdb_unlink(void) {
    char old_filename[FILENAME_MAX + 1];

    snprintfz(old_filename, FILENAME_MAX, "%s/mrg.mdb", netdata_configured_cache_dir);
    unlink(old_filename);

    snprintfz(old_filename, FILENAME_MAX, "%s/mrg-lock.mdb", netdata_configured_cache_dir);
    unlink(old_filename);

    snprintfz(old_filename, FILENAME_MAX, "%s/mrg-tmp.mdb", netdata_configured_cache_dir);
    unlink(old_filename);

    snprintfz(old_filename, FILENAME_MAX, "%s/mrg-tmp-lock.mdb", netdata_configured_cache_dir);
    unlink(old_filename);

    errno_clear();
}

#define MRG_LMDB_DBI_METADATA 0
#define MRG_LMDB_DBI_FILES 1
#define MRG_LMDB_DBI_UUIDS 2
#define MRG_LMDB_DBI_TIERS_BASE 3

struct mrg_lmdb {
    time_t base_time;
    size_t metrics;
    size_t sections;
    size_t memory;
    size_t metrics_per_transaction;
    MDB_env *env;
    MDB_dbi dbi[RRD_STORAGE_TIERS + MRG_LMDB_DBI_TIERS_BASE];
    MDB_txn *txn;
    uint32_t metrics_in_this_transaction;
    uint32_t metrics_added;
};

static bool mrg_lmdb_init(struct mrg_lmdb *lmdb, time_t base_time, size_t metrics, size_t sections, size_t metrics_per_transaction) {
    memset(lmdb, 0, sizeof(*lmdb));

    lmdb->base_time = base_time;
    lmdb->metrics = metrics;
    lmdb->sections = sections;
    lmdb->metrics_per_transaction = metrics_per_transaction;
    lmdb->memory = (sizeof(struct mrg_lmdb_value) + sizeof(uint32_t) + 16) * metrics * sections +
                   (sizeof(ND_UUID) + 16) * metrics;
    lmdb->memory += lmdb->memory / 5; // 20% more for overhead

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
    rc = mdb_env_set_maxdbs(lmdb->env, sections + 2);
    if(rc != MDB_SUCCESS) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "MRG LMDB: mdb_env_set_maxdbs() failed: %s", mdb_strerror(rc));
        mdb_env_close(lmdb->env);
        return false;
    }

    // open the environment
    {
        char filename[FILENAME_MAX + 1];
        snprintfz(filename, FILENAME_MAX, "%s/mrg-tmp", netdata_configured_cache_dir);
        rc = mdb_env_open(lmdb->env, filename, MDB_WRITEMAP | MDB_NOSYNC | MDB_NOMETASYNC | MDB_MAPASYNC | MDB_NORDAHEAD | MDB_NOSUBDIR, 0660);
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
    for(size_t i = 0; i < lmdb->sections + MRG_LMDB_DBI_TIERS_BASE; i++) {
        char db_name[32];

        if(i == MRG_LMDB_DBI_METADATA)
            snprintfz(db_name, 32, "metadata");
        else if(i == MRG_LMDB_DBI_FILES)
            snprintfz(db_name, 32, "files");
        else if(i == MRG_LMDB_DBI_UUIDS)
            snprintfz(db_name, 32, "uuids");
        else
            snprintfz(db_name, 32, "tier-%zu", i - MRG_LMDB_DBI_TIERS_BASE);

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

void mrg_lmdb_finalize(struct mrg_lmdb *lmdb) {
    if(lmdb->txn) {
        mdb_txn_commit(lmdb->txn);
        lmdb->txn = NULL;
    }

    for(size_t i = 0; i < lmdb->sections + MRG_LMDB_DBI_TIERS_BASE; i++)
        mdb_dbi_close(lmdb->env, lmdb->dbi[i]);

    mdb_env_close(lmdb->env);
}

bool mrg_lmdb_reopen_transaction(struct mrg_lmdb *lmdb, bool grow) {
    if(lmdb->txn) {
        mdb_txn_commit(lmdb->txn);
        lmdb->txn = NULL;
    }

    if(grow) {
        mrg_lmdb_finalize(lmdb);

        lmdb->memory += lmdb->memory / 5; // 20% more for overhead
        if(!mrg_lmdb_init(lmdb, lmdb->base_time, lmdb->metrics, lmdb->sections, lmdb->metrics_per_transaction)) {
            nd_log(NDLS_DAEMON, NDLP_ERR, "MRG LMDB: failed to grow the LMDB environment");
            return false;
        }
    }

    // open the transaction
    int rc = mdb_txn_begin(lmdb->env, NULL, 0, &lmdb->txn);
    if(rc != MDB_SUCCESS) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "MRG LMDB: mdb_txn_begin() failed: %s", mdb_strerror(rc));
        return false;
    }

    lmdb->metrics_in_this_transaction = 0;
    return true;
}

bool mrg_lmdb_count_metric(struct mrg_lmdb *lmdb) {
    if(!lmdb->txn || lmdb->metrics_in_this_transaction >= lmdb->metrics_per_transaction) {
        if(!mrg_lmdb_reopen_transaction(lmdb, false))
            return false;
    }

    lmdb->metrics_in_this_transaction++;
    return true;
}

int mrg_lmdb_put_auto(struct mrg_lmdb *lmdb, MDB_dbi dbi, MDB_val *key, MDB_val *data) {
    int flags = 0;
    int rc = mdb_put(lmdb->txn, dbi, key, data, flags);
    if(rc == MDB_MAP_FULL) {
        if(!mrg_lmdb_reopen_transaction(lmdb, true)) {
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

bool mrg_lmdb_add_uuid(struct mrg_lmdb *lmdb, UUIDMAP_ID uid) {
    ND_UUID uuid = uuidmap_get(uid);
    if(UUIDiszero(uuid)) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "MRG LMDB: not saving, invalid UUID found");
        return false;
    }

    lmdb->metrics_added++;

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

    return true;
}

bool mrg_lmdb_add_metric_at_tier(struct mrg_lmdb *lmdb, size_t tier, size_t id, uint32_t update_every, time_t first_time_s, time_t last_time_s) {
    MDB_val key, data;
    key.mv_size = sizeof(uint32_t);
    key.mv_data = &id;

    struct mrg_lmdb_value value;
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

bool mrg_lmdb_save(MRG *mrg) {
    mrg_lmdb_unlink();

    struct mrg_lmdb_stats stats;
    mrg_lmdb_statistics(mrg, &stats);

    if(stats.max_sections != nd_profile.storage_tiers) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "MRG LMDB: not saving, max sections %u != storage tiers %zu", stats.max_sections, nd_profile.storage_tiers);
        return false;
    }

    if(stats.max_sections > RRD_STORAGE_TIERS) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "MRG LMDB: not saving, max sections %u > max storage tiers %d", stats.max_sections, RRD_STORAGE_TIERS);
        return false;
    }

    if(stats.max_update_every >= UINT16_MAX) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "MRG LMDB: not saving, max update every %u >= %u", stats.max_update_every, (uint32_t)UINT16_MAX);
        return false;
    }

    if(!stats.metrics) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "MRG LMDB: not saving, no metrics");
        return false;
    }

    if(!stats.min_first_time_s) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "MRG LMDB: not saving, no first time");
        return false;
    }

    if(stats.min_first_time_s > stats.max_last_time_s) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "MRG LMDB: not saving, min first time %ld > max last time %ld", stats.min_first_time_s, stats.max_last_time_s);
        return false;
    }

    struct mrg_lmdb lmdb;
    if(!mrg_lmdb_init(&lmdb, stats.min_first_time_s, stats.metrics, stats.max_sections, 100000)) {
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

            if(unlikely(!mrg_lmdb_count_metric(&lmdb))) {
                nd_log(NDLS_DAEMON, NDLP_ERR, "MRG LMDB: not saving, LMDB transaction failed");
                mrg_index_read_unlock(mrg, i);
                goto failed;
            }

            if(unlikely(!mrg_lmdb_add_uuid(&lmdb, uuid_index))) {
                nd_log(NDLS_DAEMON, NDLP_ERR, "MRG LMDB: not saving, failed to add UUID");
                mrg_index_read_unlock(mrg, i);
                goto failed;
            }

            Pvoid_t sections_judy = *uuid_pvalue;
            Word_t section_index = 0;
            for(Pvoid_t *section_pvalue = JudyLFirst(sections_judy, &section_index, PJE0);
                 section_pvalue != NULL && section_pvalue != PJERR;
                 section_pvalue = JudyLNext(sections_judy, &section_index, PJE0)) {

                METRIC *m = *section_pvalue;
                struct rrdengine_instance *ctx = (struct rrdengine_instance *)m->section;

                size_t tier = SIZE_MAX;
                for(size_t t = 0; t < stats.max_sections; t++) {
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

                if(unlikely(!mrg_lmdb_add_metric_at_tier(
                        &lmdb,
                        m->partition,
                        lmdb.metrics_added,
                        m->latest_update_every_s,
                        m->first_time_s,
                        MAX(m->latest_time_s_clean, m->latest_time_s_hot)))) {
                    nd_log(NDLS_DAEMON, NDLP_ERR, "MRG LMDB: not saving, failed to add metric");
                    mrg_index_read_unlock(mrg, i);
                    goto failed;
                }
            }
        }

        mrg_index_read_unlock(mrg, i);
    }

    // TODO: save the metadata
    // 1. the current version (1)
    // 2. the base time
    // 3. the number of metrics
    // 4. the number of sections

    // TODO: save the data files
    // for each ctx in multidb_ctx[i]
    // lock ctx->datafiles->rwlock
    // traverse the linked list ctx->datafiles->first
    // for each data file, stat() it and save the size and mtime

    // TODO: move the mrg-tmp files to mrg files

    mrg_lmdb_finalize(&lmdb);
    return true;

failed:
    mrg_lmdb_finalize(&lmdb);
    mrg_lmdb_unlink();
    return false;
}

