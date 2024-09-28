// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_WINDOWS_PLUGIN_H
#define NETDATA_WINDOWS_PLUGIN_H

#include "daemon/common.h"

#define PLUGIN_WINDOWS_NAME "windows.plugin"

// https://learn.microsoft.com/es-es/windows/win32/sysinfo/kernel-objects?redirectedfrom=MSDN
// 2^24
#define WINDOWS_MAX_KERNEL_OBJECT 16777216

void *win_plugin_main(void *ptr);

extern char windows_shared_buffer[8192];

int do_GetSystemUptime(int update_every, usec_t dt);
int do_GetSystemRAM(int update_every, usec_t dt);
int do_GetSystemCPU(int update_every, usec_t dt);
int do_PerflibStorage(int update_every, usec_t dt);
int do_PerflibNetwork(int update_every, usec_t dt);
int do_PerflibProcesses(int update_every, usec_t dt);
int do_PerflibProcessor(int update_every, usec_t dt);
int do_PerflibMemory(int update_every, usec_t dt);
int do_PerflibObjects(int update_every, usec_t dt);
int do_PerflibThermalZone(int update_every, usec_t dt);
int do_PerflibWebService(int update_every, usec_t dt);
int do_PerflibMSSQL(int update_every, usec_t dt);

enum PERFLIB_PRIO {
    PRIO_WEBSITE_IIS_TRAFFIC = 21000, // PRIO selected, because APPS is using 20YYY
    PRIO_WEBSITE_IIS_FTP_FILE_TRANSFER_RATE,
    PRIO_WEBSITE_IIS_ACTIVE_CONNECTIONS_COUNT,
    PRIO_WEBSITE_IIS_REQUESTS_RATE,
    PRIO_WEBSITE_IIS_CONNECTIONS_ATTEMP,
    PRIO_WEBSITE_IIS_USERS,
    PRIO_WEBSITE_IIS_ISAPI_EXT_REQUEST_COUNT,
    PRIO_WEBSITE_IIS_ISAPI_EXT_REQUEST_RATE,
    PRIO_WEBSITE_IIS_ERRORS_RATE,
    PRIO_WEBSITE_IIS_LOGON_ATTEMPTS,
    PRIO_WEBSITE_IIS_UPTIME,

    PRIO_MSSQL_USER_CONNECTIONS,

    PRIO_MSSQL_DATABASE_DATA_FILE_SIZE,

    PRIO_MSSQL_STATS_BATCH_REQUEST,
    PRIO_MSSQL_STATS_COMPILATIONS,
    PRIO_MSSQL_STATS_RECOMPILATIONS,
    PRIO_MSSQL_STATS_AUTO_PARAMETRIZATION,
    PRIO_MSSQL_STATS_SAFE_AUTO_PARAMETRIZATION,

    PRIO_MSSQL_BLOCKED_PROCESSES,

    PRIO_MSSQL_BUFF_CACHE_HIT_RATIO,
    PRIO_MSSQL_BUFF_MAN_IOPS,
    PRIO_MSSQL_BUFF_CHECKPOINT_PAGES,
    PRIO_MSSQL_BUFF_METHODS_PAGE_SPLIT,
    PRIO_MSSQL_BUFF_PAGE_LIFE_EXPECTANCY,

    PRIO_MSSQL_MEMMGR_CONNECTION_MEMORY_BYTES,
    PRIO_MSSQL_MEMMGR_TOTAL_SERVER,
    PRIO_MSSQL_MEMMGR_EXTERNAL_BENEFIT_OF_MEMORY,
    PRIO_MSSQL_MEMMGR_PENDING_MEMORY_GRANTS,

    PRIO_MSSQL_LOCKS_WAIT,
    PRIO_MSSQL_LOCKS_DEADLOCK,

    PRIO_MSSQL_SQL_ERRORS
};

#endif //NETDATA_WINDOWS_PLUGIN_H

