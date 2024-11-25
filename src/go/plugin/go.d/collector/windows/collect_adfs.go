// SPDX-License-Identifier: GPL-3.0-or-later

package windows

import (
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/prometheus"
)

const (
	metricADFSADLoginConnectionFailuresTotal      = "windows_adfs_ad_login_connection_failures_total"
	metricADFSCertificateAuthenticationsTotal     = "windows_adfs_certificate_authentications_total"
	metricADFSDBArtifactFailureTotal              = "windows_adfs_db_artifact_failure_total"
	metricADFSDBArtifactQueryTimeSeconds          = "windows_adfs_db_artifact_query_time_seconds_total"
	metricADFSDBConfigFailureTotal                = "windows_adfs_db_config_failure_total"
	metricADFSDBQueryTimeSecondsTotal             = "windows_adfs_db_config_query_time_seconds_total"
	metricADFSDeviceAuthenticationsTotal          = "windows_adfs_device_authentications_total"
	metricADFSExternalAuthenticationsFailureTotal = "windows_adfs_external_authentications_failure_total"
	metricADFSExternalAuthenticationsSuccessTotal = "windows_adfs_external_authentications_success_total"
	metricADFSExtranetAccountLockoutsTotal        = "windows_adfs_extranet_account_lockouts_total"
	metricADFSFederatedAuthenticationsTotal       = "windows_adfs_federated_authentications_total"
	metricADFSFederationMetadataRequestsTotal     = "windows_adfs_federation_metadata_requests_total"

	metricADFSOauthAuthorizationRequestsTotal                       = "windows_adfs_oauth_authorization_requests_total"
	metricADFSOauthClientAuthenticationFailureTotal                 = "windows_adfs_oauth_client_authentication_failure_total"
	metricADFSOauthClientAuthenticationSuccessTotal                 = "windows_adfs_oauth_client_authentication_success_total"
	metricADFSOauthClientCredentialsFailureTotal                    = "windows_adfs_oauth_client_credentials_failure_total"
	metricADFSOauthClientCredentialsSuccessTotal                    = "windows_adfs_oauth_client_credentials_success_total"
	metricADFSOauthClientPrivKeyJTWAuthenticationFailureTotal       = "windows_adfs_oauth_client_privkey_jtw_authentication_failure_total"
	metricADFSOauthClientPrivKeyJWTAuthenticationSuccessTotal       = "windows_adfs_oauth_client_privkey_jwt_authentications_success_total"
	metricADFSOauthClientSecretBasicAuthenticationsFailureTotal     = "windows_adfs_oauth_client_secret_basic_authentications_failure_total"
	metricADFSADFSOauthClientSecretBasicAuthenticationsSuccessTotal = "windows_adfs_oauth_client_secret_basic_authentications_success_total"
	metricADFSOauthClientSecretPostAuthenticationsFailureTotal      = "windows_adfs_oauth_client_secret_post_authentications_failure_total"
	metricADFSOauthClientSecretPostAuthenticationsSuccessTotal      = "windows_adfs_oauth_client_secret_post_authentications_success_total"
	metricADFSOauthClientWindowsAuthenticationsFailureTotal         = "windows_adfs_oauth_client_windows_authentications_failure_total"
	metricADFSOauthClientWindowsAuthenticationsSuccessTotal         = "windows_adfs_oauth_client_windows_authentications_success_total"
	metricADFSOauthLogonCertificateRequestsFailureTotal             = "windows_adfs_oauth_logon_certificate_requests_failure_total"
	metricADFSOauthLogonCertificateTokenRequestsSuccessTotal        = "windows_adfs_oauth_logon_certificate_token_requests_success_total"
	metricADFSOauthPasswordGrantRequestsFailureTotal                = "windows_adfs_oauth_password_grant_requests_failure_total"
	metricADFSOauthPasswordGrantRequestsSuccessTotal                = "windows_adfs_oauth_password_grant_requests_success_total"
	metricADFSOauthTokenRequestsSuccessTotal                        = "windows_adfs_oauth_token_requests_success_total"

	metricADFSPassiveRequestsTotal                    = "windows_adfs_passive_requests_total"
	metricADFSPasswortAuthenticationsTotal            = "windows_adfs_passport_authentications_total"
	metricADFSPasswordChangeFailedTotal               = "windows_adfs_password_change_failed_total"
	metricADFSWPasswordChangeSucceededTotal           = "windows_adfs_password_change_succeeded_total"
	metricADFSSamlpTokenRequestsSuccessTotal          = "windows_adfs_samlp_token_requests_success_total"
	metricADFSSSOAuthenticationsFailureTotal          = "windows_adfs_sso_authentications_failure_total"
	metricADFSSSOAuthenticationsSuccessTotal          = "windows_adfs_sso_authentications_success_total"
	metricADFSTokenRequestsTotal                      = "windows_adfs_token_requests_total"
	metricADFSUserPasswordAuthenticationsFailureTotal = "windows_adfs_userpassword_authentications_failure_total"
	metricADFSUserPasswordAuthenticationsSuccessTotal = "windows_adfs_userpassword_authentications_success_total"
	metricADFSWindowsIntegratedAuthenticationsTotal   = "windows_adfs_windows_integrated_authentications_total"
	metricADFSWSFedTokenRequestsSuccessTotal          = "windows_adfs_wsfed_token_requests_success_total"
	metricADFSWSTrustTokenRequestsSuccessTotal        = "windows_adfs_wstrust_token_requests_success_total"
)

var adfsMetrics = []string{
	metricADFSADLoginConnectionFailuresTotal,
	metricADFSCertificateAuthenticationsTotal,
	metricADFSDBArtifactFailureTotal,
	metricADFSDBArtifactQueryTimeSeconds,
	metricADFSDBConfigFailureTotal,
	metricADFSDBQueryTimeSecondsTotal,
	metricADFSDeviceAuthenticationsTotal,
	metricADFSExternalAuthenticationsFailureTotal,
	metricADFSExternalAuthenticationsSuccessTotal,
	metricADFSExtranetAccountLockoutsTotal,
	metricADFSFederatedAuthenticationsTotal,
	metricADFSFederationMetadataRequestsTotal,
	metricADFSOauthAuthorizationRequestsTotal,
	metricADFSOauthClientAuthenticationFailureTotal,
	metricADFSOauthClientAuthenticationSuccessTotal,
	metricADFSOauthClientCredentialsFailureTotal,
	metricADFSOauthClientCredentialsSuccessTotal,
	metricADFSOauthClientPrivKeyJTWAuthenticationFailureTotal,
	metricADFSOauthClientPrivKeyJWTAuthenticationSuccessTotal,
	metricADFSOauthClientSecretBasicAuthenticationsFailureTotal,
	metricADFSADFSOauthClientSecretBasicAuthenticationsSuccessTotal,
	metricADFSOauthClientSecretPostAuthenticationsFailureTotal,
	metricADFSOauthClientSecretPostAuthenticationsSuccessTotal,
	metricADFSOauthClientWindowsAuthenticationsFailureTotal,
	metricADFSOauthClientWindowsAuthenticationsSuccessTotal,
	metricADFSOauthLogonCertificateRequestsFailureTotal,
	metricADFSOauthLogonCertificateTokenRequestsSuccessTotal,
	metricADFSOauthPasswordGrantRequestsFailureTotal,
	metricADFSOauthPasswordGrantRequestsSuccessTotal,
	metricADFSOauthTokenRequestsSuccessTotal,
	metricADFSPassiveRequestsTotal,
	metricADFSPasswortAuthenticationsTotal,
	metricADFSPasswordChangeFailedTotal,
	metricADFSWPasswordChangeSucceededTotal,
	metricADFSSamlpTokenRequestsSuccessTotal,
	metricADFSSSOAuthenticationsFailureTotal,
	metricADFSSSOAuthenticationsSuccessTotal,
	metricADFSTokenRequestsTotal,
	metricADFSUserPasswordAuthenticationsFailureTotal,
	metricADFSUserPasswordAuthenticationsSuccessTotal,
	metricADFSWindowsIntegratedAuthenticationsTotal,
	metricADFSWSFedTokenRequestsSuccessTotal,
	metricADFSWSTrustTokenRequestsSuccessTotal,
}

func (w *Windows) collectADFS(mx map[string]int64, pms prometheus.Series) {
	if !w.cache.collection[collectorADFS] {
		w.cache.collection[collectorADFS] = true
		w.addADFSCharts()
	}

	for _, pm := range pms.FindByNames(adfsMetrics...) {
		name := strings.TrimPrefix(pm.Name(), "windows_")
		v := pm.Value
		if strings.HasSuffix(name, "_seconds_total") {
			v *= precision
		}
		mx[name] = int64(v)
	}
}
