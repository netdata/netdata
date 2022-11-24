/** @file db_api.c
 *  @brief This is the file containing the database API
 *
 *  @author Dimitris Pantazis
 */

#include "../daemon/common.h"
#include "db_api.h"
#include <inttypes.h>
#include <stdio.h>
#include "circular_buffer.h"
#include "compression.h"
#include "helper.h"
#include "lz4.h"
#include "parser.h"

#define MAIN_DB "main.db" /**< Primary DB with just 1 table - MAIN_COLLECTIONS_TABLE **/
#define MAIN_COLLECTIONS_TABLE "LogCollections"
#define BLOB_STORE_FILENAME "logs.bin"
#define METADATA_DB_FILENAME "metadata.db"
#define LOGS_TABLE "Logs"
#define BLOBS_TABLE "Blobs"

static uv_loop_t *db_loop;
static sqlite3 *main_db;
static char *main_db_dir;  /**< Directory where all the log management databases and log blobs are stored in **/
static char *main_db_path; /**< Path of MAIN_DB **/


/**
 * @brief Throws fatal SQLite3 error
 * @details In case of a fatal SQLite3 error, the SQLite3 error code will be 
 * translated to a readable error message and logged to stderr. 
 * @param[in] rc SQLite3 error code
 * @param[in] line_no Line number where the error occurred 
 */
static inline void fatal_sqlite3_err(int rc, int line_no){
    fatal("SQLite error: %s (line %d)", sqlite3_errstr(rc), line_no);
}

/**
 * @brief Throws fatal libuv error
 * @details In case of a fatal libuv error, the libuv error code will be 
 * translated to a readable error message and logged to stderr. 
 * @param[in] rc libuv error code
 * @param[in] line_no Line number where the error occurred 
 */
static inline void fatal_libuv_err(int rc, int line_no){
    fatal("libuv error: %s (line %d)", uv_strerror(rc), line_no);
}

/**
 * @brief Get version of SQLite
 * @return String that contains the SQLite version. Must be freed.
 */
char *db_get_sqlite_version() {
    int rc = 0;
    sqlite3_stmt *stmt_get_sqlite_version;
    rc = sqlite3_prepare_v2(main_db,
                            "SELECT sqlite_version();", -1, &stmt_get_sqlite_version, NULL);
    if (unlikely(rc != SQLITE_OK)) fatal_sqlite3_err(rc, __LINE__);
    rc = sqlite3_step(stmt_get_sqlite_version);
    if (unlikely(rc != SQLITE_ROW)) fatal_sqlite3_err(rc, __LINE__);
    char *text = mallocz(sqlite3_column_bytes(stmt_get_sqlite_version, 0) + 1);
    strcpy(text, (char *)sqlite3_column_text(stmt_get_sqlite_version, 0));
    rc = sqlite3_finalize(stmt_get_sqlite_version);
    if (unlikely(rc != SQLITE_OK)) fatal_sqlite3_err(rc, __LINE__);
    return text;
}

static void db_writer(void *arg){
    int rc = 0;
    struct File_info *p_file_info = *((struct File_info **) arg);
    
    uv_loop_t *writer_loop = mallocz(sizeof(uv_loop_t));
    rc = uv_loop_init(writer_loop);
    if (unlikely(rc)) fatal_libuv_err(rc, __LINE__);

    /* Prepare LOGS_TABLE INSERT statement */
    sqlite3_stmt *stmt_logs_insert;
    rc = sqlite3_prepare_v2(p_file_info->db,
                        "INSERT INTO " LOGS_TABLE "("
                        "FK_BLOB_Id,"
                        "BLOB_Offset,"
                        "Timestamp,"
                        "Msg_compr_size,"
                        "Msg_decompr_size"
                        ") VALUES (?,?,?,?,?) ;",
                        -1, &stmt_logs_insert, NULL);
    if (unlikely(rc != SQLITE_OK)) fatal_sqlite3_err(rc, __LINE__);
    
    /* Prepare BLOBS_TABLE get total filesize statement */
    sqlite3_stmt *stmt_blobs_get_total_filesize;
    rc = sqlite3_prepare_v2(p_file_info->db,
                            "SELECT SUM(Filesize) FROM " BLOBS_TABLE " ;",
                            -1, &stmt_blobs_get_total_filesize, NULL);
    if (unlikely(rc != SQLITE_OK)) fatal_sqlite3_err(rc, __LINE__);
     
    /* Prepare BLOBS_TABLE UPDATE statement */
    sqlite3_stmt *stmt_blobs_update;
    rc = sqlite3_prepare_v2(p_file_info->db,
                            "UPDATE " BLOBS_TABLE
                            " SET Filesize = Filesize + ?"
                            " WHERE Id = ? ;",
                            -1, &stmt_blobs_update, NULL);
    if (unlikely(rc != SQLITE_OK)) fatal_sqlite3_err(rc, __LINE__);
    
    /* Prepare BLOBS_TABLE Filename rotate statement */
    sqlite3_stmt *stmt_rotate_blobs;
    rc = sqlite3_prepare_v2(p_file_info->db,
                            "UPDATE " BLOBS_TABLE
                            " SET Filename = REPLACE(Filename, ?, ?);",
                            -1, &stmt_rotate_blobs, NULL);
    if (unlikely(rc != SQLITE_OK)) fatal_sqlite3_err(rc, __LINE__);
    
    /* Prepare BLOBS_TABLE UPDATE SET zero filesize statement */
    sqlite3_stmt *stmt_blobs_set_zero_filesize;
    rc = sqlite3_prepare_v2(p_file_info->db,
                            "UPDATE " BLOBS_TABLE
                            " SET Filesize = 0"
                            " WHERE Id = ? ;",
                            -1, &stmt_blobs_set_zero_filesize, NULL);
    if (unlikely(rc != SQLITE_OK)) fatal_sqlite3_err(rc, __LINE__);
    
    /* Prepare LOGS_TABLE DELETE statement */
    sqlite3_stmt *stmt_logs_delete;
    rc = sqlite3_prepare_v2(p_file_info->db,
                            "DELETE FROM " LOGS_TABLE
                            " WHERE FK_BLOB_Id = ? ;",
                            -1, &stmt_logs_delete, NULL);
    if (unlikely(rc != SQLITE_OK)) fatal_sqlite3_err(rc, __LINE__);
        
    /* Get initial filesize of logs.bin.0 BLOB */
    sqlite3_stmt *stmt_retrieve_filesize_from_id;
    rc = sqlite3_prepare_v2(p_file_info->db,
                            "SELECT Filesize FROM " BLOBS_TABLE 
                            " WHERE Id = ? ;",
                            -1, &stmt_retrieve_filesize_from_id, NULL);
    if (unlikely(rc != SQLITE_OK)) fatal_sqlite3_err(rc, __LINE__);
    rc = sqlite3_bind_int(stmt_retrieve_filesize_from_id, 1, p_file_info->blob_write_handle_offset);
    if (unlikely(rc != SQLITE_OK)) fatal_sqlite3_err(rc, __LINE__);
    rc = sqlite3_step(stmt_retrieve_filesize_from_id);
    if (unlikely(rc != SQLITE_ROW)) fatal_sqlite3_err(rc, __LINE__);
    int64_t blob_filesize = (int64_t) sqlite3_column_int64(stmt_retrieve_filesize_from_id, 0);
    rc = sqlite3_finalize(stmt_retrieve_filesize_from_id);
    if (unlikely(rc != SQLITE_OK)) fatal_sqlite3_err(rc, __LINE__);
        
    while(1){
        uv_mutex_lock(p_file_info->db_mut);
        rc = sqlite3_exec(p_file_info->db, "BEGIN TRANSACTION;", NULL, NULL, NULL);
        if (unlikely(rc != SQLITE_OK)) fatal_sqlite3_err(rc, __LINE__);
        Circ_buff_item_t *item = circ_buff_read_item(p_file_info->circ_buff);
        while (item) {  // Retrieve buff items and store in DB until there are no more items in the buffer
            
            /* Write log message in BLOB */
            uv_fs_t write_req;
            uv_buf_t uv_buf = uv_buf_init((char *) item->text_compressed, (unsigned int) item->text_compressed_size);
            rc = uv_fs_write(writer_loop, &write_req, 
                p_file_info->blob_handles[p_file_info->blob_write_handle_offset], 
                &uv_buf, 1, blob_filesize, NULL); // Write synchronously at the end of the BLOB file
            if(unlikely(rc < 0)) fatal("Failed to write logs management BLOB");
            uv_fs_req_cleanup(&write_req);
            
            /* Write metadata of log message in LOGS_TABLE */
            rc = sqlite3_bind_int(stmt_logs_insert, 1, p_file_info->blob_write_handle_offset);
            if (unlikely(rc != SQLITE_OK)) fatal_sqlite3_err(rc, __LINE__);
            rc = sqlite3_bind_int64(stmt_logs_insert, 2, (sqlite3_int64) blob_filesize);
            if (unlikely(rc != SQLITE_OK)) fatal_sqlite3_err(rc, __LINE__);
            rc = sqlite3_bind_int64(stmt_logs_insert, 3, (sqlite3_int64) item->timestamp);
            if (unlikely(rc != SQLITE_OK)) fatal_sqlite3_err(rc, __LINE__);
            m_assert(item->timestamp > 1649175852000 && item->timestamp < 2532788652000, "item->timestamp == 0"); // Timestamp within valid range up to 2050
            rc = sqlite3_bind_int64(stmt_logs_insert, 4, (sqlite3_int64) item->text_compressed_size);
            if (unlikely(rc != SQLITE_OK)) fatal_sqlite3_err(rc, __LINE__);
            m_assert(item->text_compressed_size != 0, "item->text_compressed_size == 0");
            rc = sqlite3_bind_int64(stmt_logs_insert, 5, (sqlite3_int64)item->text_size);
            if (unlikely(rc != SQLITE_OK)) fatal_sqlite3_err(rc, __LINE__);
            m_assert(item->text_size != 0, "item->text_size == 0");
            rc = sqlite3_step(stmt_logs_insert);
            if (unlikely(rc != SQLITE_DONE)) fatal_sqlite3_err(rc, __LINE__);
            rc = sqlite3_reset(stmt_logs_insert);
            if (unlikely(rc != SQLITE_OK)) fatal_sqlite3_err(rc, __LINE__);
            
            /* Update metadata of BLOBs filesize in BLOBS_TABLE */
            rc = sqlite3_bind_int64(stmt_blobs_update, 1, (sqlite3_int64)item->text_compressed_size);
            if (unlikely(rc != SQLITE_OK)) fatal_sqlite3_err(rc, __LINE__);
            rc = sqlite3_bind_int(stmt_blobs_update, 2, p_file_info->blob_write_handle_offset);
            if (unlikely(rc != SQLITE_OK)) fatal_sqlite3_err(rc, __LINE__);
            rc = sqlite3_step(stmt_blobs_update);
            if (unlikely(rc != SQLITE_DONE)) fatal_sqlite3_err(rc, __LINE__);
            rc = sqlite3_reset(stmt_blobs_update);
            if (unlikely(rc != SQLITE_OK)) fatal_sqlite3_err(rc, __LINE__);
            
            /* Increase BLOB offset and read next log message until no more messages in buff */
            blob_filesize += (int64_t) item->text_compressed_size;
            item = circ_buff_read_item(p_file_info->circ_buff);
        }
        uv_fs_t dsync_req;
        rc = uv_fs_fdatasync(writer_loop, &dsync_req, 
            p_file_info->blob_handles[p_file_info->blob_write_handle_offset], NULL);
        uv_fs_req_cleanup(&dsync_req);
        if (unlikely(rc)) fatal_libuv_err(rc, __LINE__);
        rc = sqlite3_exec(p_file_info->db, "END TRANSACTION;", NULL, NULL, NULL);
        if (unlikely(rc != SQLITE_OK)) fatal_sqlite3_err(rc, __LINE__);
        // TODO: Should we log it if there is a fatal error in the above line, as there will be a mismatch between BLOBs and SQLite metadata?
        //sqlite3_wal_checkpoint_v2(p_file_info->db,NULL,SQLITE_CHECKPOINT_PASSIVE,0,0);
        
        /* If the filesize of the current write-to BLOB is > p_file_info->blob_max_size, rotate BLOBs */
        if(blob_filesize > p_file_info->blob_max_size){
            uv_fs_t rename_req;
            char old_path[FILENAME_MAX + 1], new_path[FILENAME_MAX + 1];

            /* 1. Rotate BLOBS_TABLE Filenames and path of actual BLOBs. 
             * Performed in 2 steps: 
             * (a) First increase all of their endings numbers by 1 and 
             * (b) then replace the maximum number with 0. */
            rc = sqlite3_exec(p_file_info->db, "BEGIN TRANSACTION;", NULL, NULL, NULL);
            if (unlikely(rc != SQLITE_OK)) fatal_sqlite3_err(rc, __LINE__);
            for(int i = BLOB_MAX_FILES - 1; i >= 0; i--){
                
                /* Rotate BLOBS_TABLE Filenames */
                rc = sqlite3_bind_int(stmt_rotate_blobs, 1, i);
                if (rc != SQLITE_OK) fatal_sqlite3_err(rc, __LINE__);
                rc = sqlite3_bind_int(stmt_rotate_blobs, 2, i + 1);
                if (rc != SQLITE_OK) fatal_sqlite3_err(rc, __LINE__);
                rc = sqlite3_step(stmt_rotate_blobs);
                if (unlikely(rc != SQLITE_DONE)) fatal_sqlite3_err(rc, __LINE__);
                rc = sqlite3_reset(stmt_rotate_blobs);
                if (unlikely(rc != SQLITE_OK)) fatal_sqlite3_err(rc, __LINE__);
                
                /* Rotate path of BLOBs */
                sprintf(old_path, "%s" BLOB_STORE_FILENAME ".%d", p_file_info->db_dir, i);
                sprintf(new_path, "%s" BLOB_STORE_FILENAME ".%d", p_file_info->db_dir, i + 1);
                rc = uv_fs_rename(writer_loop, &rename_req, old_path, new_path, NULL);
                if (unlikely(rc)) fatal_libuv_err(rc, __LINE__);
                uv_fs_req_cleanup(&rename_req);
            }
            /* Replace the maximum number with 0 in SQLite DB. */
            rc = sqlite3_bind_int(stmt_rotate_blobs, 1, BLOB_MAX_FILES);
            if (rc != SQLITE_OK) fatal_sqlite3_err(rc, __LINE__);
            rc = sqlite3_bind_int(stmt_rotate_blobs, 2, 0);
            if (rc != SQLITE_OK) fatal_sqlite3_err(rc, __LINE__);
            rc = sqlite3_step(stmt_rotate_blobs);
            if (unlikely(rc != SQLITE_DONE)) fatal_sqlite3_err(rc, __LINE__);
            rc = sqlite3_reset(stmt_rotate_blobs);
            if (unlikely(rc != SQLITE_OK)) fatal_sqlite3_err(rc, __LINE__);
            rc = sqlite3_exec(p_file_info->db, "END TRANSACTION;", NULL, NULL, NULL);
            if (unlikely(rc != SQLITE_OK)) fatal_sqlite3_err(rc, __LINE__);
            
            /* Replace the maximum number with 0 in BLOB files. */
            sprintf(old_path, "%s" BLOB_STORE_FILENAME ".%d", p_file_info->db_dir, BLOB_MAX_FILES);
            sprintf(new_path, "%s" BLOB_STORE_FILENAME ".%d", p_file_info->db_dir, 0);
            rc = uv_fs_rename(writer_loop, &rename_req, old_path, new_path, NULL);
            if (unlikely(rc)) fatal_libuv_err(rc, __LINE__);
            uv_fs_req_cleanup(&rename_req);
            
            /* (a) Update blob_write_handle_offset, (b) truncate new write-to BLOB, 
             * (c) update filesize of truncated BLOB in SQLite DB, (d) delete
             * respective logs in LOGS_TABLE for the truncated BLOB and (e)
             * reset blob_filesize */
            /* (a) */ 
            p_file_info->blob_write_handle_offset = p_file_info->blob_write_handle_offset == 1 ? BLOB_MAX_FILES : p_file_info->blob_write_handle_offset - 1;
            /* (b) */ 
            uv_fs_t trunc_req;
            rc = uv_fs_ftruncate(writer_loop, &trunc_req, p_file_info->blob_handles[p_file_info->blob_write_handle_offset], 0, NULL);						
            if (unlikely(rc)) fatal_libuv_err(rc, __LINE__);
            uv_fs_req_cleanup(&trunc_req);
            /* (c) */ 
            rc = sqlite3_bind_int(stmt_blobs_set_zero_filesize, 1, p_file_info->blob_write_handle_offset);
            if (rc != SQLITE_OK) fatal_sqlite3_err(rc, __LINE__);
            rc = sqlite3_step(stmt_blobs_set_zero_filesize);
            if (unlikely(rc != SQLITE_DONE)) fatal_sqlite3_err(rc, __LINE__);
            rc = sqlite3_reset(stmt_blobs_set_zero_filesize);
            if (unlikely(rc != SQLITE_OK)) fatal_sqlite3_err(rc, __LINE__);
            /* (d) */
            rc = sqlite3_bind_int(stmt_logs_delete, 1, p_file_info->blob_write_handle_offset);
            if (rc != SQLITE_OK) fatal_sqlite3_err(rc, __LINE__);
            rc = sqlite3_step(stmt_logs_delete);
            if (unlikely(rc != SQLITE_DONE)) fatal_sqlite3_err(rc, __LINE__);
            rc = sqlite3_reset(stmt_logs_delete);
            if (unlikely(rc != SQLITE_OK)) fatal_sqlite3_err(rc, __LINE__);
            /* (e) */
            blob_filesize = 0;

        }

        /* Update total disk usage of all BLOBs for this log source */
        rc = sqlite3_step(stmt_blobs_get_total_filesize);
        if (unlikely(rc != SQLITE_ROW)) fatal_sqlite3_err(rc, __LINE__);
        __atomic_store_n(&p_file_info->blob_total_size, sqlite3_column_int64(stmt_blobs_get_total_filesize, 0), __ATOMIC_RELAXED);
        // debug(D_LOGS_MANAG, "p_file_info->blob_total_size: %lld for: %s\n", p_file_info->blob_total_size, p_file_info->filename);
        rc = sqlite3_reset(stmt_blobs_get_total_filesize);
        if (unlikely(rc != SQLITE_OK)) fatal_sqlite3_err(rc, __LINE__);

        // TODO: Can uv_mutex_unlock(p_file_info->db_mut) be moved before if(blob_filesize > p_file_info-> blob_max_size) ?
        uv_mutex_unlock(p_file_info->db_mut);
        uv_sleep(p_file_info->buff_flush_to_db_interval * MSEC_PER_SEC);
    }
}

/**
 * @brief Process the events of the uv_loop_t related to the DB API
 */
static inline void db_loop_run(void *arg){
    UNUSED(arg);
    uv_run(db_loop, UV_RUN_DEFAULT);
}

inline void db_set_main_dir(char *dir){
    main_db_dir = dir;
}

void db_init() {
    int rc = 0;
    char *err_msg = 0;
    uv_fs_t mkdir_req;
    
    db_loop = mallocz(sizeof(uv_loop_t));
    rc = uv_loop_init(db_loop);
    if (unlikely(rc)) fatal_libuv_err(rc, __LINE__);

    main_db_path = mallocz(strlen(main_db_dir) + sizeof(MAIN_DB) + 1);
    sprintf(main_db_path, "%s/" MAIN_DB, main_db_dir);

    /* Create databases directory if it doesn't exist. */
    rc = uv_fs_mkdir(db_loop, &mkdir_req, main_db_dir, 0775, NULL);
    if(rc == 0) {
        info("DB directory created: %s", main_db_dir);
    }
    else if (rc == UV_EEXIST) {
        info("DB directory %s found", main_db_dir);
    }
    else {
        error("DB mkdir() %s/ error: %s", main_db_dir, uv_strerror(rc));
        fatal_sqlite3_err(rc, __LINE__);
    }
    uv_fs_req_cleanup(&mkdir_req);

    /* Create or open main db */
    rc = sqlite3_open(main_db_path, &main_db);
    if (unlikely(rc != SQLITE_OK)) fatal_sqlite3_err(rc, __LINE__);
    
    /* Configure main database */
    rc = sqlite3_exec(main_db,
                      "PRAGMA auto_vacuum = INCREMENTAL;"
                      "PRAGMA synchronous = 1;"
                      "PRAGMA journal_mode = WAL;"
                      "PRAGMA temp_store = MEMORY;"
                      "PRAGMA foreign_keys = ON;",
                      0, 0, &err_msg);
    if (unlikely(rc != SQLITE_OK)) {
        error("Failed to configure database");
        error("SQL error: %s\n", err_msg);
        sqlite3_free(err_msg);
        fatal_sqlite3_err(rc, __LINE__);
    } else {
        info("%s configured successfully", MAIN_DB);
    }
    
    /* Create new main DB LogCollections table if it doesn't exist */
    rc = sqlite3_exec(main_db,
                      "CREATE TABLE IF NOT EXISTS " MAIN_COLLECTIONS_TABLE "("
                      "Id 					INTEGER 	PRIMARY KEY,"
                      "Machine_GUID			TEXT		NOT NULL,"
                      "Log_Source_Path		TEXT		NOT NULL,"
                      "Type					INTEGER		NOT NULL,"
                      "DB_Dir 				TEXT 		NOT NULL"
                      ");",
                      0, 0, &err_msg);
    if (unlikely(rc != SQLITE_OK)) {
        error("Failed to create table" MAIN_COLLECTIONS_TABLE);
        error("SQL error: %s", err_msg);
        sqlite3_free(err_msg);
        fatal_sqlite3_err(rc, __LINE__);
    } 
    // else debug(D_LOGS_MANAG, "Table %s created successfully", MAIN_COLLECTIONS_TABLE);
    
    sqlite3_stmt *stmt_search_if_log_source_exists;
    rc = sqlite3_prepare_v2(main_db,
                            "SELECT COUNT(*), Id, DB_Dir FROM " MAIN_COLLECTIONS_TABLE
                            " WHERE Log_Source_Path = ? ;",
                            -1, &stmt_search_if_log_source_exists, NULL);
    if (unlikely(rc != SQLITE_OK)) fatal_sqlite3_err(rc, __LINE__);
    
    sqlite3_stmt *stmt_insert_log_collection_metadata;
    rc = sqlite3_prepare_v2(main_db,
                            "INSERT INTO " MAIN_COLLECTIONS_TABLE
                            " (Machine_GUID, Log_Source_Path, Type, DB_Dir) VALUES (?,?,?,?) ;",
                            -1, &stmt_insert_log_collection_metadata, NULL);
    if (unlikely(rc != SQLITE_OK)) fatal_sqlite3_err(rc, __LINE__);
    
    for (int i = 0; i < p_file_infos_arr->count; i++) {

        /* Initialise DB mutex and acquire lock */
        p_file_infos_arr->data[i]->db_mut = mallocz(sizeof(uv_mutex_t));
        rc = uv_mutex_init(p_file_infos_arr->data[i]->db_mut);
        if (unlikely(rc)) fatal_libuv_err(rc, __LINE__);
        uv_mutex_lock(p_file_infos_arr->data[i]->db_mut);

        rc = sqlite3_bind_text(stmt_search_if_log_source_exists, 1, p_file_infos_arr->data[i]->filename, -1, NULL);
        if (unlikely(rc != SQLITE_OK)) fatal_sqlite3_err(rc, __LINE__);
        rc = sqlite3_step(stmt_search_if_log_source_exists);
        /* COUNT(*) query should always return SQLITE_ROW */
        if (unlikely(rc != SQLITE_ROW)) fatal_sqlite3_err(rc, __LINE__);
        
        int log_source_occurences = sqlite3_column_int(stmt_search_if_log_source_exists, 0);
        // debug(D_LOGS_MANAG, "DB file occurences of %s: %d", p_file_infos_arr->data[i]->filename, log_source_occurences);
        switch (log_source_occurences) {
            case 0:  /* Log collection metadata not found in main DB - create a new record */
                
                /* Bind machine GUID */
                rc = sqlite3_bind_text(stmt_insert_log_collection_metadata, 1, localhost->machine_guid, -1, NULL);
                if (unlikely(rc != SQLITE_OK)) fatal_sqlite3_err(rc, __LINE__);

                /* Bind log source path */
                rc = sqlite3_bind_text(stmt_insert_log_collection_metadata, 2, p_file_infos_arr->data[i]->filename, -1, NULL);
                if (unlikely(rc != SQLITE_OK)) fatal_sqlite3_err(rc, __LINE__);
                
                /* Bind log type */
                rc = sqlite3_bind_int(stmt_insert_log_collection_metadata, 3, p_file_infos_arr->data[i]->log_type);
                if (unlikely(rc != SQLITE_OK)) fatal_sqlite3_err(rc, __LINE__);
                
                /* Create directory of collection of logs for the particular 
                 * log source (in the form of a UUID) and bind it. */
                uuid_t uuid;
                uuid_generate(uuid);
                char uuid_str[GUID_LEN + 1];      // ex. "1b4e28ba-2fa1-11d2-883f-0016d3cca427" + "\0"
                uuid_unparse_lower(uuid, uuid_str);
                
                char *db_dir = mallocz(snprintf(NULL, 0, "%s/%s/", main_db_dir, uuid_str) + 1);
                sprintf(db_dir, "%s/%s/", main_db_dir, uuid_str);
                
                rc = uv_fs_mkdir(db_loop, &mkdir_req, db_dir, 0775, NULL);
                if (unlikely(rc)) {
                    if(errno == EEXIST) fatal("DB directory %s exists but not found in %s.\n", db_dir, MAIN_DB);
                    fatal_libuv_err(rc, __LINE__);
                }
                uv_fs_req_cleanup(&mkdir_req);
                
                rc = sqlite3_bind_text(stmt_insert_log_collection_metadata, 4, db_dir, -1, NULL);
                if (unlikely(rc != SQLITE_OK)) fatal_sqlite3_err(rc, __LINE__);
                rc = sqlite3_step(stmt_insert_log_collection_metadata);
                if (unlikely(rc != SQLITE_DONE)) fatal_sqlite3_err(rc, __LINE__);
                    
                p_file_infos_arr->data[i]->db_dir = db_dir;
                
                rc = sqlite3_reset(stmt_insert_log_collection_metadata);
                if (unlikely(rc != SQLITE_OK)) fatal_sqlite3_err(rc, __LINE__);
                break;
                
            case 1:  // File metadata found in DB            
                p_file_infos_arr->data[i]->db_dir = mallocz((size_t)sqlite3_column_bytes(stmt_search_if_log_source_exists, 2) + 1);
                sprintf((char*) p_file_infos_arr->data[i]->db_dir, "%s", sqlite3_column_text(stmt_search_if_log_source_exists, 2));
                break;
                
            default:  // Error, file metadata can exist either 0 or 1 times in DB
                m_assert(0, "Same file stored in DB more than once!");
                fatal(	"Error: %s record encountered multiple times in DB " MAIN_COLLECTIONS_TABLE " table \n",
                        p_file_infos_arr->data[i]->filename);
        }
        rc = sqlite3_reset(stmt_search_if_log_source_exists);
        if (unlikely(rc != SQLITE_OK)) fatal_sqlite3_err(rc, __LINE__);
        
        /* Create or open metadata DBs for each log collection */
        char *db_metadata = mallocz(snprintf(NULL, 0, "%s" METADATA_DB_FILENAME, p_file_infos_arr->data[i]->db_dir) + 1);
        sprintf(db_metadata, "%s" METADATA_DB_FILENAME, p_file_infos_arr->data[i]->db_dir);
        rc = sqlite3_open(db_metadata, &p_file_infos_arr->data[i]->db);
        if (unlikely(rc != SQLITE_OK)) fatal_sqlite3_err(rc, __LINE__);
        p_file_infos_arr->data[i]->db_metadata = db_metadata;
        // freez(db_metadata);
        
        /* Configure metadata DB */
        rc = sqlite3_exec(p_file_infos_arr->data[i]->db,
                          "PRAGMA auto_vacuum = INCREMENTAL;"
                          "PRAGMA synchronous = 1;"
                          "PRAGMA journal_mode = WAL;"
                          "PRAGMA temp_store = MEMORY;"
                          "PRAGMA foreign_keys = ON;",
                          0, 0, &err_msg);
        if (unlikely(rc != SQLITE_OK)) {
            error("Failed to configure database for %s", p_file_infos_arr->data[i]->filename);
            error("SQL error: %s", err_msg);
            sqlite3_free(err_msg);
            fatal("Failed to configure database for %s\n, SQL error: %s\n", p_file_infos_arr->data[i]->filename, err_msg);
        }
        
        /* Check if BLOBS_TABLE exists or not */
        sqlite3_stmt *stmt_check_if_BLOBS_TABLE_exists;
        rc = sqlite3_prepare_v2(p_file_infos_arr->data[i]->db,
                                "SELECT COUNT(*) FROM sqlite_master" 
                                " WHERE type='table' AND name='"BLOBS_TABLE"';",
                                -1, &stmt_check_if_BLOBS_TABLE_exists, NULL);
        if (unlikely(rc != SQLITE_OK)) fatal_sqlite3_err(rc, __LINE__);
        rc = sqlite3_step(stmt_check_if_BLOBS_TABLE_exists);
        if (unlikely(rc != SQLITE_ROW)) fatal_sqlite3_err(rc, __LINE__); /* COUNT(*) query should always return SQLITE_ROW */
        
        /* If BLOBS_TABLE doesn't exist, create and populate it */
        if(sqlite3_column_int(stmt_check_if_BLOBS_TABLE_exists, 0) == 0){
            
            /* 1. Create it */
            rc = sqlite3_exec(p_file_infos_arr->data[i]->db,
                      "CREATE TABLE IF NOT EXISTS " BLOBS_TABLE "("
                      "Id 		INTEGER 	PRIMARY KEY,"
                      "Filename	TEXT		NOT NULL,"
                      "Filesize INTEGER 	NOT NULL"
                      ");",
                      0, 0, &err_msg);
            if (unlikely(rc != SQLITE_OK)) {
                sqlite3_free(err_msg);
                fatal("Failed to create %s. SQL error: %s", BLOBS_TABLE, err_msg);
            } 
            // else debug(D_LOGS_MANAG, "Table %s created successfully\n", BLOBS_TABLE);
            
            /* 2. Populate it */
            sqlite3_stmt *stmt_init_BLOBS_table;
            rc = sqlite3_prepare_v2(p_file_infos_arr->data[i]->db,
                            "INSERT INTO " BLOBS_TABLE 
                            " (Filename, Filesize) VALUES (?,?) ;",
                            -1, &stmt_init_BLOBS_table, NULL);
            if (unlikely(rc != SQLITE_OK)) fatal_sqlite3_err(rc, __LINE__);
            for( int i = 0; i < BLOB_MAX_FILES; i++){
                char *filename = mallocz(snprintf(NULL, 0, BLOB_STORE_FILENAME ".%d", i) + 1);
                sprintf(filename, BLOB_STORE_FILENAME ".%d", i);
                rc = sqlite3_bind_text(stmt_init_BLOBS_table, 1, filename, -1, NULL);
                if (rc != SQLITE_OK) fatal_sqlite3_err(rc, __LINE__);
                rc = sqlite3_bind_int64(stmt_init_BLOBS_table, 2, (sqlite3_int64) 0);
                if (rc != SQLITE_OK) fatal_sqlite3_err(rc, __LINE__);      
                rc = sqlite3_step(stmt_init_BLOBS_table);
                if (rc != SQLITE_DONE) fatal_sqlite3_err(rc, __LINE__);
                rc = sqlite3_reset(stmt_init_BLOBS_table);
                if (unlikely(rc != SQLITE_OK)) fatal_sqlite3_err(rc, __LINE__);
                freez(filename);
            }
            rc = sqlite3_finalize(stmt_init_BLOBS_table);
            if (unlikely(rc != SQLITE_OK)) fatal_sqlite3_err(rc, __LINE__);
        }
        rc = sqlite3_finalize(stmt_check_if_BLOBS_TABLE_exists);
        if (unlikely(rc != SQLITE_OK)) fatal_sqlite3_err(rc, __LINE__);
        
        /* If LOGS_TABLE doesn't exist, create it */
        rc = sqlite3_exec(p_file_infos_arr->data[i]->db,
                      "CREATE TABLE IF NOT EXISTS " LOGS_TABLE "("
                      "Id 					INTEGER 	PRIMARY KEY,"
                      "FK_BLOB_Id			INTEGER		NOT NULL,"
                      "BLOB_Offset			INTEGER		NOT NULL,"
                      "Timestamp 			INTEGER		NOT NULL,"
                      "Msg_compr_size 		INTEGER		NOT NULL,"
                      "Msg_decompr_size 	INTEGER		NOT NULL,"
                      "FOREIGN KEY (FK_BLOB_Id) REFERENCES " BLOBS_TABLE " (Id) ON DELETE CASCADE ON UPDATE CASCADE"
                      ");",
                      0, 0, &err_msg);
        if (unlikely(rc != SQLITE_OK)) {
            sqlite3_free(err_msg);
            fatal("Failed to create %s. SQL error: %s\n", LOGS_TABLE, err_msg);
        } 
        // else debug(D_LOGS_MANAG, "Table %s created successfully\n", LOGS_TABLE);
        
        // Create index on LOGS_TABLE Timestamp
        /* TODO: If this doesn't speed up queries, check SQLITE R*tree module. 
         * Requires benchmarking with/without index. */
        rc = sqlite3_exec(p_file_infos_arr->data[i]->db,
                          "CREATE INDEX IF NOT EXISTS logs_timestamps_idx "
                          "ON " LOGS_TABLE "(Timestamp);",
                          0, 0, &err_msg);
        if (unlikely(rc != SQLITE_OK)) {
            fatal("Failed to create logs_timestamps_idx. SQL error: %s", err_msg);
            sqlite3_free(err_msg);
        } 
        // else debug(D_LOGS_MANAG, "logs_timestamps_idx created successfully\n");

        /* Remove excess BLOBs beyond BLOB_MAX_FILES (from both DB and disk 
         * storage). This is useful if BLOB_MAX_FILES is reduced after an agent
         * restart (for example, if in the future it is not hardcoded, but 
         * instead it is read from the configuration file). LOGS_TABLE entries
         * should be deleted automatically (due to ON DELETE CASCADE). */
        {
            sqlite3_stmt *stmt_get_BLOBS_TABLE_size;
            rc = sqlite3_prepare_v2(p_file_infos_arr->data[i]->db,
                "SELECT MAX(Id) FROM " BLOBS_TABLE ";",
                -1, &stmt_get_BLOBS_TABLE_size, NULL);
            if (unlikely(rc != SQLITE_OK)) fatal_sqlite3_err(rc, __LINE__);
            rc = sqlite3_step(stmt_get_BLOBS_TABLE_size);
            if (unlikely(rc != SQLITE_ROW)) fatal_sqlite3_err(rc, __LINE__);
            const int blobs_table_max_id = sqlite3_column_int(stmt_get_BLOBS_TABLE_size, 0);

            sqlite3_stmt *stmt_retrieve_filename_last_digits; // This statement retrieves the last digit(s) from the Filename column of BLOBS_TABLE
            rc = sqlite3_prepare_v2(p_file_infos_arr->data[i]->db,
                "WITH split(word, str) AS ( SELECT '', (SELECT Filename FROM " BLOBS_TABLE " WHERE Id = ? ) || '.' "
                "UNION ALL SELECT substr(str, 0, instr(str, '.')), substr(str, instr(str, '.')+1) FROM split WHERE str!='' ) "
                "SELECT word FROM split WHERE word!='' ORDER BY LENGTH(str) LIMIT 1;",
                -1, &stmt_retrieve_filename_last_digits, NULL);
            if (unlikely(rc != SQLITE_OK)) fatal_sqlite3_err(rc, __LINE__);

            sqlite3_stmt *stmt_delete_row_by_id; 
            rc = sqlite3_prepare_v2(p_file_infos_arr->data[i]->db,
                "DELETE FROM " BLOBS_TABLE " WHERE Id = ?;",
                -1, &stmt_delete_row_by_id, NULL);
            if (unlikely(rc != SQLITE_OK)) fatal_sqlite3_err(rc, __LINE__);

            for (int id = 1; id <= blobs_table_max_id; id++){

                rc = sqlite3_bind_int(stmt_retrieve_filename_last_digits, 1, id);
                if (unlikely(rc != SQLITE_OK)) fatal_sqlite3_err(rc, __LINE__);
                rc = sqlite3_step(stmt_retrieve_filename_last_digits);
                if (unlikely(rc != SQLITE_ROW)) fatal_sqlite3_err(rc, __LINE__);
                int last_digits = sqlite3_column_int(stmt_retrieve_filename_last_digits, 0);
                rc = sqlite3_reset(stmt_retrieve_filename_last_digits);
                if (unlikely(rc != SQLITE_OK)) fatal_sqlite3_err(rc, __LINE__);

                /* If last_digits > BLOB_MAX_FILES - 1, then some BLOB files will need to be removed
                 * (both from DB BLOBS_TABLE and also from the disk) */
                if(last_digits > BLOB_MAX_FILES - 1){

                    // Delete entry from DB BLOBS_TABLE
                    rc = sqlite3_bind_int(stmt_delete_row_by_id, 1, id);
                    if (unlikely(rc != SQLITE_OK)) fatal_sqlite3_err(rc, __LINE__);
                    rc = sqlite3_step(stmt_delete_row_by_id);
                    if (unlikely(rc != SQLITE_DONE)) fatal_sqlite3_err(rc, __LINE__);
                    rc = sqlite3_reset(stmt_delete_row_by_id);
                    if (unlikely(rc != SQLITE_OK)) fatal_sqlite3_err(rc, __LINE__);

                    // Delete BLOB file from filesystem
                    char blob_delete_path[FILENAME_MAX + 1];
                    sprintf(blob_delete_path, "%s" BLOB_STORE_FILENAME ".%d", p_file_infos_arr->data[i]->db_dir, last_digits);
                    uv_fs_t unlink_req;
                    rc = uv_fs_unlink(db_loop, &unlink_req, blob_delete_path, NULL);
                    if (unlikely(rc)) fatal("Delete %s error: %s\n", blob_delete_path, uv_strerror(rc));
                    uv_fs_req_cleanup(&unlink_req);
                   
                }
            }
            rc = sqlite3_finalize(stmt_retrieve_filename_last_digits);
            if (unlikely(rc != SQLITE_OK)) fatal_sqlite3_err(rc, __LINE__);
            rc = sqlite3_finalize(stmt_delete_row_by_id);
            if (unlikely(rc != SQLITE_OK)) fatal_sqlite3_err(rc, __LINE__);

            /* BLOBS_TABLE ids after the deletion might not be continuous. This
             * needs to be fixed, by having the ids updated. LOGS_TABLE FKs will
             * be updated automatically (due to ON UPDATE CASCADE) */

            int old_blobs_table_ids[BLOB_MAX_FILES];
            int off = 0;
            sqlite3_stmt *stmt_retrieve_all_ids; 
            rc = sqlite3_prepare_v2(p_file_infos_arr->data[i]->db,
                "SELECT Id FROM " BLOBS_TABLE " ORDER BY Id ASC;",
                -1, &stmt_retrieve_all_ids, NULL);
            if (unlikely(rc != SQLITE_OK)) fatal_sqlite3_err(rc, __LINE__);

            rc = sqlite3_step(stmt_retrieve_all_ids);
            while(rc == SQLITE_ROW){
                old_blobs_table_ids[off++] = sqlite3_column_int(stmt_retrieve_all_ids, 0);
                rc = sqlite3_step(stmt_retrieve_all_ids);
            }
            if (unlikely(rc != SQLITE_DONE)) fatal_sqlite3_err(rc, __LINE__);
            rc = sqlite3_finalize(stmt_retrieve_all_ids);
            if (unlikely(rc != SQLITE_OK)) fatal_sqlite3_err(rc, __LINE__);

            sqlite3_stmt *stmt_update_id; 
            rc = sqlite3_prepare_v2(p_file_infos_arr->data[i]->db,
                "UPDATE " BLOBS_TABLE " SET Id = ? WHERE Id = ?;",
                -1, &stmt_update_id, NULL);
            if (unlikely(rc != SQLITE_OK)) fatal_sqlite3_err(rc, __LINE__);

            for (int i = 0; i < BLOB_MAX_FILES; i++){
                rc = sqlite3_bind_int(stmt_update_id, 1, i + 1);
                if (unlikely(rc != SQLITE_OK)) fatal_sqlite3_err(rc, __LINE__);
                rc = sqlite3_bind_int(stmt_update_id, 2, old_blobs_table_ids[i]);
                if (unlikely(rc != SQLITE_OK)) fatal_sqlite3_err(rc, __LINE__);
                rc = sqlite3_step(stmt_update_id);
                if (unlikely(rc != SQLITE_DONE)) fatal_sqlite3_err(rc, __LINE__);
                rc = sqlite3_reset(stmt_update_id);
                if (unlikely(rc != SQLITE_OK)) fatal_sqlite3_err(rc, __LINE__);
            }
            rc = sqlite3_finalize(stmt_update_id);
            if (unlikely(rc != SQLITE_OK)) fatal_sqlite3_err(rc, __LINE__);
        }

        /* Traverse BLOBS_TABLE, open logs.bin.X files and store their file handles in p_file_info array. */
        sqlite3_stmt *stmt_retrieve_metadata_from_id;
        rc = sqlite3_prepare_v2(p_file_infos_arr->data[i]->db,
                                "SELECT Filename, Filesize FROM " BLOBS_TABLE 
                                " WHERE Id = ? ;",
                                -1, &stmt_retrieve_metadata_from_id, NULL);
        if (unlikely(rc != SQLITE_OK)) fatal_sqlite3_err(rc, __LINE__);
        
        sqlite3_stmt *stmt_retrieve_total_logs_size;
        rc = sqlite3_prepare_v2(p_file_infos_arr->data[i]->db,
                                "SELECT SUM(Msg_compr_size) FROM " LOGS_TABLE 
                                " WHERE FK_BLOB_Id = ? GROUP BY FK_BLOB_Id ;",
                                -1, &stmt_retrieve_total_logs_size, NULL);
        if (unlikely(rc != SQLITE_OK)) fatal_sqlite3_err(rc, __LINE__);
        
        uv_fs_t open_req;
        for(int id = 1; id <= BLOB_MAX_FILES; id++){
            /* Open BLOB file based on filename stored in BLOBS_TABLE. */
            rc = sqlite3_bind_int(stmt_retrieve_metadata_from_id, 1, id);
            if (unlikely(rc != SQLITE_OK)) fatal_sqlite3_err(rc, __LINE__);
            rc = sqlite3_step(stmt_retrieve_metadata_from_id);
            if (unlikely(rc != SQLITE_ROW)) fatal_sqlite3_err(rc, __LINE__);
            char *filename = mallocz(snprintf(NULL, 0, "%s%s", 
                p_file_infos_arr->data[i]->db_dir, 
                sqlite3_column_text(stmt_retrieve_metadata_from_id, 0)) + 1);
            sprintf(filename, "%s%s", p_file_infos_arr->data[i]->db_dir, 
                sqlite3_column_text(stmt_retrieve_metadata_from_id, 0));
            rc = uv_fs_open(db_loop, &open_req, filename, 
                UV_FS_O_RDWR | UV_FS_O_CREAT | UV_FS_O_APPEND | UV_FS_O_RANDOM , 0644, NULL);
            if (unlikely(rc < 0)) fatal_libuv_err(rc, __LINE__);
            // debug(D_LOGS_MANAG, "Opened file: %s\n", filename);
            p_file_infos_arr->data[i]->blob_handles[id] = open_req.result; 	// open_req.result of a uv_fs_t is the file descriptor in case of the uv_fs_open
            const int64_t metadata_filesize = (int64_t) sqlite3_column_int64(stmt_retrieve_metadata_from_id, 1);
            
            /* Retrieve total log messages compressed size from LOGS_TABLE for current FK_BLOB_Id
             * Only for asserting whether correct - not used elsewhere. If no rows are returned, it means
             * it is probably the initial execution of the program so still valid (except if rc is other
             * than SQLITE_DONE, which is an error then). */
            rc = sqlite3_bind_int(stmt_retrieve_total_logs_size, 1, id);
            if (unlikely(rc != SQLITE_OK)) fatal_sqlite3_err(rc, __LINE__);
            rc = sqlite3_step(stmt_retrieve_total_logs_size);
            if (rc == SQLITE_ROW){ 
                const int64_t total_logs_filesize = (int64_t) sqlite3_column_int64(stmt_retrieve_total_logs_size, 0);
                m_assert(total_logs_filesize == metadata_filesize, "Metadata filesize != total logs filesize");
            }
            else if (unlikely(rc != SQLITE_DONE)) fatal_sqlite3_err(rc, __LINE__);
            
            /* Get filesize of BLOB file. */ 
            uv_fs_t stat_req;
            rc = uv_fs_stat(db_loop, &stat_req, filename, NULL);
            if (unlikely(rc)) fatal_libuv_err(rc, __LINE__);
            uv_stat_t *statbuf = uv_fs_get_statbuf(&stat_req);
            const int64_t blob_filesize = (int64_t) statbuf->st_size;
            uv_fs_req_cleanup(&stat_req);
            
            do{
                /* Case 1: blob_filesize == metadata_filesize (equal, either both zero or not): All good */
                if(likely(blob_filesize == metadata_filesize))
                    break;
                
                /* Case 2: blob_filesize == 0 && metadata_filesize > 0: fatal(), however could it mean that 
                 * EXT_BLOB_STORE_FILENAME was rotated but the SQLite metadata wasn't updated? So can it 
                 * maybe be recovered by un-rotating? Either way, treat as fatal() for now. */
                 // TODO: Can we avoid fatal()? 
                if(unlikely(blob_filesize == 0 && metadata_filesize > 0)){
                    fatal("blob_filesize == 0 but metadata_filesize > 0 for '%s'\n", filename);
                }
                
                /* Case 3: blob_filesize > metadata_filesize: Truncate binary to sqlite filesize, program 
                 * crashed or terminated after writing BLOBs to external file but before metadata was updated */
                if(unlikely(blob_filesize > metadata_filesize)){
                    infoerr("blob_filesize > metadata_filesize for '%s'. Will attempt to fix it.", filename);
                    uv_fs_t trunc_req;
                    rc = uv_fs_ftruncate(db_loop, &trunc_req, p_file_infos_arr->data[i]->blob_handles[id], 
                        metadata_filesize, NULL);
                    if(unlikely(rc)) fatal_libuv_err(rc, __LINE__);
                    uv_fs_req_cleanup(&trunc_req);
                    break;
                }

                /* Case 4: blob_filesize < metadata_filesize: unrecoverable (and what-should-be impossible
                 * state), delete external binaries, clear metadata record and then fatal() */
                // TODO: Delete external BLOB and clear metadata from DB, start from clean state but the most recent logs.
                if(unlikely(blob_filesize < metadata_filesize)){
                    fatal("blob_filesize < metadata_filesize for '%s'. \n", filename);
                }

                /* Case 5: default if none of the above, should never reach here, fatal() */
                fatal("invalid case when comparing blob_filesize with metadata_filesize");
            } while(0);
            
            
            /* Initialise blob_write_handle with logs.bin.0 */
            if(filename[strlen(filename) - 1] == '0')
                p_file_infos_arr->data[i]->blob_write_handle_offset = id;
                
            freez(filename);
            uv_fs_req_cleanup(&open_req);
            rc = sqlite3_reset(stmt_retrieve_total_logs_size);
            if (unlikely(rc != SQLITE_OK)) fatal_sqlite3_err(rc, __LINE__);
            rc = sqlite3_reset(stmt_retrieve_metadata_from_id);
            if (unlikely(rc != SQLITE_OK)) fatal_sqlite3_err(rc, __LINE__);
        }
        rc = sqlite3_finalize(stmt_retrieve_metadata_from_id);
        if (unlikely(rc != SQLITE_OK)) fatal_sqlite3_err(rc, __LINE__);

        /* DB initialisation finished; release lock */
        uv_mutex_unlock(p_file_infos_arr->data[i]->db_mut);
        
        /* Create synchronous writer thread, one for each log source */
        uv_thread_t *db_writer_thread = mallocz(sizeof(uv_thread_t));
        rc = uv_thread_create(db_writer_thread, db_writer, &p_file_infos_arr->data[i]);
        if (unlikely(rc)) fatal_libuv_err(rc, __LINE__);
        
    }
    rc = sqlite3_finalize(stmt_search_if_log_source_exists);
    if (unlikely(rc != SQLITE_OK)) fatal_sqlite3_err(rc, __LINE__);
    rc = sqlite3_finalize(stmt_insert_log_collection_metadata);
    if (unlikely(rc != SQLITE_OK)) fatal_sqlite3_err(rc, __LINE__);

    uv_thread_t *db_loop_run_thread = mallocz(sizeof(uv_thread_t));
    if(unlikely(uv_thread_create(db_loop_run_thread, db_loop_run, NULL))) fatal("uv_thread_create() error");
}

/**
 * @brief Search database
 * @details This function searches the database for any results matching the
 * query parameters. If any results are found, it will decompress the text
 * of each returned row and add it to the results buffer, up to a maximum
 * amount of p_query_params->quota bytes. 
 * @todo What happens in case SQLITE_CORRUPT error? See if it can be handled, for now just fatal().
 * @todo Make decompress buffer static to reduce mallocs/frees.
 */
void db_search(logs_query_params_t *const p_query_params, struct File_info *const p_file_info) {
    int rc = 0;

    // Prepare "SELECT" statement used to retrieve log metadata in case of query
    // TODO: Avoid preparing statement for each db_search call, but how if tied to specific db handle?
    // TODO: Limit number of results returned through SQLite Query to speed up search?
    sqlite3_stmt *stmt_retrieve_log_msg_metadata;
    rc = sqlite3_prepare_v2(p_file_info->db,
                            "SELECT Timestamp, Msg_compr_size , Msg_decompr_size, BLOB_Offset, " BLOBS_TABLE".Id "
                            "FROM " LOGS_TABLE " INNER JOIN " BLOBS_TABLE " "
                            "ON " LOGS_TABLE ".FK_BLOB_Id = " BLOBS_TABLE ".Id "
                            "WHERE Timestamp BETWEEN ? AND ? "
                            "ORDER BY Timestamp;",
                            -1, &stmt_retrieve_log_msg_metadata, NULL);
    if (unlikely(rc != SQLITE_OK)) fatal_sqlite3_err(rc, __LINE__);

    rc = sqlite3_exec(p_file_info->db, "BEGIN TRANSACTION;", NULL, NULL, NULL);
    if (unlikely(rc != SQLITE_OK)) fatal_sqlite3_err(rc, __LINE__);
    rc = sqlite3_bind_int64(stmt_retrieve_log_msg_metadata, 1, (sqlite3_int64)p_query_params->start_timestamp);
    if (unlikely(rc != SQLITE_OK)) fatal_sqlite3_err(rc, __LINE__);
    rc = sqlite3_bind_int64(stmt_retrieve_log_msg_metadata, 2, (sqlite3_int64)p_query_params->end_timestamp);
    if (unlikely(rc != SQLITE_OK)) fatal_sqlite3_err(rc, __LINE__);

    rc = sqlite3_step(stmt_retrieve_log_msg_metadata);
    if (unlikely(rc != SQLITE_ROW && rc != SQLITE_DONE)) fatal_sqlite3_err(rc, __LINE__);

    while (rc == SQLITE_ROW) {
        Circ_buff_item_t temp_msg = {0};

        /* Retrieve metadata from DB */
        temp_msg.timestamp = (uint64_t)sqlite3_column_int64(stmt_retrieve_log_msg_metadata, 0);
        temp_msg.text_compressed_size = (size_t)sqlite3_column_int64(stmt_retrieve_log_msg_metadata, 1);
        temp_msg.text_size = (size_t)sqlite3_column_int64(stmt_retrieve_log_msg_metadata, 2);
        int64_t blob_offset = (int64_t) sqlite3_column_int64(stmt_retrieve_log_msg_metadata, 3);
        int blob_handles_offset = sqlite3_column_int(stmt_retrieve_log_msg_metadata, 4);

        /* Retrieve compressed log messages from BLOB file */
        temp_msg.text_compressed = mallocz(temp_msg.text_compressed_size);
        uv_buf_t uv_buf = uv_buf_init(temp_msg.text_compressed, temp_msg.text_compressed_size);
        uv_fs_t read_req;
        // TODO: Using db_loop here in separate thread (although synchronously) - thread-safe ?
        rc = uv_fs_read(db_loop, &read_req, p_file_info->blob_handles[blob_handles_offset], &uv_buf, 1, blob_offset, NULL);
        if (unlikely(rc < 0)) fatal_libuv_err(rc, __LINE__);
        uv_fs_req_cleanup(&read_req);

        /* Append retrieved results to BUFFER */
        size_t old_results_len = p_query_params->results_buff->len;
        buffer_sprintf(p_query_params->results_buff, "\t\t[ %" PRIu64 ", \"", temp_msg.timestamp);
        /* In case of search_keyword, less than item.text_size space is 
         * required, but go for worst case scenario for now */
        buffer_increase(p_query_params->results_buff, temp_msg.text_size);
        if(!p_query_params->keyword || !*p_query_params->keyword || !strcmp(p_query_params->keyword, " ")){
            // TODO: decompress_text does not handle or return any errors currently. How should any errors be handled?
            decompress_text(&temp_msg, &p_query_params->results_buff->buffer[p_query_params->results_buff->len]);
            p_query_params->results_buff->len += temp_msg.text_size;
            p_query_params->results_buff->len--; // get rid of '\0'
            // Watch out! Changing the next line will break web_client_api_request_v1_logsmanagement()! 
            buffer_strcat(p_query_params->results_buff, "\", \t0],\n");
        } 
        else {
            decompress_text(&temp_msg, NULL);
            size_t res_size = 0;
            const int matches = search_keyword(temp_msg.data, temp_msg.text_size, 
                                               &p_query_params->results_buff->buffer[p_query_params->results_buff->len], 
                                               &res_size, 
                                               p_query_params->keyword, NULL, 
                                               p_query_params->ignore_case);
            freez(temp_msg.data);

            if(likely(matches > 0)) {
                m_assert(res_size > 0, "res_size can't be <= 0");
                p_query_params->results_buff->len += res_size;
                p_query_params->results_buff->len--; // get rid of '\0'
                // buffer_strcat(p_query_params->results_buff, "\"],\n");
                // Watch out! Changing the next line will break web_client_api_request_v1_logsmanagement()! 
                buffer_sprintf(p_query_params->results_buff, "\", \t%d],\n", matches);
                p_query_params->keyword_matches += matches;
            }
            else if(unlikely(matches == 0)){
                m_assert(res_size == 0, "res_size must be == 0");
                /* No keyword matches, undo timestamp buffer_sprintf() */
                p_query_params->results_buff->len = old_results_len;
            }
            else { 
                /* matches < 0 - error during keyword search */
                freez(temp_msg.text_compressed);
                break;
            }
        }
        
        freez(temp_msg.text_compressed);
        
        if(p_query_params->results_buff->len >= p_query_params->quota){
            p_query_params->end_timestamp = temp_msg.timestamp;
            break;
        }

        rc = sqlite3_step(stmt_retrieve_log_msg_metadata);
        // debug(D_LOGS_MANAG, "Query: %s\n rc:%d\n", sqlite3_expanded_sql(stmt_retrieve_log_msg_metadata), rc);
        if (unlikely(rc != SQLITE_ROW && rc != SQLITE_DONE)) fatal_sqlite3_err(rc, __LINE__);
    }

    rc = sqlite3_exec(p_file_info->db, "END TRANSACTION;", NULL, NULL, NULL);
    if (unlikely(rc != SQLITE_OK)) fatal_sqlite3_err(rc, __LINE__);
    rc = sqlite3_finalize(stmt_retrieve_log_msg_metadata);
    if (unlikely(rc != SQLITE_OK)) fatal_sqlite3_err(rc, __LINE__);
}

#define TMP_VIEW_TABLE		   "compound_view"
#define TMP_VIEW_QUERY_PREFIX  "CREATE TEMP VIEW " TMP_VIEW_TABLE " AS SELECT * FROM (SELECT * FROM '0'."\
                                LOGS_TABLE " INNER JOIN (VALUES(0)) ORDER BY Timestamp) "
#define TMP_VIEW_QUERY_BODY_1  "UNION ALL SELECT * FROM (SELECT * FROM '"
#define TMP_VIEW_QUERY_BODY_2  "'." LOGS_TABLE " INNER JOIN (VALUES("
#define TMP_VIEW_QUERY_BODY_3  ")) ORDER BY Timestamp) "
#define TMP_VIEW_QUERY_POSTFIX "ORDER BY Timestamp;" 
void db_search_compound(logs_query_params_t *const p_query_params, struct File_info *const p_file_infos[]) {
    int rc = 0;
    int pfi_off = 0;

    sqlite3 *dbt;
    rc = sqlite3_open_v2(p_file_infos[0]->db_metadata, &dbt, SQLITE_OPEN_READONLY, NULL);
    if (unlikely(rc != SQLITE_OK)) fatal_sqlite3_err(rc, __LINE__);

    rc = sqlite3_exec(dbt, "BEGIN TRANSACTION;", NULL, NULL, NULL);
    if (unlikely(rc != SQLITE_OK)) fatal_sqlite3_err(rc, __LINE__);	

    /* Attach DBs */
    sqlite3_stmt *stmt_attach_db;
    rc = sqlite3_prepare_v2(dbt,"ATTACH DATABASE ?  AS ? ;", -1, &stmt_attach_db, NULL);
    if (unlikely(rc != SQLITE_OK)) fatal_sqlite3_err(rc, __LINE__);
    for(pfi_off = 0; p_file_infos[pfi_off]; pfi_off++){
        rc = sqlite3_bind_text(stmt_attach_db, 1, p_file_infos[pfi_off]->db_metadata, -1, NULL);
        // rc = sqlite3_bind_text(stmt_attach_db, 2, METADATA_DB_FILENAME, -1, NULL);
        if (unlikely(rc != SQLITE_OK)) fatal_sqlite3_err(rc, __LINE__);
        rc = sqlite3_bind_int(stmt_attach_db, 2, pfi_off);
        if (unlikely(rc != SQLITE_OK)) fatal_sqlite3_err(rc, __LINE__);
        // debug(D_LOGS_MANAG, "Attach db query[%d]:%s", pfi_off, sqlite3_expanded_sql(stmt_attach_db));
        rc = sqlite3_step(stmt_attach_db);
        if (unlikely(rc != SQLITE_DONE)) fatal_sqlite3_err(rc, __LINE__);
        rc = sqlite3_reset(stmt_attach_db);
        if (unlikely(rc != SQLITE_OK)) fatal_sqlite3_err(rc, __LINE__);
    }

    /* Create temporary view */
    char tmp_view_query[sizeof(TMP_VIEW_QUERY_PREFIX) + (
                            sizeof(TMP_VIEW_QUERY_BODY_1) + 
                             sizeof(TMP_VIEW_QUERY_BODY_2) + 
                             sizeof(TMP_VIEW_QUERY_BODY_3) + 4 ) * (MAX_COMPOUND_QUERY_SOURCES - 1) + 
                        sizeof(TMP_VIEW_QUERY_POSTFIX) + 50] = TMP_VIEW_QUERY_PREFIX; // +50 bytes - play it safe
    int n = sizeof(TMP_VIEW_QUERY_PREFIX) - 1;
    for(pfi_off = 1; p_file_infos[pfi_off]; pfi_off++){ // Skip p_file_infos[0]
        n += snprintf(&tmp_view_query[n], sizeof(tmp_view_query), "%s%d%s%d%s", 
                        TMP_VIEW_QUERY_BODY_1, pfi_off, 
                        TMP_VIEW_QUERY_BODY_2, pfi_off, TMP_VIEW_QUERY_BODY_3);
    }
    snprintf(&tmp_view_query[n], sizeof(tmp_view_query), "%s", TMP_VIEW_QUERY_POSTFIX);

    // debug(D_LOGS_MANAG, "tmp_view_query (string):%s", tmp_view_query);

    sqlite3_stmt *stmt_create_tmp_view;
    rc = sqlite3_prepare_v2(dbt, tmp_view_query, -1, &stmt_create_tmp_view, NULL);
    if (unlikely(rc != SQLITE_OK)) fatal_sqlite3_err(rc, __LINE__);

    // debug(D_LOGS_MANAG, "stmt_create_tmp_view:%s", sqlite3_expanded_sql(stmt_create_tmp_view));
    rc = sqlite3_step(stmt_create_tmp_view);
    if (unlikely(rc != SQLITE_DONE)) fatal_sqlite3_err(rc, __LINE__);


    /* Prepare retrieval of metadata from TMP_VIEW_TABLE statement */
    // TODO: Avoid preparing statement for each db_search call, but how if tied to specific db handle?
    // TODO: Limit number of results returned through SQLite Query to speed up search?
    sqlite3_stmt *stmt_retrieve_log_msg_metadata;
    rc = sqlite3_prepare_v2(dbt,
                            "SELECT Timestamp, Msg_compr_size , Msg_decompr_size, "
                            "BLOB_Offset, FK_BLOB_Id, column1 "
                            "FROM " TMP_VIEW_TABLE " "
                            "WHERE Timestamp BETWEEN ? AND ? ;",
                            -1, &stmt_retrieve_log_msg_metadata, NULL);
    if (unlikely(rc != SQLITE_OK)) fatal_sqlite3_err(rc, __LINE__);

    /* Execute search */
    rc = sqlite3_bind_int64(stmt_retrieve_log_msg_metadata, 1, (sqlite3_int64)p_query_params->start_timestamp);
    if (unlikely(rc != SQLITE_OK)) fatal_sqlite3_err(rc, __LINE__);
    rc = sqlite3_bind_int64(stmt_retrieve_log_msg_metadata, 2, (sqlite3_int64)p_query_params->end_timestamp);
    if (unlikely(rc != SQLITE_OK)) fatal_sqlite3_err(rc, __LINE__);

    rc = sqlite3_step(stmt_retrieve_log_msg_metadata);
    if (unlikely(rc != SQLITE_ROW && rc != SQLITE_DONE)) fatal_sqlite3_err(rc, __LINE__);
    // debug(D_LOGS_MANAG, "Query: %s\n rc:%d\n", sqlite3_expanded_sql(stmt_retrieve_log_msg_metadata), rc);
    while (rc == SQLITE_ROW) {
        Circ_buff_item_t temp_msg = {0};

        /* Retrieve metadata from DB */
        temp_msg.timestamp = (uint64_t)sqlite3_column_int64(stmt_retrieve_log_msg_metadata, 0);
        temp_msg.text_compressed_size = (size_t)sqlite3_column_int64(stmt_retrieve_log_msg_metadata, 1);
        temp_msg.text_size = (size_t)sqlite3_column_int64(stmt_retrieve_log_msg_metadata, 2);
        int64_t blob_offset = (int64_t) sqlite3_column_int64(stmt_retrieve_log_msg_metadata, 3);
        int blob_handles_offset = sqlite3_column_int(stmt_retrieve_log_msg_metadata, 4);
        int db_name = sqlite3_column_int(stmt_retrieve_log_msg_metadata, 5);

        /* Retrieve compressed log messages from BLOB file */
        temp_msg.text_compressed = mallocz(temp_msg.text_compressed_size);
        uv_buf_t uv_buf = uv_buf_init(temp_msg.text_compressed, temp_msg.text_compressed_size);
        uv_fs_t read_req;
        // TODO: Using db_loop here in separate thread (although synchronously) - thread-safe ?
        rc = uv_fs_read(db_loop, &read_req, p_file_infos[db_name]->blob_handles[blob_handles_offset], &uv_buf, 1, blob_offset, NULL);
        if (unlikely(rc < 0)) fatal_libuv_err(rc, __LINE__);
        uv_fs_req_cleanup(&read_req);

        /* Append retrieved results to BUFFER */
        size_t old_results_len = p_query_params->results_buff->len;
        buffer_sprintf(p_query_params->results_buff, "\t\t[ %" PRIu64 ", \"", temp_msg.timestamp);
        /* In case of search_keyword, less than item.text_size space is 
         * required, but go for worst case scenario for now */
        buffer_increase(p_query_params->results_buff, temp_msg.text_size);
        if(!p_query_params->keyword || !*p_query_params->keyword || !strcmp(p_query_params->keyword, " ")){
            // TODO: decompress_text does not handle or return any errors currently. How should any errors be handled?
            decompress_text(&temp_msg, &p_query_params->results_buff->buffer[p_query_params->results_buff->len]);
            p_query_params->results_buff->len += temp_msg.text_size;
            p_query_params->results_buff->len--; // get rid of '\0'
            // Watch out! Changing the next line will break web_client_api_request_v1_logsmanagement()! 
            buffer_strcat(p_query_params->results_buff, "\", \t0],\n");
        } 
        else {
            decompress_text(&temp_msg, NULL);
            size_t res_size = 0;
            const int matches = search_keyword(temp_msg.data, temp_msg.text_size, 
                                               &p_query_params->results_buff->buffer[p_query_params->results_buff->len], 
                                               &res_size, 
                                               p_query_params->keyword, NULL,
                                               p_query_params->ignore_case);
            freez(temp_msg.data);

            if(likely(matches > 0)) {
                m_assert(res_size > 0, "res_size can't be <= 0");
                p_query_params->results_buff->len += res_size;
                p_query_params->results_buff->len--; // get rid of '\0'
                // buffer_strcat(p_query_params->results_buff, "\"],\n");
                // Watch out! Changing the next line will break web_client_api_request_v1_logsmanagement()! 
                buffer_sprintf(p_query_params->results_buff, "\", \t%d],\n", matches);
                p_query_params->keyword_matches += matches;
            }
            else if(unlikely(matches == 0)){
                m_assert(res_size == 0, "res_size must be == 0");
                /* No keyword matches, undo timestamp buffer_sprintf() */
                p_query_params->results_buff->len = old_results_len;
            }
            else { 
                /* matches < 0 - error during keyword search */
                freez(temp_msg.text_compressed);
                break;
            }    
        }
        
        freez(temp_msg.text_compressed);
        
        if(p_query_params->results_buff->len >= p_query_params->quota){
            p_query_params->end_timestamp = temp_msg.timestamp;
            break;
        }

        rc = sqlite3_step(stmt_retrieve_log_msg_metadata);
        // debug(D_LOGS_MANAG, "Query: %s\n rc:%d\n", sqlite3_expanded_sql(stmt_retrieve_log_msg_metadata), rc);
        if (unlikely(rc != SQLITE_ROW && rc != SQLITE_DONE)) fatal_sqlite3_err(rc, __LINE__);
    }

    rc = sqlite3_finalize(stmt_attach_db);
    if (unlikely(rc != SQLITE_OK)) fatal_sqlite3_err(rc, __LINE__);
    rc = sqlite3_finalize(stmt_retrieve_log_msg_metadata);
    if (unlikely(rc != SQLITE_OK)) fatal_sqlite3_err(rc, __LINE__);
    rc = sqlite3_finalize(stmt_create_tmp_view);
    if (unlikely(rc != SQLITE_OK)) fatal_sqlite3_err(rc, __LINE__);
    
    rc = sqlite3_exec(dbt, "END TRANSACTION;", NULL, NULL, NULL);
    if (unlikely(rc != SQLITE_OK)) fatal_sqlite3_err(rc, __LINE__);

    rc = sqlite3_close(dbt);
    if (unlikely(rc != SQLITE_OK)) fatal_sqlite3_err(rc, __LINE__);
}
