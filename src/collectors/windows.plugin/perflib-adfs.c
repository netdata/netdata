// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows_plugin.h"
#include "windows-internals.h"

struct adfs_certificate {
    bool charts_created;

    RRDSET *st_adfs_login_connection_failures;
    RRDDIM *rd_adfs_login_connection_failures;

    RRDSET *st_adfs_certificate_authentications_total;
    RRDDIM *rd_adfs_certificate_authentications_total;

    RRDSET *st_adfs_db_artifact_failure_total;
    RRDDIM *rd_adfs_db_artifact_failure_total;

    RRDSET *st_adfs_db_artifact_query_time_seconds_total;
    RRDDIM *rd_adfs_db_artifact_query_time_seconds_total;

    RRDSET *st_adfs_device_authentications_total;
    RRDDIM *rd_adfs_device_authentications_total;

    RRDSET *st_adfs_db_config_failures;
    RRDDIM *rd_adfs_db_config_failures;

    RRDSET *st_adfs_db_config_query_time_seconds_total;
    RRDDIM *rd_adfs_db_config_query_time_seconds_total;

    RRDSET *st_adfs_external_authentications;
    RRDDIM *rd_adfs_external_authentications_success;
    RRDDIM *rd_adfs_external_authentications_failure;

    RRDSET *st_adfs_federated_authentications;
    RRDDIM *rd_adfs_federated_authentications;

    RRDSET *st_adfs_federation_metadata_requests_total;
    RRDDIM *rd_adfs_federation_metadata_requests_total;

    RRDSET *st_adfs_oauth_authorization_requests_total;
    RRDDIM *rd_adfs_oauth_authorization_requests_total;

    RRDSET *st_adfs_oauth_client_authentications;
    RRDDIM *rd_adfs_oauth_client_authentications_success;
    RRDDIM *rd_adfs_oauth_client_authentications_failure;

    RRDSET *st_adfs_oauth_client_credentials_requests;
    RRDDIM *rd_adfs_oauth_client_credentials_requests_success;
    RRDDIM *rd_adfs_oauth_client_credentials_requests_failure;

    RRDSET *st_adfs_oauth_client_privkey_jwt_authentications;
    RRDDIM *rd_adfs_oauth_client_privkey_jwt_authentications_success;
    RRDDIM *rd_adfs_oauth_client_privkey_jwt_authentications_failure;

    RRDSET *st_adfs_oauth_client_secret_basic_authentications;
    RRDDIM *rd_adfs_oauth_client_secret_basic_authentications_success;
    RRDDIM *rd_adfs_oauth_client_secret_basic_authentications_failure;

    RRDSET *st_adfs_oauth_client_secret_post_authentications;
    RRDDIM *rd_adfs_oauth_client_secret_post_authentications_success;
    RRDDIM *rd_adfs_oauth_client_secret_post_authentications_failure;

    RRDSET *st_adfs_oauth_password_grant_requests;
    RRDDIM *rd_adfs_oauth_password_grant_requests_success;
    RRDDIM *rd_adfs_oauth_password_grant_requests_failure;

    RRDSET *st_adfs_passive_requests_total;
    RRDDIM *rd_adfs_passive_requests_total;

    RRDSET *st_adfs_passport_authentications_total;
    RRDDIM *rd_adfs_passport_authentications_total;

    RRDSET *st_adfs_password_change_requests;
    RRDDIM *rd_adfs_password_change_requests_success;
    RRDDIM *rd_adfs_password_change_requests_failure;

    RRDSET *st_adfs_samlp_token_requests_success_total;
    RRDDIM *rd_adfs_samlp_token_requests_success_total;

    RRDSET *st_adfs_wstrust_token_requests_success_total;
    RRDDIM *rd_adfs_wstrust_token_requests_success_total;

    RRDSET *st_adfs_sso_authentications;
    RRDDIM *rd_adfs_sso_authentications_success;
    RRDDIM *rd_adfs_sso_authentications_failure;

    RRDSET *st_adfs_token_requests_total;
    RRDDIM *rd_adfs_token_requests_total;

    RRDSET *st_adfs_sso_authentications_success;
    RRDDIM *rd_adfs_sso_authentications_success_success;
    RRDDIM *rd_adfs_sso_authentications_success_failure;

    RRDSET *st_adfs_windows_integrated_authentications_total;
    RRDDIM *rd_adfs_windows_integrated_authentications_total;

    RRDSET *st_adfs_wsfed_token_requests_success_total;
    RRDDIM *rd_adfs_wsfed_token_requests_success_total;

    COUNTER_DATA ADFSLoginConnectionFailure;
    COUNTER_DATA ADFSCertificateAuthentications;

    COUNTER_DATA ADFSDBArtifactFailures;
    COUNTER_DATA ADFSDBArtifactQueryTimeSeconds;
    COUNTER_DATA ADFSDBConfigFailures;
    COUNTER_DATA ADFSDBConfigQueryTimeSeconds;
    COUNTER_DATA ADFSDeviceAuthentications;
    COUNTER_DATA ADFSExternalAuthenticationsSuccess;
    COUNTER_DATA ADFSExternalAuthenticationsFailure;
    COUNTER_DATA ADFSFederatedAuthentications;
    COUNTER_DATA ADFSFederationMetadataRequests;

    COUNTER_DATA ADFSOauthAuthorizationRequests;
    COUNTER_DATA ADFSOauthClientAuthenticationsSuccess;
    COUNTER_DATA ADFSOauthClientAuthenticationsFailure;
    COUNTER_DATA ADFSOauthClientCredentialsSuccess;
    COUNTER_DATA ADFSOauthClientCredentialsFailure;
    COUNTER_DATA ADFSOauthClientPrivkeyJwtAuthenticationSuccess;
    COUNTER_DATA ADFSOauthClientPrivkeyJwtAuthenticationFailure;
    COUNTER_DATA ADFSOauthClientSecretBasicAuthenticationsSuccess;
    COUNTER_DATA ADFSOauthClientSecretBasicAuthenticationsFailure;
    COUNTER_DATA ADFSOauthClientSecretPostAuthenticationsSuccess;
    COUNTER_DATA ADFSOauthClientSecretPostAuthenticationsFailure;
    COUNTER_DATA ADFSOauthClientWindowsAuthenticationsSuccess;
    COUNTER_DATA ADFSOauthClientWindowsAuthenticationsFailure;
    COUNTER_DATA ADFSOauthPasswordGrantRequestsSuccess;
    COUNTER_DATA ADFSOauthPasswordGrantRequestsFailure;
    COUNTER_DATA ADFSOauthTokenRequestsSuccess;
    COUNTER_DATA ADFSPassiveRequests;
    COUNTER_DATA ADFSPassportAuthentications;
    COUNTER_DATA ADFSPasswordChangeRequestsSuccess;
    COUNTER_DATA ADFSPasswordChangeRequestsFailure;
    COUNTER_DATA ADFSSAMLPTokenRequests;
    COUNTER_DATA ADFSWSTrustTokenRequestsSuccess;
    COUNTER_DATA ADFSSSOAuthenticationsSuccess;
    COUNTER_DATA ADFSSSOAuthenticationsFailure;
    COUNTER_DATA ADFSTokenRequests;
    COUNTER_DATA ADFSUserPasswordAuthenticationsSuccess;
    COUNTER_DATA ADFSUserPasswordAuthenticationsFailure;
    COUNTER_DATA ADFSWindowsIntegratedAuthentications;
    COUNTER_DATA ADFSWSFedTokenRequestsSuccess;
} adfs = {
    .charts_created = false,

    .st_adfs_login_connection_failures = NULL,
    .st_adfs_certificate_authentications_total = NULL,
    .st_adfs_db_artifact_failure_total = NULL,
    .st_adfs_db_artifact_query_time_seconds_total = NULL,
    .st_adfs_db_config_failures = NULL,
    .st_adfs_db_config_query_time_seconds_total = NULL,
    .st_adfs_device_authentications_total = NULL,
    .st_adfs_external_authentications = NULL,
    .st_adfs_federated_authentications = NULL,
    .st_adfs_federation_metadata_requests_total = NULL,
    .st_adfs_oauth_authorization_requests_total = NULL,
    .st_adfs_oauth_client_authentications = NULL,
    .st_adfs_oauth_client_credentials_requests = NULL,
    .st_adfs_oauth_client_privkey_jwt_authentications = NULL,
    .st_adfs_oauth_client_secret_basic_authentications = NULL,
    .st_adfs_oauth_client_secret_post_authentications = NULL,
    .st_adfs_oauth_password_grant_requests = NULL,
    .st_adfs_passive_requests_total = NULL,
    .st_adfs_passport_authentications_total = NULL,
    .st_adfs_password_change_requests = NULL,
    .st_adfs_samlp_token_requests_success_total = NULL,
    .st_adfs_wstrust_token_requests_success_total = NULL,
    .st_adfs_sso_authentications = NULL,
    .st_adfs_token_requests_total = NULL,
    .st_adfs_sso_authentications_success = NULL,
    .st_adfs_windows_integrated_authentications_total = NULL,
    .st_adfs_wsfed_token_requests_success_total = NULL,

    .ADFSLoginConnectionFailure.key = "AD Login Connection Failures",
    .ADFSCertificateAuthentications.key = "Certificate Authentications",

    .ADFSDBArtifactFailures.key = "Artifact Database Connection Failures",
    .ADFSDBArtifactQueryTimeSeconds.key = "Average Artifact Database Query Time",
    .ADFSDBConfigFailures.key = "Configuration Database Connection Failures",
    .ADFSDBConfigQueryTimeSeconds.key = "Average Config Database Query Time",
    .ADFSDeviceAuthentications.key = "Device Authentications",
    .ADFSExternalAuthenticationsSuccess.key = "External Authentications",
    .ADFSExternalAuthenticationsFailure.key = "External Authentication Failures",
    .ADFSFederatedAuthentications.key = "Federated Authentications",
    .ADFSFederationMetadataRequests.key = "Federation Metadata Requests",

    .ADFSOauthAuthorizationRequests.key = "OAuth AuthZ Requests",
    .ADFSOauthClientAuthenticationsSuccess.key = "OAuth Client Authentications",
    .ADFSOauthClientAuthenticationsFailure.key = "OAuth Client Authentications Failures",
    .ADFSOauthClientPrivkeyJwtAuthenticationSuccess.key = "OAuth Client Private Key Jwt Authentications",
    .ADFSOauthClientPrivkeyJwtAuthenticationFailure.key = "OAuth Client Private Key Jwt Authentication Failures",
    .ADFSOauthClientSecretBasicAuthenticationsSuccess.key = "OAuth Client Secret Post Authentication",
    .ADFSOauthClientSecretBasicAuthenticationsFailure.key = "OAuth Client Secret Post Authentication Failures",
    .ADFSOauthClientSecretPostAuthenticationsSuccess.key = "OAuth Client Secret Post Authentication",
    .ADFSOauthClientSecretPostAuthenticationsFailure.key = "OAuth Client Secret Post Authentication Failures",
    .ADFSOauthPasswordGrantRequestsSuccess.key = "OAuth Password Grant Request Failures",
    .ADFSOauthPasswordGrantRequestsFailure.key = "OAuth Password Grant Request",
    .ADFSOauthTokenRequestsSuccess.key = "OAuth Token Requests",
    .ADFSPassiveRequests.key = "Passive Requests",
    .ADFSPassportAuthentications.key = "Microsoft Passport Authentications",
    .ADFSPasswordChangeRequestsSuccess.key = "Password Change Successful Requests",
    .ADFSPasswordChangeRequestsFailure.key = "Password Change Failed Requests",
    .ADFSSAMLPTokenRequests.key = "SAML-P Token Requests",
    .ADFSWSTrustTokenRequestsSuccess.key = "WS-Trust Token Requests",
    .ADFSSSOAuthenticationsSuccess.key = "SSO Authentication Failures",
    .ADFSSSOAuthenticationsFailure.key = "SSO Authentications",
    .ADFSTokenRequests.key = "Token Requests",
    .ADFSUserPasswordAuthenticationsSuccess.key = "SSO Authentications",
    .ADFSUserPasswordAuthenticationsFailure.key = "SSO Authentication Failures",
    .ADFSWindowsIntegratedAuthentications.key = "Windows Integrated Authentications",
    .ADFSWSFedTokenRequestsSuccess.key = "WS-Fed Token Requests"};

static void initialize(void)
{
    ;
}

void netdata_adfs_login_connection_failures(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &adfs.ADFSLoginConnectionFailure)) {
        return;
    }

    if (!adfs.st_adfs_login_connection_failures) {
        adfs.st_adfs_login_connection_failures = rrdset_create_localhost(
            "adfs",
            "ad_login_connection_failures",
            NULL,
            "ad",
            "adfs.ad_login_connection_failures",
            "Connection failures",
            "failures/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibADFS",
            PRIO_ADFS_LOGIN_CONNECTION_FAILURES_TOTAL,
            update_every,
            RRDSET_TYPE_LINE);

        adfs.rd_adfs_login_connection_failures =
            rrddim_add(adfs.st_adfs_login_connection_failures, "connection", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        adfs.st_adfs_login_connection_failures,
        adfs.rd_adfs_login_connection_failures,
        (collected_number)adfs.ADFSLoginConnectionFailure.current.Data);
    rrdset_done(adfs.st_adfs_login_connection_failures);
}

void netdata_adfs_certificate_authentications(
    PERF_DATA_BLOCK *pDataBlock,
    PERF_OBJECT_TYPE *pObjectType,
    int update_every)
{
    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &adfs.ADFSCertificateAuthentications)) {
        return;
    }

    if (!adfs.st_adfs_certificate_authentications_total) {
        adfs.st_adfs_certificate_authentications_total = rrdset_create_localhost(
            "adfs",
            "certificate_authentications",
            NULL,
            "ad",
            "adfs.certificate_authentications",
            "User Certificate authentications",
            "authentications/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibADFS",
            PRIO_ADFS_CERTIFICATE_AUTHENTICATION_TOTAL,
            update_every,
            RRDSET_TYPE_LINE);

        adfs.rd_adfs_certificate_authentications_total = rrddim_add(
            adfs.st_adfs_certificate_authentications_total, "authentications", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        adfs.st_adfs_certificate_authentications_total,
        adfs.rd_adfs_certificate_authentications_total,
        (collected_number)adfs.ADFSCertificateAuthentications.current.Data);
    rrdset_done(adfs.st_adfs_certificate_authentications_total);
}

void netdata_adfs_db_artifacts_failure(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &adfs.ADFSDBArtifactFailures)) {
        return;
    }

    if (!adfs.st_adfs_db_artifact_failure_total) {
        adfs.st_adfs_db_artifact_failure_total = rrdset_create_localhost(
            "adfs",
            "db_artifact_failures",
            NULL,
            "db artifact",
            "adfs.db_artifact_failures",
            "Connection failures to the artifact database",
            "failures/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibADFS",
            PRIO_ADFS_DB_ARTIFACT_FAILURE_TOTAL,
            update_every,
            RRDSET_TYPE_LINE);

        adfs.rd_adfs_db_artifact_failure_total =
            rrddim_add(adfs.st_adfs_db_artifact_failure_total, "connection", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        adfs.st_adfs_db_artifact_failure_total,
        adfs.rd_adfs_db_artifact_failure_total,
        (collected_number)adfs.ADFSDBArtifactFailures.current.Data);
    rrdset_done(adfs.st_adfs_db_artifact_failure_total);
}

void netdata_adfs_db_artifact_query_time_seconds(
    PERF_DATA_BLOCK *pDataBlock,
    PERF_OBJECT_TYPE *pObjectType,
    int update_every)
{
    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &adfs.ADFSDBArtifactQueryTimeSeconds)) {
        return;
    }

    if (!adfs.st_adfs_db_artifact_query_time_seconds_total) {
        adfs.st_adfs_db_artifact_query_time_seconds_total = rrdset_create_localhost(
            "adfs",
            "db_artifact_query_time_seconds",
            NULL,
            "db artifact",
            "adfs.db_artifact_query_time_seconds",
            "Time taken for an artifact database query",
            "seconds/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibADFS",
            PRIO_ADFS_DB_ARTIFACT_QUERY_TYME_SECONDS_TOTAL,
            update_every,
            RRDSET_TYPE_LINE);

        adfs.rd_adfs_db_artifact_query_time_seconds_total = rrddim_add(
            adfs.st_adfs_db_artifact_query_time_seconds_total, "query_time", NULL, 1, 1000, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        adfs.st_adfs_db_artifact_query_time_seconds_total,
        adfs.rd_adfs_db_artifact_query_time_seconds_total,
        (collected_number)adfs.ADFSDBArtifactQueryTimeSeconds.current.Data);
    rrdset_done(adfs.st_adfs_db_artifact_query_time_seconds_total);
}

void netdata_adfs_device_authentications(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &adfs.ADFSDeviceAuthentications)) {
        return;
    }

    if (!adfs.st_adfs_device_authentications_total) {
        adfs.st_adfs_device_authentications_total = rrdset_create_localhost(
            "adfs",
            "device_authentications",
            NULL,
            "auth",
            "adfs.device_authentications",
            "Device authentications",
            "authentications/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibADFS",
            PRIO_ADFS_DEVICE_AUTHENTICATIONS_TOTAL,
            update_every,
            RRDSET_TYPE_LINE);

        adfs.rd_adfs_device_authentications_total = rrddim_add(
            adfs.st_adfs_device_authentications_total, "authentications", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        adfs.st_adfs_device_authentications_total,
        adfs.rd_adfs_device_authentications_total,
        (collected_number)adfs.ADFSDeviceAuthentications.current.Data);
    rrdset_done(adfs.st_adfs_device_authentications_total);
}

void netdata_adfs_external_authentications(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &adfs.ADFSExternalAuthenticationsSuccess) ||
        !perflibGetObjectCounter(pDataBlock, pObjectType, &adfs.ADFSExternalAuthenticationsFailure)) {
        return;
    }

    if (!adfs.st_adfs_external_authentications) {
        adfs.st_adfs_external_authentications = rrdset_create_localhost(
            "adfs",
            "external_authentications",
            NULL,
            "auth",
            "adfs.external_authentications",
            "Authentications from external MFA providers",
            "authentications/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibADFS",
            PRIO_ADFS_EXTERNAL_AUTHENTICATION_TOTAL,
            update_every,
            RRDSET_TYPE_LINE);

        adfs.rd_adfs_external_authentications_success =
            rrddim_add(adfs.st_adfs_external_authentications, "success", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

        adfs.rd_adfs_external_authentications_failure =
            rrddim_add(adfs.st_adfs_external_authentications, "failure", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        adfs.st_adfs_external_authentications,
        adfs.rd_adfs_external_authentications_success,
        (collected_number)adfs.ADFSExternalAuthenticationsSuccess.current.Data);

    rrddim_set_by_pointer(
        adfs.st_adfs_external_authentications,
        adfs.rd_adfs_external_authentications_failure,
        (collected_number)adfs.ADFSExternalAuthenticationsFailure.current.Data);

    rrdset_done(adfs.st_adfs_external_authentications);
}

static bool do_ADFS(PERF_DATA_BLOCK *pDataBlock, int update_every)
{
    PERF_OBJECT_TYPE *pObjectType = perflibFindObjectTypeByName(pDataBlock, "Certification Authority");
    if (!pObjectType)
        return false;

    static void (*doADFS[])(PERF_DATA_BLOCK *, PERF_OBJECT_TYPE *, int) = {
        netdata_adfs_login_connection_failures,
        netdata_adfs_certificate_authentications,

        netdata_adfs_db_artifacts_failure,
        netdata_adfs_db_artifact_query_time_seconds,
        netdata_adfs_device_authentications,
        netdata_adfs_external_authentications,

        // This must be the end
        NULL};
    return true;
}

int do_PerflibADFS(int update_every, usec_t dt __maybe_unused)
{
    static bool initialized = false;

    if (unlikely(!initialized)) {
        initialize();
        initialized = true;
    }

    DWORD id = RegistryFindIDByName("AD FS");
    if (id == PERFLIB_REGISTRY_NAME_NOT_FOUND)
        return -1;

    PERF_DATA_BLOCK *pDataBlock = perflibGetPerformanceData(id);
    if (!pDataBlock)
        return -1;

    do_ADFS(pDataBlock, update_every);

    return 0;
}
