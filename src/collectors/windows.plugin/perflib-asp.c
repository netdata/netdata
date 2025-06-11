// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows_plugin.h"
#include "windows-internals.h"

struct asp_app {
    RRDSET *st_asp_anonymous_request;
    RRDSET *st_asp_compilations_total;
    RRDSET *st_asp_errors_during_processing;
    RRDSET *st_asp_errors_during_compilation;
    RRDSET *st_asp_errors_during_execution;
    RRDSET *st_asp_errors_unhandled_execution_sec;
    RRDSET *st_asp_requests_byte_total_in;
    RRDSET *st_asp_requests_byte_total_out;
    RRDSET *st_asp_requests_executing;
    RRDSET *st_asp_requests_failed;
    RRDSET *st_asp_requests_not_found;
    RRDSET *st_asp_requests_not_authorized;
    RRDSET *st_asp_requests_timeout;
    RRDSET *st_asp_requests_successed;
    RRDSET *st_asp_requests_sec;
    RRDSET *st_asp_sessions_active;
    RRDSET *st_asp_sessions_abandoned;
    RRDSET *st_asp_sessions_timed_out;
    RRDSET *st_asp_transactions_aborted;
    RRDSET *st_asp_transactions_commited;
    RRDSET *st_asp_transactions_pending;
    RRDSET *st_asp_transactions_per_sec;
    RRDSET *st_asp_error_events_raised;
    RRDSET *st_asp_error_events_raised_per_sec;
    RRDSET *st_asp_request_error_events_raised_per_sec;
    RRDSET *st_asp_audit_failures_events_raised;
    RRDSET *st_asp_membership_authentication_success;
    RRDSET *st_asp_membership_authentication_failure;
    RRDSET *rd_asp_form_authentication_failure;

    RRDDIM *rd_asp_anonymous_request;
    RRDDIM *rd_asp_compilations_total;
    RRDDIM *rd_asp_errors_during_processing;
    RRDDIM *rd_asp_errors_during_compilation;
    RRDDIM *rd_asp_errors_during_execution;
    RRDDIM *rd_asp_errors_unhandled_execution_sec;
    RRDDIM *rd_asp_requests_byte_total_in;
    RRDDIM *rd_asp_requests_byte_total_out;
    RRDDIM *rd_asp_requests_executing;
    RRDDIM *rd_asp_requests_failed;
    RRDDIM *rd_asp_requests_not_found;
    RRDDIM *rd_asp_requests_not_authorized;
    RRDDIM *rd_asp_requests_timeout;
    RRDDIM *rd_asp_requests_successed;
    RRDDIM *rd_asp_requests_sec;
    RRDDIM *rd_asp_sessions_active;
    RRDDIM *rd_asp_sessions_abandoned;
    RRDDIM *rd_asp_sessions_timed_out;
    RRDDIM *rd_asp_transactions_aborted;
    RRDDIM *rd_asp_transactions_commited;
    RRDDIM *rd_asp_transactions_pending;
    RRDDIM *rd_asp_transactions_per_sec;
    RRDDIM *rd_asp_error_events_raised_per_sec;
    RRDDIM *rd_asp_request_error_events_raised_per_sec;
    RRDDIM *rd_asp_audit_failures_events_raised;
    RRDDIM *rd_asp_membership_authentication_success;
    RRDDIM *rd_asp_membership_authentication_failure;
    RRDDIM *rd_asp_form_authentication_failure;

    COUNTER_DATA aspAnonymousRequest;
    COUNTER_DATA aspCompilationsTotal;
    COUNTER_DATA aspErrorsDuringProcessing;
    COUNTER_DATA aspErrorsDuringCompilation;
    COUNTER_DATA aspErrorsDuringExecution;
    COUNTER_DATA aspErrorsUnhandledExecutionSec;
    COUNTER_DATA aspRequestsBytesTotalIn;
    COUNTER_DATA aspRequestsBytesTotalOut;
    COUNTER_DATA aspRequestsExecuting;
    COUNTER_DATA aspRequestsFailed;
    COUNTER_DATA aspRequestsNotFound;
    COUNTER_DATA aspRequestsNotAuthorized;
    COUNTER_DATA aspRequestsTimeout;
    COUNTER_DATA aspRequestsSuccessed;
    COUNTER_DATA aspRequestsPerSec;
    COUNTER_DATA aspSessionsActive;
    COUNTER_DATA aspSessionsAbandoned;
    COUNTER_DATA aspSessionsTimeOut;
    COUNTER_DATA aspTransactionsAborted;
    COUNTER_DATA aspTransactionsCommited;
    COUNTER_DATA aspTransactionsPending;
    COUNTER_DATA aspTransactionsPerSec;
    COUNTER_DATA aspErrorEventsRaisedPerSec;
    COUNTER_DATA aspRequestErrorEventsRaisedPerSec;
    COUNTER_DATA aspAuditFailuresEventsRaised;
    COUNTER_DATA aspMembershipAuthenticationSuccess;
    COUNTER_DATA aspMembershipAuthenticationFailure;
    COUNTER_DATA aspFormAuthenticationFailure;
};

DICTIONARY *asp_apps;

static void asp_app_initialize_variables(struct asp_app *ap) {
    ap->aspAnonymousRequest.key = "Anonymous Requests/Sec";
    ap->aspCompilationsTotal.key = "Compilations Total";
    ap->aspErrorsDuringProcessing.key = "Errors During Preprocessing";
    ap->aspErrorsDuringCompilation.key = "Errors During Compilation";
    ap->aspErrorsDuringExecution.key = "Errors During Execution";
    ap->aspErrorsUnhandledExecutionSec.key = "Errors Unhandled During Execution/Sec";
    ap->aspRequestsBytesTotalIn.key = "Request Bytes In Total";
    ap->aspRequestsBytesTotalOut.key = "Request Bytes Out Total";
    ap->aspRequestsExecuting.key = "Requests Executing";
    ap->aspRequestsFailed.key = "Requests Failed";
    ap->aspRequestsNotFound.key = "Requests Not Found";
    ap->aspRequestsNotAuthorized.key = "Requests Not Authorized";
    ap->aspRequestsTimeout.key = "Requests Timed Out";
    ap->aspRequestsSuccessed.key = "Requests Succeeded";
    ap->aspRequestsPerSec.key = "Requests/Sec";
    ap->aspSessionsActive.key = "Sessions Active";
    ap->aspSessionsAbandoned.key = "Sessions Abandoned";
    ap->aspSessionsTimeOut.key = "Sessions Timed Out";
    ap->aspTransactionsAborted.key = "Transactions Aborted";
    ap->aspTransactionsCommited.key = "Transactions Committed";
    ap->aspTransactionsPending.key = "Transactions Pending";
    ap->aspTransactionsPerSec.key = "Transactions/Sec";
    ap->aspErrorEventsRaisedPerSec.key = "Events Raised/Sec";
    ap->aspRequestErrorEventsRaisedPerSec.key = "Error Events Raised/Sec";
    ap->aspAuditFailuresEventsRaised.key = "Audit Failure Events Raised";
    ap->aspMembershipAuthenticationSuccess.key = "Membership Authentication Success";
    ap->aspMembershipAuthenticationFailure.key = "Membership Authentication Failure";
};
}

static void
dict_asp_insert_app_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused)
{
    struct asp_app *ap = value;

    asp_app_initialize_variables(ap);
}

static void initialize(void)
{
    asp_apps = dictionary_create_advanced(
            DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE, NULL, sizeof(struct asp_app));
    dictionary_register_insert_callback(asp_apps, dict_asp_insert_app_cb, NULL);
}

static void netdata_asp_application_restarts(COUNTER_DATA *value, int update_every)
{
    static RRDSET *st_asp_application_restarts = NULL;
    static RRDDIM *rd_asp_application_restarts = NULL;

    if (unlikely(!st_asp_application_restarts)) {
        st_asp_application_restarts = rrdset_create_localhost(
                "aspnet",
                "application_restarts",
                NULL,
                "aspnet",
                "aspnet.application_restarts",
                "Number of times the application has been restarted.",
                "restarts",
                PLUGIN_WINDOWS_NAME,
                "PerflibASP",
                PRIO_ASP_APPLICATION_RESTART,
                update_every,
                RRDSET_TYPE_LINE);

        rd_asp_application_restarts = rrddim_add(st_asp_application_restarts, "restarts", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    }

    rrddim_set_by_pointer(st_asp_application_restarts, rd_asp_application_restarts, (collected_number)value->current.Data);
    rrdset_done(st_asp_application_restarts);
}

static void netdata_ask_worker_process_restarts(COUNTER_DATA *value, int update_every)
{
    static RRDSET *st_asp_worker_process_restarts = NULL;
    static RRDDIM *rd_asp_worker_process_restarts = NULL;

    if (unlikely(!st_asp_worker_process_restarts)) {
        st_asp_worker_process_restarts = rrdset_create_localhost(
                "aspnet",
                "worker_process_restarts",
                NULL,
                "aspnet",
                "aspnet.worker_process_restarts",
                "Number of times a worker process has restarted on the machine.",
                "restarts",
                PLUGIN_WINDOWS_NAME,
                "PerflibASP",
                PRIO_ASP_WORKER_PROCESS_RESTART,
                update_every,
                RRDSET_TYPE_LINE);

        rd_asp_worker_process_restarts = rrddim_add(st_asp_worker_process_restarts, "restarts", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    }

    rrddim_set_by_pointer(st_asp_worker_process_restarts, rd_asp_worker_process_restarts, (collected_number)value->current.Data);
    rrdset_done(st_asp_worker_process_restarts);
}

static void netdata_asp_global_objects(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every) {
    static COUNTER_DATA appApplicationRestarts = {.key = "Application Restarts"};
    static COUNTER_DATA appWorkerProcessRestarts = {.key = "Worker Process Restarts"};

    if (perflibGetObjectCounter(pDataBlock, pObjectType, &appApplicationRestarts))
        netdata_asp_application_restarts(&appApplicationRestarts, update_every);

    if (perflibGetObjectCounter(pDataBlock, pObjectType, &appWorkerProcessRestarts))
        netdata_ask_worker_process_restarts(&appWorkerProcessRestarts, update_every);
}

static void netdata_apps_anonymous_request(struct asp_app *aa, char *app, int update_every)
{
    if (unlikely(!aa->st_asp_anonymous_request)) {
        char id[RRD_ID_LENGTH_MAX + 1];
        snprintfz(id, RRD_ID_LENGTH_MAX, "aspnet_app_%s_anonymous_request", app);

        aa->st_asp_anonymous_request = rrdset_create_localhost(
                "aspnet",
                id,
                NULL,
                "aspnet",
                "aspnet.anonymous_request",
                "Number of requests utilizing anonymous authentication.",
                "requests",
                PLUGIN_WINDOWS_NAME,
                "PerflibASP",
                PRIO_ASP_APP_ANONYMOUS_REQUEST,
                update_every,
                RRDSET_TYPE_LINE);

        aa->rd_asp_anonymous_request =
                rrddim_add(aa->st_asp_anonymous_request, "request", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

        rrdlabels_add(aa->st_asp_anonymous_request->rrdlabels, "aspnet_app", app, RRDLABEL_SRC_AUTO);
    }

    rrddim_set_by_pointer(
            aa->st_asp_anonymous_request,
            aa->rd_asp_anonymous_request,
            (collected_number)aa->aspAnonymousRequest.current.Data);
    rrdset_done(aa->st_asp_anonymous_request);
}

static void netdata_apps_compilations_total(struct asp_app *aa, char *app, int update_every) {
    if (unlikely(!aa->st_asp_compilations_total)) {
        char id[RRD_ID_LENGTH_MAX + 1];
        snprintfz(id, RRD_ID_LENGTH_MAX, "aspnet_app_%s_compilation_totals", app);

        aa->st_asp_compilations_total = rrdset_create_localhost(
                "aspnet",
                id,
                NULL,
                "aspnet",
                "aspnet.compilation_totals",
                "Number of source files dynamically compiled.",
                "compilations",
                PLUGIN_WINDOWS_NAME,
                "PerflibASP",
                PRIO_ASP_APP_COMPILATION_TOTALS,
                update_every,
                RRDSET_TYPE_LINE);

        aa->rd_asp_compilations_total =
                rrddim_add(aa->st_asp_compilations_total, "compilation", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

        rrdlabels_add(aa->st_asp_compilations_total->rrdlabels, "aspnet_app", app, RRDLABEL_SRC_AUTO);
    }

    rrddim_set_by_pointer(
            aa->st_asp_compilations_total,
            aa->rd_asp_compilations_total,
            (collected_number) aa->aspCompilationsTotal.current.Data);
    rrdset_done(aa->st_asp_compilations_total);
}

static void netdata_asp_apps_objects(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every) {
    PERF_INSTANCE_DEFINITION *pi = NULL;
    for (LONG i = 0; i < pObjectType->NumInstances; i++) {
        pi = perflibForEachInstance(pDataBlock, pObjectType, pi);
        if (!pi)
            break;

        if (!getInstanceName(pDataBlock, pObjectType, pi, windows_shared_buffer, sizeof(windows_shared_buffer)))
            strncpyz(windows_shared_buffer, "[unknown]", sizeof(windows_shared_buffer) - 1);

        if (strcasecmp(windows_shared_buffer, "_Total") == 0)
            continue;

        struct asp_app *aa = dictionary_set(asp_apps, windows_shared_buffer, NULL, sizeof(*aa));
        if (!aa)
            continue;

        if (perflibGetObjectCounter(pDataBlock, pObjectType, &aa->aspAnonymousRequest))
            netdata_apps_anonymous_request(aa, windows_shared_buffer, update_every);

        if (perflibGetObjectCounter(pDataBlock, pObjectType, &aa->aspCompilationsTotal))
            netdata_apps_compilations_total(aa, windows_shared_buffer, update_every);
    }
}

struct netdata_exchange_objects {
    void (*fnct)(PERF_DATA_BLOCK *, PERF_OBJECT_TYPE *, int);
    char *object;
} asp_obj[] = {
        {.fnct = netdata_asp_global_objects, .object = "ASP.NET"},
        {.fnct = netdata_asp_apps_objects, .object = "ASP.NET Applications"},

        // This is the end of the loop
        {.fnct = NULL, .object = NULL}};
};

int do_PerflibASP(int update_every, usec_t dt __maybe_unused)
{
    static bool initialized = false;

    if (unlikely(!initialized)) {
        initialize();
        initialized = true;
    }

    for (LONG i = 0; asp_obj[i].fnct; i++) {
        DWORD id = RegistryFindIDByName(asp_obj[i].object);
        if (id == PERFLIB_REGISTRY_NAME_NOT_FOUND)
            continue;

        PERF_DATA_BLOCK *pDataBlock = perflibGetPerformanceData(id);
        if (!pDataBlock)
            continue;

        PERF_OBJECT_TYPE *pObjectType = perflibFindObjectTypeByName(pDataBlock, asp_obj[i].object);
        if (!pObjectType)
            continue;

        asp_obj[i].fnct(pDataBlock, pObjectType, update_every);
    }

    return 0;
}
