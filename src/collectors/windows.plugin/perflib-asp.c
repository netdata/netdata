// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows_plugin.h"
#include "windows-internals.h"

struct asp_app {
    RRDSET *st_aspnet_anonymous_request;
    RRDSET *st_aspnet_compilations_total;
    RRDSET *st_aspnet_errors_during_preprocessing;
    RRDSET *st_aspnet_errors_during_compilation;
    RRDSET *st_aspnet_errors_during_execution;
    RRDSET *st_aspnet_errors_unhandled_execution_sec;
    RRDSET *st_aspnet_requests_byte_total;
    RRDSET *st_aspnet_requests_executing;
    RRDSET *st_aspnet_requests_failed;
    RRDSET *st_aspnet_requests_not_found;
    RRDSET *st_aspnet_requests_in_application_queue;
    RRDSET *st_aspnet_requests_not_authorized;
    RRDSET *st_aspnet_requests_timeout;
    RRDSET *st_aspnet_requests_succeeded;
    RRDSET *st_aspnet_requests_sec;
    RRDSET *st_aspnet_sessions_active;
    RRDSET *st_aspnet_sessions_abandoned;
    RRDSET *st_aspnet_sessions_timed_out;
    RRDSET *st_aspnet_transactions_aborted;
    RRDSET *st_aspnet_transactions_commited;
    RRDSET *st_aspnet_transactions_pending;
    RRDSET *st_aspnet_transactions_per_sec;
    RRDSET *st_aspnet_error_events_raised;
    RRDSET *st_aspnet_error_events_raised_per_sec;
    RRDSET *st_aspnet_request_error_events_raised_per_sec;
    RRDSET *st_aspnet_audit_failures_events_raised;
    RRDSET *st_aspnet_membership_authentication_success;
    RRDSET *st_aspnet_membership_authentication_failure;
    RRDSET *rd_aspnet_form_authentication_failure;

    RRDDIM *rd_aspnet_anonymous_request;
    RRDDIM *rd_aspnet_compilations_total;
    RRDDIM *rd_aspnet_errors_during_preprocessing;
    RRDDIM *rd_aspnet_errors_during_compilation;
    RRDDIM *rd_aspnet_errors_during_execution;
    RRDDIM *rd_aspnet_errors_unhandled_execution_sec;
    RRDDIM *rd_aspnet_requests_byte_total_in;
    RRDDIM *rd_aspnet_requests_byte_total_out;
    RRDDIM *rd_aspnet_requests_executing;
    RRDDIM *rd_aspnet_requests_failed;
    RRDDIM *rd_aspnet_requests_not_found;
    RRDDIM *rd_aspnet_requests_in_application_queue;
    RRDDIM *rd_aspnet_requests_not_authorized;
    RRDDIM *rd_aspnet_requests_timeout;
    RRDDIM *rd_aspnet_requests_successed;
    RRDDIM *rd_aspnet_requests_sec;
    RRDDIM *rd_aspnet_sessions_active;
    RRDDIM *rd_aspnet_sessions_abandoned;
    RRDDIM *rd_aspnet_sessions_timed_out;
    RRDDIM *rd_aspnet_transactions_aborted;
    RRDDIM *rd_aspnet_transactions_commited;
    RRDDIM *rd_aspnet_transactions_pending;
    RRDDIM *rd_aspnet_transactions_per_sec;
    RRDDIM *rd_aspnet_error_events_raised_per_sec;
    RRDDIM *rd_aspnet_request_error_events_raised_per_sec;
    RRDDIM *rd_aspnet_audit_failures_events_raised;
    RRDDIM *rd_aspnet_membership_authentication_success;
    RRDDIM *rd_aspnet_membership_authentication_failure;
    RRDDIM *rd_aspnet_form_authentication_failure;

    COUNTER_DATA aspnetAnonymousRequestPerSec;
    COUNTER_DATA aspnetCompilationsTotal;
    COUNTER_DATA aspnetErrorsDuringPreProcessing;
    COUNTER_DATA aspnetErrorsDuringCompilation;
    COUNTER_DATA aspnetErrorsDuringExecution;
    COUNTER_DATA aspnetErrorsUnhandledExecutionSec;
    COUNTER_DATA aspnetRequestsBytesTotalIn;
    COUNTER_DATA aspnetRequestsBytesTotalOut;
    COUNTER_DATA aspnetRequestsExecuting;
    COUNTER_DATA aspnetRequestsFailed;
    COUNTER_DATA aspnetRequestsNotFound;
    COUNTER_DATA aspnetRequestsInApplicationQueue;
    COUNTER_DATA aspnetRequestsNotAuthorized;
    COUNTER_DATA aspnetRequestsTimeout;
    COUNTER_DATA aspnetRequestsSucceeded;
    COUNTER_DATA aspnetSessionsActive;
    COUNTER_DATA aspnetSessionsAbandoned;
    COUNTER_DATA aspnetSessionsTimeOut;
    COUNTER_DATA aspnetTransactionsAborted;
    COUNTER_DATA aspnetTransactionsCommited;
    COUNTER_DATA aspnetTransactionsPending;
    COUNTER_DATA aspnetTransactionsPerSec;
    COUNTER_DATA aspnetErrorEventsRaisedPerSec;
    COUNTER_DATA aspnetRequestErrorEventsRaisedPerSec;
    COUNTER_DATA aspnetAuditFailuresEventsRaised;
    COUNTER_DATA aspnetMembershipAuthenticationSuccess;
    COUNTER_DATA aspnetMembershipAuthenticationFailure;
    COUNTER_DATA aspnetFormAuthenticationFailure;
};

DICTIONARY *asp_apps;

static void asp_app_initialize_variables(struct asp_app *ap)
{
    ap->aspnetAnonymousRequestPerSec.key = "Anonymous Requests/Sec";
    ap->aspnetCompilationsTotal.key = "Compilations Total";
    ap->aspnetErrorsDuringPreProcessing.key = "Errors During Preprocessing";
    ap->aspnetErrorsDuringCompilation.key = "Errors During Compilation";
    ap->aspnetErrorsDuringExecution.key = "Errors During Execution";
    ap->aspnetErrorsUnhandledExecutionSec.key = "Errors Unhandled During Execution/Sec";
    ap->aspnetRequestsBytesTotalIn.key = "Request Bytes In Total";
    ap->aspnetRequestsBytesTotalOut.key = "Request Bytes Out Total";
    ap->aspnetRequestsExecuting.key = "Requests Executing";
    ap->aspnetRequestsFailed.key = "Requests Failed";
    ap->aspnetRequestsNotFound.key = "Requests Not Found";
    ap->aspnetRequestsInApplicationQueue.key = "Requests In Application Queue";
    ap->aspnetRequestsNotAuthorized.key = "Requests Not Authorized";
    ap->aspnetRequestsTimeout.key = "Requests Timed Out";
    ap->aspnetRequestsSucceeded.key = "Requests Succeeded";
    ap->aspnetSessionsActive.key = "Sessions Active";
    ap->aspnetSessionsAbandoned.key = "Sessions Abandoned";
    ap->aspnetSessionsTimeOut.key = "Sessions Timed Out";
    ap->aspnetTransactionsAborted.key = "Transactions Aborted";
    ap->aspnetTransactionsCommited.key = "Transactions Committed";
    ap->aspnetTransactionsPending.key = "Transactions Pending";
    ap->aspnetTransactionsPerSec.key = "Transactions/Sec";
    ap->aspnetErrorEventsRaisedPerSec.key = "Events Raised/Sec";
    ap->aspnetRequestErrorEventsRaisedPerSec.key = "Error Events Raised/Sec";
    ap->aspnetAuditFailuresEventsRaised.key = "Audit Failure Events Raised";
    ap->aspnetMembershipAuthenticationSuccess.key = "Membership Authentication Success";
    ap->aspnetMembershipAuthenticationFailure.key = "Membership Authentication Failure";
    ap->aspnetFormAuthenticationFailure.key = "Forms Authentication Failure";
};

static void
dict_aspnet_insert_app_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused)
{
    struct asp_app *ap = value;

    asp_app_initialize_variables(ap);
}

static void initialize(void)
{
    asp_apps = dictionary_create_advanced(
        DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE, NULL, sizeof(struct asp_app));
    dictionary_register_insert_callback(asp_apps, dict_aspnet_insert_app_cb, NULL);
}

static void netdata_aspnet_application_restarts(COUNTER_DATA *value, int update_every)
{
    static RRDSET *st_aspnet_application_restarts = NULL;
    static RRDDIM *rd_aspnet_application_restarts = NULL;

    if (unlikely(!st_aspnet_application_restarts)) {
        st_aspnet_application_restarts = rrdset_create_localhost(
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

        rd_aspnet_application_restarts =
            rrddim_add(st_aspnet_application_restarts, "restarts", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    }

    rrddim_set_by_pointer(
        st_aspnet_application_restarts, rd_aspnet_application_restarts, (collected_number)value->current.Data);
    rrdset_done(st_aspnet_application_restarts);
}

static void netdata_ask_worker_process_restarts(COUNTER_DATA *value, int update_every)
{
    static RRDSET *st_aspnet_worker_process_restarts = NULL;
    static RRDDIM *rd_aspnet_worker_process_restarts = NULL;

    if (unlikely(!st_aspnet_worker_process_restarts)) {
        st_aspnet_worker_process_restarts = rrdset_create_localhost(
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

        rd_aspnet_worker_process_restarts =
            rrddim_add(st_aspnet_worker_process_restarts, "restarts", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    }

    rrddim_set_by_pointer(
        st_aspnet_worker_process_restarts, rd_aspnet_worker_process_restarts, (collected_number)value->current.Data);
    rrdset_done(st_aspnet_worker_process_restarts);
}

static void netdata_aspnet_global_objects(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    static COUNTER_DATA appApplicationRestarts = {.key = "Application Restarts"};
    static COUNTER_DATA appWorkerProcessRestarts = {.key = "Worker Process Restarts"};

    if (perflibGetObjectCounter(pDataBlock, pObjectType, &appApplicationRestarts))
        netdata_aspnet_application_restarts(&appApplicationRestarts, update_every);

    if (perflibGetObjectCounter(pDataBlock, pObjectType, &appWorkerProcessRestarts))
        netdata_ask_worker_process_restarts(&appWorkerProcessRestarts, update_every);
}

static void netdata_apps_anonymous_request(struct asp_app *aa, char *app, int update_every)
{
    if (unlikely(!aa->st_aspnet_anonymous_request)) {
        char id[RRD_ID_LENGTH_MAX + 1];
        snprintfz(id, RRD_ID_LENGTH_MAX, "aspnet_app_%s_anonymous_request", app);

        aa->st_aspnet_anonymous_request = rrdset_create_localhost(
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

        aa->rd_aspnet_anonymous_request =
            rrddim_add(aa->st_aspnet_anonymous_request, "request", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

        rrdlabels_add(aa->st_aspnet_anonymous_request->rrdlabels, "aspnet_app", app, RRDLABEL_SRC_AUTO);
    }

    rrddim_set_by_pointer(
        aa->st_aspnet_anonymous_request,
        aa->rd_aspnet_anonymous_request,
        (collected_number)aa->aspnetAnonymousRequestPerSec.current.Data);
    rrdset_done(aa->st_aspnet_anonymous_request);
}

static void netdata_apps_compilations_total(struct asp_app *aa, char *app, int update_every)
{
    if (unlikely(!aa->st_aspnet_compilations_total)) {
        char id[RRD_ID_LENGTH_MAX + 1];
        snprintfz(id, RRD_ID_LENGTH_MAX, "aspnet_app_%s_compilation_totals", app);

        aa->st_aspnet_compilations_total = rrdset_create_localhost(
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

        aa->rd_aspnet_compilations_total =
            rrddim_add(aa->st_aspnet_compilations_total, "compilation", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

        rrdlabels_add(aa->st_aspnet_compilations_total->rrdlabels, "aspnet_app", app, RRDLABEL_SRC_AUTO);
    }

    rrddim_set_by_pointer(
        aa->st_aspnet_compilations_total,
        aa->rd_aspnet_compilations_total,
        (collected_number)aa->aspnetCompilationsTotal.current.Data);
    rrdset_done(aa->st_aspnet_compilations_total);
}

static void netdata_apps_errors_during_preprocessing(struct asp_app *aa, char *app, int update_every)
{
    if (unlikely(!aa->st_aspnet_errors_during_preprocessing)) {
        char id[RRD_ID_LENGTH_MAX + 1];
        snprintfz(id, RRD_ID_LENGTH_MAX, "aspnet_app_%s_errors_during_preprocessing", app);

        aa->st_aspnet_errors_during_preprocessing = rrdset_create_localhost(
            "aspnet",
            id,
            NULL,
            "aspnet",
            "aspnet.errors_during_preprocessing",
            "Errors During Preprocessing.",
            "errors",
            PLUGIN_WINDOWS_NAME,
            "PerflibASP",
            PRIO_ASP_ERRORS_DURING_PREPROCESSING,
            update_every,
            RRDSET_TYPE_LINE);

        aa->rd_aspnet_errors_during_preprocessing =
            rrddim_add(aa->st_aspnet_errors_during_preprocessing, "preprocessing", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

        rrdlabels_add(aa->st_aspnet_errors_during_preprocessing->rrdlabels, "aspnet_app", app, RRDLABEL_SRC_AUTO);
    }

    rrddim_set_by_pointer(
        aa->st_aspnet_errors_during_preprocessing,
        aa->rd_aspnet_errors_during_preprocessing,
        (collected_number)aa->aspnetErrorsDuringPreProcessing.current.Data);
    rrdset_done(aa->st_aspnet_errors_during_preprocessing);
}

static void netdata_apps_errors_during_compilation(struct asp_app *aa, char *app, int update_every)
{
    if (unlikely(!aa->st_aspnet_errors_during_compilation)) {
        char id[RRD_ID_LENGTH_MAX + 1];
        snprintfz(id, RRD_ID_LENGTH_MAX, "aspnet_app_%s_errors_during_compilation", app);

        aa->st_aspnet_errors_during_compilation = rrdset_create_localhost(
            "aspnet",
            id,
            NULL,
            "aspnet",
            "aspnet.errors_during_compilation",
            "Errors During Compilation.",
            "errors",
            PLUGIN_WINDOWS_NAME,
            "PerflibASP",
            PRIO_ASP_ERRORS_DURING_COMPILATION,
            update_every,
            RRDSET_TYPE_LINE);

        aa->rd_aspnet_errors_during_compilation =
            rrddim_add(aa->st_aspnet_errors_during_compilation, "compilation", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

        rrdlabels_add(aa->st_aspnet_errors_during_compilation->rrdlabels, "aspnet_app", app, RRDLABEL_SRC_AUTO);
    }

    rrddim_set_by_pointer(
        aa->st_aspnet_errors_during_compilation,
        aa->rd_aspnet_errors_during_compilation,
        (collected_number)aa->aspnetErrorsDuringCompilation.current.Data);
    rrdset_done(aa->st_aspnet_errors_during_compilation);
}

static void netdata_apps_errors_during_execution(struct asp_app *aa, char *app, int update_every)
{
    if (unlikely(!aa->st_aspnet_errors_during_execution)) {
        char id[RRD_ID_LENGTH_MAX + 1];
        snprintfz(id, RRD_ID_LENGTH_MAX, "aspnet_app_%s_errors_during_execution", app);

        aa->st_aspnet_errors_during_execution = rrdset_create_localhost(
            "aspnet",
            id,
            NULL,
            "aspnet",
            "aspnet.errors_during_execution",
            "Errors During Execution.",
            "errors",
            PLUGIN_WINDOWS_NAME,
            "PerflibASP",
            PRIO_ASP_ERRORS_DURING_EXECUTION,
            update_every,
            RRDSET_TYPE_LINE);

        aa->rd_aspnet_errors_during_execution =
            rrddim_add(aa->st_aspnet_errors_during_execution, "execution", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

        rrdlabels_add(aa->st_aspnet_errors_during_execution->rrdlabels, "aspnet_app", app, RRDLABEL_SRC_AUTO);
    }

    rrddim_set_by_pointer(
        aa->st_aspnet_errors_during_execution,
        aa->rd_aspnet_errors_during_execution,
        (collected_number)aa->aspnetErrorsDuringExecution.current.Data);
    rrdset_done(aa->st_aspnet_errors_during_execution);
}

static void netdata_apps_errors_during_unhandled_execution(struct asp_app *aa, char *app, int update_every)
{
    if (unlikely(!aa->st_aspnet_errors_unhandled_execution_sec)) {
        char id[RRD_ID_LENGTH_MAX + 1];
        snprintfz(id, RRD_ID_LENGTH_MAX, "aspnet_app_%s_errors_during_unhandled_execution", app);

        aa->st_aspnet_errors_unhandled_execution_sec = rrdset_create_localhost(
            "aspnet",
            id,
            NULL,
            "aspnet",
            "aspnet.errors_during_unhandled_execution",
            "Errors Unhandled During Execution.",
            "errors",
            PLUGIN_WINDOWS_NAME,
            "PerflibASP",
            PRIO_ASP_ERRORS_UNHANDLED_EXECUTION_PER_SEC,
            update_every,
            RRDSET_TYPE_LINE);

        aa->rd_aspnet_errors_unhandled_execution_sec =
            rrddim_add(aa->st_aspnet_errors_unhandled_execution_sec, "unhandled", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

        rrdlabels_add(aa->st_aspnet_errors_unhandled_execution_sec->rrdlabels, "aspnet_app", app, RRDLABEL_SRC_AUTO);
    }

    rrddim_set_by_pointer(
        aa->st_aspnet_errors_unhandled_execution_sec,
        aa->rd_aspnet_errors_unhandled_execution_sec,
        (collected_number)aa->aspnetErrorsUnhandledExecutionSec.current.Data);
    rrdset_done(aa->st_aspnet_errors_unhandled_execution_sec);
}

static inline void netdata_aspnet_apps_runtime_errors(
    PERF_DATA_BLOCK *pDataBlock,
    PERF_OBJECT_TYPE *pObjectType,
    struct asp_app *aa,
    int update_every)
{
    if (perflibGetObjectCounter(pDataBlock, pObjectType, &aa->aspnetErrorsDuringPreProcessing))
        netdata_apps_errors_during_preprocessing(aa, windows_shared_buffer, update_every);

    if (perflibGetObjectCounter(pDataBlock, pObjectType, &aa->aspnetErrorsDuringCompilation))
        netdata_apps_errors_during_compilation(aa, windows_shared_buffer, update_every);

    if (perflibGetObjectCounter(pDataBlock, pObjectType, &aa->aspnetErrorsDuringExecution))
        netdata_apps_errors_during_execution(aa, windows_shared_buffer, update_every);

    if (perflibGetObjectCounter(pDataBlock, pObjectType, &aa->aspnetErrorsUnhandledExecutionSec))
        netdata_apps_errors_during_unhandled_execution(aa, windows_shared_buffer, update_every);
}

static void netdata_apps_requests_bytes(struct asp_app *aa, char *app, int update_every)
{
    if (unlikely(!aa->st_aspnet_requests_byte_total)) {
        char id[RRD_ID_LENGTH_MAX + 1];
        snprintfz(id, RRD_ID_LENGTH_MAX, "aspnet_app_%s_requests_byte_total", app);

        aa->st_aspnet_requests_byte_total = rrdset_create_localhost(
            "aspnet",
            id,
            NULL,
            "aspnet",
            "aspnet.requests_byte_total",
            "Size of responses and request.",
            "bytes",
            PLUGIN_WINDOWS_NAME,
            "PerflibASP",
            PRIO_ASP_REQUESTS_BYTES_TOTAL,
            update_every,
            RRDSET_TYPE_LINE);

        aa->rd_aspnet_requests_byte_total_in =
            rrddim_add(aa->st_aspnet_requests_byte_total, "in", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

        aa->rd_aspnet_requests_byte_total_out =
            rrddim_add(aa->st_aspnet_requests_byte_total, "out", NULL, -1, 1, RRD_ALGORITHM_ABSOLUTE);

        rrdlabels_add(aa->st_aspnet_requests_byte_total->rrdlabels, "aspnet_app", app, RRDLABEL_SRC_AUTO);
    }

    rrddim_set_by_pointer(
        aa->st_aspnet_requests_byte_total,
        aa->rd_aspnet_requests_byte_total_in,
        (collected_number)aa->aspnetRequestsBytesTotalIn.current.Data);

    rrddim_set_by_pointer(
        aa->st_aspnet_requests_byte_total,
        aa->rd_aspnet_requests_byte_total_out,
        (collected_number)aa->aspnetRequestsBytesTotalOut.current.Data);
    rrdset_done(aa->st_aspnet_requests_byte_total);
}

static void netdata_apps_requests_executing(struct asp_app *aa, char *app, int update_every)
{
    if (unlikely(!aa->st_aspnet_requests_executing)) {
        char id[RRD_ID_LENGTH_MAX + 1];
        snprintfz(id, RRD_ID_LENGTH_MAX, "aspnet_app_%s_requests_executing", app);

        aa->st_aspnet_requests_executing = rrdset_create_localhost(
            "aspnet",
            id,
            NULL,
            "aspnet",
            "aspnet.requests_executing",
            "The number of requests currently executing.",
            "requests",
            PLUGIN_WINDOWS_NAME,
            "PerflibASP",
            PRIO_ASP_REQUESTS_EXECUTING,
            update_every,
            RRDSET_TYPE_LINE);

        aa->rd_aspnet_requests_executing =
            rrddim_add(aa->st_aspnet_requests_executing, "executing", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

        rrdlabels_add(aa->st_aspnet_requests_executing->rrdlabels, "aspnet_app", app, RRDLABEL_SRC_AUTO);
    }

    rrddim_set_by_pointer(
        aa->st_aspnet_requests_executing,
        aa->rd_aspnet_requests_executing,
        (collected_number)aa->aspnetRequestsBytesTotalIn.current.Data);
    rrdset_done(aa->st_aspnet_requests_executing);
}

static void netdata_apps_requests_failed(struct asp_app *aa, char *app, int update_every)
{
    if (unlikely(!aa->st_aspnet_requests_failed)) {
        char id[RRD_ID_LENGTH_MAX + 1];
        snprintfz(id, RRD_ID_LENGTH_MAX, "aspnet_app_%s_requests_failed", app);

        aa->st_aspnet_requests_failed = rrdset_create_localhost(
            "aspnet",
            id,
            NULL,
            "aspnet",
            "aspnet.requests_failed",
            "Number of failed requests.",
            "requests",
            PLUGIN_WINDOWS_NAME,
            "PerflibASP",
            PRIO_ASP_REQUESTS_FAILED,
            update_every,
            RRDSET_TYPE_LINE);

        aa->rd_aspnet_requests_failed =
            rrddim_add(aa->st_aspnet_requests_failed, "failed", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

        rrdlabels_add(aa->st_aspnet_requests_failed->rrdlabels, "aspnet_app", app, RRDLABEL_SRC_AUTO);
    }

    rrddim_set_by_pointer(
        aa->st_aspnet_requests_failed,
        aa->rd_aspnet_requests_failed,
        (collected_number)aa->aspnetRequestsFailed.current.Data);
    rrdset_done(aa->st_aspnet_requests_failed);
}

static void netdata_apps_requests_not_found(struct asp_app *aa, char *app, int update_every)
{
    if (unlikely(!aa->st_aspnet_requests_not_found)) {
        char id[RRD_ID_LENGTH_MAX + 1];
        snprintfz(id, RRD_ID_LENGTH_MAX, "aspnet_app_%s_requests_not_found", app);

        aa->st_aspnet_requests_not_found = rrdset_create_localhost(
            "aspnet",
            id,
            NULL,
            "aspnet",
            "aspnet.requests_not_found",
            "Requests for resources that were not found.",
            "requests",
            PLUGIN_WINDOWS_NAME,
            "PerflibASP",
            PRIO_ASP_REQUESTS_NOT_FOUND,
            update_every,
            RRDSET_TYPE_LINE);

        aa->rd_aspnet_requests_not_found =
            rrddim_add(aa->st_aspnet_requests_not_found, "not found", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

        rrdlabels_add(aa->st_aspnet_requests_not_found->rrdlabels, "aspnet_app", app, RRDLABEL_SRC_AUTO);
    }

    rrddim_set_by_pointer(
        aa->st_aspnet_requests_not_found,
        aa->rd_aspnet_requests_not_found,
        (collected_number)aa->aspnetRequestsNotFound.current.Data);
    rrdset_done(aa->st_aspnet_requests_not_found);
}

static void netdata_apps_requests_in_application_queue(struct asp_app *aa, char *app, int update_every)
{
    if (unlikely(!aa->st_aspnet_requests_in_application_queue)) {
        char id[RRD_ID_LENGTH_MAX + 1];
        snprintfz(id, RRD_ID_LENGTH_MAX, "aspnet_app_%s_requests_in_application_queue", app);

        aa->st_aspnet_requests_in_application_queue = rrdset_create_localhost(
            "aspnet",
            id,
            NULL,
            "aspnet",
            "aspnet.requests_in_application_queue",
            "Requests in the application queue.",
            "requests",
            PLUGIN_WINDOWS_NAME,
            "PerflibASP",
            PRIO_ASP_REQUESTS_IN_APPLICATION_QUEUE,
            update_every,
            RRDSET_TYPE_LINE);

        aa->rd_aspnet_requests_in_application_queue =
            rrddim_add(aa->st_aspnet_requests_in_application_queue, "queue", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

        rrdlabels_add(aa->st_aspnet_requests_in_application_queue->rrdlabels, "aspnet_app", app, RRDLABEL_SRC_AUTO);
    }

    rrddim_set_by_pointer(
        aa->st_aspnet_requests_in_application_queue,
        aa->rd_aspnet_requests_in_application_queue,
        (collected_number)aa->aspnetRequestsInApplicationQueue.current.Data);
    rrdset_done(aa->st_aspnet_requests_in_application_queue);
}

static void netdata_apps_requests_not_authorized(struct asp_app *aa, char *app, int update_every)
{
    if (unlikely(!aa->st_aspnet_requests_not_authorized)) {
        char id[RRD_ID_LENGTH_MAX + 1];
        snprintfz(id, RRD_ID_LENGTH_MAX, "aspnet_app_%s_requests_not_authorized", app);

        aa->st_aspnet_requests_not_authorized = rrdset_create_localhost(
            "aspnet",
            id,
            NULL,
            "aspnet",
            "aspnet.requests_not_authorized",
            "Requests in the application queue.",
            "requests",
            PLUGIN_WINDOWS_NAME,
            "PerflibASP",
            PRIO_ASP_REQUESTS_NOT_AUTHORIZED,
            update_every,
            RRDSET_TYPE_LINE);

        aa->rd_aspnet_requests_not_authorized =
            rrddim_add(aa->st_aspnet_requests_not_authorized, "queue", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

        rrdlabels_add(aa->st_aspnet_requests_not_authorized->rrdlabels, "aspnet_app", app, RRDLABEL_SRC_AUTO);
    }

    rrddim_set_by_pointer(
        aa->st_aspnet_requests_not_authorized,
        aa->rd_aspnet_requests_not_authorized,
        (collected_number)aa->aspnetRequestsNotAuthorized.current.Data);
    rrdset_done(aa->st_aspnet_requests_not_authorized);
}

static void netdata_apps_requests_timeout(struct asp_app *aa, char *app, int update_every)
{
    if (unlikely(!aa->st_aspnet_requests_timeout)) {
        char id[RRD_ID_LENGTH_MAX + 1];
        snprintfz(id, RRD_ID_LENGTH_MAX, "aspnet_app_%s_requests_timeout", app);

        aa->st_aspnet_requests_timeout = rrdset_create_localhost(
            "aspnet",
            id,
            NULL,
            "aspnet",
            "aspnet.requests_timeout",
            "Requests that timed out.",
            "requests",
            PLUGIN_WINDOWS_NAME,
            "PerflibASP",
            PRIO_ASP_REQUESTS_TIMEOUT,
            update_every,
            RRDSET_TYPE_LINE);

        aa->rd_aspnet_requests_timeout =
            rrddim_add(aa->st_aspnet_requests_timeout, "timeout", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

        rrdlabels_add(aa->st_aspnet_requests_timeout->rrdlabels, "aspnet_app", app, RRDLABEL_SRC_AUTO);
    }

    rrddim_set_by_pointer(
        aa->st_aspnet_requests_timeout,
        aa->rd_aspnet_requests_timeout,
        (collected_number)aa->aspnetRequestsTimeout.current.Data);
    rrdset_done(aa->st_aspnet_requests_timeout);
}

static void netdata_apps_requests_succeeded(struct asp_app *aa, char *app, int update_every)
{
    if (unlikely(!aa->st_aspnet_requests_succeeded)) {
        char id[RRD_ID_LENGTH_MAX + 1];
        snprintfz(id, RRD_ID_LENGTH_MAX, "aspnet_app_%s_requests_succeeded", app);

        aa->st_aspnet_requests_succeeded = rrdset_create_localhost(
            "aspnet",
            id,
            NULL,
            "aspnet",
            "aspnet.requests_succeeded",
            "Requests that executed successfully.",
            "requests",
            PLUGIN_WINDOWS_NAME,
            "PerflibASP",
            PRIO_ASP_REQUESTS_SUCCEEDED,
            update_every,
            RRDSET_TYPE_LINE);

        aa->rd_aspnet_requests_succeeded =
            rrddim_add(aa->st_aspnet_requests_succeeded, "success", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

        rrdlabels_add(aa->st_aspnet_requests_succeeded->rrdlabels, "aspnet_app", app, RRDLABEL_SRC_AUTO);
    }

    rrddim_set_by_pointer(
        aa->st_aspnet_requests_succeeded,
        aa->rd_aspnet_requests_succeeded,
        (collected_number)aa->aspnetRequestsSucceeded.current.Data);
    rrdset_done(aa->st_aspnet_requests_succeeded);
}

static inline void netdata_aspnet_apps_requests(
    PERF_DATA_BLOCK *pDataBlock,
    PERF_OBJECT_TYPE *pObjectType,
    struct asp_app *aa,
    int update_every)
{
    if (perflibGetObjectCounter(pDataBlock, pObjectType, &aa->aspnetRequestsBytesTotalIn) &&
        perflibGetObjectCounter(pDataBlock, pObjectType, &aa->aspnetRequestsBytesTotalOut))
        netdata_apps_requests_bytes(aa, windows_shared_buffer, update_every);

    if (perflibGetObjectCounter(pDataBlock, pObjectType, &aa->aspnetRequestsExecuting))
        netdata_apps_requests_executing(aa, windows_shared_buffer, update_every);

    if (perflibGetObjectCounter(pDataBlock, pObjectType, &aa->aspnetRequestsFailed))
        netdata_apps_requests_failed(aa, windows_shared_buffer, update_every);

    if (perflibGetObjectCounter(pDataBlock, pObjectType, &aa->aspnetRequestsNotFound))
        netdata_apps_requests_not_found(aa, windows_shared_buffer, update_every);

    if (perflibGetObjectCounter(pDataBlock, pObjectType, &aa->aspnetRequestsInApplicationQueue))
        netdata_apps_requests_in_application_queue(aa, windows_shared_buffer, update_every);

    if (perflibGetObjectCounter(pDataBlock, pObjectType, &aa->aspnetRequestsNotAuthorized))
        netdata_apps_requests_not_authorized(aa, windows_shared_buffer, update_every);

    if (perflibGetObjectCounter(pDataBlock, pObjectType, &aa->aspnetRequestsTimeout))
        netdata_apps_requests_timeout(aa, windows_shared_buffer, update_every);

    if (perflibGetObjectCounter(pDataBlock, pObjectType, &aa->aspnetRequestsSucceeded))
        netdata_apps_requests_succeeded(aa, windows_shared_buffer, update_every);
}

static void netdata_aspnet_apps_objects(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
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

        if (perflibGetObjectCounter(pDataBlock, pObjectType, &aa->aspnetAnonymousRequestPerSec))
            netdata_apps_anonymous_request(aa, windows_shared_buffer, update_every);

        if (perflibGetObjectCounter(pDataBlock, pObjectType, &aa->aspnetCompilationsTotal))
            netdata_apps_compilations_total(aa, windows_shared_buffer, update_every);

        netdata_aspnet_apps_runtime_errors(pDataBlock, pObjectType, app, update_every);

        netdata_aspnet_apps_requests(pDataBlock, pObjectType, app, update_every);
    }
}

struct netdata_exchange_objects {
    void (*fnct)(PERF_DATA_BLOCK *, PERF_OBJECT_TYPE *, int);
    char *object;
} asp_obj[] = {
    {.fnct = netdata_aspnet_global_objects, .object = "ASP.NET"},
    {.fnct = netdata_aspnet_apps_objects, .object = "ASP.NET Applications"},

    // This is the end of the loop
    {.fnct = NULL, .object = NULL}};
}
;

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
