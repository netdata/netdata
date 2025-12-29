// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows_plugin.h"
#include "windows-internals.h"

struct adcs_certificate {
    char *name;

    RRDSET *st_adcs_requests_total;
    RRDDIM *rd_adcs_requests_total;

    RRDSET *st_adcs_failed_requests_total;
    RRDDIM *rd_adcs_failed_requests_total;

    RRDSET *st_adcs_issued_requests_total;
    RRDDIM *rd_adcs_issued_requests_total;

    RRDSET *st_adcs_pending_requests_total;
    RRDDIM *rd_adcs_pending_requests_total;

    RRDSET *st_adcs_request_processing_time_seconds;
    RRDDIM *rd_adcs_request_processing_time_seconds;

    RRDSET *st_adcs_retrievals_total;
    RRDDIM *rd_adcs_retrievals_total;

    RRDSET *st_adcs_retrievals_processing_time_seconds;
    RRDDIM *rd_adcs_retrievals_processing_time_seconds;

    RRDSET *st_adcs_request_cryptographic_signing_time_seconds;
    RRDDIM *rd_adcs_request_cryptographic_signing_time_seconds;

    RRDSET *st_adcs_request_policy_module_processing_time_seconds;
    RRDDIM *rd_adcs_request_policy_module_processing_time_seconds;

    RRDSET *st_adcs_challenge_responses_total;
    RRDDIM *rd_adcs_challenge_responses_total;

    RRDSET *st_adcs_challenge_response_processing_time_seconds;
    RRDDIM *rd_adcs_challenge_response_processing_time_seconds;

    RRDSET *st_adcs_signed_certificate_timestamp_lists_total;
    RRDDIM *rd_adcs_signed_certificate_timestamp_lists_total;

    RRDSET *st_adcs_signed_certificate_timestamp_list_processing_time_seconds;
    RRDDIM *rd_adcs_signed_certificate_timestamp_list_processing_time_seconds;

    COUNTER_DATA ADCSRequestsTotal;
    COUNTER_DATA ADCSFailedRequestsTotal;
    COUNTER_DATA ADCSIssuedRequestsTotal;
    COUNTER_DATA ADCSPendingRequestsTotal;
    COUNTER_DATA ADCSRequestProcessingTime;
    COUNTER_DATA ADCSRetrievalsTotal;
    COUNTER_DATA ADCSRetrievalsProcessingTime;
    COUNTER_DATA ADCSRequestCryptoSigningTime;
    COUNTER_DATA ADCSRequestPolicyModuleProcessingTime;
    COUNTER_DATA ADCSChallengeResponseResponsesTotal;
    COUNTER_DATA ADCSChallengeResponseProcessingTime;
    COUNTER_DATA ADCSSignedCertTimestampListsTotal;
    COUNTER_DATA ADCSSignedCertTimestampListProcessingTime;
};

static DICTIONARY *adcs_certificates = NULL;

void dict_adcs_insert_certificate_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused)
{
    struct adcs_certificate *ptr = value;
    const char *name = dictionary_acquired_item_name((DICTIONARY_ITEM *)item);

    ptr->name = strdupz(name);

    ptr->ADCSRequestsTotal.key = "Requests/sec";
    ptr->ADCSFailedRequestsTotal.key = "Failed Requests/sec";
    ptr->ADCSIssuedRequestsTotal.key = "Issued Requests/sec";
    ptr->ADCSPendingRequestsTotal.key = "Pending Requests/sec";
    ptr->ADCSRequestProcessingTime.key = "Request processing time (ms)";
    ptr->ADCSRetrievalsTotal.key = "Retrievals/sec";
    ptr->ADCSRetrievalsProcessingTime.key = "Retrieval processing time (ms)";
    ptr->ADCSRequestCryptoSigningTime.key = "Request cryptographic signing time (ms)";
    ptr->ADCSRequestPolicyModuleProcessingTime.key = "Request policy module processing time (ms)";
    ptr->ADCSChallengeResponseResponsesTotal.key = "Challenge Responses/sec";
    ptr->ADCSChallengeResponseProcessingTime.key = "Challenge Response processing time (ms)";
    ptr->ADCSSignedCertTimestampListsTotal.key = "Signed Certificate Timestamp Lists/sec";
    ptr->ADCSSignedCertTimestampListProcessingTime.key = "Signed Certificate Timestamp List processing time (ms)";
}

static void initialize(void)
{
    adcs_certificates = dictionary_create_advanced(
        DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE, NULL, sizeof(struct adcs_certificate));

    dictionary_register_insert_callback(adcs_certificates, dict_adcs_insert_certificate_cb, NULL);
}

static void netdata_adcs_requests(
    struct adcs_certificate *ac,
    PERF_DATA_BLOCK *pDataBlock,
    PERF_OBJECT_TYPE *pObjectType,
    int update_every)
{
    char id[RRD_ID_LENGTH_MAX + 1];
    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &ac->ADCSRequestsTotal)) {
        return;
    }

    if (!ac->st_adcs_requests_total) {
        snprintfz(id, RRD_ID_LENGTH_MAX, "cert_%s_requests", ac->name);
        ac->st_adcs_requests_total = rrdset_create_localhost(
            "adcs",
            id,
            NULL,
            "requests",
            "adcs.cert_requests",
            "Certificate requests processed",
            "requests/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibADCS",
            PRIO_ADCS_CERT_REQUESTS_TOTAL,
            update_every,
            RRDSET_TYPE_LINE);

        ac->rd_adcs_requests_total =
            rrddim_add(ac->st_adcs_requests_total, "requests", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

        rrdlabels_add(ac->st_adcs_requests_total->rrdlabels, "cert", ac->name, RRDLABEL_SRC_AUTO);
    }

    rrddim_set_by_pointer(
        ac->st_adcs_requests_total, ac->rd_adcs_requests_total, (collected_number)ac->ADCSRequestsTotal.current.Data);
    rrdset_done(ac->st_adcs_requests_total);
}

static void netdata_adcs_requests_processing_time(
    struct adcs_certificate *ac,
    PERF_DATA_BLOCK *pDataBlock,
    PERF_OBJECT_TYPE *pObjectType,
    int update_every)
{
    char id[RRD_ID_LENGTH_MAX + 1];
    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &ac->ADCSRequestProcessingTime)) {
        return;
    }

    if (!ac->st_adcs_request_processing_time_seconds) {
        snprintfz(id, RRD_ID_LENGTH_MAX, "cert_%s_request_processing_time", ac->name);
        ac->st_adcs_request_processing_time_seconds = rrdset_create_localhost(
            "adcs",
            id,
            NULL,
            "requests",
            "adcs.cert_request_processing_time",
            "Certificate last request processing time",
            "seconds",
            PLUGIN_WINDOWS_NAME,
            "PerflibADCS",
            PRIO_ADCS_CERT_REQUESTS_PROCESSING_TIME,
            update_every,
            RRDSET_TYPE_LINE);

        ac->rd_adcs_request_processing_time_seconds = rrddim_add(
            ac->st_adcs_request_processing_time_seconds, "processing_time", NULL, 1, 1000, RRD_ALGORITHM_ABSOLUTE);

        rrdlabels_add(ac->st_adcs_request_processing_time_seconds->rrdlabels, "cert", ac->name, RRDLABEL_SRC_AUTO);
    }

    rrddim_set_by_pointer(
        ac->st_adcs_request_processing_time_seconds,
        ac->rd_adcs_request_processing_time_seconds,
        (collected_number)ac->ADCSRequestProcessingTime.current.Data);
    rrdset_done(ac->st_adcs_request_processing_time_seconds);
}

static void netdata_adcs_retrievals(
    struct adcs_certificate *ac,
    PERF_DATA_BLOCK *pDataBlock,
    PERF_OBJECT_TYPE *pObjectType,
    int update_every)
{
    char id[RRD_ID_LENGTH_MAX + 1];
    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &ac->ADCSRetrievalsTotal)) {
        return;
    }

    if (!ac->st_adcs_retrievals_total) {
        snprintfz(id, RRD_ID_LENGTH_MAX, "cert_%s_retrievals", ac->name);
        ac->st_adcs_retrievals_total = rrdset_create_localhost(
            "adcs",
            id,
            NULL,
            "retrievals",
            "adcs.cert_retrievals",
            "Total of certificate retrievals",
            "retrievals/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibADCS",
            PRIO_ADCS_CERT_RETRIVALS,
            update_every,
            RRDSET_TYPE_LINE);

        ac->rd_adcs_retrievals_total =
            rrddim_add(ac->st_adcs_retrievals_total, "retrievals", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

        rrdlabels_add(ac->st_adcs_retrievals_total->rrdlabels, "cert", ac->name, RRDLABEL_SRC_AUTO);
    }

    rrddim_set_by_pointer(
        ac->st_adcs_retrievals_total,
        ac->rd_adcs_retrievals_total,
        (collected_number)ac->ADCSRetrievalsTotal.current.Data);
    rrdset_done(ac->st_adcs_retrievals_total);
}

static void netdata_adcs_failed_requets(
    struct adcs_certificate *ac,
    PERF_DATA_BLOCK *pDataBlock,
    PERF_OBJECT_TYPE *pObjectType,
    int update_every)
{
    char id[RRD_ID_LENGTH_MAX + 1];
    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &ac->ADCSFailedRequestsTotal)) {
        return;
    }

    if (!ac->st_adcs_failed_requests_total) {
        snprintfz(id, RRD_ID_LENGTH_MAX, "cert_%s_failed_requests", ac->name);
        ac->st_adcs_failed_requests_total = rrdset_create_localhost(
            "adcs",
            id,
            NULL,
            "requests",
            "adcs.cert_failed_requests",
            "Certificate failed requests processed",
            "requests/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibADCS",
            PRIO_ADCS_CERT_FAILED_REQUESTS,
            update_every,
            RRDSET_TYPE_LINE);

        ac->rd_adcs_failed_requests_total =
            rrddim_add(ac->st_adcs_failed_requests_total, "failed", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

        rrdlabels_add(ac->st_adcs_failed_requests_total->rrdlabels, "cert", ac->name, RRDLABEL_SRC_AUTO);
    }

    rrddim_set_by_pointer(
        ac->st_adcs_failed_requests_total,
        ac->rd_adcs_failed_requests_total,
        (collected_number)ac->ADCSFailedRequestsTotal.current.Data);
    rrdset_done(ac->st_adcs_failed_requests_total);
}

static void netdata_adcs_issued_requets(
    struct adcs_certificate *ac,
    PERF_DATA_BLOCK *pDataBlock,
    PERF_OBJECT_TYPE *pObjectType,
    int update_every)
{
    char id[RRD_ID_LENGTH_MAX + 1];
    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &ac->ADCSIssuedRequestsTotal)) {
        return;
    }

    if (!ac->st_adcs_issued_requests_total) {
        snprintfz(id, RRD_ID_LENGTH_MAX, "cert_%s_issued_requests", ac->name);
        ac->st_adcs_issued_requests_total = rrdset_create_localhost(
            "adcs",
            id,
            NULL,
            "requests",
            "adcs.cert_issued_requests",
            "Certificate issued requests processed",
            "requests/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibADCS",
            PRIO_ADCS_CERT_ISSUED_REQUESTS,
            update_every,
            RRDSET_TYPE_LINE);

        ac->rd_adcs_issued_requests_total =
            rrddim_add(ac->st_adcs_issued_requests_total, "issued", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

        rrdlabels_add(ac->st_adcs_issued_requests_total->rrdlabels, "cert", ac->name, RRDLABEL_SRC_AUTO);
    }

    rrddim_set_by_pointer(
        ac->st_adcs_issued_requests_total,
        ac->rd_adcs_issued_requests_total,
        (collected_number)ac->ADCSIssuedRequestsTotal.current.Data);
    rrdset_done(ac->st_adcs_issued_requests_total);
}

static void netdata_adcs_pending_requets(
    struct adcs_certificate *ac,
    PERF_DATA_BLOCK *pDataBlock,
    PERF_OBJECT_TYPE *pObjectType,
    int update_every)
{
    char id[RRD_ID_LENGTH_MAX + 1];
    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &ac->ADCSPendingRequestsTotal)) {
        return;
    }

    if (!ac->st_adcs_pending_requests_total) {
        snprintfz(id, RRD_ID_LENGTH_MAX, "cert_%s_pending_requests", ac->name);
        ac->st_adcs_pending_requests_total = rrdset_create_localhost(
            "adcs",
            id,
            NULL,
            "requests",
            "adcs.cert_pending_requests",
            "Certificate pending requests processed",
            "requests/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibADCS",
            PRIO_ADCS_CERT_PENDING_REQUESTS,
            update_every,
            RRDSET_TYPE_LINE);

        ac->rd_adcs_pending_requests_total =
            rrddim_add(ac->st_adcs_pending_requests_total, "pending", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

        rrdlabels_add(ac->st_adcs_pending_requests_total->rrdlabels, "cert", ac->name, RRDLABEL_SRC_AUTO);
    }

    rrddim_set_by_pointer(
        ac->st_adcs_pending_requests_total,
        ac->rd_adcs_pending_requests_total,
        (collected_number)ac->ADCSPendingRequestsTotal.current.Data);
    rrdset_done(ac->st_adcs_pending_requests_total);
}

static void netdata_adcs_challenge_response(
    struct adcs_certificate *ac,
    PERF_DATA_BLOCK *pDataBlock,
    PERF_OBJECT_TYPE *pObjectType,
    int update_every)
{
    char id[RRD_ID_LENGTH_MAX + 1];
    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &ac->ADCSChallengeResponseResponsesTotal)) {
        return;
    }

    if (!ac->st_adcs_challenge_responses_total) {
        snprintfz(id, RRD_ID_LENGTH_MAX, "cert_%s_challenge_responses", ac->name);
        ac->st_adcs_challenge_responses_total = rrdset_create_localhost(
            "adcs",
            id,
            NULL,
            "responses",
            "adcs.cert_challenge_responses",
            "Certificate challenge responses",
            "responses/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibADCS",
            PRIO_ADCS_CERT_CHALLENGE_RESPONSES,
            update_every,
            RRDSET_TYPE_LINE);

        ac->rd_adcs_challenge_responses_total =
            rrddim_add(ac->st_adcs_challenge_responses_total, "challenge", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

        rrdlabels_add(ac->st_adcs_challenge_responses_total->rrdlabels, "cert", ac->name, RRDLABEL_SRC_AUTO);
    }

    rrddim_set_by_pointer(
        ac->st_adcs_challenge_responses_total,
        ac->rd_adcs_challenge_responses_total,
        (collected_number)ac->ADCSChallengeResponseResponsesTotal.current.Data);
    rrdset_done(ac->st_adcs_challenge_responses_total);
}

static void netdata_adcs_retrieval_processing(
    struct adcs_certificate *ac,
    PERF_DATA_BLOCK *pDataBlock,
    PERF_OBJECT_TYPE *pObjectType,
    int update_every)
{
    char id[RRD_ID_LENGTH_MAX + 1];
    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &ac->ADCSRetrievalsProcessingTime)) {
        return;
    }

    if (!ac->st_adcs_retrievals_processing_time_seconds) {
        snprintfz(id, RRD_ID_LENGTH_MAX, "cert_%s_retrievals_processing_time", ac->name);
        ac->st_adcs_retrievals_processing_time_seconds = rrdset_create_localhost(
            "adcs",
            id,
            NULL,
            "retrievals",
            "adcs.cert_retrieval_processing_time",
            "Certificate last retrieval processing time",
            "seconds",
            PLUGIN_WINDOWS_NAME,
            "PerflibADCS",
            PRIO_ADCS_CERT_RETRIEVAL_PROCESSING_TIME,
            update_every,
            RRDSET_TYPE_LINE);

        ac->rd_adcs_retrievals_processing_time_seconds = rrddim_add(
            ac->st_adcs_retrievals_processing_time_seconds,
            "processing_time",
            NULL,
            1,
            1000,
            RRD_ALGORITHM_ABSOLUTE);

        rrdlabels_add(
            ac->st_adcs_retrievals_processing_time_seconds->rrdlabels, "cert", ac->name, RRDLABEL_SRC_AUTO);
    }

    rrddim_set_by_pointer(
        ac->st_adcs_retrievals_processing_time_seconds,
        ac->rd_adcs_retrievals_processing_time_seconds,
        (collected_number)ac->ADCSRetrievalsProcessingTime.current.Data);
    rrdset_done(ac->st_adcs_retrievals_processing_time_seconds);
}

static void netdata_adcs_crypto_signing_time(
    struct adcs_certificate *ac,
    PERF_DATA_BLOCK *pDataBlock,
    PERF_OBJECT_TYPE *pObjectType,
    int update_every)
{
    char id[RRD_ID_LENGTH_MAX + 1];
    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &ac->ADCSRetrievalsProcessingTime)) {
        return;
    }

    if (!ac->st_adcs_request_cryptographic_signing_time_seconds) {
        snprintfz(id, RRD_ID_LENGTH_MAX, "cert_%s_request_cryptographic_signing_time", ac->name);
        ac->st_adcs_request_cryptographic_signing_time_seconds = rrdset_create_localhost(
            "adcs",
            id,
            NULL,
            "timings",
            "adcs.cert_request_cryptographic_signing_time",
            "Certificate last signing operation request time",
            "seconds",
            PLUGIN_WINDOWS_NAME,
            "PerflibADCS",
            PRIO_ADCS_CERT_REQ_CRYPTO_SIGNING_TIME,
            update_every,
            RRDSET_TYPE_LINE);

        ac->rd_adcs_request_cryptographic_signing_time_seconds = rrddim_add(
            ac->st_adcs_request_cryptographic_signing_time_seconds,
            "singing_time",
            NULL,
            1,
            1000,
            RRD_ALGORITHM_ABSOLUTE);

        rrdlabels_add(
            ac->st_adcs_request_cryptographic_signing_time_seconds->rrdlabels, "cert", ac->name, RRDLABEL_SRC_AUTO);
    }

    rrddim_set_by_pointer(
        ac->st_adcs_request_cryptographic_signing_time_seconds,
        ac->rd_adcs_request_cryptographic_signing_time_seconds,
        (collected_number)ac->ADCSRequestCryptoSigningTime.current.Data);
    rrdset_done(ac->st_adcs_request_cryptographic_signing_time_seconds);
}

static void netdata_adcs_policy_mod_processing_time(
    struct adcs_certificate *ac,
    PERF_DATA_BLOCK *pDataBlock,
    PERF_OBJECT_TYPE *pObjectType,
    int update_every)
{
    char id[RRD_ID_LENGTH_MAX + 1];
    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &ac->ADCSRequestPolicyModuleProcessingTime)) {
        return;
    }

    if (!ac->st_adcs_request_policy_module_processing_time_seconds) {
        snprintfz(id, RRD_ID_LENGTH_MAX, "cert_%s_request_policy_module_processing_time", ac->name);
        ac->st_adcs_request_policy_module_processing_time_seconds = rrdset_create_localhost(
            "adcs",
            id,
            NULL,
            "timings",
            "adcs.cert_request_policy_module_processing",
            "Certificate last policy module processing request time",
            "seconds",
            PLUGIN_WINDOWS_NAME,
            "PerflibADCS",
            PRIO_ADCS_CERT_REQ_POLICY_MODULE_PROCESS_TIME,
            update_every,
            RRDSET_TYPE_LINE);

        ac->rd_adcs_request_policy_module_processing_time_seconds = rrddim_add(
            ac->st_adcs_request_policy_module_processing_time_seconds,
            "processing_time",
            NULL,
            1,
            1000,
            RRD_ALGORITHM_ABSOLUTE);

        rrdlabels_add(
            ac->st_adcs_request_policy_module_processing_time_seconds->rrdlabels, "cert", ac->name, RRDLABEL_SRC_AUTO);
    }

    rrddim_set_by_pointer(
        ac->st_adcs_request_policy_module_processing_time_seconds,
        ac->rd_adcs_request_policy_module_processing_time_seconds,
        (collected_number)ac->ADCSRequestPolicyModuleProcessingTime.current.Data);
    rrdset_done(ac->st_adcs_request_policy_module_processing_time_seconds);
}

static void netdata_adcs_challenge_response_processing_time(
    struct adcs_certificate *ac,
    PERF_DATA_BLOCK *pDataBlock,
    PERF_OBJECT_TYPE *pObjectType,
    int update_every)
{
    char id[RRD_ID_LENGTH_MAX + 1];
    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &ac->ADCSChallengeResponseProcessingTime)) {
        return;
    }

    if (!ac->st_adcs_challenge_response_processing_time_seconds) {
        snprintfz(id, RRD_ID_LENGTH_MAX, "cert_%s_challenge_response_processing_time", ac->name);
        ac->st_adcs_challenge_response_processing_time_seconds = rrdset_create_localhost(
            "adcs",
            id,
            NULL,
            "timings",
            "adcs.cert_challenge_response_processing_time",
            "Certificate last challenge response time",
            "seconds",
            PLUGIN_WINDOWS_NAME,
            "PerflibADCS",
            PRIO_ADCS_CERT_CHALLENGE_RESP_PROCESSING_TIME,
            update_every,
            RRDSET_TYPE_LINE);

        ac->rd_adcs_challenge_response_processing_time_seconds = rrddim_add(
            ac->st_adcs_challenge_response_processing_time_seconds,
            "processing_time",
            NULL,
            1,
            1000,
            RRD_ALGORITHM_ABSOLUTE);

        rrdlabels_add(
            ac->st_adcs_challenge_response_processing_time_seconds->rrdlabels, "cert", ac->name, RRDLABEL_SRC_AUTO);
    }

    rrddim_set_by_pointer(
        ac->st_adcs_challenge_response_processing_time_seconds,
        ac->rd_adcs_challenge_response_processing_time_seconds,
        (collected_number)ac->ADCSChallengeResponseProcessingTime.current.Data);
    rrdset_done(ac->st_adcs_challenge_response_processing_time_seconds);
}

static void netdata_adcs_signed_certificate_timetamp_list(
    struct adcs_certificate *ac,
    PERF_DATA_BLOCK *pDataBlock,
    PERF_OBJECT_TYPE *pObjectType,
    int update_every)
{
    char id[RRD_ID_LENGTH_MAX + 1];
    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &ac->ADCSSignedCertTimestampListsTotal)) {
        return;
    }

    if (!ac->st_adcs_signed_certificate_timestamp_lists_total) {
        snprintfz(id, RRD_ID_LENGTH_MAX, "cert_%s_signed_certificate_timestamp_lists", ac->name);
        ac->st_adcs_signed_certificate_timestamp_lists_total = rrdset_create_localhost(
            "adcs",
            id,
            NULL,
            "timings",
            "adcs.cert_signed_certificate_timestamp_lists",
            "Certificate Signed Certificate Timestamp Lists processed",
            "lists/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibADCS",
            PRIO_ADCS_CERT_SIGNED_CERTIFICATE_TIMESTAMP_LIST,
            update_every,
            RRDSET_TYPE_LINE);

        ac->rd_adcs_signed_certificate_timestamp_lists_total = rrddim_add(
            ac->st_adcs_signed_certificate_timestamp_lists_total,
            "processing_time",
            NULL,
            1,
            1,
            RRD_ALGORITHM_INCREMENTAL);

        rrdlabels_add(
            ac->st_adcs_signed_certificate_timestamp_lists_total->rrdlabels, "cert", ac->name, RRDLABEL_SRC_AUTO);
    }

    rrddim_set_by_pointer(
        ac->st_adcs_signed_certificate_timestamp_lists_total,
        ac->rd_adcs_signed_certificate_timestamp_lists_total,
        (collected_number)ac->ADCSSignedCertTimestampListsTotal.current.Data);
    rrdset_done(ac->st_adcs_signed_certificate_timestamp_lists_total);
}

static void netdata_adcs_signed_certificate_timetamp_list_processing(
    struct adcs_certificate *ac,
    PERF_DATA_BLOCK *pDataBlock,
    PERF_OBJECT_TYPE *pObjectType,
    int update_every)
{
    char id[RRD_ID_LENGTH_MAX + 1];
    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &ac->ADCSSignedCertTimestampListProcessingTime)) {
        return;
    }

    if (!ac->st_adcs_signed_certificate_timestamp_list_processing_time_seconds) {
        snprintfz(id, RRD_ID_LENGTH_MAX, "cert_%s_signed_certificate_timestamp_list_processing_time", ac->name);
        ac->st_adcs_signed_certificate_timestamp_list_processing_time_seconds = rrdset_create_localhost(
            "adcs",
            id,
            NULL,
            "timings",
            "adcs.cert_signed_certificate_timestamp_list_processing_time",
            "Certificate last Signed Certificate Timestamp List process time",
            "seconds",
            PLUGIN_WINDOWS_NAME,
            "PerflibADCS",
            PRIO_ADCS_CERT_SIGNED_CERTIFICATE_TIMESTAMP_PROC_TIME_LIST,
            update_every,
            RRDSET_TYPE_LINE);

        ac->rd_adcs_signed_certificate_timestamp_list_processing_time_seconds = rrddim_add(
            ac->st_adcs_signed_certificate_timestamp_list_processing_time_seconds,
            "list",
            NULL,
            1,
            1000,
            RRD_ALGORITHM_ABSOLUTE);

        rrdlabels_add(
            ac->st_adcs_signed_certificate_timestamp_list_processing_time_seconds->rrdlabels,
            "cert",
            ac->name,
            RRDLABEL_SRC_AUTO);
    }

    rrddim_set_by_pointer(
        ac->st_adcs_signed_certificate_timestamp_list_processing_time_seconds,
        ac->rd_adcs_signed_certificate_timestamp_list_processing_time_seconds,
        (collected_number)ac->ADCSSignedCertTimestampListProcessingTime.current.Data);
    rrdset_done(ac->st_adcs_signed_certificate_timestamp_list_processing_time_seconds);
}

#define CERTIFICATION_AUTHORITY "Certification Authority"
static bool do_ADCS(PERF_DATA_BLOCK *pDataBlock, int update_every)
{
    PERF_OBJECT_TYPE *pObjectType = perflibFindObjectTypeByName(pDataBlock, CERTIFICATION_AUTHORITY);
    if (!pObjectType)
        return false;

    static void (*doADCS[])(struct adcs_certificate *, PERF_DATA_BLOCK *, PERF_OBJECT_TYPE *, int) = {
            netdata_adcs_requests,
            netdata_adcs_requests_processing_time,
            netdata_adcs_retrievals,
            netdata_adcs_failed_requets,
            netdata_adcs_issued_requets,
            netdata_adcs_pending_requets,
            netdata_adcs_challenge_response,
            netdata_adcs_retrieval_processing,
            netdata_adcs_crypto_signing_time,
            netdata_adcs_policy_mod_processing_time,
            netdata_adcs_challenge_response_processing_time,
            netdata_adcs_signed_certificate_timetamp_list,
            netdata_adcs_signed_certificate_timetamp_list_processing,

        // This must be the end
        NULL};

    PERF_INSTANCE_DEFINITION *pi = NULL;
    for (LONG i = 0; i < pObjectType->NumInstances; i++) {
        pi = perflibForEachInstance(pDataBlock, pObjectType, pi);
        if (!pi)
            break;

        if (!getInstanceName(pDataBlock, pObjectType, pi, windows_shared_buffer, sizeof(windows_shared_buffer)))
            strncpyz(windows_shared_buffer, "[unknown]", sizeof(windows_shared_buffer) - 1);

        if (strcasecmp(windows_shared_buffer, "_Total") == 0)
            continue;

        struct adcs_certificate *ptr = dictionary_set(adcs_certificates, windows_shared_buffer, NULL, sizeof(*ptr));

        for (int j = 0; doADCS[j] ;j++)
            doADCS[j](ptr, pDataBlock, pObjectType, update_every);
    }

    return true;
}

int do_PerflibADCS(int update_every, usec_t dt __maybe_unused)
{
    static bool initialized = false;

    if (unlikely(!initialized)) {
        initialize();
        initialized = true;
    }

    DWORD id = RegistryFindIDByName(CERTIFICATION_AUTHORITY);
    if (id == PERFLIB_REGISTRY_NAME_NOT_FOUND)
        return -1;

    PERF_DATA_BLOCK *pDataBlock = perflibGetPerformanceData(id);
    if (!pDataBlock)
        return -1;

    if (!do_ADCS(pDataBlock, update_every))
        return -1;

    return 0;
}
