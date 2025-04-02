// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows_plugin.h"
#include "windows-internals.h"

// Codes
// https://learn.microsoft.com/en-us/sql/connect/odbc/cpp-code-example-app-connect-access-sql-db?view=sql-server-ver16

void netdata_MSSQL_cleanup_env(SQLHENV hEnv)
{
    if (hEnv) {
        SQLFreeHandle(SQL_HANDLE_ENV, hEnv);
    }
}

SQLHENV netdata_MSSQL_initialize_env()
{
    SQLHENV hEnv = SQL_NULL_HENV;
    // Allocate an environment
    SQLRETURN ret = SQLAllocHandle(SQL_HANDLE_ENV, SQL_NULL_HANDLE, &hEnv);
    if (ret == SQL_SUCCESS || ret == SQL_SUCCESS_WITH_INFO) {
        // Register application
        ret = SQLSetEnvAttr(hEnv, SQL_ATTR_ODBC_VERSION, (SQLPOINTER)SQL_OV_ODBC3, 0);
        if (ret == SQL_SUCCESS || ret == SQL_SUCCESS_WITH_INFO) {
            return hEnv;
        }
    } else {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "Unable to allocate MSSQL environment handle. Error %d", ret);
        return SQL_NULL_HENV;
    }

    nd_log(NDLS_COLLECTORS, NDLP_ERR, "Cannot register MSSQL application. Error %d", ret);
    return SQL_NULL_HENV;
}

SQLHDBC netdata_MSSQL_start_connection(SQLHENV hEnv, char *dbconnstr)
{
#define NETDATA_MSSQL_MAX_CONNECTION_TRY (5)
    static int limit = 0;
    if (!hEnv || limit >= NETDATA_MSSQL_MAX_CONNECTION_TRY)
        return SQL_NULL_HDBC;

    // Allocate the connection
    SQLHDBC netdataDBC = SQL_NULL_HDBC;
    SQLRETURN ret = SQLAllocHandle(SQL_HANDLE_DBC, hEnv, &netdataDBC);
    if (ret == SQL_SUCCESS || ret == SQL_SUCCESS_WITH_INFO) {
        ret = SQLDriverConnect(hEnv, NULL, dbconnstr, SQL_NTS, NULL, 0, NULL, SQL_DRIVER_NOPROMPT);
        if (ret == SQL_SUCCESS || ret == SQL_SUCCESS_WITH_INFO) {
            limit = 0;
            return netdataDBC;
        }
    } else {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "Cannot allocate MSSQL connection. Error %d", ret);
        return SQL_NULL_HDBC;
    }

    limit++;
    nd_log(NDLS_COLLECTORS, NDLP_ERR, "Cannot connect to MSSQL server (Try %d/%d. Error %d", limit, NETDATA_MSSQL_MAX_CONNECTION_TRY, ret);

    return SQL_NULL_HDBC;
}

void netdata_MSSQL_close_connection(SQLHDBC hDBC)
{
    // TODO: Add keep alive message and option
    if (hDBC != SQL_NULL_HDBC) {
 //       SQLDisconnect(netdataDBC);
        SQLFreeHandle(SQL_HANDLE_DBC, hDBC);
    }
}
