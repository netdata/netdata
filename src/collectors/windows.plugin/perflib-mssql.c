// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows_plugin.h"
#include "windows-internals.h"

#include "perflib-mssql-queries.h"

void do_mssql_access_methods(PERF_DATA_BLOCK *pDataBlock, struct mssql_instance *mi, int update_every)
{
    char id[RRD_ID_LENGTH_MAX + 1];

    PERF_OBJECT_TYPE *pObjectType =
            perflibFindObjectTypeByName(pDataBlock, mi->objectName[NETDATA_MSSQL_ACCESS_METHODS]);
    if (unlikely(!pObjectType))
        return;

    if (likely(perflibGetObjectCounter(pDataBlock, pObjectType, &mi->MSSQLAccessMethodPageSplits))) {
        if (unlikely(!mi->st_access_method_page_splits)) {
            snprintfz(id, RRD_ID_LENGTH_MAX, "instance_%s_accessmethods_page_splits", mi->instanceID);
            netdata_fix_chart_name(id);
            mi->st_access_method_page_splits = rrdset_create_localhost(
                    "mssql",
                    id,
                    NULL,
                    "buffer cache",
                    "mssql.instance_accessmethods_page_splits",
                    "Page splits",
                    "splits/s",
                    PLUGIN_WINDOWS_NAME,
                    "PerflibMSSQL",
                    PRIO_MSSQL_BUFF_METHODS_PAGE_SPLIT,
                    update_every,
                    RRDSET_TYPE_LINE);

            mi->rd_access_method_page_splits =
                    rrddim_add(mi->st_access_method_page_splits, "page", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

            rrdlabels_add(
                    mi->st_access_method_page_splits->rrdlabels, "mssql_instance", mi->instanceID, RRDLABEL_SRC_AUTO);
        }

        rrddim_set_by_pointer(
                mi->st_access_method_page_splits,
                mi->rd_access_method_page_splits,
                (collected_number)mi->MSSQLAccessMethodPageSplits.current.Data);
        rrdset_done(mi->st_access_method_page_splits);
    }
}

void do_mssql_auto_parameterization_attempts(struct mssql_instance *mi, int update_every)
{
    if (unlikely(!mi->st_stats_auto_param)) {
        char id[RRD_ID_LENGTH_MAX + 1];
        snprintfz(id, RRD_ID_LENGTH_MAX, "instance_%s_sqlstats_auto_parameterization_attempts", mi->instanceID);
        netdata_fix_chart_name(id);
        mi->st_stats_auto_param = rrdset_create_localhost(
                "mssql",
                id,
                NULL,
                "sql activity",
                "mssql.instance_sqlstats_auto_parameterization_attempts",
                "Failed auto-parameterization attempts",
                "attempts/s",
                PLUGIN_WINDOWS_NAME,
                "PerflibMSSQL",
                PRIO_MSSQL_STATS_AUTO_PARAMETRIZATION,
                update_every,
                RRDSET_TYPE_LINE);

        mi->rd_stats_auto_param =
                rrddim_add(mi->st_stats_auto_param, "failed", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

        rrdlabels_add(mi->st_stats_auto_param->rrdlabels, "mssql_instance", mi->instanceID, RRDLABEL_SRC_AUTO);
    }

    rrddim_set_by_pointer(
            mi->st_stats_auto_param,
            mi->rd_stats_auto_param,
            (collected_number)mi->MSSQLStatsAutoParameterization.current.Data);
    rrdset_done(mi->st_stats_auto_param);
}

void do_mssql_batch_requests(struct mssql_instance *mi, int update_every)
{
    if (unlikely(!mi->st_stats_batch_request)) {
        char id[RRD_ID_LENGTH_MAX + 1];
        snprintfz(id, RRD_ID_LENGTH_MAX, "instance_%s_sqlstats_batch_requests", mi->instanceID);
        netdata_fix_chart_name(id);
        mi->st_stats_batch_request = rrdset_create_localhost(
                "mssql",
                id,
                NULL,
                "sql activity",
                "mssql.instance_sqlstats_batch_requests",
                "Total of batches requests",
                "requests/s",
                PLUGIN_WINDOWS_NAME,
                "PerflibMSSQL",
                PRIO_MSSQL_STATS_BATCH_REQUEST,
                update_every,
                RRDSET_TYPE_LINE);

        mi->rd_stats_batch_request =
                rrddim_add(mi->st_stats_batch_request, "batch", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

        rrdlabels_add(mi->st_stats_batch_request->rrdlabels, "mssql_instance", mi->instanceID, RRDLABEL_SRC_AUTO);
    }

    rrddim_set_by_pointer(
            mi->st_stats_batch_request,
            mi->rd_stats_batch_request,
            (collected_number)mi->MSSQLStatsBatchRequests.current.Data);
    rrdset_done(mi->st_stats_batch_request);
}

void do_mssql_safe_auto_parameterization_attempts(struct mssql_instance *mi, int update_every)
{
    if (unlikely(!mi->st_stats_safe_auto)) {
        char id[RRD_ID_LENGTH_MAX + 1];
        snprintfz(
                id, RRD_ID_LENGTH_MAX, "instance_%s_sqlstats_safe_auto_parameterization_attempts", mi->instanceID);
        netdata_fix_chart_name(id);
        mi->st_stats_safe_auto = rrdset_create_localhost(
                "mssql",
                id,
                NULL,
                "sql activity",
                "mssql.instance_sqlstats_safe_auto_parameterization_attempts",
                "Safe auto-parameterization attempts",
                "attempts/s",
                PLUGIN_WINDOWS_NAME,
                "PerflibMSSQL",
                PRIO_MSSQL_STATS_SAFE_AUTO_PARAMETRIZATION,
                update_every,
                RRDSET_TYPE_LINE);

        mi->rd_stats_safe_auto = rrddim_add(mi->st_stats_safe_auto, "safe", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

        rrdlabels_add(mi->st_stats_safe_auto->rrdlabels, "mssql_instance", mi->instanceID, RRDLABEL_SRC_AUTO);
    }

    rrddim_set_by_pointer(
            mi->st_stats_safe_auto,
            mi->rd_stats_safe_auto,
            (collected_number)mi->MSSQLStatSafeAutoParameterization.current.Data);
    rrdset_done(mi->st_stats_safe_auto);
}

void do_mssql_statistics_perflib(PERF_DATA_BLOCK *pDataBlock, struct mssql_instance *mi, int update_every)
{
    PERF_OBJECT_TYPE *pObjectType = perflibFindObjectTypeByName(pDataBlock, mi->objectName[NETDATA_MSSQL_SQL_STATS]);
    if (unlikely(!pObjectType))
        return;

    if (likely(perflibGetObjectCounter(pDataBlock, pObjectType, &mi->MSSQLStatsAutoParameterization)))
        do_mssql_auto_parameterization_attempts(mi, update_every);

    if (likely(perflibGetObjectCounter(pDataBlock, pObjectType, &mi->MSSQLStatsBatchRequests)))
        do_mssql_batch_requests(mi, update_every);

    if (likely(perflibGetObjectCounter(pDataBlock, pObjectType, &mi->MSSQLStatSafeAutoParameterization)))
        do_mssql_safe_auto_parameterization_attempts(mi, update_every);
}

void do_mssql_memmgr_connection_memory_bytes(struct mssql_instance *mi, int update_every)
{
    if (unlikely(!mi->st_conn_memory)) {
        char id[RRD_ID_LENGTH_MAX + 1];
        snprintfz(id, RRD_ID_LENGTH_MAX, "instance_%s_memmgr_connection_memory_bytes", mi->instanceID);
        netdata_fix_chart_name(id);
        mi->st_conn_memory = rrdset_create_localhost(
                "mssql",
                id,
                NULL,
                "memory",
                "mssql.instance_memmgr_connection_memory_bytes",
                "Amount of dynamic memory to maintain connections",
                "bytes",
                PLUGIN_WINDOWS_NAME,
                "PerflibMSSQL",
                PRIO_MSSQL_MEMMGR_CONNECTION_MEMORY_BYTES,
                update_every,
                RRDSET_TYPE_LINE);

        mi->rd_conn_memory = rrddim_add(mi->st_conn_memory, "memory", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

        rrdlabels_add(mi->st_conn_memory->rrdlabels, "mssql_instance", mi->instanceID, RRDLABEL_SRC_AUTO);
    }

    rrddim_set_by_pointer(
            mi->st_conn_memory,
            mi->rd_conn_memory,
            (collected_number)(mi->MSSQLConnectionMemoryBytes.current.Data * 1024));
    rrdset_done(mi->st_conn_memory);
}

void do_mssql_memmgr_external_benefit_of_memory(struct mssql_instance *mi, int update_every)
{
    if (unlikely(!mi->st_ext_benefit_mem)) {
        char id[RRD_ID_LENGTH_MAX + 1];
        snprintfz(id, RRD_ID_LENGTH_MAX, "instance_%s_memmgr_external_benefit_of_memory", mi->instanceID);
        netdata_fix_chart_name(id);
        mi->st_ext_benefit_mem = rrdset_create_localhost(
                "mssql",
                id,
                NULL,
                "memory",
                "mssql.instance_memmgr_external_benefit_of_memory",
                "Performance benefit from adding memory to a specific cache",
                "bytes",
                PLUGIN_WINDOWS_NAME,
                "PerflibMSSQL",
                PRIO_MSSQL_MEMMGR_EXTERNAL_BENEFIT_OF_MEMORY,
                update_every,
                RRDSET_TYPE_LINE);

        mi->rd_ext_benefit_mem = rrddim_add(mi->st_ext_benefit_mem, "benefit", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

        rrdlabels_add(mi->st_ext_benefit_mem->rrdlabels, "mssql_instance", mi->instanceID, RRDLABEL_SRC_AUTO);
    }

    rrddim_set_by_pointer(
            mi->st_ext_benefit_mem,
            mi->rd_ext_benefit_mem,
            (collected_number)mi->MSSQLExternalBenefitOfMemory.current.Data);
    rrdset_done(mi->st_ext_benefit_mem);
}

void do_mssql_memmgr_pending_memory_grants(struct mssql_instance *mi, int update_every)
{
    if (unlikely(!mi->st_pending_mem_grant)) {
        char id[RRD_ID_LENGTH_MAX + 1];
        snprintfz(id, RRD_ID_LENGTH_MAX, "instance_%s_memmgr_pending_memory_grants", mi->instanceID);
        netdata_fix_chart_name(id);
        mi->st_pending_mem_grant = rrdset_create_localhost(
                "mssql",
                id,
                NULL,
                "memory",
                "mssql.instance_memmgr_pending_memory_grants",
                "Process waiting for memory grant",
                "processes",
                PLUGIN_WINDOWS_NAME,
                "PerflibMSSQL",
                PRIO_MSSQL_MEMMGR_PENDING_MEMORY_GRANTS,
                update_every,
                RRDSET_TYPE_LINE);

        mi->rd_pending_mem_grant =
                rrddim_add(mi->st_pending_mem_grant, "pending", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

        rrdlabels_add(mi->st_pending_mem_grant->rrdlabels, "mssql_instance", mi->instanceID, RRDLABEL_SRC_AUTO);
    }

    rrddim_set_by_pointer(
            mi->st_pending_mem_grant,
            mi->rd_pending_mem_grant,
            (collected_number)mi->MSSQLPendingMemoryGrants.current.Data);

    rrdset_done(mi->st_pending_mem_grant);
}

void do_mssql_memmgr_memmgr_server_memory(struct mssql_instance *mi, int update_every)
{
    if (unlikely(!mi->st_mem_tot_server)) {
        char id[RRD_ID_LENGTH_MAX + 1];
        snprintfz(id, RRD_ID_LENGTH_MAX, "instance_%s_memmgr_server_memory", mi->instanceID);
        netdata_fix_chart_name(id);
        mi->st_mem_tot_server = rrdset_create_localhost(
                "mssql",
                id,
                NULL,
                "memory",
                "mssql.instance_memmgr_server_memory",
                "Memory committed",
                "bytes",
                PLUGIN_WINDOWS_NAME,
                "PerflibMSSQL",
                PRIO_MSSQL_MEMMGR_TOTAL_SERVER,
                update_every,
                RRDSET_TYPE_LINE);

        mi->rd_mem_tot_server = rrddim_add(mi->st_mem_tot_server, "memory", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

        rrdlabels_add(mi->st_mem_tot_server->rrdlabels, "mssql_instance", mi->instanceID, RRDLABEL_SRC_AUTO);
    }

    rrddim_set_by_pointer(
            mi->st_mem_tot_server,
            mi->rd_mem_tot_server,
            (collected_number)(mi->MSSQLTotalServerMemory.current.Data * 1024));

    rrdset_done(mi->st_mem_tot_server);
}

void do_mssql_memory_mgr(PERF_DATA_BLOCK *pDataBlock, struct mssql_instance *mi, int update_every)
{
    char id[RRD_ID_LENGTH_MAX + 1];

    PERF_OBJECT_TYPE *pObjectType = perflibFindObjectTypeByName(pDataBlock, mi->objectName[NETDATA_MSSQL_MEMORY]);
    if (unlikely(!pObjectType))
        return;

    if (likely(perflibGetObjectCounter(pDataBlock, pObjectType, &mi->MSSQLConnectionMemoryBytes)))
        do_mssql_memmgr_connection_memory_bytes(mi, update_every);

    if (likely(perflibGetObjectCounter(pDataBlock, pObjectType, &mi->MSSQLExternalBenefitOfMemory))) {
        do_mssql_memmgr_external_benefit_of_memory(mi, update_every);
    }

    if (likely(perflibGetObjectCounter(pDataBlock, pObjectType, &mi->MSSQLPendingMemoryGrants))) {
        do_mssql_memmgr_pending_memory_grants(mi, update_every);
    }

    if (likely(perflibGetObjectCounter(pDataBlock, pObjectType, &mi->MSSQLTotalServerMemory))) {
        do_mssql_memmgr_memmgr_server_memory(mi, update_every);
    }
}

void do_mssql_errors(PERF_DATA_BLOCK *pDataBlock, struct mssql_instance *mi, int update_every)
{
    PERF_OBJECT_TYPE *pObjectType = perflibFindObjectTypeByName(pDataBlock, mi->objectName[NETDATA_MSSQL_SQL_ERRORS]);
    if (unlikely(!pObjectType))
        return;

    if (likely(perflibGetObjectCounter(pDataBlock, pObjectType, &mi->MSSQLSQLErrorsTotal))) {
        if (unlikely(!mi->st_sql_errors)) {
            char id[RRD_ID_LENGTH_MAX + 1];
            snprintfz(id, RRD_ID_LENGTH_MAX, "instance_%s_sql_errors_total", mi->instanceID);
            netdata_fix_chart_name(id);
            mi->st_sql_errors = rrdset_create_localhost(
                    "mssql",
                    id,
                    NULL,
                    "errors",
                    "mssql.instance_sql_errors",
                    "Errors",
                    "errors/s",
                    PLUGIN_WINDOWS_NAME,
                    "PerflibMSSQL",
                    PRIO_MSSQL_SQL_ERRORS,
                    update_every,
                    RRDSET_TYPE_LINE);

            mi->rd_sql_errors = rrddim_add(mi->st_sql_errors, "errors", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

            rrdlabels_add(mi->st_sql_errors->rrdlabels, "mssql_instance", mi->instanceID, RRDLABEL_SRC_AUTO);
        }

        rrddim_set_by_pointer(
                mi->st_sql_errors, mi->rd_sql_errors, (collected_number)mi->MSSQLAccessMethodPageSplits.current.Data);
        rrdset_done(mi->st_sql_errors);
    }
}

void do_mssql_user_connections(struct mssql_instance *mi, int update_every)
{
    if (unlikely(!mi->st_user_connections)) {
        char id[RRD_ID_LENGTH_MAX + 1];
        snprintfz(id, RRD_ID_LENGTH_MAX, "instance_%s_user_connections", mi->instanceID);
        netdata_fix_chart_name(id);
        mi->st_user_connections = rrdset_create_localhost(
                "mssql",
                id,
                NULL,
                "connections",
                "mssql.instance_user_connections",
                "User connections",
                "connections",
                PLUGIN_WINDOWS_NAME,
                "PerflibMSSQL",
                PRIO_MSSQL_USER_CONNECTIONS,
                update_every,
                RRDSET_TYPE_LINE);

        mi->rd_user_connections = rrddim_add(mi->st_user_connections, "user", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

        rrdlabels_add(mi->st_user_connections->rrdlabels, "mssql_instance", mi->instanceID, RRDLABEL_SRC_AUTO);
    }

    rrddim_set_by_pointer(
            mi->st_user_connections, mi->rd_user_connections, (collected_number)mi->MSSQLUserConnections.current.Data);
    rrdset_done(mi->st_user_connections);
}

void do_mssql_blocked_processes(struct mssql_instance *mi, int update_every)
{
    if (unlikely(!mi->st_process_blocked)) {
        char id[RRD_ID_LENGTH_MAX + 1];
        snprintfz(id, RRD_ID_LENGTH_MAX, "instance_%s_blocked_process", mi->instanceID);
        netdata_fix_chart_name(id);
        mi->st_process_blocked = rrdset_create_localhost(
                "mssql",
                id,
                NULL,
                "processes",
                "mssql.instance_blocked_processes",
                "Blocked processes",
                "process",
                PLUGIN_WINDOWS_NAME,
                "PerflibMSSQL",
                PRIO_MSSQL_BLOCKED_PROCESSES,
                update_every,
                RRDSET_TYPE_LINE);

        mi->rd_process_blocked = rrddim_add(mi->st_process_blocked, "blocked", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

        rrdlabels_add(mi->st_process_blocked->rrdlabels, "mssql_instance", mi->instanceID, RRDLABEL_SRC_AUTO);
    }

    rrddim_set_by_pointer(
            mi->st_process_blocked, mi->rd_process_blocked, (collected_number)mi->MSSQLBlockedProcesses.current.Data);
    rrdset_done(mi->st_process_blocked);
}

void do_mssql_general_stats(PERF_DATA_BLOCK *pDataBlock, struct mssql_instance *mi, int update_every)
{
    PERF_OBJECT_TYPE *pObjectType =
            perflibFindObjectTypeByName(pDataBlock, mi->objectName[NETDATA_MSSQL_GENERAL_STATS]);
    if (unlikely(!pObjectType))
        return;

    if (unlikely(!mi->conn) || unlikely(!mi->conn->collect_user_connections)) {
        if (likely(perflibGetObjectCounter(pDataBlock, pObjectType, &mi->MSSQLUserConnections))) {
            do_mssql_user_connections(mi, update_every);
        }
    }

    if (likely(perflibGetObjectCounter(pDataBlock, pObjectType, &mi->MSSQLBlockedProcesses))) {
        do_mssql_blocked_processes(mi, update_every);
    }
}
