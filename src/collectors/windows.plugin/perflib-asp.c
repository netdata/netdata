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

static void asp_global_objects(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every) {
}

static void asp_apps_objects(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every) {
}

struct netdata_exchange_objects {
    void (*fnct)(PERF_DATA_BLOCK *, PERF_OBJECT_TYPE *, int);
    char *object;
} asp_obj[] = {
        {.fnct = asp_global_objects, .object = "ASP.NET"},
        {.fnct = asp_apps_objects, .object = "ASP.NET Applications"},

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
