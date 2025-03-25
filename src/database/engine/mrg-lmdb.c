// SPDX-License-Identifier: GPL-3.0-or-later

#include "libnetdata/libnetdata.h"
#include "mrg-lmdb.h"
#include "mrg-internals.h"

// LMDB environment and database handles structure
typedef struct lmdb_instance {
    MDB_env *env;
    char path[RRDENG_PATH_MAX];
    SPINLOCK spinlock;
    struct lmdb_instance *next;
} LMDB_INSTANCE;

// Linked list of LMDB environments (one per tier)
static LMDB_INSTANCE *lmdb_instances = NULL;
static SPINLOCK lmdb_instances_spinlock = SPINLOCK_INITIALIZER;

// LMDB database names
#define MRG_DB_META "meta"         // Metadata about the MRG dump
#define MRG_DB_METRICS "metrics"   // Metric entries
#define MRG_DB_DATAFILES "files"   // Datafile information

// Maximum number of named databases to use
#define MRG_DBS 3

// Initial LMDB map size (1MB)
#define MRG_INITIAL_MAP_SIZE (1ULL * 1024 * 1024)

// Maximum LMDB map size (4GB)
#define MRG_MAX_MAP_SIZE (4ULL * 1024 * 1024 * 1024)

// Metadata keys
#define META_KEY_VERSION "version"
#define META_KEY_TIMESTAMP "timestamp"
#define META_KEY_DATAFILES_COUNT "datafiles_count"
#define META_KEY_METRICS_COUNT "metrics_count"

// Current version of the MRG dump format
#define MRG_DUMP_VERSION 1

// Helper function to convert real timestamp to relative time since base
static inline uint32_t time_to_lmdb_time(time_t real_time) {
    if (real_time <= METRIC_LMDB_TIME_BASE)
        return 0;

    return (uint32_t)(real_time - METRIC_LMDB_TIME_BASE);
}

// Helper function to convert relative timestamp back to real time
static inline time_t lmdb_time_to_real_time(uint32_t lmdb_time) {
    return (time_t)lmdb_time + METRIC_LMDB_TIME_BASE;
}

// Find or create an LMDB instance for the given path
static LMDB_INSTANCE *get_lmdb_instance(const char *path) {
    LMDB_INSTANCE *instance;

    spinlock_lock(&lmdb_instances_spinlock);

    // Try to find an existing instance
    for (instance = lmdb_instances; instance; instance = instance->next) {
        if (strcmp(instance->path, path) == 0) {
            spinlock_unlock(&lmdb_instances_spinlock);
            return instance;
        }
    }

    // Create a new instance
    instance = callocz(1, sizeof(LMDB_INSTANCE));
    strncpy(instance->path, path, sizeof(instance->path) - 1);
    spinlock_init(&instance->spinlock);

    // Add to linked list
    instance->next = lmdb_instances;
    lmdb_instances = instance;

    spinlock_unlock(&lmdb_instances_spinlock);
    return instance;
}

// Helper function to get an LMDB transaction
static int mrg_lmdb_transaction_get(MDB_env *env, MDB_txn **txn, unsigned int flags) {
    int rc;

    if (!env)
        return -1;

    rc = mdb_txn_begin(env, NULL, flags, txn);
    if (rc != 0) {
        netdata_log_error("LMDB: Failed to start transaction: %s", mdb_strerror(rc));
        return -1;
    }

    return 0;
}

// Helper function to grow the LMDB map size as needed
static int mrg_lmdb_grow_mapsize(MDB_env *env, MDB_txn *txn) {
    int rc;
    size_t current_mapsize = 0;
    size_t new_mapsize;
    MDB_envinfo info;

    // Get current map size
    mdb_env_info(env, &info);
    current_mapsize = info.me_mapsize;

    // Calculate new map size (double current size)
    new_mapsize = current_mapsize * 2;

    // Ensure we don't exceed the maximum
    if (new_mapsize > MRG_MAX_MAP_SIZE)
        new_mapsize = MRG_MAX_MAP_SIZE;

    if (new_mapsize <= current_mapsize)
        return 0; // No need to grow

    // We need to abort the current transaction, resize, and retry
    mdb_txn_abort(txn);

    netdata_log_info("LMDB: Growing map size from %zu MB to %zu MB",
                     current_mapsize / (1024 * 1024),
                     new_mapsize / (1024 * 1024));

    rc = mdb_env_set_mapsize(env, new_mapsize);
    if (rc != 0) {
        netdata_log_error("LMDB: Failed to grow map size: %s", mdb_strerror(rc));
        return -1;
    }

    return 1; // Signal that the transaction needs to be restarted
}

// Initialize LMDB environment for a specific tier
int mrg_lmdb_init(const char *path) {
    int rc;
    char lmdb_path[RRDENG_PATH_MAX];
    LMDB_INSTANCE *instance;

    // Get or create LMDB instance
    instance = get_lmdb_instance(path);

    // Acquire lock to ensure thread safety
    spinlock_lock(&instance->spinlock);

    // Check if we're already initialized
    if (instance->env) {
        spinlock_unlock(&instance->spinlock);
        return 0;
    }

    // Create LMDB directory
    snprintf(lmdb_path, RRDENG_PATH_MAX, "%s/mrg_lmdb", path);

    // Create directory if it doesn't exist
    if (mkdir(lmdb_path, 0755) == -1 && errno != EEXIST) {
        netdata_log_error("LMDB: Failed to create directory %s: %s", lmdb_path, strerror(errno));
        spinlock_unlock(&instance->spinlock);
        return -1;
    }

    // Initialize LMDB environment
    rc = mdb_env_create(&instance->env);
    if (rc != 0) {
        netdata_log_error("LMDB: Failed to create environment: %s", mdb_strerror(rc));
        spinlock_unlock(&instance->spinlock);
        return -1;
    }

    // Set maximum number of databases
    rc = mdb_env_set_maxdbs(instance->env, MRG_DBS);
    if (rc != 0) {
        netdata_log_error("LMDB: Failed to set max DBs: %s", mdb_strerror(rc));
        mdb_env_close(instance->env);
        instance->env = NULL;
        spinlock_unlock(&instance->spinlock);
        return -1;
    }

    // Set initial map size
    rc = mdb_env_set_mapsize(instance->env, MRG_INITIAL_MAP_SIZE);
    if (rc != 0) {
        netdata_log_error("LMDB: Failed to set map size: %s", mdb_strerror(rc));
        mdb_env_close(instance->env);
        instance->env = NULL;
        spinlock_unlock(&instance->spinlock);
        return -1;
    }

    // Open environment
    rc = mdb_env_open(instance->env, lmdb_path, MDB_NOSUBDIR, 0664);
    if (rc != 0) {
        netdata_log_error("LMDB: Failed to open environment: %s", mdb_strerror(rc));
        mdb_env_close(instance->env);
        instance->env = NULL;
        spinlock_unlock(&instance->spinlock);
        return -1;
    }

    netdata_log_info("LMDB: MRG database initialized at %s", lmdb_path);
    spinlock_unlock(&instance->spinlock);
    return 0;
}

// Collect datafile information for the given context
static int mrg_collect_datafile_info(struct rrdengine_instance *ctx, MRG_DATAFILE_INFO **datafiles, size_t *count) {
    struct rrdengine_datafile *df;
    struct stat st;
    size_t df_count = 0;
    char full_path[RRDENG_PATH_MAX];

    // First, count the datafiles
    uv_rwlock_rdlock(&ctx->datafiles.rwlock);
    for (df = ctx->datafiles.first; df; df = df->next)
        df_count++;

    // Allocate memory for datafiles info
    *datafiles = mallocz(df_count * sizeof(MRG_DATAFILE_INFO));
    *count = df_count;

    // Fill the datafile info
    size_t i = 0;
    for (df = ctx->datafiles.first; df; df = df->next) {
        generate_datafilepath(df, full_path, sizeof(full_path));
        strncpy((*datafiles)[i].filename, full_path, RRDENG_PATH_MAX - 1);

        if (stat(full_path, &st) == 0) {
            (*datafiles)[i].file_size = (uint64_t)st.st_size;
            (*datafiles)[i].last_modified = st.st_mtime;
        } else {
            netdata_log_error("LMDB: Cannot stat datafile %s: %s", full_path, strerror(errno));
            (*datafiles)[i].file_size = 0;
            (*datafiles)[i].last_modified = 0;
        }

        i++;
    }

    uv_rwlock_rdunlock(&ctx->datafiles.rwlock);
    return 0;
}

// Validate datafiles against stored info
static int mrg_validate_datafiles(struct rrdengine_instance *ctx, MDB_txn *txn) {
    int rc;
    MDB_dbi dbi;
    MDB_cursor *cursor;
    MDB_val key, value;
    struct stat st;
    size_t datafiles_in_db = 0;
    size_t datafiles_on_disk = 0;

    // Open datafiles database
    rc = mdb_dbi_open(txn, MRG_DB_DATAFILES, 0, &dbi);
    if (rc != 0) {
        netdata_log_error("LMDB: Failed to open datafiles DB: %s", mdb_strerror(rc));
        return -1;
    }

    // Get a cursor for the datafiles DB
    rc = mdb_cursor_open(txn, dbi, &cursor);
    if (rc != 0) {
        netdata_log_error("LMDB: Failed to open cursor: %s", mdb_strerror(rc));
        return -1;
    }

    // Count datafiles on disk
    uv_rwlock_rdlock(&ctx->datafiles.rwlock);
    for (struct rrdengine_datafile *df = ctx->datafiles.first; df; df = df->next)
        datafiles_on_disk++;
    uv_rwlock_rdunlock(&ctx->datafiles.rwlock);

    // Initialize cursor
    rc = mdb_cursor_get(cursor, &key, &value, MDB_FIRST);

    // Check each datafile in the database
    while (rc == 0) {
        datafiles_in_db++;

        MRG_DATAFILE_INFO *df_info = (MRG_DATAFILE_INFO *)value.mv_data;

        // Validate the file exists and has the same attributes
        if (stat(df_info->filename, &st) != 0) {
            netdata_log_error("LMDB: Datafile %s from MRG dump not found on disk", df_info->filename);
            mdb_cursor_close(cursor);
            return -1;
        }

        // Check file size and modification time
        if ((uint64_t)st.st_size != df_info->file_size || st.st_mtime != df_info->last_modified) {
            netdata_log_error("LMDB: Datafile %s has changed (size or timestamp mismatch)", df_info->filename);
            mdb_cursor_close(cursor);
            return -1;
        }

        // Get next datafile
        rc = mdb_cursor_get(cursor, &key, &value, MDB_NEXT);
    }

    mdb_cursor_close(cursor);

    // We need an exact match of files - if there are more files on disk,
    // we need to scan the new ones
    if (datafiles_on_disk > datafiles_in_db) {
        netdata_log_info("LMDB: Found %zu new datafiles, need to update MRG",
                         datafiles_on_disk - datafiles_in_db);
        return 1; // Return 1 to indicate new files found
    }

    // If we have fewer files, something is wrong
    if (datafiles_on_disk < datafiles_in_db) {
        netdata_log_error("LMDB: Missing datafiles on disk (%zu vs %zu in MRG dump)",
                          datafiles_on_disk, datafiles_in_db);
        return -1;
    }

    return 0;
}

// Store datafile information for validation
static int mrg_lmdb_store_datafiles(MDB_txn *txn, struct rrdengine_instance *ctx,
                                    MRG_DATAFILE_INFO *datafiles, size_t count) {
    int rc;
    MDB_dbi dbi;

    // Open or create datafiles DB
    rc = mdb_dbi_open(txn, MRG_DB_DATAFILES, MDB_CREATE, &dbi);
    if (rc != 0) {
        netdata_log_error("LMDB: Failed to open datafiles DB: %s", mdb_strerror(rc));
        return rc;
    }

    // Delete any existing entries
    rc = mdb_drop(txn, dbi, 0);
    if (rc != 0) {
        netdata_log_error("LMDB: Failed to clear datafiles DB: %s", mdb_strerror(rc));
        return rc;
    }

    // Store each datafile info
    for (size_t i = 0; i < count; i++) {
        MDB_val key, value;

        key.mv_size = sizeof(int);
        key.mv_data = &i;  // Use index as key

        value.mv_size = sizeof(MRG_DATAFILE_INFO);
        value.mv_data = &datafiles[i];

        rc = mdb_put(txn, dbi, &key, &value, 0);
        if (rc != 0) {
            netdata_log_error("LMDB: Failed to store datafile info: %s", mdb_strerror(rc));
            return rc;
        }
    }

    // Store metadata about datafiles count
    MDB_dbi meta_dbi;
    rc = mdb_dbi_open(txn, MRG_DB_META, MDB_CREATE, &meta_dbi);
    if (rc != 0) {
        netdata_log_error("LMDB: Failed to open metadata DB: %s", mdb_strerror(rc));
        return rc;
    }

    MDB_val meta_key, meta_value;
    meta_key.mv_size = strlen(META_KEY_DATAFILES_COUNT) + 1;
    meta_key.mv_data = META_KEY_DATAFILES_COUNT;
    meta_value.mv_size = sizeof(size_t);
    meta_value.mv_data = &count;

    rc = mdb_put(txn, meta_dbi, &meta_key, &meta_value, 0);
    if (rc != 0) {
        netdata_log_error("LMDB: Failed to store datafiles count: %s", mdb_strerror(rc));
        return rc;
    }

    return 0;
}

// Store metadata for MRG dump
static int mrg_lmdb_store_metadata(MDB_txn *txn, size_t metrics_count) {
    int rc;
    MDB_dbi dbi;
    MDB_val key, value;
    uint32_t version = MRG_DUMP_VERSION;
    time_t timestamp = now_realtime_sec();

    // Open metadata database
    rc = mdb_dbi_open(txn, MRG_DB_META, MDB_CREATE, &dbi);
    if (rc != 0) {
        netdata_log_error("LMDB: Failed to open metadata DB: %s", mdb_strerror(rc));
        return rc;
    }

    // Store version
    key.mv_size = strlen(META_KEY_VERSION) + 1;
    key.mv_data = META_KEY_VERSION;
    value.mv_size = sizeof(uint32_t);
    value.mv_data = &version;

    rc = mdb_put(txn, dbi, &key, &value, 0);
    if (rc != 0) {
        netdata_log_error("LMDB: Failed to store version: %s", mdb_strerror(rc));
        return rc;
    }

    // Store timestamp
    key.mv_size = strlen(META_KEY_TIMESTAMP) + 1;
    key.mv_data = META_KEY_TIMESTAMP;
    value.mv_size = sizeof(time_t);
    value.mv_data = &timestamp;

    rc = mdb_put(txn, dbi, &key, &value, 0);
    if (rc != 0) {
        netdata_log_error("LMDB: Failed to store timestamp: %s", mdb_strerror(rc));
        return rc;
    }

    // Store metrics count
    key.mv_size = strlen(META_KEY_METRICS_COUNT) + 1;
    key.mv_data = META_KEY_METRICS_COUNT;
    value.mv_size = sizeof(size_t);
    value.mv_data = &metrics_count;

    rc = mdb_put(txn, dbi, &key, &value, 0);
    if (rc != 0) {
        netdata_log_error("LMDB: Failed to store metrics count: %s", mdb_strerror(rc));
        return rc;
    }

    return 0;
}

// Callback for iterating through the MRG to save metrics
typedef struct {
    MDB_txn *txn;
    MDB_dbi dbi;
    size_t count;
    Word_t section;
} MRG_SAVE_CONTEXT;

// Helper function to save a single metric
static inline int mrg_save_metric(MRG_SAVE_CONTEXT *ctx, METRIC *metric) {
    int rc;
    MDB_val key, value;
    METRIC_LMDB_KEY lmdb_key;
    METRIC_LMDB_VALUE lmdb_value;
    time_t first_time_s, last_time_s;
    uint32_t update_every_s;

    // Get metric details
    nd_uuid_t *uuid = mrg_metric_uuid(main_mrg, metric);
    mrg_metric_get_retention(main_mrg, metric, &first_time_s, &last_time_s, &update_every_s);

    // Skip metrics with no retention
    if (first_time_s == 0 || last_time_s == 0)
        return 0;

    // Fill LMDB key structure (just UUID, tier is implicit from DB location)
    uuid_copy(lmdb_key.uuid.uuid, *uuid);

    // Fill LMDB value structure
    lmdb_value.update_every = update_every_s;
    lmdb_value.first_time_s = time_to_lmdb_time(first_time_s);
    lmdb_value.last_time_s = time_to_lmdb_time(last_time_s);

    // Use UUID as key
    key.mv_size = sizeof(METRIC_LMDB_KEY);
    key.mv_data = &lmdb_key;

    // Use value structure
    value.mv_size = sizeof(METRIC_LMDB_VALUE);
    value.mv_data = &lmdb_value;

    // Store in LMDB
    rc = mdb_put(ctx->txn, ctx->dbi, &key, &value, 0);
    if (rc != 0) {
        if (rc == MDB_MAP_FULL) {
            return MDB_MAP_FULL;  // Signal to grow map size
        }

        netdata_log_error("LMDB: Failed to store metric: %s", mdb_strerror(rc));
        return -1;
    }

    ctx->count++;
    return 0;
}

// Callback for traversing MRG
static int mrg_traverse_save_callback(METRIC *metric, void *data) {
    MRG_SAVE_CONTEXT *ctx = (MRG_SAVE_CONTEXT *)data;

    // Skip metrics from other sections
    if (mrg_metric_section(main_mrg, metric) != ctx->section)
        return 0;

    int rc = mrg_save_metric(ctx, metric);
    if (rc == MDB_MAP_FULL) {
        return MDB_MAP_FULL;  // Signal to grow map size
    }
    return rc;
}

// Save MRG to LMDB for a specific section
int mrg_lmdb_save(Word_t section, const char *path) {
    int rc;
    MDB_txn *txn = NULL;
    MDB_dbi dbi;
    MRG_SAVE_CONTEXT save_ctx;
    MRG_DATAFILE_INFO *datafiles = NULL;
    size_t datafiles_count = 0;
    int retry_count = 0;
    LMDB_INSTANCE *instance;
    struct rrdengine_instance *ctx = (struct rrdengine_instance *)section;

    // Initialize LMDB for this tier if needed
    rc = mrg_lmdb_init(path);
    if (rc != 0) {
        return -1;
    }

    // Get LMDB instance
    instance = get_lmdb_instance(path);
    if (!instance || !instance->env) {
        netdata_log_error("LMDB: Failed to get LMDB instance for %s", path);
        return -1;
    }

    netdata_log_info("LMDB: Saving MRG for tier %d...", ctx->config.tier);

    // Collect datafile information
    rc = mrg_collect_datafile_info(ctx, &datafiles, &datafiles_count);
    if (rc != 0) {
        netdata_log_error("LMDB: Failed to collect datafile information");
        return -1;
    }

retry_transaction:
    // Begin transaction
    rc = mrg_lmdb_transaction_get(instance->env, &txn, 0);
    if (rc != 0) {
        freez(datafiles);
        return -1;
    }

    // Open metrics database
    rc = mdb_dbi_open(txn, MRG_DB_METRICS, MDB_CREATE, &dbi);
    if (rc != 0) {
        netdata_log_error("LMDB: Failed to open metrics DB: %s", mdb_strerror(rc));
        mdb_txn_abort(txn);
        freez(datafiles);
        return -1;
    }

    // Clear any existing data
    rc = mdb_drop(txn, dbi, 0);
    if (rc != 0) {
        netdata_log_error("LMDB: Failed to clear metrics DB: %s", mdb_strerror(rc));
        mdb_txn_abort(txn);
        freez(datafiles);
        return -1;
    }

    // Set up save context
    save_ctx.txn = txn;
    save_ctx.dbi = dbi;
    save_ctx.count = 0;
    save_ctx.section = section;

    // TODO: Fill this in with proper MRG traversal when API is available
    // For now, we'd need to adapt to traverse the MRG and call mrg_save_metric

    // This would be the ideal implementation:
    // rc = mrg_traverse(main_mrg, mrg_traverse_save_callback, &save_ctx);

    // Store datafiles information
    rc = mrg_lmdb_store_datafiles(txn, ctx, datafiles, datafiles_count);
    if (rc == MDB_MAP_FULL && retry_count < 10) {
        // Try to grow the map size and retry
        rc = mrg_lmdb_grow_mapsize(instance->env, txn);
        if (rc == 1) {
            retry_count++;
            goto retry_transaction;
        }
    }
    else if (rc != 0) {
        mdb_txn_abort(txn);
        freez(datafiles);
        return -1;
    }

    // Store metadata
    rc = mrg_lmdb_store_metadata(txn, save_ctx.count);
    if (rc == MDB_MAP_FULL && retry_count < 10) {
        // Try to grow the map size and retry
        rc = mrg_lmdb_grow_mapsize(instance->env, txn);
        if (rc == 1) {
            retry_count++;
            goto retry_transaction;
        }
    }
    else if (rc != 0) {
        mdb_txn_abort(txn);
        freez(datafiles);
        return -1;
    }

    // Commit transaction
    rc = mdb_txn_commit(txn);
    if (rc == MDB_MAP_FULL && retry_count < 10) {
        // Try to grow the map size and retry
        txn = NULL; // Already aborted by commit failure
        rc = mrg_lmdb_grow_mapsize(instance->env, txn);
        if (rc == 1) {
            retry_count++;
            goto retry_transaction;
        }
    }

    if (rc != 0) {
        netdata_log_error("LMDB: Failed to commit transaction: %s", mdb_strerror(rc));
        // No need to abort txn here, commit already does it on failure
        freez(datafiles);
        return -1;
    }

    netdata_log_info("LMDB: Successfully saved %zu metrics for tier %d",
                     save_ctx.count, ctx->config.tier);

    freez(datafiles);
    return 0;
}

// Load MRG from LMDB for a specific section
int mrg_lmdb_load(Word_t section, const char *path) {
    int rc;
    MDB_txn *txn;
    MDB_dbi dbi;
    MDB_cursor *cursor;
    MDB_val key, value;
    size_t metrics_loaded = 0;
    LMDB_INSTANCE *instance;
    struct rrdengine_instance *ctx = (struct rrdengine_instance *)section;

    // Get LMDB instance
    instance = get_lmdb_instance(path);
    if (!instance || !instance->env) {
        // Initialize LMDB if it doesn't exist
        rc = mrg_lmdb_init(path);
        if (rc != 0 || !(instance = get_lmdb_instance(path)) || !instance->env) {
            netdata_log_error("LMDB: Failed to initialize LMDB for %s", path);
            return -1;
        }
    }

    netdata_log_info("LMDB: Loading MRG for tier %d...", ctx->config.tier);

    // Begin read-only transaction
    rc = mrg_lmdb_transaction_get(instance->env, &txn, MDB_RDONLY);
    if (rc != 0) {
        return -1;
    }

    // Validate datafiles
    rc = mrg_validate_datafiles(ctx, txn);
    if (rc != 0) {
        mdb_txn_abort(txn);
        if (rc == 1) {
            netdata_log_info("LMDB: New datafiles found, partial load required");
            return 1; // Return 1 to indicate new files found
        }
        return -1;
    }

    // Open metrics database
    rc = mdb_dbi_open(txn, MRG_DB_METRICS, 0, &dbi);
    if (rc != 0) {
        netdata_log_error("LMDB: Failed to open metrics DB: %s", mdb_strerror(rc));
        mdb_txn_abort(txn);
        return -1;
    }

    // Create cursor
    rc = mdb_cursor_open(txn, dbi, &cursor);
    if (rc != 0) {
        netdata_log_error("LMDB: Failed to open cursor: %s", mdb_strerror(rc));
        mdb_txn_abort(txn);
        return -1;
    }

    // Iterate through all metrics
    rc = mdb_cursor_get(cursor, &key, &value, MDB_FIRST);
    while (rc == 0) {
        METRIC_LMDB_KEY *lmdb_key = (METRIC_LMDB_KEY *)key.mv_data;
        METRIC_LMDB_VALUE *lmdb_value = (METRIC_LMDB_VALUE *)value.mv_data;

        // Skip metrics with no retention
        if (lmdb_value->first_time_s != 0 && lmdb_value->last_time_s != 0) {
            // Convert timestamps back to real time
            time_t first_time_s = lmdb_time_to_real_time(lmdb_value->first_time_s);
            time_t last_time_s = lmdb_time_to_real_time(lmdb_value->last_time_s);

            // Create MRG entry
            MRG_ENTRY entry = {
                .uuid = (nd_uuid_t *)&lmdb_key->uuid,
                .section = section,  // Use the passed section
                .first_time_s = first_time_s,
                .last_time_s = last_time_s,
                .latest_update_every_s = lmdb_value->update_every
            };

            // Add metric to MRG
            bool added;
            METRIC *metric = mrg_metric_add_and_acquire(main_mrg, entry, &added);
            if (metric) {
                metrics_loaded++;
                mrg_metric_release(main_mrg, metric);
            }
        }

        // Get next metric
        rc = mdb_cursor_get(cursor, &key, &value, MDB_NEXT);
    }

    mdb_cursor_close(cursor);
    mdb_txn_abort(txn);

    netdata_log_info("LMDB: Successfully loaded %zu metrics for tier %d",
                     metrics_loaded, ctx->config.tier);

    return 0;
}

// Close LMDB environment for a specific path
void mrg_lmdb_close(const char *path) {
    LMDB_INSTANCE *instance, *prev = NULL;

    spinlock_lock(&lmdb_instances_spinlock);

    for (instance = lmdb_instances; instance; instance = instance->next) {
        if (strcmp(instance->path, path) == 0) {
            // Remove from list
            if (prev)
                prev->next = instance->next;
            else
                lmdb_instances = instance->next;

            spinlock_lock(&instance->spinlock);

            if (instance->env) {
                mdb_env_sync(instance->env, 1);
                mdb_env_close(instance->env);
                instance->env = NULL;
            }

            spinlock_unlock(&instance->spinlock);

            freez(instance);
            break;
        }
        prev = instance;
    }

    spinlock_unlock(&lmdb_instances_spinlock);
}

// Close all LMDB environments (for cleanup)
void mrg_lmdb_close_all(void) {
    LMDB_INSTANCE *instance, *next;

    spinlock_lock(&lmdb_instances_spinlock);

    for (instance = lmdb_instances; instance; instance = next) {
        next = instance->next;

        spinlock_lock(&instance->spinlock);

        if (instance->env) {
            mdb_env_sync(instance->env, 1);
            mdb_env_close(instance->env);
            instance->env = NULL;
        }

        spinlock_unlock(&instance->spinlock);

        freez(instance);
    }

    lmdb_instances = NULL;

    spinlock_unlock(&lmdb_instances_spinlock);
}
