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

static void initialize(void) {
    adcs_certificates = dictionary_create_advanced(DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE,
                                                   NULL,
                                                   sizeof(struct adcs_certificate));

    dictionary_register_insert_callback(adcs_certificates, dict_adcs_insert_certificate_cb, NULL);
}

static bool do_ADCS(PERF_DATA_BLOCK *pDataBlock, int update_every) {
    PERF_OBJECT_TYPE *pObjectType = perflibFindObjectTypeByName(pDataBlock, "Certification Authority");
    if (!pObjectType)
        return false;

    PERF_INSTANCE_DEFINITION *pi = NULL;
    for(LONG i = 0; i < pObjectType->NumInstances ; i++) {
        pi = perflibForEachInstance(pDataBlock, pObjectType, pi);
        if (!pi) break;

        if (!getInstanceName(pDataBlock, pObjectType, pi, windows_shared_buffer, sizeof(windows_shared_buffer)))
            strncpyz(windows_shared_buffer, "[unknown]", sizeof(windows_shared_buffer) - 1);

        if(strcasecmp(windows_shared_buffer, "_Total") == 0)
            continue;

        struct adcs_certificate *ptr = dictionary_set(adcs_certificates,
                                                      windows_shared_buffer,
                                                      NULL,
                                                      sizeof(*ptr));
    }

    return true;
}

int do_PerflibADCS(int update_every, usec_t dt __maybe_unused) {
    static bool initialized = false;

    if(unlikely(!initialized)) {
        initialize();
        initialized = true;
    }

    DWORD id = RegistryFindIDByName("Certification Authority");
    if(id == PERFLIB_REGISTRY_NAME_NOT_FOUND)
        return -1;

    PERF_DATA_BLOCK *pDataBlock = perflibGetPerformanceData(id);
    if(!pDataBlock) return -1;

    do_ADCS(pDataBlock, update_every);

    return 0;
}
