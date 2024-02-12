// SPDX-License-Identifier: GPL-3.0-or-later

package windows

import (
	"strings"

	"github.com/netdata/go.d.plugin/pkg/prometheus"
)

const (
	metricADCSRequestsTotal                         = "windows_adcs_requests_total"
	metricADCSFailedRequestsTotal                   = "windows_adcs_failed_requests_total"
	metricADCSIssuedRequestsTotal                   = "windows_adcs_issued_requests_total"
	metricADCSPendingRequestsTotal                  = "windows_adcs_pending_requests_total"
	metricADCSRequestProcessingTime                 = "windows_adcs_request_processing_time_seconds"
	metricADCSRetrievalsTotal                       = "windows_adcs_retrievals_total"
	metricADCSRetrievalsProcessingTime              = "windows_adcs_retrievals_processing_time_seconds"
	metricADCSRequestCryptoSigningTime              = "windows_adcs_request_cryptographic_signing_time_seconds"
	metricADCSRequestPolicyModuleProcessingTime     = "windows_adcs_request_policy_module_processing_time_seconds"
	metricADCSChallengeResponseResponsesTotal       = "windows_adcs_challenge_responses_total"
	metricADCSChallengeResponseProcessingTime       = "windows_adcs_challenge_response_processing_time_seconds"
	metricADCSSignedCertTimestampListsTotal         = "windows_adcs_signed_certificate_timestamp_lists_total"
	metricADCSSignedCertTimestampListProcessingTime = "windows_adcs_signed_certificate_timestamp_list_processing_time_seconds"
)

func (w *Windows) collectADCS(mx map[string]int64, pms prometheus.Series) {
	pms = pms.FindByNames(
		metricADCSRequestsTotal,
		metricADCSFailedRequestsTotal,
		metricADCSIssuedRequestsTotal,
		metricADCSPendingRequestsTotal,
		metricADCSRequestProcessingTime,
		metricADCSRetrievalsTotal,
		metricADCSRetrievalsProcessingTime,
		metricADCSRequestCryptoSigningTime,
		metricADCSRequestPolicyModuleProcessingTime,
		metricADCSChallengeResponseResponsesTotal,
		metricADCSChallengeResponseProcessingTime,
		metricADCSSignedCertTimestampListsTotal,
		metricADCSSignedCertTimestampListProcessingTime,
	)

	seen := make(map[string]bool)

	for _, pm := range pms {
		if tmpl := pm.Labels.Get("cert_template"); tmpl != "" && tmpl != "_Total" {
			seen[tmpl] = true
			name := strings.TrimPrefix(pm.Name(), "windows_adcs_")
			v := pm.Value
			if strings.HasSuffix(pm.Name(), "_seconds") {
				v *= precision
			}
			mx["adcs_cert_template_"+tmpl+"_"+name] += int64(v)
		}
	}

	for template := range seen {
		if !w.cache.adcs[template] {
			w.cache.adcs[template] = true
			w.addCertificateTemplateCharts(template)
		}
	}
	for template := range w.cache.adcs {
		if !seen[template] {
			delete(w.cache.adcs, template)
			w.removeCertificateTemplateCharts(template)
		}
	}
}
