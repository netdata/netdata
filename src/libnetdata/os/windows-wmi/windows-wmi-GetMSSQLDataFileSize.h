// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_WINDOWS_WMI_GETMSSQLDATAFILESIZE_H
#define NETDATA_WINDOWS_WMI_GETMSSQLDATAFILESIZE_H

#include "windows-wmi.h"

#if defined(OS_WINDOWS)

// https://learn.microsoft.com/en-us/sql/relational-databases/databases/database-identifiers?view=sql-server-ver16
#define NETDATA_MSSQL_MAX_DB_NAME 128

extern DICTIONARY *DatabaseSize;

size_t GetSQLDataFileSizeWMI();

#endif

#endif //NETDATA_WINDOWS_WMI_GETMSSQLDATAFILESIZE_H
