// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows_plugin.h"
#include "windows-internals.h"

struct adfs_certificate {
    bool charts_created;

    // AD/ADFS
    RRDSET *st_adfs_login_connection_failures;
    RRDDIM *rd_adfs_login_connection_failures;

    RRDSET *st_adfs_certificate_authentications_total;
    RRDDIM *rd_adfs_certificate_authentications_total;

    // DB Artifacts
    RRDSET *st_adfs_db_artifact_failure_total;
    RRDDIM *rd_adfs_db_artifact_failure_total;

    RRDSET *st_adfs_db_artifact_query_time_seconds_total;
    RRDDIM *rd_adfs_db_artifact_query_time_seconds_total;

    // DB Config
    RRDSET *st_adfs_db_config_failures;
    RRDDIM *rd_adfs_db_config_failures;

    RRDSET *st_adfs_db_config_query_time_seconds_total;
    RRDDIM *rd_adfs_db_config_query_time_seconds_total;

    // Auth
    RRDSET *st_adfs_device_authentications_total;
    RRDDIM *rd_adfs_device_authentications_total;

    RRDSET *st_adfs_external_authentications;
    RRDDIM *rd_adfs_external_authentications_success;
    RRDDIM *rd_adfs_external_authentications_failure;

    RRDSET *st_adfs_federation_authentications;
    RRDDIM *rd_adfs_federation_authentications;

    RRDSET *st_adfs_federation_metadata_authentications;
    RRDDIM *rd_adfs_federation_metadata_authentications;

    // OAuth
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

    RRDSET *st_adfs_oauth_client_windows_authentications;
    RRDDIM *rd_adfs_oauth_client_windows_authentications_success_total;
    RRDDIM *rd_adfs_oauth_client_windows_authentications_failure_total;

    RRDSET *st_adfs_oauth_logon_certificate_requests;
    RRDDIM *rd_adfs_oauth_logon_certificate_requests_success_total;
    RRDDIM *rd_adfs_oauth_logon_certificate_requests_failure_total;

    RRDSET *st_adfs_oauth_password_grant_requests;
    RRDDIM *rd_adfs_oauth_password_grant_requests_success;
    RRDDIM *rd_adfs_oauth_password_grant_requests_failure;

    RRDSET *st_adfs_oauth_token_requests_success;
    RRDDIM *rd_adfs_oauth_token_requests_success;

    // Requests
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

    RRDSET *st_adfs_user_password_authentications;
    RRDDIM *rd_adfs_user_password_authentications_success;
    RRDDIM *rd_adfs_user_password_authentications_failure;

    RRDSET *st_adfs_windows_integrated_authentications_total;
    RRDDIM *rd_adfs_windows_integrated_authentications_total;

    RRDSET *st_adfs_wsfed_token_requests_success_total;
    RRDDIM *rd_adfs_wsfed_token_requests_success_total;

    // AD/ADFS
    COUNTER_DATA ADFSLoginConnectionFailure;
    COUNTER_DATA ADFSCertificateAuthentications;

    // DB Artifacts
    COUNTER_DATA ADFSDBArtifactFailures;
    COUNTER_DATA ADFSDBArtifactQueryTimeSeconds;

    // DB Config
    COUNTER_DATA ADFSDBConfigFailures;
    COUNTER_DATA ADFSDBConfigQueryTimeSeconds;

    // Auth
    COUNTER_DATA ADFSDeviceAuthentications;
    COUNTER_DATA ADFSExternalAuthenticationsSuccess;
    COUNTER_DATA ADFSExternalAuthenticationsFailure;
    COUNTER_DATA ADFSFederationAuthentications;
    COUNTER_DATA ADFSFederationMetadataAuthentications;
    COUNTER_DATA ADFSOauthAuthorizationRequests;

    // OAUTH
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
    COUNTER_DATA ADFSOauthLogonCertificateRequestsSuccess;
    COUNTER_DATA ADFSOauthLogonCertificateRequestsFailure;
    COUNTER_DATA ADFSOauthPasswordGrantRequestsSuccess;
    COUNTER_DATA ADFSOauthPasswordGrantRequestsFailure;
    COUNTER_DATA ADFSOauthTokenRequestsSuccess;

    // Requests
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

    // AD/ADFS
    .st_adfs_login_connection_failures = NULL,
    .st_adfs_certificate_authentications_total = NULL,

    // DB Artifacts
    .st_adfs_db_artifact_failure_total = NULL,
    .st_adfs_db_artifact_query_time_seconds_total = NULL,

    // DB Config
    .st_adfs_db_config_failures = NULL,
    .st_adfs_db_config_query_time_seconds_total = NULL,

    // Auth
    .st_adfs_device_authentications_total = NULL,
    .st_adfs_external_authentications = NULL,
    .st_adfs_federation_authentications = NULL,
    .st_adfs_federation_metadata_authentications = NULL,

    // OAuth
    .st_adfs_oauth_authorization_requests_total = NULL,
    .st_adfs_oauth_client_authentications = NULL,
    .st_adfs_oauth_client_credentials_requests = NULL,
    .st_adfs_oauth_client_privkey_jwt_authentications = NULL,
    .st_adfs_oauth_client_secret_basic_authentications = NULL,
    .st_adfs_oauth_client_secret_post_authentications = NULL,
    .st_adfs_oauth_client_windows_authentications = NULL,
    .st_adfs_oauth_logon_certificate_requests = NULL,
    .st_adfs_oauth_password_grant_requests = NULL,
    .st_adfs_oauth_token_requests_success = NULL,

    // Requests
    .st_adfs_passive_requests_total = NULL,
    .st_adfs_passport_authentications_total = NULL,
    .st_adfs_password_change_requests = NULL,
    .st_adfs_samlp_token_requests_success_total = NULL,
    .st_adfs_wstrust_token_requests_success_total = NULL,
    .st_adfs_sso_authentications = NULL,
    .st_adfs_token_requests_total = NULL,
    .st_adfs_user_password_authentications = NULL,
    .st_adfs_windows_integrated_authentications_total = NULL,
    .st_adfs_wsfed_token_requests_success_total = NULL,

    // AD/ADFS
    .ADFSLoginConnectionFailure.key = "AD Login Connection Failures",
    .ADFSCertificateAuthentications.key = "Certificate Authentications",

    // DB Artifacts
    .ADFSDBArtifactFailures.key = "Artifact Database Connection Failures",
    .ADFSDBArtifactQueryTimeSeconds.key = "Average Artifact Database Query Time",

    // DB Config
    .ADFSDBConfigFailures.key = "Configuration Database Connection Failures",
    .ADFSDBConfigQueryTimeSeconds.key = "Average Config Database Query Time",

    // Auth
    .ADFSDeviceAuthentications.key = "Device Authentications",
    .ADFSExternalAuthenticationsSuccess.key = "External Authentications",
    .ADFSExternalAuthenticationsFailure.key = "External Authentication Failures",
    .ADFSFederationAuthentications.key = "Federated Authentications",
    .ADFSFederationMetadataAuthentications.key = "Federation Metadata Requests",
    .ADFSOauthAuthorizationRequests.key = "OAuth AuthZ Requests",

    // OAuth
    .ADFSOauthClientAuthenticationsSuccess.key = "OAuth Client Authentications",
    .ADFSOauthClientAuthenticationsFailure.key = "OAuth Client Authentications Failures",
    .ADFSOauthClientCredentialsSuccess.key = "OAuth Client Credentials Requests",
    .ADFSOauthClientCredentialsFailure.key = "OAuth Client Credentials Request Failures",
    .ADFSOauthClientPrivkeyJwtAuthenticationSuccess.key = "OAuth Client Private Key Jwt Authentications",
    .ADFSOauthClientPrivkeyJwtAuthenticationFailure.key = "OAuth Client Private Key Jwt Authentication Failures",
    .ADFSOauthClientSecretBasicAuthenticationsSuccess.key = "OAuth Client Secret Basic Authentications",
    .ADFSOauthClientSecretBasicAuthenticationsFailure.key = "OAuth Client Secret Basic Authentication Failures",
    .ADFSOauthClientSecretPostAuthenticationsSuccess.key = "OAuth Client Secret Post Authentication",
    .ADFSOauthClientSecretPostAuthenticationsFailure.key = "OAuth Client Secret Post Authentication Failures",
    .ADFSOauthClientWindowsAuthenticationsSuccess.key = "OAuth Client Windows Integrated Authentication",
    .ADFSOauthClientWindowsAuthenticationsFailure.key = "OAuth Client Windows Integrated Authentication Failures",
    .ADFSOauthLogonCertificateRequestsSuccess.key = "OAuth Logon Certificate Token Requests",
    .ADFSOauthLogonCertificateRequestsFailure.key = "OAuth Logon Certificate Request Failures",
    .ADFSOauthPasswordGrantRequestsSuccess.key = "OAuth Password Grant Requests",
    .ADFSOauthPasswordGrantRequestsFailure.key = "OAuth Password Grant Request Failures",
    .ADFSOauthTokenRequestsSuccess.key = "OAuth Token Requests",

    // Requests
    .ADFSPassiveRequests.key = "Passive Requests",
    .ADFSPassportAuthentications.key = "Microsoft Passport Authentications",
    .ADFSPasswordChangeRequestsSuccess.key = "Password Change Successful Requests",
    .ADFSPasswordChangeRequestsFailure.key = "Password Change Failed Requests",
    .ADFSSAMLPTokenRequests.key = "SAML-P Token Requests",
    .ADFSWSTrustTokenRequestsSuccess.key = "WS-Trust Token Requests",
    .ADFSSSOAuthenticationsSuccess.key = "SSO Authentications",
    .ADFSSSOAuthenticationsFailure.key = "SSO Authentication Failures",
    .ADFSTokenRequests.key = "Token Requests",
    .ADFSUserPasswordAuthenticationsSuccess.key = "U/P Authentications",
    .ADFSUserPasswordAuthenticationsFailure.key = "U/P Authentication Failures",
    .ADFSWindowsIntegratedAuthentications.key = "Windows Integrated Authentications",
    .ADFSWSFedTokenRequestsSuccess.key = "WS-Fed Token Requests",
};

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

void netdata_adfs_db_artifacts_query_time_seconds(
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

void netdata_adfs_db_config_failure(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &adfs.ADFSDBConfigFailures)) {
        return;
    }

    if (!adfs.st_adfs_db_config_failures) {
        adfs.st_adfs_db_config_failures = rrdset_create_localhost(
            "adfs",
            "db_config_failures",
            NULL,
            "db config",
            "adfs.db_config_failures",
            "Connection failures to the configuration database",
            "failures/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibADFS",
            PRIO_ADFS_DB_CONFIG_FAILURE_TOTAL,
            update_every,
            RRDSET_TYPE_LINE);

        adfs.rd_adfs_db_config_failures =
            rrddim_add(adfs.st_adfs_db_config_failures, "connection", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        adfs.st_adfs_db_config_failures,
        adfs.rd_adfs_db_config_failures,
        (collected_number)adfs.ADFSDBConfigFailures.current.Data);
    rrdset_done(adfs.st_adfs_db_config_failures);
}

void netdata_adfs_db_config_query_time_seconds(
    PERF_DATA_BLOCK *pDataBlock,
    PERF_OBJECT_TYPE *pObjectType,
    int update_every)
{
    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &adfs.ADFSDBConfigQueryTimeSeconds)) {
        return;
    }

    if (!adfs.st_adfs_db_config_query_time_seconds_total) {
        adfs.st_adfs_db_config_query_time_seconds_total = rrdset_create_localhost(
            "adfs",
            "db_config_query_time_seconds",
            NULL,
            "db config",
            "adfs.db_config_query_time_seconds",
            "Time taken for a configuration database query",
            "seconds/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibADFS",
            PRIO_ADFS_DB_CONFIG_QUERY_TYME_SECONDS_TOTAL,
            update_every,
            RRDSET_TYPE_LINE);

        adfs.rd_adfs_db_config_query_time_seconds_total = rrddim_add(
            adfs.st_adfs_db_config_query_time_seconds_total, "query_time", NULL, 1, 1000, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        adfs.st_adfs_db_config_query_time_seconds_total,
        adfs.rd_adfs_db_config_query_time_seconds_total,
        (collected_number)adfs.ADFSDBConfigQueryTimeSeconds.current.Data);
    rrdset_done(adfs.st_adfs_db_config_query_time_seconds_total);
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

void netdata_adfs_federated_authentications(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &adfs.ADFSFederationAuthentications)) {
        return;
    }

    if (!adfs.st_adfs_federation_authentications) {
        adfs.st_adfs_federation_authentications = rrdset_create_localhost(
            "adfs",
            "federated_authentications",
            NULL,
            "auth",
            "adfs.federated_authentications",
            "Authentications from Federated Sources",
            "authentications/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibADFS",
            PRIO_ADFS_FEDERATION_AUTHENTICATION_TOTAL,
            update_every,
            RRDSET_TYPE_LINE);

        adfs.rd_adfs_federation_authentications = rrddim_add(
            adfs.st_adfs_federation_authentications, "authentications", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        adfs.st_adfs_federation_authentications,
        adfs.rd_adfs_federation_authentications,
        (collected_number)adfs.ADFSFederationAuthentications.current.Data);

    rrdset_done(adfs.st_adfs_federation_authentications);
}

void netdata_adfs_federation_metadata_authentications(
    PERF_DATA_BLOCK *pDataBlock,
    PERF_OBJECT_TYPE *pObjectType,
    int update_every)
{
    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &adfs.ADFSFederationMetadataAuthentications)) {
        return;
    }

    if (!adfs.st_adfs_federation_metadata_authentications) {
        adfs.st_adfs_federation_metadata_authentications = rrdset_create_localhost(
            "adfs",
            "federation_metadata_requests",
            NULL,
            "auth",
            "adfs.federation_metadata_requests",
            "Federation Metadata requests",
            "requests/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibADFS",
            PRIO_ADFS_FEDERATION_REQUESTS_AUTHENTICATION_TOTAL,
            update_every,
            RRDSET_TYPE_LINE);

        adfs.rd_adfs_federation_metadata_authentications = rrddim_add(
            adfs.st_adfs_federation_metadata_authentications, "requests", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        adfs.st_adfs_federation_metadata_authentications,
        adfs.rd_adfs_federation_metadata_authentications,
        (collected_number)adfs.ADFSFederationMetadataAuthentications.current.Data);

    rrdset_done(adfs.st_adfs_federation_metadata_authentications);
}

void netdata_adfs_oauth_authorization_requests(
    PERF_DATA_BLOCK *pDataBlock,
    PERF_OBJECT_TYPE *pObjectType,
    int update_every)
{
    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &adfs.ADFSOauthAuthorizationRequests)) {
        return;
    }

    if (!adfs.st_adfs_oauth_authorization_requests_total) {
        adfs.st_adfs_oauth_authorization_requests_total = rrdset_create_localhost(
            "adfs",
            "oauth_authorization_requests",
            NULL,
            "oauth",
            "adfs.oauth_authorization_requests",
            "Incoming requests to the OAuth Authorization endpoint",
            "requests/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibADFS",
            PRIO_ADFS_OAUTH_AUTHORIZED_REQUEST,
            update_every,
            RRDSET_TYPE_LINE);

        adfs.rd_adfs_oauth_authorization_requests_total = rrddim_add(
            adfs.st_adfs_oauth_authorization_requests_total, "requests", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        adfs.st_adfs_oauth_authorization_requests_total,
        adfs.rd_adfs_oauth_authorization_requests_total,
        (collected_number)adfs.ADFSOauthAuthorizationRequests.current.Data);

    rrdset_done(adfs.st_adfs_oauth_authorization_requests_total);
}

void netdata_adfs_oauth_client_authentication(
    PERF_DATA_BLOCK *pDataBlock,
    PERF_OBJECT_TYPE *pObjectType,
    int update_every)
{
    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &adfs.ADFSOauthClientAuthenticationsSuccess) ||
        !perflibGetObjectCounter(pDataBlock, pObjectType, &adfs.ADFSOauthClientAuthenticationsFailure)) {
        return;
    }

    if (!adfs.st_adfs_oauth_client_authentications) {
        adfs.st_adfs_oauth_client_authentications = rrdset_create_localhost(
            "adfs",
            "oauth_client_authentications",
            NULL,
            "oauth",
            "adfs.oauth_client_authentications",
            "OAuth client authentications",
            "authentications/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibADFS",
            PRIO_ADFS_OAUTH_CLIENT_AUTHORIZATION_REQUEST,
            update_every,
            RRDSET_TYPE_LINE);

        adfs.rd_adfs_oauth_client_authentications_success =
            rrddim_add(adfs.st_adfs_oauth_client_authentications, "success", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

        adfs.rd_adfs_oauth_client_authentications_failure = rrddim_add(
            adfs.st_adfs_oauth_client_authentications, "failure", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        adfs.st_adfs_oauth_client_authentications,
        adfs.rd_adfs_oauth_client_authentications_success,
        (collected_number)adfs.ADFSOauthClientAuthenticationsSuccess.current.Data);

    rrddim_set_by_pointer(
        adfs.st_adfs_oauth_client_authentications,
        adfs.rd_adfs_oauth_client_authentications_failure,
        (collected_number)adfs.ADFSOauthClientAuthenticationsFailure.current.Data);

    rrdset_done(adfs.st_adfs_oauth_client_authentications);
}

void netdata_adfs_oauth_client_credentials_requests(
    PERF_DATA_BLOCK *pDataBlock,
    PERF_OBJECT_TYPE *pObjectType,
    int update_every)
{
    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &adfs.ADFSOauthClientCredentialsSuccess) ||
        !perflibGetObjectCounter(pDataBlock, pObjectType, &adfs.ADFSOauthClientCredentialsFailure)) {
        return;
    }

    if (!adfs.st_adfs_oauth_client_credentials_requests) {
        adfs.st_adfs_oauth_client_credentials_requests = rrdset_create_localhost(
            "adfs",
            "oauth_client_credentials_requests",
            NULL,
            "oauth",
            "adfs.oauth_client_credentials_requests",
            "OAuth client credentials requests",
            "requests/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibADFS",
            PRIO_ADFS_OAUTH_CLIENT_CREDENTIAL_REQUEST,
            update_every,
            RRDSET_TYPE_LINE);

        adfs.rd_adfs_oauth_client_credentials_requests_success = rrddim_add(
            adfs.st_adfs_oauth_client_credentials_requests, "success", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

        adfs.rd_adfs_oauth_client_credentials_requests_failure = rrddim_add(
            adfs.st_adfs_oauth_client_credentials_requests, "failure", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        adfs.st_adfs_oauth_client_credentials_requests,
        adfs.rd_adfs_oauth_client_credentials_requests_success,
        (collected_number)adfs.ADFSOauthClientCredentialsSuccess.current.Data);

    rrddim_set_by_pointer(
        adfs.st_adfs_oauth_client_credentials_requests,
        adfs.rd_adfs_oauth_client_credentials_requests_failure,
        (collected_number)adfs.ADFSOauthClientCredentialsFailure.current.Data);

    rrdset_done(adfs.st_adfs_oauth_client_credentials_requests);
}

void netdata_adfs_oauth_client_privkey_jwt_authentications(
    PERF_DATA_BLOCK *pDataBlock,
    PERF_OBJECT_TYPE *pObjectType,
    int update_every)
{
    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &adfs.ADFSOauthClientPrivkeyJwtAuthenticationSuccess) ||
        !perflibGetObjectCounter(pDataBlock, pObjectType, &adfs.ADFSOauthClientPrivkeyJwtAuthenticationFailure)) {
        return;
    }

    if (!adfs.st_adfs_oauth_client_privkey_jwt_authentications) {
        adfs.st_adfs_oauth_client_privkey_jwt_authentications = rrdset_create_localhost(
            "adfs",
            "oauth_client_privkey_jwt_authentications",
            NULL,
            "oauth",
            "adfs.oauth_client_privkey_jwt_authentications",
            "OAuth client private key JWT authentications",
            "authentications/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibADFS",
            PRIO_ADFS_OAUTH_CLIENT_PRV_KEY_JWT_AUTH,
            update_every,
            RRDSET_TYPE_LINE);

        adfs.rd_adfs_oauth_client_privkey_jwt_authentications_success = rrddim_add(
            adfs.st_adfs_oauth_client_privkey_jwt_authentications, "success", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

        adfs.rd_adfs_oauth_client_privkey_jwt_authentications_failure = rrddim_add(
            adfs.st_adfs_oauth_client_privkey_jwt_authentications, "failure", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        adfs.st_adfs_oauth_client_privkey_jwt_authentications,
        adfs.rd_adfs_oauth_client_privkey_jwt_authentications_success,
        (collected_number)adfs.ADFSOauthClientPrivkeyJwtAuthenticationSuccess.current.Data);

    rrddim_set_by_pointer(
        adfs.st_adfs_oauth_client_privkey_jwt_authentications,
        adfs.rd_adfs_oauth_client_privkey_jwt_authentications_failure,
        (collected_number)adfs.ADFSOauthClientPrivkeyJwtAuthenticationFailure.current.Data);

    rrdset_done(adfs.st_adfs_oauth_client_privkey_jwt_authentications);
}

void netdata_adfs_oauth_client_secret_basic_authentications(
    PERF_DATA_BLOCK *pDataBlock,
    PERF_OBJECT_TYPE *pObjectType,
    int update_every)
{
    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &adfs.ADFSOauthClientSecretBasicAuthenticationsSuccess) ||
        !perflibGetObjectCounter(pDataBlock, pObjectType, &adfs.ADFSOauthClientSecretBasicAuthenticationsFailure)) {
        return;
    }

    if (!adfs.st_adfs_oauth_client_secret_basic_authentications) {
        adfs.st_adfs_oauth_client_secret_basic_authentications = rrdset_create_localhost(
            "adfs",
            "oauth_client_secret_basic_authentications",
            NULL,
            "oauth",
            "adfs.oauth_client_secret_basic_authentications",
            "OAuth client secret basic authentications",
            "authentications/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibADFS",
            PRIO_ADFS_OAUTH_CLIENT_SECRET_BASIC_AUTH,
            update_every,
            RRDSET_TYPE_LINE);

        adfs.rd_adfs_oauth_client_secret_basic_authentications_success = rrddim_add(
            adfs.st_adfs_oauth_client_secret_basic_authentications, "success", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

        adfs.rd_adfs_oauth_client_secret_basic_authentications_failure = rrddim_add(
            adfs.st_adfs_oauth_client_secret_basic_authentications, "failure", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        adfs.st_adfs_oauth_client_secret_basic_authentications,
        adfs.rd_adfs_oauth_client_secret_basic_authentications_success,
        (collected_number)adfs.ADFSOauthClientSecretBasicAuthenticationsSuccess.current.Data);

    rrddim_set_by_pointer(
        adfs.st_adfs_oauth_client_secret_basic_authentications,
        adfs.rd_adfs_oauth_client_secret_basic_authentications_failure,
        (collected_number)adfs.ADFSOauthClientSecretBasicAuthenticationsFailure.current.Data);

    rrdset_done(adfs.st_adfs_oauth_client_secret_basic_authentications);
}

void netdata_adfs_oauth_client_secret_post_authentications(
    PERF_DATA_BLOCK *pDataBlock,
    PERF_OBJECT_TYPE *pObjectType,
    int update_every)
{
    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &adfs.ADFSOauthClientSecretPostAuthenticationsSuccess) ||
        !perflibGetObjectCounter(pDataBlock, pObjectType, &adfs.ADFSOauthClientSecretPostAuthenticationsFailure)) {
        return;
    }

    if (!adfs.st_adfs_oauth_client_secret_post_authentications) {
        adfs.st_adfs_oauth_client_secret_post_authentications = rrdset_create_localhost(
            "adfs",
            "oauth_client_secret_post_authentications",
            NULL,
            "oauth",
            "adfs.oauth_client_secret_post_authentications",
            "OAuth client secret post authentications",
            "authentications/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibADFS",
            PRIO_ADFS_OAUTH_CLIENT_SECRET_POST_AUTH,
            update_every,
            RRDSET_TYPE_LINE);

        adfs.rd_adfs_oauth_client_secret_post_authentications_success = rrddim_add(
            adfs.st_adfs_oauth_client_secret_post_authentications, "success", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

        adfs.rd_adfs_oauth_client_secret_post_authentications_failure = rrddim_add(
            adfs.st_adfs_oauth_client_secret_post_authentications, "failure", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        adfs.st_adfs_oauth_client_secret_post_authentications,
        adfs.rd_adfs_oauth_client_secret_post_authentications_success,
        (collected_number)adfs.ADFSOauthClientSecretPostAuthenticationsSuccess.current.Data);

    rrddim_set_by_pointer(
        adfs.st_adfs_oauth_client_secret_post_authentications,
        adfs.rd_adfs_oauth_client_secret_post_authentications_failure,
        (collected_number)adfs.ADFSOauthClientSecretPostAuthenticationsFailure.current.Data);

    rrdset_done(adfs.st_adfs_oauth_client_secret_post_authentications);
}

void netdata_adfs_oauth_client_windows_authentications(
    PERF_DATA_BLOCK *pDataBlock,
    PERF_OBJECT_TYPE *pObjectType,
    int update_every)
{
    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &adfs.ADFSOauthClientWindowsAuthenticationsSuccess) ||
        !perflibGetObjectCounter(pDataBlock, pObjectType, &adfs.ADFSOauthClientWindowsAuthenticationsFailure)) {
        return;
    }

    if (!adfs.st_adfs_oauth_client_windows_authentications) {
        adfs.st_adfs_oauth_client_windows_authentications = rrdset_create_localhost(
            "adfs",
            "oauth_client_windows_authentications",
            NULL,
            "oauth",
            "adfs.oauth_client_windows_authentications",
            "OAuth client windows integrated authentications",
            "authentications/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibADFS",
            PRIO_ADFS_OAUTH_CLIENT_WINDOWS_AUTH,
            update_every,
            RRDSET_TYPE_LINE);

        adfs.rd_adfs_oauth_client_windows_authentications_success_total = rrddim_add(
            adfs.st_adfs_oauth_client_windows_authentications, "success", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

        adfs.rd_adfs_oauth_client_windows_authentications_failure_total = rrddim_add(
            adfs.st_adfs_oauth_client_windows_authentications, "failure", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        adfs.st_adfs_oauth_client_windows_authentications,
        adfs.rd_adfs_oauth_client_windows_authentications_success_total,
        (collected_number)adfs.ADFSOauthClientWindowsAuthenticationsSuccess.current.Data);

    rrddim_set_by_pointer(
        adfs.st_adfs_oauth_client_windows_authentications,
        adfs.rd_adfs_oauth_client_windows_authentications_failure_total,
        (collected_number)adfs.ADFSOauthClientWindowsAuthenticationsFailure.current.Data);

    rrdset_done(adfs.st_adfs_oauth_client_windows_authentications);
}

void netdata_adfs_oauth_logon_certificate_request(
    PERF_DATA_BLOCK *pDataBlock,
    PERF_OBJECT_TYPE *pObjectType,
    int update_every)
{
    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &adfs.ADFSOauthLogonCertificateRequestsSuccess) ||
        !perflibGetObjectCounter(pDataBlock, pObjectType, &adfs.ADFSOauthLogonCertificateRequestsFailure)) {
        return;
    }

    if (!adfs.st_adfs_oauth_logon_certificate_requests) {
        adfs.st_adfs_oauth_logon_certificate_requests = rrdset_create_localhost(
            "adfs",
            "oauth_logon_certificate_requests",
            NULL,
            "oauth",
            "adfs.oauth_logon_certificate_requests",
            "OAuth logon certificate requests",
            "requests/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibADFS",
            PRIO_ADFS_OAUTH_CLIENT_WINDOWS_AUTH,
            update_every,
            RRDSET_TYPE_LINE);

        adfs.rd_adfs_oauth_logon_certificate_requests_success_total =
            rrddim_add(adfs.st_adfs_oauth_logon_certificate_requests, "success", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

        adfs.rd_adfs_oauth_logon_certificate_requests_failure_total =
            rrddim_add(adfs.st_adfs_oauth_logon_certificate_requests, "failure", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        adfs.st_adfs_oauth_logon_certificate_requests,
        adfs.rd_adfs_oauth_logon_certificate_requests_success_total,
        (collected_number)adfs.ADFSOauthLogonCertificateRequestsSuccess.current.Data);

    rrddim_set_by_pointer(
        adfs.st_adfs_oauth_logon_certificate_requests,
        adfs.rd_adfs_oauth_logon_certificate_requests_failure_total,
        (collected_number)adfs.ADFSOauthLogonCertificateRequestsFailure.current.Data);

    rrdset_done(adfs.st_adfs_oauth_logon_certificate_requests);
}

void netdata_adfs_oauth_password_grant_requests(
    PERF_DATA_BLOCK *pDataBlock,
    PERF_OBJECT_TYPE *pObjectType,
    int update_every)
{
    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &adfs.ADFSOauthPasswordGrantRequestsSuccess) ||
        !perflibGetObjectCounter(pDataBlock, pObjectType, &adfs.ADFSOauthPasswordGrantRequestsFailure)) {
        return;
    }

    if (!adfs.st_adfs_oauth_password_grant_requests) {
        adfs.st_adfs_oauth_password_grant_requests = rrdset_create_localhost(
            "adfs",
            "oauth_password_grant_requests",
            NULL,
            "oauth",
            "adfs.oauth_password_grant_requests",
            "OAuth password grant requests",
            "authentications/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibADFS",
            PRIO_ADFS_OAUTH_TOKEN_REQUESTS_SUCCESS,
            update_every,
            RRDSET_TYPE_LINE);

        adfs.rd_adfs_oauth_password_grant_requests_success =
            rrddim_add(adfs.st_adfs_oauth_password_grant_requests, "success", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

        adfs.rd_adfs_oauth_password_grant_requests_failure =
            rrddim_add(adfs.st_adfs_oauth_password_grant_requests, "failure", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        adfs.st_adfs_oauth_password_grant_requests,
        adfs.rd_adfs_oauth_password_grant_requests_success,
        (collected_number)adfs.ADFSOauthPasswordGrantRequestsSuccess.current.Data);

    rrddim_set_by_pointer(
        adfs.st_adfs_oauth_password_grant_requests,
        adfs.rd_adfs_oauth_password_grant_requests_failure,
        (collected_number)adfs.ADFSOauthPasswordGrantRequestsFailure.current.Data);

    rrdset_done(adfs.st_adfs_oauth_password_grant_requests);
}

void netdata_adfs_oauth_token_requests_success(
    PERF_DATA_BLOCK *pDataBlock,
    PERF_OBJECT_TYPE *pObjectType,
    int update_every)
{
    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &adfs.ADFSOauthTokenRequestsSuccess)) {
        return;
    }

    if (!adfs.st_adfs_oauth_token_requests_success) {
        adfs.st_adfs_oauth_token_requests_success = rrdset_create_localhost(
            "adfs",
            "oauth_token_requests_success",
            NULL,
            "oauth",
            "adfs.oauth_token_requests",
            "Successful RP token requests over OAuth protocol",
            "requests/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibADFS",
            PRIO_ADFS_OAUTH_CLIENT_CREDENTIAL_REQUEST,
            update_every,
            RRDSET_TYPE_LINE);

        adfs.rd_adfs_oauth_token_requests_success =
            rrddim_add(adfs.st_adfs_oauth_token_requests_success, "success", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        adfs.st_adfs_oauth_token_requests_success,
        adfs.rd_adfs_oauth_token_requests_success,
        (collected_number)adfs.ADFSOauthTokenRequestsSuccess.current.Data);

    rrdset_done(adfs.st_adfs_oauth_token_requests_success);
}

void netdata_adfs_passive_requests_chart(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &adfs.ADFSPassiveRequests)) {
        return;
    }

    if (!adfs.st_adfs_passive_requests_total) {
        adfs.st_adfs_passive_requests_total = rrdset_create_localhost(
            "adfs",
            "passive_requests",
            NULL,
            "requests",
            "adfs.passive_requests",
            "Passive requests",
            "requests/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibADFS",
            PRIO_ADFS_PASSIVE_REQUESTS,
            update_every,
            RRDSET_TYPE_LINE);

        adfs.rd_adfs_passive_requests_total =
            rrddim_add(adfs.st_adfs_passive_requests_total, "passive", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        adfs.st_adfs_passive_requests_total,
        adfs.rd_adfs_passive_requests_total,
        (collected_number)adfs.ADFSPassiveRequests.current.Data);

    rrdset_done(adfs.st_adfs_passive_requests_total);
}

void netdata_adfs_passport_authentications_chart(
    PERF_DATA_BLOCK *pDataBlock,
    PERF_OBJECT_TYPE *pObjectType,
    int update_every)
{
    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &adfs.ADFSPassportAuthentications)) {
        return;
    }

    if (!adfs.st_adfs_passport_authentications_total) {
        adfs.st_adfs_passport_authentications_total = rrdset_create_localhost(
            "adfs",
            "passport_authentications",
            NULL,
            "auth",
            "adfs.passport_authentications",
            "Microsoft Passport SSO authentications",
            "authentications/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibADFS",
            PRIO_ADFS_PASSPORT_AUTHENTICATOR,
            update_every,
            RRDSET_TYPE_LINE);

        adfs.rd_adfs_passport_authentications_total =
            rrddim_add(adfs.st_adfs_passport_authentications_total, "passport", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        adfs.st_adfs_passport_authentications_total,
        adfs.rd_adfs_passport_authentications_total,
        (collected_number)adfs.ADFSPassportAuthentications.current.Data);

    rrdset_done(adfs.st_adfs_passport_authentications_total);
}

void netdata_adfs_password_change_request(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &adfs.ADFSPasswordChangeRequestsSuccess) ||
        !perflibGetObjectCounter(pDataBlock, pObjectType, &adfs.ADFSPasswordChangeRequestsFailure)) {
        return;
    }

    if (!adfs.st_adfs_password_change_requests) {
        adfs.st_adfs_password_change_requests = rrdset_create_localhost(
            "adfs",
            "password_change_requests",
            NULL,
            "auth",
            "adfs.password_change_requests",
            "Password change requests",
            "requests/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibADFS",
            PRIO_ADFS_PASSWORD_CHANGE_REQUEST,
            update_every,
            RRDSET_TYPE_LINE);

        adfs.rd_adfs_password_change_requests_success =
            rrddim_add(adfs.st_adfs_password_change_requests, "success", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

        adfs.rd_adfs_password_change_requests_failure =
            rrddim_add(adfs.st_adfs_password_change_requests, "failure", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        adfs.st_adfs_password_change_requests,
        adfs.rd_adfs_password_change_requests_success,
        (collected_number)adfs.ADFSPasswordChangeRequestsSuccess.current.Data);

    rrddim_set_by_pointer(
        adfs.st_adfs_password_change_requests,
        adfs.rd_adfs_password_change_requests_failure,
        (collected_number)adfs.ADFSPasswordChangeRequestsFailure.current.Data);

    rrdset_done(adfs.st_adfs_password_change_requests);
}

void netdata_adfs_samlp_token_requests(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &adfs.ADFSSAMLPTokenRequests)) {
        return;
    }

    if (!adfs.st_adfs_samlp_token_requests_success_total) {
        adfs.st_adfs_samlp_token_requests_success_total = rrdset_create_localhost(
            "adfs",
            "samlp_token_requests_success",
            NULL,
            "requests",
            "adfs.samlp_token_requests_success",
            "Successful RP token requests over SAML-P protocol",
            "requests/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibADFS",
            PRIO_ADFS_SAMLP_TOKEN_REQUESTS,
            update_every,
            RRDSET_TYPE_LINE);

        adfs.rd_adfs_samlp_token_requests_success_total = rrddim_add(
            adfs.st_adfs_samlp_token_requests_success_total, "success", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        adfs.st_adfs_samlp_token_requests_success_total,
        adfs.rd_adfs_samlp_token_requests_success_total,
        (collected_number)adfs.ADFSSAMLPTokenRequests.current.Data);

    rrdset_done(adfs.st_adfs_samlp_token_requests_success_total);
}

void netdata_adfs_wstrust_token_requests(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &adfs.ADFSWSTrustTokenRequestsSuccess)) {
        return;
    }

    if (!adfs.st_adfs_wstrust_token_requests_success_total) {
        adfs.st_adfs_wstrust_token_requests_success_total = rrdset_create_localhost(
            "adfs",
            "wstrust_token_requests_success",
            NULL,
            "requests",
            "adfs.wstrust_token_requests_success",
            "Successful RP token requests over WS-Trust protocol",
            "requests/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibADFS",
            PRIO_ADFS_TRUST_TOKEN_SUCCESS,
            update_every,
            RRDSET_TYPE_LINE);

        adfs.rd_adfs_wstrust_token_requests_success_total = rrddim_add(
            adfs.st_adfs_wstrust_token_requests_success_total, "success", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        adfs.st_adfs_wstrust_token_requests_success_total,
        adfs.rd_adfs_wstrust_token_requests_success_total,
        (collected_number)adfs.ADFSWSTrustTokenRequestsSuccess.current.Data);

    rrdset_done(adfs.st_adfs_wstrust_token_requests_success_total);
}

void netdata_adfs_sso_authentications(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &adfs.ADFSSSOAuthenticationsSuccess) ||
        !perflibGetObjectCounter(pDataBlock, pObjectType, &adfs.ADFSSSOAuthenticationsFailure)) {
        return;
    }

    if (!adfs.st_adfs_sso_authentications) {
        adfs.st_adfs_sso_authentications = rrdset_create_localhost(
            "adfs",
            "sso_authentications",
            NULL,
            "auth",
            "adfs.sso_authentications",
            "SSO authentications",
            "authentications/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibADFS",
            PRIO_ADFS_SSO_AUTH,
            update_every,
            RRDSET_TYPE_LINE);

        adfs.rd_adfs_sso_authentications_success =
            rrddim_add(adfs.st_adfs_sso_authentications, "success", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

        adfs.rd_adfs_sso_authentications_failure =
            rrddim_add(adfs.st_adfs_sso_authentications, "failure", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        adfs.st_adfs_sso_authentications,
        adfs.rd_adfs_sso_authentications_success,
        (collected_number)adfs.ADFSSSOAuthenticationsSuccess.current.Data);

    rrddim_set_by_pointer(
        adfs.st_adfs_sso_authentications,
        adfs.rd_adfs_sso_authentications_failure,
        (collected_number)adfs.ADFSSSOAuthenticationsFailure.current.Data);

    rrdset_done(adfs.st_adfs_sso_authentications);
}

void netdata_adfs_token_request(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &adfs.ADFSTokenRequests)) {
        return;
    }

    if (!adfs.st_adfs_token_requests_total) {
        adfs.st_adfs_token_requests_total = rrdset_create_localhost(
            "adfs",
            "token_requests",
            NULL,
            "requests",
            "adfs.token_requests",
            "Token access requests",
            "requests/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibADFS",
            PRIO_ADFS_TRUST_TOKEN_SUCCESS,
            update_every,
            RRDSET_TYPE_LINE);

        adfs.rd_adfs_token_requests_total =
            rrddim_add(adfs.st_adfs_token_requests_total, "requests", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        adfs.st_adfs_token_requests_total,
        adfs.rd_adfs_token_requests_total,
        (collected_number)adfs.ADFSTokenRequests.current.Data);

    rrdset_done(adfs.st_adfs_token_requests_total);
}

void netdata_adfs_user_pass_auth(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &adfs.ADFSUserPasswordAuthenticationsSuccess) ||
        !perflibGetObjectCounter(pDataBlock, pObjectType, &adfs.ADFSUserPasswordAuthenticationsFailure)) {
        return;
    }

    if (!adfs.st_adfs_user_password_authentications) {
        adfs.st_adfs_user_password_authentications = rrdset_create_localhost(
            "adfs",
            "userpassword_authentications",
            NULL,
            "auth",
            "adfs.userpassword_authentications",
            "AD U/P authentications",
            "authentications/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibADFS",
            PRIO_ADFS_USER_PASS_AUTH,
            update_every,
            RRDSET_TYPE_LINE);

        adfs.rd_adfs_user_password_authentications_success =
            rrddim_add(adfs.st_adfs_user_password_authentications, "success", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

        adfs.rd_adfs_user_password_authentications_failure =
            rrddim_add(adfs.st_adfs_user_password_authentications, "failure", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        adfs.st_adfs_user_password_authentications,
        adfs.rd_adfs_user_password_authentications_success,
        (collected_number)adfs.ADFSUserPasswordAuthenticationsSuccess.current.Data);

    rrddim_set_by_pointer(
        adfs.st_adfs_user_password_authentications,
        adfs.rd_adfs_user_password_authentications_failure,
        (collected_number)adfs.ADFSUserPasswordAuthenticationsFailure.current.Data);

    rrdset_done(adfs.st_adfs_user_password_authentications);
}

void netdata_windows_integrated_auth(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &adfs.ADFSWindowsIntegratedAuthentications)) {
        return;
    }

    if (!adfs.st_adfs_windows_integrated_authentications_total) {
        adfs.st_adfs_windows_integrated_authentications_total = rrdset_create_localhost(
            "adfs",
            "windows_integrated_authentications",
            NULL,
            "auth",
            "adfs.windows_integrated_authentications",
            "Windows integrated authentications using Kerberos or NTLM",
            "authentications/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibADFS",
            PRIO_ADFS_TRUST_TOKEN_SUCCESS,
            update_every,
            RRDSET_TYPE_LINE);

        adfs.rd_adfs_windows_integrated_authentications_total = rrddim_add(
            adfs.st_adfs_windows_integrated_authentications_total,
            "authentications",
            NULL,
            1,
            1,
            RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        adfs.st_adfs_windows_integrated_authentications_total,
        adfs.rd_adfs_windows_integrated_authentications_total,
        (collected_number)adfs.ADFSWindowsIntegratedAuthentications.current.Data);

    rrdset_done(adfs.st_adfs_windows_integrated_authentications_total);
}

void netdata_wsfed_token_requests(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &adfs.ADFSWSFedTokenRequestsSuccess)) {
        return;
    }

    if (!adfs.st_adfs_wsfed_token_requests_success_total) {
        adfs.st_adfs_wsfed_token_requests_success_total = rrdset_create_localhost(
            "adfs",
            "wsfed_token_requests_success",
            NULL,
            "requests",
            "adfs.wsfed_token_requests_success",
            "Successful RP token requests over WS-Fed protocol",
            "requests/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibADFS",
            PRIO_ADFS_WSFED_TOKEN_REQUESTS,
            update_every,
            RRDSET_TYPE_LINE);

        adfs.rd_adfs_wsfed_token_requests_success_total = rrddim_add(
            adfs.st_adfs_wsfed_token_requests_success_total, "success", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        adfs.st_adfs_wsfed_token_requests_success_total,
        adfs.rd_adfs_wsfed_token_requests_success_total,
        (collected_number)adfs.ADFSWSFedTokenRequestsSuccess.current.Data);

    rrdset_done(adfs.st_adfs_wsfed_token_requests_success_total);
}

static bool do_ADFS(PERF_DATA_BLOCK *pDataBlock, int update_every)
{
    PERF_OBJECT_TYPE *pObjectType = perflibFindObjectTypeByName(pDataBlock, "AD FS");
    if (!pObjectType)
        return false;

    static void (*doADFS[])(PERF_DATA_BLOCK *, PERF_OBJECT_TYPE *, int) = {
        // ADFS/AD
        netdata_adfs_login_connection_failures,
        netdata_adfs_certificate_authentications,

        // DB Artifacts
        netdata_adfs_db_artifacts_failure,
        netdata_adfs_db_artifacts_query_time_seconds,

        // DB Config
        netdata_adfs_db_config_failure,
        netdata_adfs_db_config_query_time_seconds,

        // Auth
        netdata_adfs_device_authentications,
        netdata_adfs_external_authentications,
        netdata_adfs_federated_authentications,
        netdata_adfs_federation_metadata_authentications,

        // OAuth
        netdata_adfs_oauth_authorization_requests,
        netdata_adfs_oauth_client_authentication,
        netdata_adfs_oauth_client_credentials_requests,
        netdata_adfs_oauth_client_privkey_jwt_authentications,
        netdata_adfs_oauth_client_secret_basic_authentications,
        netdata_adfs_oauth_client_secret_post_authentications,
        netdata_adfs_oauth_client_windows_authentications,
        netdata_adfs_oauth_logon_certificate_request,
        netdata_adfs_oauth_password_grant_requests,
        netdata_adfs_oauth_token_requests_success,

        // Requests
        netdata_adfs_passive_requests_chart,
        netdata_adfs_passport_authentications_chart,
        netdata_adfs_password_change_request,
        netdata_adfs_samlp_token_requests,
        netdata_adfs_wstrust_token_requests,
        netdata_adfs_sso_authentications,
        netdata_adfs_token_request,
        netdata_adfs_user_pass_auth,
        netdata_windows_integrated_auth,
        netdata_wsfed_token_requests,

        // This must be the end
        NULL};

    DWORD i;
    for (i = 0; doADFS[i]; i++) {
        doADFS[i](pDataBlock, pObjectType, update_every);
    }

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
