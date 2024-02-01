// SPDX-License-Identifier: GPL-3.0-or-later


/** @file db_api.c
 *  @brief This is the file implementing the API to the 
 *         logs management database.
 */

#include "daemon/common.h"
#include "db_api.h"
#include <inttypes.h>
#include <stdio.h>
#include "circular_buffer.h"
#include "helper.h"
#include "lz4.h"
#include "parser.h"

#define MAIN_DB                "main.db"        /**< Primary DB with metadata for all the logs managemt collections **/
#define MAIN_COLLECTIONS_TABLE "LogCollections" /*< Table name where logs collections metadata is stored in MAIN_DB **/
#define BLOB_STORE_FILENAME    "logs.bin."      /*< Filename of BLOBs where logs are stored in **/
#define METADATA_DB_FILENAME   "metadata.db"    /**< Metadata DB for each log collection **/
#define LOGS_TABLE             "Logs"           /*< Table name where logs metadata is stored in METADATA_DB_FILENAME **/
#define BLOBS_TABLE            "Blobs"          /*< Table name where BLOBs metadata is stored in METADATA_DB_FILENAME **/

#define LOGS_MANAG_DB_VERSION 1

static sqlite3 *main_db = NULL;   /**< SQLite DB handler for MAIN_DB **/
static char *main_db_dir = NULL;  /**< Directory where all the log management databases and log blobs are stored in **/
static char *main_db_path = NULL; /**< Path of MAIN_DB **/

/* -------------------------------------------------------------------------- */
/*                            Database migrations                             */
/* -------------------------------------------------------------------------- */

/**
 * @brief No-op database migration, just to bump up starting version.
 * @param database Unused
 * @param name Unused
 * @return Always 0.
 */
static int do_migration_noop(sqlite3 *database, const char *name){
    UNUSED(database);
    UNUSED(name);
    collector_info("Running database migration %s", name);
    return 0;
}

typedef struct database_func_migration_list{
    char *name;
    int (*func)(sqlite3 *database, const char *name);
} DATABASE_FUNC_MIGRATION_LIST;

DATABASE_FUNC_MIGRATION_LIST migration_list_main_db[] = {
    {.name = MAIN_DB" v0 to v1",  .func = do_migration_noop},
    // the terminator of this array
    {.name = NULL, .func = NULL}
};

DATABASE_FUNC_MIGRATION_LIST migration_list_metadata_db[] = {
    {.name = METADATA_DB_FILENAME " v0 to v1",  .func = do_migration_noop},
    // the terminator of this array
    {.name = NULL, .func = NULL}
};

typedef enum {
    ERR_TYPE_OTHER,
    ERR_TYPE_SQLITE,
    ERR_TYPE_LIBUV,
} logs_manag_db_error_t;

/**
 * @brief Logs a database error
 * @param[in] log_source Log source that caused the error
 * @param[in] error_type Type of error
 * @param[in] rc Error code
 * @param[in] line Line number where the error occurred (__LINE__)
 * @param[in] file Source file where the error occurred (__FILE__)
 * @param[in] func Function where the error occurred    (__FUNCTION__)
 */
static void throw_error(const char *const log_source, 
                        const logs_manag_db_error_t error_type,
                        const int rc, const int line, 
                        const char *const file, const char *const func){
    collector_error("[%s]: %s database error: (%d) %s (%s:%s:%d))",
                        log_source ? log_source : "-", 
                        error_type == ERR_TYPE_OTHER ? "" : ERR_TYPE_SQLITE ? "SQLite" : "libuv",
                        rc, error_type == ERR_TYPE_OTHER ? "" : ERR_TYPE_SQLITE ? sqlite3_errstr(rc) : uv_strerror(rc), 
                        file, func, line);
}

/**
 * @brief Get or set user_version of database.
 * @param db SQLite database to act upon.
 * @param set_user_version If <= 0, just get user_version. Otherwise, set
 * user_version first, before returning it.
 * @return Database user_version or -1 in case of error.
 */
int db_user_version(sqlite3 *const db, const int set_user_version){
    if(unlikely(!db)) return -1;
    int rc = 0;
    if(set_user_version <= 0){
        sqlite3_stmt *stmt_get_user_version;
        rc = sqlite3_prepare_v2(db, "PRAGMA user_version;", -1, &stmt_get_user_version, NULL);
        if (unlikely(SQLITE_OK != rc)) {
            throw_error(NULL, ERR_TYPE_SQLITE, rc, __LINE__, __FILE__, __FUNCTION__);
            return -1;
        }
        rc = sqlite3_step(stmt_get_user_version);
        if (unlikely(SQLITE_ROW != rc)) {
            throw_error(NULL, ERR_TYPE_SQLITE, rc, __LINE__, __FILE__, __FUNCTION__);
            return -1;
        }
        int current_user_version = sqlite3_column_int(stmt_get_user_version, 0);
        rc = sqlite3_finalize(stmt_get_user_version);
        if (unlikely(SQLITE_OK != rc)) {
            throw_error(NULL, ERR_TYPE_SQLITE, rc, __LINE__, __FILE__, __FUNCTION__);
            return -1;
        }
        return current_user_version;
    } else {
        char buf[25];
        snprintfz(buf, 25, "PRAGMA user_version=%d;", set_user_version);
        rc = sqlite3_exec(db, buf, NULL, NULL, NULL);
        if (unlikely(SQLITE_OK!= rc)) {
            throw_error(NULL, ERR_TYPE_SQLITE, rc, __LINE__, __FILE__, __FUNCTION__);
            return -1;
        }
        return set_user_version;
    }
}

static void db_writer_db_mode_none(void *arg){
    struct File_info *const p_file_info = (struct File_info *) arg;
    Circ_buff_item_t *item;
    
    while(__atomic_load_n(&p_file_info->state, __ATOMIC_RELAXED) == LOG_SRC_READY){
        uv_rwlock_rdlock(&p_file_info->circ_buff->buff_realloc_rwlock);
        do{ item = circ_buff_read_item(p_file_info->circ_buff);} while(item);
        circ_buff_read_done(p_file_info->circ_buff);
        uv_rwlock_rdunlock(&p_file_info->circ_buff->buff_realloc_rwlock);
        for(int i = 0; i < p_file_info->buff_flush_to_db_interval * 4; i++){
            if(__atomic_load_n(&p_file_info->state, __ATOMIC_RELAXED) != LOG_SRC_READY)
                break;
            sleep_usec(250 * USEC_PER_MS);
        }
    }
}

#define return_db_writer_db_mode_none(p_file_info, do_mut_unlock) do {          \
    p_file_info->db_mode = LOGS_MANAG_DB_MODE_NONE;                             \
    freez((void *) p_file_info->db_dir);                                        \
    p_file_info->db_dir = strdupz("");                                          \
    freez((void *) p_file_info->db_metadata);                                   \
    p_file_info->db_metadata = NULL;                                            \
    sqlite3_finalize(stmt_logs_insert);                                         \
    sqlite3_finalize(stmt_blobs_get_total_filesize);                            \
    sqlite3_finalize(stmt_blobs_update);                                        \
    sqlite3_finalize(stmt_blobs_set_zero_filesize);                             \
    sqlite3_finalize(stmt_logs_delete);                                         \
    if(do_mut_unlock){                                                          \
        uv_mutex_unlock(p_file_info->db_mut);                                   \
        uv_rwlock_rdunlock(&p_file_info->circ_buff->buff_realloc_rwlock);       \
    }                                                                           \
    if(__atomic_load_n(&p_file_info->state, __ATOMIC_RELAXED) == LOG_SRC_READY) \
        return fatal_assert(!uv_thread_create(  p_file_info->db_writer_thread,  \
                                                db_writer_db_mode_none,         \
                                                p_file_info));                  \
} while(0)

static void db_writer_db_mode_full(void *arg){
    int rc = 0;
    struct File_info *const p_file_info = (struct File_info *) arg;

    sqlite3_stmt *stmt_logs_insert = NULL;
    sqlite3_stmt *stmt_blobs_get_total_filesize = NULL;
    sqlite3_stmt *stmt_blobs_update = NULL;
    sqlite3_stmt *stmt_blobs_set_zero_filesize = NULL;
    sqlite3_stmt *stmt_logs_delete = NULL;
    
    /* Prepare LOGS_TABLE INSERT statement */
    rc = sqlite3_prepare_v2(p_file_info->db,
                        "INSERT INTO " LOGS_TABLE "("
                        "FK_BLOB_Id,"
                        "BLOB_Offset,"
                        "Timestamp,"
                        "Msg_compr_size,"
                        "Msg_decompr_size,"
                        "Num_lines"
                        ") VALUES (?,?,?,?,?,?) ;",
                        -1, &stmt_logs_insert, NULL);
    if (unlikely(SQLITE_OK != rc)) {
        throw_error(p_file_info->chartname, ERR_TYPE_SQLITE, rc, __LINE__, __FILE__, __FUNCTION__);
        return_db_writer_db_mode_none(p_file_info, 0);
    }
    
    /* Prepare BLOBS_TABLE get total filesize statement */
    rc = sqlite3_prepare_v2(p_file_info->db,
                            "SELECT SUM(Filesize) FROM " BLOBS_TABLE " ;",
                            -1, &stmt_blobs_get_total_filesize, NULL);
    if (unlikely(SQLITE_OK != rc)) {
        throw_error(p_file_info->chartname, ERR_TYPE_SQLITE, rc, __LINE__, __FILE__, __FUNCTION__);
        return_db_writer_db_mode_none(p_file_info, 0);
    }
     
    /* Prepare BLOBS_TABLE UPDATE statement */
    rc = sqlite3_prepare_v2(p_file_info->db,
                            "UPDATE " BLOBS_TABLE
                            " SET Filesize = Filesize + ?"
                            " WHERE Id = ? ;",
                            -1, &stmt_blobs_update, NULL);
    if (unlikely(SQLITE_OK != rc)) {
        throw_error(p_file_info->chartname, ERR_TYPE_SQLITE, rc, __LINE__, __FILE__, __FUNCTION__);
        return_db_writer_db_mode_none(p_file_info, 0);
    }
    
    /* Prepare BLOBS_TABLE UPDATE SET zero filesize statement */
    rc = sqlite3_prepare_v2(p_file_info->db,
                            "UPDATE " BLOBS_TABLE
                            " SET Filesize = 0"
                            " WHERE Id = ? ;",
                            -1, &stmt_blobs_set_zero_filesize, NULL);
    if (unlikely(SQLITE_OK != rc)) {
        throw_error(p_file_info->chartname, ERR_TYPE_SQLITE, rc, __LINE__, __FILE__, __FUNCTION__);
        return_db_writer_db_mode_none(p_file_info, 0);
    }
    
    /* Prepare LOGS_TABLE DELETE statement */
    rc = sqlite3_prepare_v2(p_file_info->db,
                            "DELETE FROM " LOGS_TABLE
                            " WHERE FK_BLOB_Id = ? ;",
                            -1, &stmt_logs_delete, NULL);
    if (unlikely(SQLITE_OK != rc)) {
        throw_error(p_file_info->chartname, ERR_TYPE_SQLITE, rc, __LINE__, __FILE__, __FUNCTION__);
        return_db_writer_db_mode_none(p_file_info, 0);
    }
        
    /* Get initial filesize of logs.bin.0 BLOB */
    sqlite3_stmt *stmt_retrieve_filesize_from_id = NULL;
    if(unlikely(
        SQLITE_OK != (rc = sqlite3_prepare_v2(p_file_info->db,  
                                                "SELECT Filesize FROM " BLOBS_TABLE 
                                                " WHERE Id = ? ;",
                                                -1, &stmt_retrieve_filesize_from_id, NULL)) ||
        SQLITE_OK != (rc = sqlite3_bind_int(stmt_retrieve_filesize_from_id, 1, 
                                                p_file_info->blob_write_handle_offset)) ||
        SQLITE_ROW != (rc = sqlite3_step(stmt_retrieve_filesize_from_id))
        )){
            throw_error(p_file_info->chartname, ERR_TYPE_SQLITE, rc, __LINE__, __FILE__, __FUNCTION__);
            return_db_writer_db_mode_none(p_file_info, 0);
    }
    int64_t blob_filesize = (int64_t) sqlite3_column_int64(stmt_retrieve_filesize_from_id, 0);
    rc = sqlite3_finalize(stmt_retrieve_filesize_from_id);
    if (unlikely(SQLITE_OK != rc)) {
        throw_error(p_file_info->chartname, ERR_TYPE_SQLITE, rc, __LINE__, __FILE__, __FUNCTION__);
        return_db_writer_db_mode_none(p_file_info, 0);
    }

    struct timespec ts_db_write_start, ts_db_write_end, ts_db_rotate_end;
    while(__atomic_load_n(&p_file_info->state, __ATOMIC_RELAXED) == LOG_SRC_READY){
        clock_gettime(CLOCK_THREAD_CPUTIME_ID, &ts_db_write_start);

        uv_rwlock_rdlock(&p_file_info->circ_buff->buff_realloc_rwlock);
        uv_mutex_lock(p_file_info->db_mut);
        
        /* ---------------------------------------------------------------------
         * Read items from circular buffer and store them in disk BLOBs. 
         * After that, SQLite metadata is updated.
         * ------------------------------------------------------------------ */
        Circ_buff_item_t *item = circ_buff_read_item(p_file_info->circ_buff);
        while (item) {
            m_assert(TEST_MS_TIMESTAMP_VALID(item->timestamp), "item->timestamp == 0"); 
            m_assert(item->text_compressed_size != 0, "item->text_compressed_size == 0");
            m_assert(item->text_size != 0, "item->text_size == 0");

            /* Write logs in BLOB */
            uv_fs_t write_req;
            uv_buf_t uv_buf = uv_buf_init((char *) item->text_compressed, (unsigned int) item->text_compressed_size);
            rc = uv_fs_write(   NULL, &write_req, 
                                p_file_info->blob_handles[p_file_info->blob_write_handle_offset], 
                                &uv_buf, 1, blob_filesize, NULL); // Write synchronously at the end of the BLOB file
            uv_fs_req_cleanup(&write_req);
            if(unlikely(rc < 0)){
                throw_error(p_file_info->chartname, ERR_TYPE_LIBUV, rc, __LINE__, __FILE__, __FUNCTION__);
                circ_buff_read_done(p_file_info->circ_buff);
                return_db_writer_db_mode_none(p_file_info, 1);
            }
            
            /* Ensure data is flushed to BLOB via fdatasync() */
            uv_fs_t dsync_req;
            rc = uv_fs_fdatasync(   NULL, &dsync_req, 
                                    p_file_info->blob_handles[p_file_info->blob_write_handle_offset], NULL);
            uv_fs_req_cleanup(&dsync_req);
            if (unlikely(rc)){
                throw_error(p_file_info->chartname, ERR_TYPE_LIBUV, rc, __LINE__, __FILE__, __FUNCTION__);
                circ_buff_read_done(p_file_info->circ_buff);
                return_db_writer_db_mode_none(p_file_info, 1);
            }
            
            if(unlikely(
                /* Write metadata of logs in LOGS_TABLE */
                SQLITE_OK != (rc = sqlite3_exec(p_file_info->db, "BEGIN TRANSACTION;", NULL, NULL, NULL)) ||
                SQLITE_OK != (rc = sqlite3_bind_int(stmt_logs_insert, 1, p_file_info->blob_write_handle_offset)) ||
                SQLITE_OK != (rc = sqlite3_bind_int64(stmt_logs_insert, 2, (sqlite3_int64) blob_filesize)) ||
                SQLITE_OK != (rc = sqlite3_bind_int64(stmt_logs_insert, 3, (sqlite3_int64) item->timestamp)) ||
                SQLITE_OK != (rc = sqlite3_bind_int64(stmt_logs_insert, 4, (sqlite3_int64) item->text_compressed_size)) ||
                SQLITE_OK != (rc = sqlite3_bind_int64(stmt_logs_insert, 5, (sqlite3_int64)item->text_size)) ||
                SQLITE_OK != (rc = sqlite3_bind_int64(stmt_logs_insert, 6, (sqlite3_int64)item->num_lines)) ||
                SQLITE_DONE != (rc = sqlite3_step(stmt_logs_insert)) || 
                SQLITE_OK != (rc = sqlite3_reset(stmt_logs_insert)) ||

                /* Update metadata of BLOBs filesize in BLOBS_TABLE */
                SQLITE_OK != (rc = sqlite3_bind_int64(stmt_blobs_update, 1, (sqlite3_int64)item->text_compressed_size)) ||
                SQLITE_OK != (rc = sqlite3_bind_int(stmt_blobs_update, 2, p_file_info->blob_write_handle_offset)) ||
                SQLITE_DONE != (rc = sqlite3_step(stmt_blobs_update)) ||
                SQLITE_OK != (rc = sqlite3_reset(stmt_blobs_update)) ||
                SQLITE_OK != (rc = sqlite3_exec(p_file_info->db, "END TRANSACTION;", NULL, NULL, NULL))
                )) {
                    throw_error(p_file_info->chartname, ERR_TYPE_SQLITE, rc, __LINE__, __FILE__, __FUNCTION__);
                    rc = sqlite3_exec(p_file_info->db, "ROLLBACK;", NULL, NULL, NULL);
                    if (unlikely(SQLITE_OK != rc)) 
                        throw_error(p_file_info->chartname, ERR_TYPE_SQLITE, rc, __LINE__, __FILE__, __FUNCTION__);
                    circ_buff_read_done(p_file_info->circ_buff);
                    return_db_writer_db_mode_none(p_file_info, 1);
            }

            /* TODO: Should we log it if there is a fatal error in the transaction, 
             * as there will be a mismatch between BLOBs and SQLite metadata? */

            /* Increase BLOB offset and read next log message until no more messages in buff */
            blob_filesize += (int64_t) item->text_compressed_size;
            item = circ_buff_read_item(p_file_info->circ_buff);
        }
        circ_buff_read_done(p_file_info->circ_buff);

        clock_gettime(CLOCK_THREAD_CPUTIME_ID, &ts_db_write_end);

        /* ---------------------------------------------------------------------
         * If the filesize of the current write-to BLOB is > 
         * p_file_info->blob_max_size, then perform a BLOBs rotation. 
         * ------------------------------------------------------------------ */
        if(blob_filesize > p_file_info->blob_max_size){
            uv_fs_t rename_req;
            char old_path[FILENAME_MAX + 1], new_path[FILENAME_MAX + 1];

            /* Rotate path of BLOBs */
            for(int i = BLOB_MAX_FILES - 1; i >= 0; i--){
                snprintfz(old_path, FILENAME_MAX, "%s" BLOB_STORE_FILENAME "%d", p_file_info->db_dir, i);
                snprintfz(new_path, FILENAME_MAX, "%s" BLOB_STORE_FILENAME "%d", p_file_info->db_dir, i + 1);
                rc = uv_fs_rename(NULL, &rename_req, old_path, new_path, NULL);
                uv_fs_req_cleanup(&rename_req);
                if (unlikely(rc)){
                    //TODO: This error case needs better handling, as it will result in mismatch with sqlite metadata.
                    //      We probably require a WAL or something similar.
                    throw_error(p_file_info->chartname, ERR_TYPE_LIBUV, rc, __LINE__, __FILE__, __FUNCTION__);
                    return_db_writer_db_mode_none(p_file_info, 1);
                }
            }
            
            /* Replace the maximum number with 0 in BLOB files. */
            snprintfz(old_path, FILENAME_MAX, "%s" BLOB_STORE_FILENAME "%d", p_file_info->db_dir, BLOB_MAX_FILES);
            snprintfz(new_path, FILENAME_MAX, "%s" BLOB_STORE_FILENAME "%d", p_file_info->db_dir, 0);
            rc = uv_fs_rename(NULL, &rename_req, old_path, new_path, NULL);
            uv_fs_req_cleanup(&rename_req);
            if (unlikely(rc)){
                //TODO: This error case needs better handling, as it will result in mismatch with sqlite metadata.
                //      We probably require a WAL or something similar.
                throw_error(p_file_info->chartname, ERR_TYPE_LIBUV, rc, __LINE__, __FILE__, __FUNCTION__);
                return_db_writer_db_mode_none(p_file_info, 1);
            }

            /* Rotate BLOBS_TABLE Filenames */
            rc = sqlite3_exec(p_file_info->db,
                        "UPDATE " BLOBS_TABLE
                        " SET Filename = REPLACE( "
                        "   Filename, " 
                        "   substr(Filename, -1), "
                        "   case when " 
                        "     (cast(substr(Filename, -1) AS INTEGER) < (" LOGS_MANAG_STR(BLOB_MAX_FILES) " - 1)) then " 
                        "     substr(Filename, -1) + 1 else 0 end);",
                        NULL, NULL, NULL);
            if (unlikely(rc != SQLITE_OK)) {
                throw_error(p_file_info->chartname, ERR_TYPE_SQLITE, rc, __LINE__, __FILE__, __FUNCTION__);
                //TODO: Undo rotation if possible?
                return_db_writer_db_mode_none(p_file_info, 1);
            }
            
            /* -----------------------------------------------------------------
             * (a) Update blob_write_handle_offset, 
             * (b) truncate new write-to BLOB, 
             * (c) update filesize of truncated BLOB in SQLite DB, 
             * (d) delete respective logs in LOGS_TABLE for the truncated BLOB and 
             * (e) reset blob_filesize
             * -------------------------------------------------------------- */
            /* (a) */ 
            p_file_info->blob_write_handle_offset = 
                p_file_info->blob_write_handle_offset == 1 ? BLOB_MAX_FILES : p_file_info->blob_write_handle_offset - 1;

            /* (b) */ 
            uv_fs_t trunc_req;
            rc = uv_fs_ftruncate(NULL, &trunc_req, p_file_info->blob_handles[p_file_info->blob_write_handle_offset], 0, NULL);
            uv_fs_req_cleanup(&trunc_req);
            if (unlikely(rc)){
                //TODO: This error case needs better handling, as it will result in mismatch with sqlite metadata.
                //      We probably require a WAL or something similar.
                throw_error(p_file_info->chartname, ERR_TYPE_LIBUV, rc, __LINE__, __FILE__, __FUNCTION__);
                return_db_writer_db_mode_none(p_file_info, 1);
            }
            
            /* (c) */ 
            if(unlikely(
                SQLITE_OK != (rc = sqlite3_exec(p_file_info->db, "BEGIN TRANSACTION;", NULL, NULL, NULL)) ||
                SQLITE_OK != (rc = sqlite3_bind_int(stmt_blobs_set_zero_filesize, 1, p_file_info->blob_write_handle_offset)) ||
                SQLITE_DONE != (rc = sqlite3_step(stmt_blobs_set_zero_filesize)) ||
                SQLITE_OK != (rc = sqlite3_reset(stmt_blobs_set_zero_filesize)) ||
                
                /* (d) */
                SQLITE_OK != (rc = sqlite3_bind_int(stmt_logs_delete, 1, p_file_info->blob_write_handle_offset)) ||
                SQLITE_DONE != (rc = sqlite3_step(stmt_logs_delete)) ||
                SQLITE_OK != (rc = sqlite3_reset(stmt_logs_delete)) ||
                SQLITE_OK != (rc = sqlite3_exec(p_file_info->db, "END TRANSACTION;", NULL, NULL, NULL))
                )) {
                    throw_error(p_file_info->chartname, ERR_TYPE_SQLITE, rc, __LINE__, __FILE__, __FUNCTION__);
                    rc = sqlite3_exec(p_file_info->db, "ROLLBACK;", NULL, NULL, NULL);
                    if (unlikely(SQLITE_OK != rc)) 
                        throw_error(p_file_info->chartname, ERR_TYPE_SQLITE, rc, __LINE__, __FILE__, __FUNCTION__);
                    return_db_writer_db_mode_none(p_file_info, 1);
            }

            /* (e) */
            blob_filesize = 0;

        }

        clock_gettime(CLOCK_THREAD_CPUTIME_ID, &ts_db_rotate_end);

        /* Update database write & rotate timings for this log source */
        __atomic_store_n(&p_file_info->db_write_duration,
                        (ts_db_write_end.tv_sec - ts_db_write_start.tv_sec) * NSEC_PER_SEC + 
                        (ts_db_write_end.tv_nsec - ts_db_write_start.tv_nsec), __ATOMIC_RELAXED);
        __atomic_store_n(&p_file_info->db_rotate_duration,
                        (ts_db_rotate_end.tv_sec - ts_db_write_end.tv_sec) * NSEC_PER_SEC + 
                        (ts_db_rotate_end.tv_nsec - ts_db_write_end.tv_nsec), __ATOMIC_RELAXED);

        /* Update total disk usage of all BLOBs for this log source */
        rc = sqlite3_step(stmt_blobs_get_total_filesize);
        if (unlikely(SQLITE_ROW != rc)) {
            throw_error(p_file_info->chartname, ERR_TYPE_SQLITE, rc, __LINE__, __FILE__, __FUNCTION__);
            return_db_writer_db_mode_none(p_file_info, 1);
        }
        __atomic_store_n(&p_file_info->blob_total_size, sqlite3_column_int64(stmt_blobs_get_total_filesize, 0), __ATOMIC_RELAXED);
        rc = sqlite3_reset(stmt_blobs_get_total_filesize);
        if (unlikely(SQLITE_OK != rc)) {
            throw_error(p_file_info->chartname, ERR_TYPE_SQLITE, rc, __LINE__, __FILE__, __FUNCTION__);
            return_db_writer_db_mode_none(p_file_info, 1);
        }

        // TODO: Can uv_mutex_unlock(p_file_info->db_mut) be moved before if(blob_filesize > p_file_info-> blob_max_size) ?
        uv_mutex_unlock(p_file_info->db_mut);
        uv_rwlock_rdunlock(&p_file_info->circ_buff->buff_realloc_rwlock);
        for(int i = 0; i < p_file_info->buff_flush_to_db_interval * 4; i++){
            if(__atomic_load_n(&p_file_info->state, __ATOMIC_RELAXED) != LOG_SRC_READY)
                break;
            sleep_usec(250 * USEC_PER_MS);
        }
    }

    return_db_writer_db_mode_none(p_file_info, 0);
}

inline void db_set_main_dir(char *const dir){
    main_db_dir = dir;
}

int db_init() {
    int rc = 0;
    char *err_msg = 0;
    uv_fs_t mkdir_req;
    
    if(unlikely(!main_db_dir || !*main_db_dir)){
        rc = -1;
        collector_error("main_db_dir is unset");
        throw_error(NULL, ERR_TYPE_OTHER, rc, __LINE__, __FILE__, __FUNCTION__);
        goto return_error;
    }
    size_t main_db_path_len = strlen(main_db_dir) + sizeof(MAIN_DB) + 1;
    main_db_path = mallocz(main_db_path_len);
    snprintfz(main_db_path, main_db_path_len, "%s/" MAIN_DB, main_db_dir);

    /* Create databases directory if it doesn't exist. */
    rc = uv_fs_mkdir(NULL, &mkdir_req, main_db_dir, 0775, NULL);
    uv_fs_req_cleanup(&mkdir_req);
    if(rc == 0) collector_info("DB directory created: %s", main_db_dir);
    else if (rc == UV_EEXIST) collector_info("DB directory %s found", main_db_dir);
    else {
        throw_error(NULL, ERR_TYPE_LIBUV, rc, __LINE__, __FILE__, __FUNCTION__);
        goto return_error;
    }

    /* Create or open main db */
    rc = sqlite3_open(main_db_path, &main_db);
    if (unlikely(rc != SQLITE_OK)){
        throw_error(MAIN_DB, ERR_TYPE_SQLITE, rc, __LINE__, __FILE__, __FUNCTION__);
        goto return_error;
    }
    
    /* Configure main database */
    rc = sqlite3_exec(main_db,
                      "PRAGMA auto_vacuum = INCREMENTAL;"
                      "PRAGMA synchronous = 1;"
                      "PRAGMA journal_mode = WAL;"
                      "PRAGMA temp_store = MEMORY;"
                      "PRAGMA foreign_keys = ON;",
                      0, 0, &err_msg);
    if (unlikely(rc != SQLITE_OK)) {
        collector_error("Failed to configure database, SQL error: %s\n", err_msg);
        throw_error(MAIN_DB, ERR_TYPE_SQLITE, rc, __LINE__, __FILE__, __FUNCTION__);
        goto return_error;
    } else collector_info("%s configured successfully", MAIN_DB);

    /* Execute pending main database migrations */
    int main_db_ver = db_user_version(main_db, -1);
    if (likely(LOGS_MANAG_DB_VERSION == main_db_ver))
        collector_info("Logs management %s database version is %d (no migration needed)", MAIN_DB, main_db_ver);
    else {
        for(int ver = main_db_ver; ver < LOGS_MANAG_DB_VERSION && migration_list_main_db[ver].func; ver++){
            rc = (migration_list_main_db[ver].func)(main_db, migration_list_main_db[ver].name);
            if (unlikely(rc)){
                collector_error("Logs management %s database migration from version %d to version %d failed", MAIN_DB, ver, ver + 1);
                throw_error(MAIN_DB, ERR_TYPE_SQLITE, rc, __LINE__, __FILE__, __FUNCTION__);
                goto return_error;
            }
            db_user_version(main_db, ver + 1);
        }
    }
    
    /* Create new main DB LogCollections table if it doesn't exist */
    rc = sqlite3_exec(main_db,
                      "CREATE TABLE IF NOT EXISTS " MAIN_COLLECTIONS_TABLE "("
                      "Id               INTEGER     PRIMARY KEY,"
                      "Stream_Tag       TEXT        NOT NULL,"
                      "Log_Source_Path  TEXT        NOT NULL,"
                      "Type             INTEGER     NOT NULL,"
                      "DB_Dir           TEXT        NOT NULL,"
                      "UNIQUE(Stream_Tag, DB_Dir) "
                      ");",
                      0, 0, &err_msg);
    if (unlikely(SQLITE_OK != rc)) {
        collector_error("Failed to create table" MAIN_COLLECTIONS_TABLE "SQL error: %s", err_msg);
        throw_error(MAIN_DB, ERR_TYPE_SQLITE, rc, __LINE__, __FILE__, __FUNCTION__);
        goto return_error;
    }
    
    sqlite3_stmt *stmt_search_if_log_source_exists = NULL;
    rc = sqlite3_prepare_v2(main_db,
                            "SELECT COUNT(*), Id, DB_Dir FROM " MAIN_COLLECTIONS_TABLE
                            " WHERE Stream_Tag = ? AND Log_Source_Path = ? AND Type = ? ;",
                            -1, &stmt_search_if_log_source_exists, NULL);
    if (unlikely(SQLITE_OK != rc)){
        throw_error(MAIN_DB, ERR_TYPE_SQLITE, rc, __LINE__, __FILE__, __FUNCTION__);
        goto return_error;
    }

    
    sqlite3_stmt *stmt_insert_log_collection_metadata = NULL;
    rc = sqlite3_prepare_v2(main_db,
                            "INSERT INTO " MAIN_COLLECTIONS_TABLE
                            " (Stream_Tag, Log_Source_Path, Type, DB_Dir) VALUES (?,?,?,?) ;",
                            -1, &stmt_insert_log_collection_metadata, NULL);
    if (unlikely(SQLITE_OK != rc)){
        throw_error(MAIN_DB, ERR_TYPE_SQLITE, rc, __LINE__, __FILE__, __FUNCTION__);
        goto return_error;
    }
    
    for (int i = 0; i < p_file_infos_arr->count; i++) {

        struct File_info *const p_file_info = p_file_infos_arr->data[i];

        if(p_file_info->db_mode == LOGS_MANAG_DB_MODE_NONE){
            p_file_info->db_dir = strdupz("");
            p_file_info->db_writer_thread = mallocz(sizeof(uv_thread_t));
            rc = uv_thread_create(p_file_info->db_writer_thread, db_writer_db_mode_none, p_file_info);
            if (unlikely(rc)){
                throw_error(p_file_info->chartname, ERR_TYPE_LIBUV, rc, __LINE__, __FILE__, __FUNCTION__);
                goto return_error;
            }
        }
        else if(p_file_info->db_mode == LOGS_MANAG_DB_MODE_FULL){

            p_file_info->db_mut = mallocz(sizeof(uv_mutex_t));
            rc = uv_mutex_init(p_file_info->db_mut);
            if (unlikely(rc)) fatal("Failed to initialize uv_mutex_t");
            uv_mutex_lock(p_file_info->db_mut);

            // This error check will be used a lot, so define it here. 
            #define do_sqlite_error_check(p_file_info, rc, rc_expctd) do {                                      \
                if(unlikely(rc_expctd != rc)) {                                                                 \
                    throw_error(p_file_info->chartname, ERR_TYPE_SQLITE, rc, __LINE__, __FILE__, __FUNCTION__);\
                    uv_mutex_unlock(p_file_info->db_mut);                                                       \
                    goto return_error;                                                                          \
                }                                                                                               \
            } while(0)

            if(unlikely(
                    SQLITE_OK != (rc = sqlite3_bind_text(stmt_search_if_log_source_exists, 1, p_file_info->stream_guid, -1, NULL)) ||
                    SQLITE_OK != (rc = sqlite3_bind_text(stmt_search_if_log_source_exists, 2, p_file_info->filename, -1, NULL)) ||
                    SQLITE_OK != (rc = sqlite3_bind_int(stmt_search_if_log_source_exists, 3, p_file_info->log_type)) ||
                    /* COUNT(*) query should always return SQLITE_ROW */
                    SQLITE_ROW != (rc = sqlite3_step(stmt_search_if_log_source_exists)))){
                throw_error(p_file_info->chartname, ERR_TYPE_SQLITE, rc, __LINE__, __FILE__, __FUNCTION__);
                uv_mutex_unlock(p_file_info->db_mut);
                goto return_error;
            }

            const int log_source_occurences = sqlite3_column_int(stmt_search_if_log_source_exists, 0);
            switch (log_source_occurences) {
                case 0: { /* Log collection metadata not found in main DB - create a new record */
                                        
                    /* Create directory of collection of logs for the particular 
                     * log source (in the form of a UUID) and bind it. */
                    uuid_t uuid;
                    uuid_generate(uuid);
                    char uuid_str[UUID_STR_LEN];      // ex. "1b4e28ba-2fa1-11d2-883f-0016d3cca427" + "\0"
                    uuid_unparse_lower(uuid, uuid_str);
                    
                    p_file_info->db_dir = mallocz(snprintf(NULL, 0, "%s/%s/", main_db_dir, uuid_str) + 1);
                    sprintf((char *) p_file_info->db_dir, "%s/%s/", main_db_dir, uuid_str);
                    
                    rc = uv_fs_mkdir(NULL, &mkdir_req, p_file_info->db_dir, 0775, NULL);
                    uv_fs_req_cleanup(&mkdir_req);
                    if (unlikely(rc)) {
                        if(errno == EEXIST) 
                            collector_error("DB directory %s exists but not found in %s.\n", p_file_info->db_dir, MAIN_DB);
                        throw_error(p_file_info->chartname, ERR_TYPE_LIBUV, rc, __LINE__, __FILE__, __FUNCTION__);
                        uv_mutex_unlock(p_file_info->db_mut);
                        goto return_error;
                    }

                    if(unlikely(
                            SQLITE_OK != (rc = sqlite3_bind_text(stmt_insert_log_collection_metadata, 1, p_file_info->stream_guid, -1, NULL)) ||
                            SQLITE_OK != (rc = sqlite3_bind_text(stmt_insert_log_collection_metadata, 2, p_file_info->filename, -1, NULL)) ||                    
                            SQLITE_OK != (rc = sqlite3_bind_int(stmt_insert_log_collection_metadata, 3, p_file_info->log_type)) ||
                            SQLITE_OK != (rc = sqlite3_bind_text(stmt_insert_log_collection_metadata, 4, p_file_info->db_dir, -1, NULL)) ||
                            SQLITE_DONE != (rc = sqlite3_step(stmt_insert_log_collection_metadata)) ||
                            SQLITE_OK != (rc = sqlite3_reset(stmt_insert_log_collection_metadata)))) {
                        throw_error(p_file_info->chartname, ERR_TYPE_SQLITE, rc, __LINE__, __FILE__, __FUNCTION__);
                        uv_mutex_unlock(p_file_info->db_mut);
                        goto return_error;
                    }
                    
                    break;
                }
                    
                case 1: { /* File metadata found in DB */
                    p_file_info->db_dir = mallocz((size_t)sqlite3_column_bytes(stmt_search_if_log_source_exists, 2) + 1);
                    sprintf((char*) p_file_info->db_dir, "%s", sqlite3_column_text(stmt_search_if_log_source_exists, 2));
                    break;
                }
                    
                default: { /* Error, file metadata can exist either 0 or 1 times in DB */
                    m_assert(0, "Same file stored in DB more than once!");
                    collector_error("[%s]: Record encountered multiple times in DB " MAIN_COLLECTIONS_TABLE " table \n",
                                    p_file_info->filename);
                    throw_error(p_file_info->chartname, ERR_TYPE_OTHER, rc, __LINE__, __FILE__, __FUNCTION__);
                    uv_mutex_unlock(p_file_info->db_mut);
                    goto return_error;
                }
            }
            rc = sqlite3_reset(stmt_search_if_log_source_exists);
            do_sqlite_error_check(p_file_info, rc, SQLITE_OK);
            
            /* Create or open metadata DBs for each log collection */
            p_file_info->db_metadata = mallocz(snprintf(NULL, 0, "%s" METADATA_DB_FILENAME, p_file_info->db_dir) + 1);
            sprintf((char *) p_file_info->db_metadata, "%s" METADATA_DB_FILENAME, p_file_info->db_dir);
            rc = sqlite3_open(p_file_info->db_metadata, &p_file_info->db);
            do_sqlite_error_check(p_file_info, rc, SQLITE_OK);
            
            /* Configure metadata DB */
            rc = sqlite3_exec(p_file_info->db,
                            "PRAGMA auto_vacuum = INCREMENTAL;"
                            "PRAGMA synchronous = 1;"
                            "PRAGMA journal_mode = WAL;"
                            "PRAGMA temp_store = MEMORY;"
                            "PRAGMA foreign_keys = ON;",
                            0, 0, &err_msg);
            if (unlikely(rc != SQLITE_OK)) {
                collector_error("[%s]: Failed to configure database, SQL error: %s", p_file_info->filename, err_msg);
                throw_error(p_file_info->chartname, ERR_TYPE_SQLITE, rc, __LINE__, __FILE__, __FUNCTION__);
                uv_mutex_unlock(p_file_info->db_mut);
                goto return_error;
            }

            /* Execute pending metadata database migrations */
            collector_info("[%s]: About to execute " METADATA_DB_FILENAME " migrations", p_file_info->chartname);
            int metadata_db_ver = db_user_version(p_file_info->db, -1);
            if (likely(LOGS_MANAG_DB_VERSION == metadata_db_ver)) {
                collector_info( "[%s]: Logs management " METADATA_DB_FILENAME " database version is %d (no migration needed)", 
                                p_file_info->chartname, metadata_db_ver);
            } else {
                for(int ver = metadata_db_ver; ver < LOGS_MANAG_DB_VERSION && migration_list_metadata_db[ver].func; ver++){
                    rc = (migration_list_metadata_db[ver].func)(p_file_info->db, migration_list_metadata_db[ver].name);
                    if (unlikely(rc)){
                        collector_error("[%s]: Logs management " METADATA_DB_FILENAME " database migration from version %d to version %d failed", 
                                        p_file_info->chartname, ver, ver + 1);
                        throw_error(MAIN_DB, ERR_TYPE_SQLITE, rc, __LINE__, __FILE__, __FUNCTION__);
                        uv_mutex_unlock(p_file_info->db_mut);
                        goto return_error;
                    }
                    db_user_version(p_file_info->db, ver + 1);
                }
            }

            /* -----------------------------------------------------------------
             * Create BLOBS_TABLE and LOGS_TABLE if they don't exist. Do it 
             * as a transaction, so that it can all be rolled back if something
             * goes wrong.
             * -------------------------------------------------------------- */
            {
                rc = sqlite3_exec(p_file_info->db, "BEGIN TRANSACTION;", NULL, NULL, NULL);
                do_sqlite_error_check(p_file_info, rc, SQLITE_OK);

                /* Check if BLOBS_TABLE exists or not */
                sqlite3_stmt *stmt_check_if_BLOBS_TABLE_exists = NULL;
                rc = sqlite3_prepare_v2(p_file_info->db,
                                        "SELECT COUNT(*) FROM sqlite_master" 
                                        " WHERE type='table' AND name='"BLOBS_TABLE"';",
                                        -1, &stmt_check_if_BLOBS_TABLE_exists, NULL);
                do_sqlite_error_check(p_file_info, rc, SQLITE_OK);
                rc = sqlite3_step(stmt_check_if_BLOBS_TABLE_exists);
                do_sqlite_error_check(p_file_info, rc, SQLITE_ROW);
                
                /* If BLOBS_TABLE doesn't exist, create and populate it */
                if(sqlite3_column_int(stmt_check_if_BLOBS_TABLE_exists, 0) == 0){
                    
                    /* 1. Create it */
                    rc = sqlite3_exec(p_file_info->db,
                            "CREATE TABLE IF NOT EXISTS " BLOBS_TABLE "("
                            "Id         INTEGER     PRIMARY KEY,"
                            "Filename   TEXT        NOT NULL,"
                            "Filesize   INTEGER     NOT NULL"
                            ");",
                            0, 0, &err_msg);
                    if (unlikely(SQLITE_OK != rc)) {
                        collector_error("[%s]: Failed to create " BLOBS_TABLE ", SQL error: %s", p_file_info->chartname, err_msg);
                        throw_error(p_file_info->chartname, ERR_TYPE_SQLITE, rc, __LINE__, __FILE__, __FUNCTION__);
                        uv_mutex_unlock(p_file_info->db_mut);
                        goto return_error;
                    } else collector_info("[%s]: Table " BLOBS_TABLE " created successfully", p_file_info->chartname);
                    
                    /* 2. Populate it */
                    sqlite3_stmt *stmt_init_BLOBS_table = NULL;
                    rc = sqlite3_prepare_v2(p_file_info->db,
                                    "INSERT INTO " BLOBS_TABLE 
                                    " (Filename, Filesize) VALUES (?,?) ;",
                                    -1, &stmt_init_BLOBS_table, NULL);
                    do_sqlite_error_check(p_file_info, rc, SQLITE_OK);

                    for(int t = 0; t < BLOB_MAX_FILES; t++){
                        char filename[FILENAME_MAX + 1];
                        snprintfz(filename, FILENAME_MAX, BLOB_STORE_FILENAME "%d", t);
                        if(unlikely( 
                                SQLITE_OK != (rc = sqlite3_bind_text(stmt_init_BLOBS_table, 1, filename, -1, NULL)) ||
                                SQLITE_OK != (rc = sqlite3_bind_int64(stmt_init_BLOBS_table, 2, (sqlite3_int64) 0)) ||
                                SQLITE_DONE != (rc = sqlite3_step(stmt_init_BLOBS_table)) ||
                                SQLITE_OK != (rc = sqlite3_reset(stmt_init_BLOBS_table)))){
                            throw_error(p_file_info->chartname, ERR_TYPE_SQLITE, rc, __LINE__, __FILE__, __FUNCTION__);
                            uv_mutex_unlock(p_file_info->db_mut);
                            goto return_error;
                        }
                    }
                    rc = sqlite3_finalize(stmt_init_BLOBS_table);
                    do_sqlite_error_check(p_file_info, rc, SQLITE_OK);
                }
                rc = sqlite3_finalize(stmt_check_if_BLOBS_TABLE_exists);
                do_sqlite_error_check(p_file_info, rc, SQLITE_OK);
                
                /* If LOGS_TABLE doesn't exist, create it */
                rc = sqlite3_exec(p_file_info->db,
                            "CREATE TABLE IF NOT EXISTS " LOGS_TABLE "("
                            "Id                 INTEGER     PRIMARY KEY,"
                            "FK_BLOB_Id         INTEGER     NOT NULL,"
                            "BLOB_Offset        INTEGER     NOT NULL,"
                            "Timestamp          INTEGER     NOT NULL,"
                            "Msg_compr_size     INTEGER     NOT NULL,"
                            "Msg_decompr_size   INTEGER     NOT NULL,"
                            "Num_lines          INTEGER     NOT NULL,"
                            "FOREIGN KEY (FK_BLOB_Id) REFERENCES " BLOBS_TABLE " (Id) ON DELETE CASCADE ON UPDATE CASCADE"
                            ");",
                            0, 0, &err_msg);
                if (unlikely(SQLITE_OK != rc)) {
                    collector_error("[%s]: Failed to create " LOGS_TABLE ", SQL error: %s", p_file_info->chartname, err_msg);
                    throw_error(p_file_info->chartname, ERR_TYPE_SQLITE, rc, __LINE__, __FILE__, __FUNCTION__);
                    uv_mutex_unlock(p_file_info->db_mut);
                    goto return_error;
                } else collector_info("[%s]: Table " LOGS_TABLE " created successfully", p_file_info->chartname);
                
                /* Create index on LOGS_TABLE Timestamp
                 * TODO: If this doesn't speed up queries, check SQLITE R*tree 
                *        module. Requires benchmarking with/without index. */
                rc = sqlite3_exec(p_file_info->db,
                                    "CREATE INDEX IF NOT EXISTS logs_timestamps_idx "
                                    "ON " LOGS_TABLE "(Timestamp);",
                                    0, 0, &err_msg);
                if (unlikely(SQLITE_OK != rc)) {
                    collector_error("[%s]: Failed to create logs_timestamps_idx, SQL error: %s", p_file_info->chartname, err_msg);
                    throw_error(p_file_info->chartname, ERR_TYPE_SQLITE, rc, __LINE__, __FILE__, __FUNCTION__);
                    uv_mutex_unlock(p_file_info->db_mut);
                    goto return_error;
                } else collector_info("[%s]: logs_timestamps_idx created successfully", p_file_info->chartname);

                rc = sqlite3_exec(p_file_info->db, "END TRANSACTION;", NULL, NULL, NULL);
                do_sqlite_error_check(p_file_info, rc, SQLITE_OK);
            }


            /* -----------------------------------------------------------------
             * Remove excess BLOBs beyond BLOB_MAX_FILES (from both DB and disk 
             * storage). 
             * 
             * This is useful if BLOB_MAX_FILES is reduced after an agent 
             * restart (for example, if in the future it is not hardcoded, 
             * but instead it is read from the configuration file). LOGS_TABLE 
             * entries should be deleted automatically (due to ON DELETE CASCADE). 
             * -------------------------------------------------------------- */
            {
                sqlite3_stmt *stmt_get_BLOBS_TABLE_size = NULL;
                rc = sqlite3_prepare_v2(p_file_info->db,
                                        "SELECT MAX(Id) FROM " BLOBS_TABLE ";",
                                        -1, &stmt_get_BLOBS_TABLE_size, NULL);
                do_sqlite_error_check(p_file_info, rc, SQLITE_OK);
                rc = sqlite3_step(stmt_get_BLOBS_TABLE_size);
                do_sqlite_error_check(p_file_info, rc, SQLITE_ROW);

                const int blobs_table_max_id = sqlite3_column_int(stmt_get_BLOBS_TABLE_size, 0);

                sqlite3_stmt *stmt_retrieve_filename_last_digits = NULL; // This statement retrieves the last digit(s) from the Filename column of BLOBS_TABLE
                rc = sqlite3_prepare_v2(p_file_info->db,
                    "WITH split(word, str) AS ( SELECT '', (SELECT Filename FROM " BLOBS_TABLE " WHERE Id = ? ) || '.' "
                    "UNION ALL SELECT substr(str, 0, instr(str, '.')), substr(str, instr(str, '.')+1) FROM split WHERE str!='' ) "
                    "SELECT word FROM split WHERE word!='' ORDER BY LENGTH(str) LIMIT 1;",
                    -1, &stmt_retrieve_filename_last_digits, NULL);
                do_sqlite_error_check(p_file_info, rc, SQLITE_OK);

                sqlite3_stmt *stmt_delete_row_by_id = NULL; 
                rc = sqlite3_prepare_v2(p_file_info->db,
                    "DELETE FROM " BLOBS_TABLE " WHERE Id = ?;",
                    -1, &stmt_delete_row_by_id, NULL);
                do_sqlite_error_check(p_file_info, rc, SQLITE_OK);

                for (int id = 1; id <= blobs_table_max_id; id++){

                    rc = sqlite3_bind_int(stmt_retrieve_filename_last_digits, 1, id);
                    do_sqlite_error_check(p_file_info, rc, SQLITE_OK);
                    rc = sqlite3_step(stmt_retrieve_filename_last_digits);
                    do_sqlite_error_check(p_file_info, rc, SQLITE_ROW);
                    int last_digits = sqlite3_column_int(stmt_retrieve_filename_last_digits, 0);
                    rc = sqlite3_reset(stmt_retrieve_filename_last_digits);
                    do_sqlite_error_check(p_file_info, rc, SQLITE_OK);

                    /* If last_digits > BLOB_MAX_FILES - 1, then some BLOB files 
                     * will need to be removed (both from DB BLOBS_TABLE and 
                     * also from the disk). */
                    if(last_digits > BLOB_MAX_FILES - 1){

                        /* Delete BLOB file from filesystem */
                        char blob_delete_path[FILENAME_MAX + 1];
                        snprintfz(blob_delete_path, FILENAME_MAX, "%s" BLOB_STORE_FILENAME "%d", p_file_info->db_dir, last_digits);
                        uv_fs_t unlink_req;
                        rc = uv_fs_unlink(NULL, &unlink_req, blob_delete_path, NULL);
                        uv_fs_req_cleanup(&unlink_req);
                        if (unlikely(rc)) {
                            // TODO: If there is an erro here, the entry won't be deleted from BLOBS_TABLE. What to do?
                            throw_error(p_file_info->chartname, ERR_TYPE_LIBUV, rc, __LINE__, __FILE__, __FUNCTION__);
                            uv_mutex_unlock(p_file_info->db_mut);
                            goto return_error;
                        }
                        do_sqlite_error_check(p_file_info, rc, SQLITE_OK);

                        /* Delete entry from DB BLOBS_TABLE */
                        rc = sqlite3_bind_int(stmt_delete_row_by_id, 1, id);
                        do_sqlite_error_check(p_file_info, rc, SQLITE_OK);
                        rc = sqlite3_step(stmt_delete_row_by_id);
                        do_sqlite_error_check(p_file_info, rc, SQLITE_DONE);
                        rc = sqlite3_reset(stmt_delete_row_by_id);
                        do_sqlite_error_check(p_file_info, rc, SQLITE_OK);
                    }
                }
                rc = sqlite3_finalize(stmt_retrieve_filename_last_digits);
                do_sqlite_error_check(p_file_info, rc, SQLITE_OK);
                rc = sqlite3_finalize(stmt_delete_row_by_id);
                do_sqlite_error_check(p_file_info, rc, SQLITE_OK);

                /* -------------------------------------------------------------
                 * BLOBS_TABLE ids after the deletion might not be contiguous. 
                 * This needs to be fixed, by having the ids updated. 
                 * LOGS_TABLE FKs will be updated automatically 
                 * (due to ON UPDATE CASCADE). 
                 * ---------------------------------------------------------- */

                int old_blobs_table_ids[BLOB_MAX_FILES];
                int off = 0;
                sqlite3_stmt *stmt_retrieve_all_ids = NULL; 
                rc = sqlite3_prepare_v2(p_file_info->db,
                    "SELECT Id FROM " BLOBS_TABLE " ORDER BY Id ASC;",
                    -1, &stmt_retrieve_all_ids, NULL);
                do_sqlite_error_check(p_file_info, rc, SQLITE_OK);

                rc = sqlite3_step(stmt_retrieve_all_ids);
                while(rc == SQLITE_ROW){
                    old_blobs_table_ids[off++] = sqlite3_column_int(stmt_retrieve_all_ids, 0);
                    rc = sqlite3_step(stmt_retrieve_all_ids);
                }
                do_sqlite_error_check(p_file_info, rc, SQLITE_DONE);
                rc = sqlite3_finalize(stmt_retrieve_all_ids);
                do_sqlite_error_check(p_file_info, rc, SQLITE_OK);

                sqlite3_stmt *stmt_update_id = NULL; 
                rc = sqlite3_prepare_v2(p_file_info->db,
                    "UPDATE " BLOBS_TABLE " SET Id = ? WHERE Id = ?;",
                    -1, &stmt_update_id, NULL);
                do_sqlite_error_check(p_file_info, rc, SQLITE_OK);

                for (int t = 0; t < BLOB_MAX_FILES; t++){
                    if(unlikely(
                            SQLITE_OK != (rc = sqlite3_bind_int(stmt_update_id, 1, t + 1)) ||
                            SQLITE_OK != (rc = sqlite3_bind_int(stmt_update_id, 2, old_blobs_table_ids[t])) ||
                            SQLITE_DONE != (rc = sqlite3_step(stmt_update_id)) ||
                            SQLITE_OK != (rc = sqlite3_reset(stmt_update_id)))) {
                        throw_error(p_file_info->chartname, ERR_TYPE_SQLITE, rc, __LINE__, __FILE__, __FUNCTION__);
                        uv_mutex_unlock(p_file_info->db_mut);
                        goto return_error;
                    }
                }
                rc = sqlite3_finalize(stmt_update_id);
                do_sqlite_error_check(p_file_info, rc, SQLITE_OK);
            }

            /* -----------------------------------------------------------------
             * Traverse BLOBS_TABLE, open logs.bin.X files and store their 
             * file handles in p_file_info array. 
             * -------------------------------------------------------------- */
            sqlite3_stmt *stmt_retrieve_metadata_from_id = NULL;
            rc = sqlite3_prepare_v2(p_file_info->db,
                                    "SELECT Filename, Filesize FROM " BLOBS_TABLE 
                                    " WHERE Id = ? ;",
                                    -1, &stmt_retrieve_metadata_from_id, NULL);
            do_sqlite_error_check(p_file_info, rc, SQLITE_OK);
            
            sqlite3_stmt *stmt_retrieve_total_logs_size = NULL;
            rc = sqlite3_prepare_v2(p_file_info->db,
                                    "SELECT SUM(Msg_compr_size) FROM " LOGS_TABLE 
                                    " WHERE FK_BLOB_Id = ? GROUP BY FK_BLOB_Id ;",
                                    -1, &stmt_retrieve_total_logs_size, NULL);
            do_sqlite_error_check(p_file_info, rc, SQLITE_OK);
            
            uv_fs_t open_req;
            for(int id = 1; id <= BLOB_MAX_FILES; id++){

                /* Open BLOB file based on filename stored in BLOBS_TABLE. */
                rc = sqlite3_bind_int(stmt_retrieve_metadata_from_id, 1, id);
                do_sqlite_error_check(p_file_info, rc, SQLITE_OK);
                rc = sqlite3_step(stmt_retrieve_metadata_from_id);
                do_sqlite_error_check(p_file_info, rc, SQLITE_ROW);

                char filename[FILENAME_MAX + 1] = {0};
                snprintfz(filename, FILENAME_MAX, "%s%s", p_file_info->db_dir, 
                            sqlite3_column_text(stmt_retrieve_metadata_from_id, 0));
                rc = uv_fs_open(NULL, &open_req, filename,  
                                UV_FS_O_RDWR | UV_FS_O_CREAT | UV_FS_O_APPEND | UV_FS_O_RANDOM,
                                0644, NULL);
                if (unlikely(rc < 0)){
                    uv_fs_req_cleanup(&open_req);
                    throw_error(p_file_info->chartname, ERR_TYPE_LIBUV, rc, __LINE__, __FILE__, __FUNCTION__);
                    uv_mutex_unlock(p_file_info->db_mut);
                    goto return_error;
                }

                // open_req.result of a uv_fs_t is the file descriptor in case of the uv_fs_open
                p_file_info->blob_handles[id] = open_req.result;
                uv_fs_req_cleanup(&open_req);

                const int64_t metadata_filesize = (int64_t) sqlite3_column_int64(stmt_retrieve_metadata_from_id, 1);
                
                /* -------------------------------------------------------------
                 * Retrieve total log messages compressed size from LOGS_TABLE 
                 * for current FK_BLOB_Id.
                 * Only to assert whether correct - not used elsewhere. 
                 * 
                 * If no rows are returned, it means it is probably the initial 
                 * execution of the program so still valid (except if rc is other
                 * than SQLITE_DONE, which is an error then). 
                 * ---------------------------------------------------------- */
                rc = sqlite3_bind_int(stmt_retrieve_total_logs_size, 1, id);
                do_sqlite_error_check(p_file_info, rc, SQLITE_OK);
                rc = sqlite3_step(stmt_retrieve_total_logs_size);
                if (SQLITE_ROW == rc){ 
                    const int64_t total_logs_filesize = (int64_t) sqlite3_column_int64(stmt_retrieve_total_logs_size, 0);
                    if(unlikely(total_logs_filesize != metadata_filesize)){
                        throw_error(p_file_info->chartname, ERR_TYPE_OTHER, rc, __LINE__, __FILE__, __FUNCTION__);
                        uv_mutex_unlock(p_file_info->db_mut);
                        goto return_error;
                    }
                } else do_sqlite_error_check(p_file_info, rc, SQLITE_DONE);
                
                
                /* Get filesize of BLOB file. */ 
                uv_fs_t stat_req;
                rc = uv_fs_stat(NULL, &stat_req, filename, NULL);
                if (unlikely(rc)){
                    uv_fs_req_cleanup(&stat_req);
                    throw_error(p_file_info->chartname, ERR_TYPE_LIBUV, rc, __LINE__, __FILE__, __FUNCTION__);
                    uv_mutex_unlock(p_file_info->db_mut);
                    goto return_error;
                }
                const int64_t blob_filesize = (int64_t) stat_req.statbuf.st_size;
                uv_fs_req_cleanup(&stat_req);
                
                do{
                    /* Case 1: blob_filesize == metadata_filesize (equal, either both zero or not): All good */
                    if(likely(blob_filesize == metadata_filesize))
                        break;
                    
                    /* Case 2: blob_filesize == 0 && metadata_filesize > 0: fatal(), however could it mean that 
                     * EXT_BLOB_STORE_FILENAME was rotated but the SQLite metadata wasn't updated? So can it 
                     * maybe be recovered by un-rotating? Either way, treat as fatal error for now. */
                    // TODO: Can we avoid fatal()? 
                    if(unlikely(blob_filesize == 0 && metadata_filesize > 0)){
                        collector_error("[%s]: blob_filesize == 0 but metadata_filesize > 0 for '%s'\n", 
                                        p_file_info->chartname, filename);
                        throw_error(p_file_info->chartname, ERR_TYPE_OTHER, rc, __LINE__, __FILE__, __FUNCTION__);
                        uv_mutex_unlock(p_file_info->db_mut);
                        goto return_error;
                    }
                    
                    /* Case 3: blob_filesize > metadata_filesize: Truncate binary to sqlite filesize, program 
                     * crashed or terminated after writing BLOBs to external file but before metadata was updated */
                    if(unlikely(blob_filesize > metadata_filesize)){
                        collector_info("[%s]: blob_filesize > metadata_filesize for '%s'. Will attempt to fix it.", 
                                        p_file_info->chartname, filename);
                        uv_fs_t trunc_req;
                        rc = uv_fs_ftruncate(NULL, &trunc_req, p_file_info->blob_handles[id], metadata_filesize, NULL);
                        uv_fs_req_cleanup(&trunc_req);
                        if(unlikely(rc)) {
                            throw_error(p_file_info->chartname, ERR_TYPE_LIBUV, rc, __LINE__, __FILE__, __FUNCTION__);
                            uv_mutex_unlock(p_file_info->db_mut);
                            goto return_error;
                        }
                        break;
                    }

                    /* Case 4: blob_filesize < metadata_filesize: unrecoverable, 
                     * maybe rotation went horrible wrong?
                     * TODO: Delete external BLOB and clear metadata from DB, 
                     *       start from clean state but the most recent logs. */
                    if(unlikely(blob_filesize < metadata_filesize)){
                        collector_info("[%s]: blob_filesize < metadata_filesize for '%s'.", 
                                        p_file_info->chartname, filename);
                        throw_error(p_file_info->chartname, ERR_TYPE_OTHER, rc, __LINE__, __FILE__, __FUNCTION__);
                        uv_mutex_unlock(p_file_info->db_mut);
                        goto return_error;
                    }

                    /* Case 5: default if none of the above, should never reach here, fatal() */
                    m_assert(0, "Code should not reach here");
                    throw_error(p_file_info->chartname, ERR_TYPE_OTHER, rc, __LINE__, __FILE__, __FUNCTION__);
                    uv_mutex_unlock(p_file_info->db_mut);
                    goto return_error;
                } while(0);
                
                
                /* Initialise blob_write_handle with logs.bin.0 */
                if(filename[strlen(filename) - 1] == '0') 
                    p_file_info->blob_write_handle_offset = id;
                    
                rc = sqlite3_reset(stmt_retrieve_total_logs_size);
                do_sqlite_error_check(p_file_info, rc, SQLITE_OK);
                rc = sqlite3_reset(stmt_retrieve_metadata_from_id);
                do_sqlite_error_check(p_file_info, rc, SQLITE_OK);
            }

            rc = sqlite3_finalize(stmt_retrieve_metadata_from_id);
            do_sqlite_error_check(p_file_info, rc, SQLITE_OK);

            /* Prepare statements to be used in single database queries */
            rc = sqlite3_prepare_v2(p_file_info->db,
                                    "SELECT Timestamp, Msg_compr_size , Msg_decompr_size, "
                                    "BLOB_Offset, " BLOBS_TABLE".Id, Num_lines "
                                    "FROM " LOGS_TABLE " INNER JOIN " BLOBS_TABLE " "
                                    "ON " LOGS_TABLE ".FK_BLOB_Id = " BLOBS_TABLE ".Id "
                                    "WHERE Timestamp >= ? AND Timestamp <= ? "
                                    "ORDER BY Timestamp;",
                                    -1, &p_file_info->stmt_get_log_msg_metadata_asc, NULL);
            do_sqlite_error_check(p_file_info, rc, SQLITE_OK);

            rc = sqlite3_prepare_v2(p_file_info->db,
                                    "SELECT Timestamp, Msg_compr_size , Msg_decompr_size, "
                                    "BLOB_Offset, " BLOBS_TABLE".Id, Num_lines "
                                    "FROM " LOGS_TABLE " INNER JOIN " BLOBS_TABLE " "
                                    "ON " LOGS_TABLE ".FK_BLOB_Id = " BLOBS_TABLE ".Id "
                                    "WHERE Timestamp <= ? AND Timestamp >= ? "
                                    "ORDER BY Timestamp DESC;",
                                    -1, &p_file_info->stmt_get_log_msg_metadata_desc, NULL);
            do_sqlite_error_check(p_file_info, rc, SQLITE_OK);

            /* DB initialisation finished; release lock */
            uv_mutex_unlock(p_file_info->db_mut);
            
            /* Create synchronous writer thread, one for each log source */
            p_file_info->db_writer_thread = mallocz(sizeof(uv_thread_t));
            rc = uv_thread_create(p_file_info->db_writer_thread, db_writer_db_mode_full, p_file_info);
            if (unlikely(rc)){
                throw_error(p_file_info->chartname, ERR_TYPE_LIBUV, rc, __LINE__, __FILE__, __FUNCTION__);
                goto return_error;
            }
        }
    }
    rc = sqlite3_finalize(stmt_search_if_log_source_exists);
    if (unlikely(rc != SQLITE_OK)){
        throw_error(MAIN_DB, ERR_TYPE_SQLITE, rc, __LINE__, __FILE__, __FUNCTION__);
        // TODO: Some additional cleanup required here, e.g. terminate db_writer_thread. 
        goto return_error;
    }
    rc = sqlite3_finalize(stmt_insert_log_collection_metadata);
    if (unlikely(rc != SQLITE_OK)){
        throw_error(MAIN_DB, ERR_TYPE_SQLITE, rc, __LINE__, __FILE__, __FUNCTION__);
        // TODO: Some additional cleanup required here, e.g. terminate db_writer_thread. 
        goto return_error;
    }

    return 0;

return_error:
    freez(main_db_path);
    main_db_path = NULL;

    sqlite3_close(main_db); // No-op if main_db == NULL
    sqlite3_free(err_msg); // No-op if err_msg == NULL

    m_assert(rc != 0, "rc should not be == 0 in case of error");
    return rc == 0 ? -1 : rc;
}

/**
 * @brief Search database(s) for logs
 * @details This function searches one or more databases for any results 
 * matching the query parameters. If any results are found, it will decompress 
 * the text of each returned row and add it to the results buffer, up to a 
 * maximum amount of p_query_params->quota bytes (unless timed out). 
 * @todo Make decompress buffer static to reduce mallocs/frees.
 * @todo Limit number of results returned through SQLite Query to speed up search?
 */
void db_search(logs_query_params_t *const p_query_params, struct File_info *const p_file_infos[]) {
    int rc = 0;

    sqlite3_stmt *stmt_get_log_msg_metadata;
    sqlite3 *dbt = NULL; // Used only when multiple DBs are searched
        
    if(!p_file_infos[1]){ /* Single DB to be searched */
        stmt_get_log_msg_metadata = p_query_params->order_by_asc ? 
            p_file_infos[0]->stmt_get_log_msg_metadata_asc : p_file_infos[0]->stmt_get_log_msg_metadata_desc;
        if(unlikely(
            SQLITE_OK != (rc = sqlite3_bind_int64(stmt_get_log_msg_metadata, 1, p_query_params->req_from_ts)) ||
            SQLITE_OK != (rc = sqlite3_bind_int64(stmt_get_log_msg_metadata, 2, p_query_params->req_to_ts)) ||
            (SQLITE_ROW != (rc = sqlite3_step(stmt_get_log_msg_metadata)) && (SQLITE_DONE != rc))
        )){
            throw_error(p_file_infos[0]->chartname, ERR_TYPE_SQLITE, rc, __LINE__, __FILE__, __FUNCTION__);
            // TODO: If there are errors here, should db_writer_db_mode_full() be terminated?
            sqlite3_reset(stmt_get_log_msg_metadata);
            return;
        }
    } else { /* Multiple DBs to be searched */
        sqlite3_stmt *stmt_attach_db;
        sqlite3_stmt *stmt_create_tmp_view;
        int pfi_off = 0;
        
        /* Open a new DB connection on the first log source DB and attach other DBs */
        if(unlikely(
            SQLITE_OK != (rc = sqlite3_open_v2(p_file_infos[0]->db_metadata, &dbt, SQLITE_OPEN_READONLY, NULL)) ||
            SQLITE_OK != (rc = sqlite3_prepare_v2(dbt,"ATTACH DATABASE ?  AS ? ;", -1, &stmt_attach_db, NULL))
        )){
            throw_error(p_file_infos[0]->chartname, ERR_TYPE_SQLITE, rc, __LINE__, __FILE__, __FUNCTION__);
            sqlite3_close_v2(dbt);
            return;
        }
        for(pfi_off = 0; p_file_infos[pfi_off]; pfi_off++){
            if(unlikely(
                SQLITE_OK != (rc = sqlite3_bind_text(stmt_attach_db, 1, p_file_infos[pfi_off]->db_metadata, -1, NULL)) ||
                SQLITE_OK != (rc = sqlite3_bind_int(stmt_attach_db, 2, pfi_off)) ||
                SQLITE_DONE != (rc = sqlite3_step(stmt_attach_db)) ||
                SQLITE_OK != (rc = sqlite3_reset(stmt_attach_db))
            )){
                throw_error(p_file_infos[pfi_off]->chartname, ERR_TYPE_SQLITE, rc, __LINE__, __FILE__, __FUNCTION__);
                sqlite3_close_v2(dbt);
                return;
            }
        }

        /* Create temporary view, then prepare retrieval of metadata from 
        * TMP_VIEW_TABLE statement and execute search.
        * TODO: Limit number of results returned through SQLite Query to speed up search? */
        #define TMP_VIEW_TABLE		   "compound_view"
        #define TMP_VIEW_QUERY_PREFIX  "CREATE TEMP VIEW " TMP_VIEW_TABLE " AS SELECT * FROM (SELECT * FROM '0'."\
                                        LOGS_TABLE " INNER JOIN (VALUES(0)) ORDER BY Timestamp) "
        #define TMP_VIEW_QUERY_BODY_1  "UNION ALL SELECT * FROM (SELECT * FROM '"
        #define TMP_VIEW_QUERY_BODY_2  "'." LOGS_TABLE " INNER JOIN (VALUES("
        #define TMP_VIEW_QUERY_BODY_3  ")) ORDER BY Timestamp) "
        #define TMP_VIEW_QUERY_POSTFIX "ORDER BY Timestamp;" 

        char tmp_view_query[sizeof(TMP_VIEW_QUERY_PREFIX) + (
                                sizeof(TMP_VIEW_QUERY_BODY_1) + 
                                sizeof(TMP_VIEW_QUERY_BODY_2) + 
                                sizeof(TMP_VIEW_QUERY_BODY_3) + 4 
                            ) * (LOGS_MANAG_MAX_COMPOUND_QUERY_SOURCES - 1) + 
                            sizeof(TMP_VIEW_QUERY_POSTFIX) + 
                            50 /* +50 bytes to play it safe */] = TMP_VIEW_QUERY_PREFIX; 
        int pos = sizeof(TMP_VIEW_QUERY_PREFIX) - 1;
        for(pfi_off = 1; p_file_infos[pfi_off]; pfi_off++){ // Skip p_file_infos[0]
            int n = snprintf(&tmp_view_query[pos], sizeof(tmp_view_query) - pos, "%s%d%s%d%s", 
                            TMP_VIEW_QUERY_BODY_1, pfi_off, 
                            TMP_VIEW_QUERY_BODY_2, pfi_off, 
                            TMP_VIEW_QUERY_BODY_3);
            
            if (n < 0 || n >= (int) sizeof(tmp_view_query) - pos){
                throw_error(p_file_infos[pfi_off]->chartname, ERR_TYPE_OTHER, n, __LINE__, __FILE__, __FUNCTION__);
                sqlite3_close_v2(dbt);
                return;
            }
            pos += n;
        }
        snprintf(&tmp_view_query[pos], sizeof(tmp_view_query) - pos, "%s", TMP_VIEW_QUERY_POSTFIX);

        if(unlikely(
            SQLITE_OK !=    (rc = sqlite3_prepare_v2(dbt, tmp_view_query, -1, &stmt_create_tmp_view, NULL)) ||
            SQLITE_DONE !=  (rc = sqlite3_step(stmt_create_tmp_view)) ||
            SQLITE_OK !=    (rc = sqlite3_prepare_v2(dbt, p_query_params->order_by_asc ?

                                    "SELECT Timestamp, Msg_compr_size , Msg_decompr_size, "
                                    "BLOB_Offset, FK_BLOB_Id, Num_lines, column1 "
                                    "FROM " TMP_VIEW_TABLE " "
                                    "WHERE Timestamp >= ? AND Timestamp <= ?;" : 

                                    /* TODO: The following can also be done by defining 
                                     * a descending order tmp_view_query, which will 
                                     * probably be faster. Needs to be measured. */

                                    "SELECT Timestamp, Msg_compr_size , Msg_decompr_size, "
                                    "BLOB_Offset, FK_BLOB_Id, Num_lines, column1 "
                                    "FROM " TMP_VIEW_TABLE " "
                                    "WHERE Timestamp <= ? AND Timestamp >= ? ORDER BY Timestamp DESC;",

                                    -1, &stmt_get_log_msg_metadata, NULL)) ||
            SQLITE_OK !=    (rc = sqlite3_bind_int64(stmt_get_log_msg_metadata, 1, 
                                                        (sqlite3_int64)p_query_params->req_from_ts)) ||
            SQLITE_OK !=    (rc = sqlite3_bind_int64(stmt_get_log_msg_metadata, 2, 
                                                        (sqlite3_int64)p_query_params->req_to_ts)) ||
            (SQLITE_ROW !=  (rc = sqlite3_step(stmt_get_log_msg_metadata)) && (SQLITE_DONE != rc))
        )){
            throw_error(p_file_infos[0]->chartname, ERR_TYPE_SQLITE, rc, __LINE__, __FILE__, __FUNCTION__);
            sqlite3_close_v2(dbt);
            return;
        }
    }

    Circ_buff_item_t tmp_itm = {0};
    
    BUFFER *const res_buff = p_query_params->results_buff;
    logs_query_res_hdr_t res_hdr = { // results header
        .timestamp = p_query_params->act_to_ts,
        .text_size = 0,
        .matches = 0,
        .log_source = "",
        .log_type = "",
        .basename = "",
        .filename = "",
        .chartname =""
    }; 
    size_t text_compressed_size_max = 0;
    
    while (rc == SQLITE_ROW) {

        /* Retrieve metadata from DB */
        tmp_itm.timestamp = (msec_t)sqlite3_column_int64(stmt_get_log_msg_metadata, 0);
        tmp_itm.text_compressed_size = (size_t)sqlite3_column_int64(stmt_get_log_msg_metadata, 1);
        tmp_itm.text_size = (size_t)sqlite3_column_int64(stmt_get_log_msg_metadata, 2);
        int64_t blob_offset = (int64_t) sqlite3_column_int64(stmt_get_log_msg_metadata, 3);
        int blob_handles_offset = sqlite3_column_int(stmt_get_log_msg_metadata, 4);
        unsigned long num_lines = (unsigned long) sqlite3_column_int64(stmt_get_log_msg_metadata, 5);
        int db_off = p_file_infos[1] ? sqlite3_column_int(stmt_get_log_msg_metadata, 6) : 0;

        /* If exceeding quota or timeout is reached and new timestamp 
         * is different than previous, terminate query. */
        if((res_buff->len >= p_query_params->quota || terminate_logs_manag_query(p_query_params)) && 
                tmp_itm.timestamp != res_hdr.timestamp){
            p_query_params->act_to_ts = res_hdr.timestamp;
            break;
        }

        res_hdr.timestamp = tmp_itm.timestamp;
        snprintfz(res_hdr.log_source, sizeof(res_hdr.log_source), "%s", log_src_t_str[p_file_infos[db_off]->log_source]);
        snprintfz(res_hdr.log_type, sizeof(res_hdr.log_type), "%s", log_src_type_t_str[p_file_infos[db_off]->log_type]);
        snprintfz(res_hdr.basename, sizeof(res_hdr.basename), "%s", p_file_infos[db_off]->file_basename);
        snprintfz(res_hdr.filename, sizeof(res_hdr.filename), "%s", p_file_infos[db_off]->filename);
        snprintfz(res_hdr.chartname, sizeof(res_hdr.chartname), "%s", p_file_infos[db_off]->chartname);

        /* Retrieve compressed log messages from BLOB file */
        if(tmp_itm.text_compressed_size > text_compressed_size_max){
            text_compressed_size_max = tmp_itm.text_compressed_size;
            tmp_itm.text_compressed = reallocz(tmp_itm.text_compressed, text_compressed_size_max);
        }
        uv_fs_t read_req;
        uv_buf_t uv_buf = uv_buf_init(tmp_itm.text_compressed, tmp_itm.text_compressed_size);
        rc = uv_fs_read(NULL, 
                        &read_req, 
                        p_file_infos[db_off]->blob_handles[blob_handles_offset], 
                        &uv_buf, 1, blob_offset, NULL);
        uv_fs_req_cleanup(&read_req);
        if (unlikely(rc < 0)){
            throw_error(NULL, ERR_TYPE_LIBUV, rc, __LINE__, __FILE__, __FUNCTION__);
            break;
        }
        
        /* Append retrieved results to BUFFER.
         * In the case of search_keyword(), less than sizeof(res_hdr) + tmp_itm.text_size 
         *space may be required, but go for worst case scenario for now */
        buffer_increase(res_buff, sizeof(res_hdr) + tmp_itm.text_size);
                                                        
        if(!p_query_params->keyword || !*p_query_params->keyword || !strcmp(p_query_params->keyword, " ")){
            rc = LZ4_decompress_safe(tmp_itm.text_compressed, 
                                    &res_buff->buffer[res_buff->len + sizeof(res_hdr)], 
                                    tmp_itm.text_compressed_size, 
                                    tmp_itm.text_size);
            
            if(unlikely(rc < 0)){
                throw_error(p_file_infos[db_off]->chartname, ERR_TYPE_OTHER, rc, __LINE__, __FILE__, __FUNCTION__);
                break; 
            }

            res_hdr.matches = num_lines;
            res_hdr.text_size = tmp_itm.text_size;
        } 
        else {
            tmp_itm.data = mallocz(tmp_itm.text_size);
            rc = LZ4_decompress_safe(tmp_itm.text_compressed, 
                                    tmp_itm.data, 
                                    tmp_itm.text_compressed_size, 
                                    tmp_itm.text_size);
            
            if(unlikely(rc < 0)){
                freez(tmp_itm.data);
                throw_error(p_file_infos[db_off]->chartname, ERR_TYPE_OTHER, rc, __LINE__, __FILE__, __FUNCTION__);
                break; 
            }

            res_hdr.matches = search_keyword(   tmp_itm.data, tmp_itm.text_size, 
                                                &res_buff->buffer[res_buff->len + sizeof(res_hdr)], 
                                                &res_hdr.text_size, p_query_params->keyword, NULL, 
                                                p_query_params->ignore_case);
            freez(tmp_itm.data);

            m_assert(   (res_hdr.matches > 0 && res_hdr.text_size > 0) || 
                        (res_hdr.matches == 0 && res_hdr.text_size == 0), 
                        "res_hdr.matches and res_hdr.text_size must both be > 0 or == 0.");

            if(unlikely(res_hdr.matches < 0)){ /* res_hdr.matches < 0 - error during keyword search */
                throw_error(p_file_infos[db_off]->chartname, ERR_TYPE_LIBUV, rc, __LINE__, __FILE__, __FUNCTION__);
                break; 
            } 
        }

        if(res_hdr.text_size){
            res_buff->buffer[res_buff->len + sizeof(res_hdr) + res_hdr.text_size - 1] = '\n'; // replace '\0' with '\n' 
            memcpy(&res_buff->buffer[res_buff->len], &res_hdr, sizeof(res_hdr));
            res_buff->len += sizeof(res_hdr) + res_hdr.text_size; 
            p_query_params->num_lines += res_hdr.matches;
        }

        m_assert(TEST_MS_TIMESTAMP_VALID(res_hdr.timestamp), "res_hdr.timestamp is invalid");

        rc = sqlite3_step(stmt_get_log_msg_metadata);
        if (unlikely(rc != SQLITE_ROW && rc != SQLITE_DONE)){
            throw_error(p_file_infos[db_off]->chartname, ERR_TYPE_SQLITE, rc, __LINE__, __FILE__, __FUNCTION__);
            // TODO: If there are errors here, should db_writer_db_mode_full() be terminated?
            break;
        }
    }  

    if(tmp_itm.text_compressed) 
        freez(tmp_itm.text_compressed);

    if(p_file_infos[1]) 
        rc = sqlite3_close_v2(dbt);
    else 
        rc = sqlite3_reset(stmt_get_log_msg_metadata);

    if (unlikely(SQLITE_OK != rc)) 
        throw_error(p_file_infos[0]->chartname, ERR_TYPE_SQLITE, rc, __LINE__, __FILE__, __FUNCTION__);
}
