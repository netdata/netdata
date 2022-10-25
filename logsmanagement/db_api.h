/** @file db_api.h
 *  @brief Header of db_api.c
 *
 *  @author Dimitris Pantazis
 */

#ifndef DB_API_H_
#define DB_API_H_

#include "../database/sqlite/sqlite3.h"
#include <uv.h>
#include "query.h"
#include "file_info.h"	

#define LOGS_MANAG_DB_SUBPATH "/logs_management_db"

void db_set_lock(uv_mutex_t *db_mut);
void db_release_lock(uv_mutex_t *db_mut);
char *db_get_sqlite_version(void);
void db_set_main_dir(char *dir);
void db_init(void);
void db_search(logs_query_params_t *const p_query_params, struct File_info *const p_file_info);
void db_search_compound(logs_query_params_t *const p_query_params, struct File_info *const p_file_infos[]);

#endif  // DB_API_H_
