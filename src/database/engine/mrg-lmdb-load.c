// SPDX-License-Identifier: GPL-3.0-or-later

#include "mrg-lmdb.h"

// Function to retrieve a uint64_t value from the metadata database
static bool mrg_lmdb_get_meta_uint64(struct mrg_lmdb *lmdb, const char *key, uint64_t *value) {
    MDB_val k, v;
    k.mv_size = strlen(key);
    k.mv_data = (void *)key;

    int rc = mdb_get(lmdb->txn, lmdb->dbi[MRG_LMDB_DBI_METADATA], &k, &v);
    if (rc != MDB_SUCCESS) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "MRG LMDB: mdb_get() for key '%s' failed: %s", key, mdb_strerror(rc));
        return false;
    }

    if (v.mv_size != sizeof(uint64_t)) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "MRG LMDB: Invalid size for metadata value '%s'", key);
        return false;
    }

    memcpy(value, v.mv_data, sizeof(uint64_t));
    return true;
}

// Function to retrieve a file entry from the files database
static bool mrg_lmdb_get_file(struct mrg_lmdb *lmdb, uint32_t id, struct mrg_lmdb_file_value *file) {
    MDB_val key, data;
    key.mv_size = sizeof(id);
    key.mv_data = &id;

    int rc = mdb_get(lmdb->txn, lmdb->dbi[MRG_LMDB_DBI_FILES], &key, &data);
    if (rc != MDB_SUCCESS) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "MRG LMDB: mdb_get() for file id %u failed: %s", id, mdb_strerror(rc));
        return false;
    }

    if (data.mv_size != sizeof(struct mrg_lmdb_file_value)) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "MRG LMDB: Invalid size for file value");
        return false;
    }

    memcpy(file, data.mv_data, sizeof(struct mrg_lmdb_file_value));
    return true;
}

// Function to retrieve a UUID from the UUIDs database
static bool mrg_lmdb_get_uuid(struct mrg_lmdb *lmdb, uint32_t id, ND_UUID *uuid) {
    MDB_val key, data;
    key.mv_size = sizeof(id);
    key.mv_data = &id;

    int rc = mdb_get(lmdb->txn, lmdb->dbi[MRG_LMDB_DBI_UUIDS], &key, &data);
    if (rc != MDB_SUCCESS) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "MRG LMDB: mdb_get() for UUID id %u failed: %s", id, mdb_strerror(rc));
        return false;
    }

    if (data.mv_size != sizeof(ND_UUID)) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "MRG LMDB: Invalid size for UUID value");
        return false;
    }

    memcpy(uuid, data.mv_data, sizeof(*uuid));
    return true;
}

// Function to retrieve a metric entry from a tier database
static bool mrg_lmdb_get_metric_at_tier(struct mrg_lmdb *lmdb, size_t tier, uint32_t id, struct mrg_lmdb_metric_value *metric) {
    MDB_val key, data;
    key.mv_size = sizeof(id);
    key.mv_data = &id;

    int rc = mdb_get(lmdb->txn, lmdb->dbi[tier + MRG_LMDB_DBI_TIERS_BASE], &key, &data);
    if (rc != MDB_SUCCESS) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "MRG LMDB: mdb_get() for metric id %u at tier %zu failed: %s", id, tier, mdb_strerror(rc));
        return false;
    }

    if (data.mv_size != sizeof(struct mrg_lmdb_metric_value)) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "MRG LMDB: Invalid size for metric value");
        return false;
    }

    memcpy(metric, data.mv_data, sizeof(struct mrg_lmdb_metric_value));
    return true;
}

// Function to verify that expected files exist on the filesystem
static bool mrg_lmdb_verify_files(struct mrg_lmdb *lmdb) {
    uint32_t file_count = 0;

    // Get cursor for the files database
    MDB_cursor *cursor;
    int rc = mdb_cursor_open(lmdb->txn, lmdb->dbi[MRG_LMDB_DBI_FILES], &cursor);
    if (rc != MDB_SUCCESS) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "MRG LMDB: mdb_cursor_open() failed: %s", mdb_strerror(rc));
        return false;
    }

    MDB_val key, data;
    rc = mdb_cursor_get(cursor, &key, &data, MDB_FIRST);
    while (rc == MDB_SUCCESS) {
        struct mrg_lmdb_file_value *t = (struct mrg_lmdb_file_value *)data.mv_data;
        struct mrg_lmdb_file_value file_value;
        memcpy(&file_value, t, sizeof(struct mrg_lmdb_file_value));

        // Ensure the tier is valid
        if (file_value.tier >= lmdb->tiers) {
            nd_log(NDLS_DAEMON, NDLP_ERR, "MRG LMDB: Invalid tier %zu in file record", file_value.tier);
            mdb_cursor_close(cursor);
            return false;
        }

        // Check if the file exists on disk
        char filepath[FILENAME_MAX + 1];
        if(file_value.tier == 0)
            snprintfz(filepath, FILENAME_MAX, "%s/dbengine/" DATAFILE_PREFIX RRDENG_FILE_NUMBER_PRINT_TMPL DATAFILE_EXTENSION,
                      netdata_configured_cache_dir, (unsigned)1, (unsigned)file_value.fileno);
        else
            snprintfz(filepath, FILENAME_MAX, "%s/dbengine-tier%u/" DATAFILE_PREFIX RRDENG_FILE_NUMBER_PRINT_TMPL DATAFILE_EXTENSION,
                      netdata_configured_cache_dir, (unsigned)file_value.tier, (unsigned)1, (unsigned)file_value.fileno);

        struct stat st;
        if (stat(filepath, &st) != 0) {
            nd_log(NDLS_DAEMON, NDLP_WARNING, "MRG LMDB: Datafile %s not found", filepath);
            mdb_cursor_close(cursor);
            return false;
        }
        else {
            // Optionally verify file size and modification time
            if (st.st_size != (off_t)file_value.size) {
                nd_log(NDLS_DAEMON, NDLP_WARNING, "MRG LMDB: Datafile %s size mismatch: expected %zu, found %ld",
                       filepath, file_value.size, (long)st.st_size);
                mdb_cursor_close(cursor);
                return false;
            }

            usec_t file_mtime = STAT_GET_MTIME_SEC(st) * USEC_PER_SEC + STAT_GET_MTIME_NSEC(st) / 1000;
            if (file_mtime != file_value.mtime) {
                nd_log(NDLS_DAEMON, NDLP_WARNING, "MRG LMDB: Datafile %s modification time mismatch", filepath);
                mdb_cursor_close(cursor);
                return false;
            }
        }

        file_count++;
        rc = mdb_cursor_get(cursor, &key, &data, MDB_NEXT);
    }

    mdb_cursor_close(cursor);

    if (rc != MDB_NOTFOUND) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "MRG LMDB: Error reading files: %s", mdb_strerror(rc));
        return false;
    }

    nd_log(NDLS_DAEMON, NDLP_INFO, "MRG LMDB: Verified %u files", file_count);
    return true;
}

// Main function to load metrics from LMDB into MRG
bool mrg_lmdb_load(MRG *mrg) {
    usec_t started = now_monotonic_usec();

    // First check if the LMDB database exists
    char filename[FILENAME_MAX + 1];
    snprintfz(filename, FILENAME_MAX, "%s/" MRG_LMDB_FILE, netdata_configured_cache_dir);
    if (access(filename, F_OK) != 0) {
        nd_log(NDLS_DAEMON, NDLP_INFO, "MRG LMDB: Database file %s does not exist", filename);
        mrg_lmdb_unlink_all();
        return false;
    }

    struct mrg_lmdb lmdb;
    if (!mrg_lmdb_init(&lmdb, MRG_LMDB_MODE_LOAD, 0, 0, nd_profile.storage_tiers, false)) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "MRG LMDB: Failed to initialize LMDB for loading");
        mrg_lmdb_unlink_all();
        return false;
    }

    // Read metadata
    uint64_t version, base_time, metrics_count, tiers_count;
    if (!mrg_lmdb_get_meta_uint64(&lmdb, "version", &version) ||
        !mrg_lmdb_get_meta_uint64(&lmdb, "base_time", &base_time) ||
        !mrg_lmdb_get_meta_uint64(&lmdb, "metrics", &metrics_count) ||
        !mrg_lmdb_get_meta_uint64(&lmdb, "tiers", &tiers_count)) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "MRG LMDB: Failed to read metadata");
        goto failed;
    }

    // Check version compatibility
    if (version != 1) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "MRG LMDB: Unsupported version %lu", version);
        goto failed;
    }

    // Update base time
    lmdb.base_time = base_time;

    // Verify that the required number of tiers exist
    if (tiers_count != nd_profile.storage_tiers) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "MRG LMDB: wrong number of tiers (%lu in lmdb, %zu expected)",
               tiers_count, nd_profile.storage_tiers);
        goto failed;
    }

    // Verify files before loading metrics
    if (!mrg_lmdb_verify_files(&lmdb)) {
        nd_log(NDLS_DAEMON, NDLP_WARNING, "MRG LMDB: Some database files are missing or changed");
        goto failed;
    }

    time_t now_s = now_realtime_sec();
    uint32_t metrics_loaded = 0;
    uint32_t metrics_skipped = 0;

    // Process all metric UUIDs
    for (uint32_t id = 0; id < metrics_count; id++) {
        ND_UUID uuid;
        if (!mrg_lmdb_get_uuid(&lmdb, id, &uuid)) {
            nd_log(NDLS_DAEMON, NDLP_ERR, "MRG LMDB: Failed to read UUID for metric %u", id);
            goto failed;
        }

        // For each UUID, process all tiers
        for (size_t tier = 0; tier < tiers_count; tier++) {
            struct mrg_lmdb_metric_value metric_value;
            if (!mrg_lmdb_get_metric_at_tier(&lmdb, tier, id, &metric_value)) {
                // This UUID might not exist in all tiers, which is normal
                continue;
            }

            // Calculate actual times from base time
            time_t first_time_s = lmdb.base_time + metric_value.first_time;
            time_t last_time_s = lmdb.base_time + metric_value.last_time;
            uint32_t update_every_s = metric_value.update_every;

            // Check if the context exists
            if (!multidb_ctx[tier]) {
                nd_log(NDLS_DAEMON, NDLP_WARNING, "MRG LMDB: Tier %zu context is not initialized", tier);
                goto failed;
            }

            // Update the metric registry
            mrg_update_metric_retention_and_granularity_by_uuid(
                mrg,
                (Word_t)multidb_ctx[tier],
                &uuid.uuid,
                first_time_s,
                last_time_s,
                update_every_s,
                now_s
            );

            metrics_loaded++;
        }
    }

    if(!mrg_lmdb_finalize(&lmdb, false))
        goto failed;

    usec_t ended = now_monotonic_usec();
    char dt[32];
    duration_snprintf(dt, sizeof(dt), ended - started, "us", false);
    nd_log(NDLS_DAEMON, NDLP_INFO, "MRG LMDB: Loaded %u metrics, skipped %u metrics, in %s",
           metrics_loaded, metrics_skipped, dt);

    mrg_lmdb_unlink_all();

    return metrics_loaded > 0;

failed:
    mrg_lmdb_finalize(&lmdb, false);
    mrg_lmdb_unlink_all();
    return false;
}
