// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_MSSQL_API_H
#define NETDATA_MSSQL_API_H

#include "database/rrd.h"

#define PLUGIN_WINDOWS_NAME "windows.plugin"

#include <sql.h>
#include <sqlext.h>

struct netdata_mssql_conn {
    const char *driver;
    const char *server;
    const char *address;
    const char *username;
    const char *password;
    bool windows_auth;
};

SQLHENV netdata_MSSQL_initialize_env();
SQLHDBC netdata_MSSQL_start_connection(SQLHENV hEnv, SQLCHAR *dbconnstr);
void netdata_MSSQL_cleanup_env(SQLHENV hEnv);
void netdata_MSSQL_close_connection(SQLHDBC netdataDBC);

#endif //NETDATA_MSSQL_API_H
