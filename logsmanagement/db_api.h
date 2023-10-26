// SPDX-License-Identifier: GPL-3.0-or-later

/** @file db_api.h
 *  @brief Header of db_api.c
 */

#ifndef DB_API_H_
#define DB_API_H_

#include "../database/sqlite/sqlite3.h"
#include <uv.h>
#include "query.h"
#include "file_info.h"	

#define LOGS_MANAG_DB_SUBPATH "/logs_management_db"

int db_user_version(sqlite3 *const db, const int set_user_version);
void db_set_main_dir(char *const dir);
int db_init(void);
void db_search(logs_query_params_t *const p_query_params, struct File_info *const p_file_infos[]);

#endif  // DB_API_H_
