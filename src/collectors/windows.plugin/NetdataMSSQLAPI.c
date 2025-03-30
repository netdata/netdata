// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows_plugin.h"
#include "windows-internals.h"

// Codes
// https://learn.microsoft.com/en-us/sql/connect/odbc/cpp-code-example-app-connect-access-sql-db?view=sql-server-ver16

#include <sql.h>
#include <sqlext.h>

static SQLHENV netdataEnv = NULL;
static SQLHDBC netdataDBC = NULL;

static void netdata_cleanup_MSSQL_variables() {
    if (netdataDBC) {
        SQLDisconnect(netdataDBC);
        SQLFreeHandle(SQL_HANDLE_DBC, netdataDBC);
    }

    SQLFreeHandle(SQL_HANDLE_ENV, netdataEnv);
    netdataEnv = NULL;
    netdataDBC = NULL;
}

void netdata_initialize_MSSQL_env()
{
    if (netdataEnv && netdataDBC)
        return;

    // Allocate an environment
    if (SQLAllocHandle(SQL_HANDLE_ENV, SQL_NULL_HANDLE, &netdataEnv) != SQL_SUCCESS) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "Unable to allocate an environment handle");
        goto endMSSQLEnv;
    }

    // Register application
    if (SQLSetEnvAttr(netdataEnv, SQL_ATTR_ODBC_VERSION, (SQLPOINTER)SQL_OV_ODBC3, 0) != SQL_SUCCESS) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "Cannot register MSSQL application");
        goto endMSSQLEnv;
    }

    return;
endMSSQLEnv:
    netdata_cleanup_MSSQL_variables();
}

int netdata_start_MSSQL_connection()
{
    if (!netdataEnv || !netdataDBC)
        return -1;

    // Allocate the connection
    if (SQLAllocHandle(SQL_HANDLE_DBC, netdataEnv, &netdataDBC) != SQL_SUCCESS) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "Cannot Allocate connection");
        goto endConnection;
    }

    return 0;
endConnection:
    netdata_cleanup_MSSQL_variables();

    return -1;
}
