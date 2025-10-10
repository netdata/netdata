// SPDX-License-Identifier: GPL-3.0-or-later

package vernemq

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/web"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	dataConfigJSON, _ = os.ReadFile("testdata/config.json")
	dataConfigYAML, _ = os.ReadFile("testdata/config.yaml")

	dataVer1101Metrics, _ = os.ReadFile("testdata/v1.10.1/metrics.txt")
	dataVer201Metrics, _  = os.ReadFile("testdata/v2.0.1/metrics.txt")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON":     dataConfigJSON,
		"dataConfigYAML":     dataConfigYAML,
		"dataVer1101Metrics": dataVer1101Metrics,
		"dataVer201Metrics":  dataVer201Metrics,
	} {
		require.NotNil(t, data, name)
	}
}

func TestCollector_ConfigurationSerialize(t *testing.T) {
	module.TestConfigurationSerialize(t, &Collector{}, dataConfigJSON, dataConfigYAML)
}

func TestCollector_Init(t *testing.T) {
	tests := map[string]struct {
		wantFail bool
		config   Config
	}{
		"success with default": {
			wantFail: false,
			config:   New().Config,
		},
		"fail when URL not set": {
			wantFail: true,
			config: Config{
				HTTPConfig: web.HTTPConfig{
					RequestConfig: web.RequestConfig{URL: ""},
				},
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr := New()
			collr.Config = test.config

			if test.wantFail {
				assert.Error(t, collr.Init(context.Background()))
			} else {
				assert.NoError(t, collr.Init(context.Background()))
			}
		})
	}
}

func TestCollector_Charts(t *testing.T) {
	assert.NotNil(t, New().Charts())
}

func TestCollector_Cleanup(t *testing.T) {
	assert.NotPanics(t, func() { New().Cleanup(context.Background()) })
}

func TestCollector_Check(t *testing.T) {
	tests := map[string]struct {
		wantFail bool
		prepare  func(t *testing.T) (collr *Collector, cleanup func())
	}{
		"success on valid response v1.10.1": {
			wantFail: false,
			prepare:  caseOkVer1101,
		},
		"success on valid response v2.0.1": {
			wantFail: false,
			prepare:  caseOkVer201,
		},
		"fail on unexpected Prometheus": {
			wantFail: true,
			prepare:  caseUnexpectedPrometheusMetrics,
		},
		"fail on invalid data response": {
			wantFail: true,
			prepare:  caseInvalidDataResponse,
		},
		"fail on connection refused": {
			wantFail: true,
			prepare:  caseConnectionRefused,
		},
		"fail on 404 response": {
			wantFail: true,
			prepare:  case404,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr, cleanup := test.prepare(t)
			defer cleanup()

			if test.wantFail {
				assert.Error(t, collr.Check(context.Background()))
			} else {
				assert.NoError(t, collr.Check(context.Background()))
			}
		})
	}
}

func TestCollector_Collect(t *testing.T) {
	tests := map[string]struct {
		prepare         func(t *testing.T) (collr *Collector, cleanup func())
		wantNumOfCharts int
		wantMetrics     map[string]int64
	}{
		"success on valid response ver 1.10.1": {
			prepare:         caseOkVer1101,
			wantNumOfCharts: len(nodeChartsTmpl) + len(nodeMqtt4ChartsTmpl) + len(nodeMqtt5ChartsTmpl),
			wantMetrics: map[string]int64{
				"node_VerneMQ@172.17.0.2_bytes_received":                                                        36796908,
				"node_VerneMQ@172.17.0.2_bytes_sent":                                                            23361693,
				"node_VerneMQ@172.17.0.2_client_expired":                                                        0,
				"node_VerneMQ@172.17.0.2_cluster_bytes_dropped":                                                 0,
				"node_VerneMQ@172.17.0.2_cluster_bytes_received":                                                0,
				"node_VerneMQ@172.17.0.2_cluster_bytes_sent":                                                    0,
				"node_VerneMQ@172.17.0.2_mqtt4_client_keepalive_expired":                                        1,
				"node_VerneMQ@172.17.0.2_mqtt4_mqtt_connack_accepted_sent":                                      0,
				"node_VerneMQ@172.17.0.2_mqtt4_mqtt_connack_bad_credentials_sent":                               0,
				"node_VerneMQ@172.17.0.2_mqtt4_mqtt_connack_identifier_rejected_sent":                           0,
				"node_VerneMQ@172.17.0.2_mqtt4_mqtt_connack_not_authorized_sent":                                0,
				"node_VerneMQ@172.17.0.2_mqtt4_mqtt_connack_sent":                                               338956,
				"node_VerneMQ@172.17.0.2_mqtt4_mqtt_connack_sent_return_code_bad_username_or_password":          4,
				"node_VerneMQ@172.17.0.2_mqtt4_mqtt_connack_sent_return_code_client_identifier_not_valid":       0,
				"node_VerneMQ@172.17.0.2_mqtt4_mqtt_connack_sent_return_code_not_authorized":                    4,
				"node_VerneMQ@172.17.0.2_mqtt4_mqtt_connack_sent_return_code_server_unavailable":                0,
				"node_VerneMQ@172.17.0.2_mqtt4_mqtt_connack_sent_return_code_success":                           338948,
				"node_VerneMQ@172.17.0.2_mqtt4_mqtt_connack_sent_return_code_unsupported_protocol_version":      0,
				"node_VerneMQ@172.17.0.2_mqtt4_mqtt_connack_server_unavailable_sent":                            0,
				"node_VerneMQ@172.17.0.2_mqtt4_mqtt_connack_unacceptable_protocol_sent":                         0,
				"node_VerneMQ@172.17.0.2_mqtt4_mqtt_connect_received":                                           338956,
				"node_VerneMQ@172.17.0.2_mqtt4_mqtt_disconnect_received":                                        107,
				"node_VerneMQ@172.17.0.2_mqtt4_mqtt_invalid_msg_size_error":                                     0,
				"node_VerneMQ@172.17.0.2_mqtt4_mqtt_pingreq_received":                                           205,
				"node_VerneMQ@172.17.0.2_mqtt4_mqtt_pingresp_sent":                                              205,
				"node_VerneMQ@172.17.0.2_mqtt4_mqtt_puback_invalid_error":                                       0,
				"node_VerneMQ@172.17.0.2_mqtt4_mqtt_puback_received":                                            525694,
				"node_VerneMQ@172.17.0.2_mqtt4_mqtt_puback_sent":                                                537068,
				"node_VerneMQ@172.17.0.2_mqtt4_mqtt_pubcomp_invalid_error":                                      0,
				"node_VerneMQ@172.17.0.2_mqtt4_mqtt_pubcomp_received":                                           0,
				"node_VerneMQ@172.17.0.2_mqtt4_mqtt_pubcomp_sent":                                               0,
				"node_VerneMQ@172.17.0.2_mqtt4_mqtt_publish_auth_error":                                         0,
				"node_VerneMQ@172.17.0.2_mqtt4_mqtt_publish_error":                                              0,
				"node_VerneMQ@172.17.0.2_mqtt4_mqtt_publish_received":                                           537088,
				"node_VerneMQ@172.17.0.2_mqtt4_mqtt_publish_sent":                                               525721,
				"node_VerneMQ@172.17.0.2_mqtt4_mqtt_pubrec_invalid_error":                                       0,
				"node_VerneMQ@172.17.0.2_mqtt4_mqtt_pubrec_received":                                            0,
				"node_VerneMQ@172.17.0.2_mqtt4_mqtt_pubrec_sent":                                                0,
				"node_VerneMQ@172.17.0.2_mqtt4_mqtt_pubrel_received":                                            0,
				"node_VerneMQ@172.17.0.2_mqtt4_mqtt_pubrel_sent":                                                0,
				"node_VerneMQ@172.17.0.2_mqtt4_mqtt_suback_sent":                                                122,
				"node_VerneMQ@172.17.0.2_mqtt4_mqtt_subscribe_auth_error":                                       0,
				"node_VerneMQ@172.17.0.2_mqtt4_mqtt_subscribe_error":                                            0,
				"node_VerneMQ@172.17.0.2_mqtt4_mqtt_subscribe_received":                                         122,
				"node_VerneMQ@172.17.0.2_mqtt4_mqtt_unsuback_sent":                                              108,
				"node_VerneMQ@172.17.0.2_mqtt4_mqtt_unsubscribe_error":                                          0,
				"node_VerneMQ@172.17.0.2_mqtt4_mqtt_unsubscribe_received":                                       108,
				"node_VerneMQ@172.17.0.2_mqtt5_client_keepalive_expired":                                        0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_auth_received":                                              0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_auth_received_reason_code_continue_authentication":          0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_auth_received_reason_code_reauthenticate":                   0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_auth_received_reason_code_success":                          0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_auth_sent":                                                  0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_auth_sent_reason_code_continue_authentication":              0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_auth_sent_reason_code_reauthenticate":                       0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_auth_sent_reason_code_success":                              0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_connack_sent":                                               0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_connack_sent_reason_code_bad_authentication_method":         0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_connack_sent_reason_code_bad_username_or_password":          0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_connack_sent_reason_code_banned":                            0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_connack_sent_reason_code_client_identifier_not_valid":       0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_connack_sent_reason_code_connection_rate_exceeded":          0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_connack_sent_reason_code_impl_specific_error":               0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_connack_sent_reason_code_malformed_packet":                  0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_connack_sent_reason_code_not_authorized":                    0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_connack_sent_reason_code_packet_too_large":                  0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_connack_sent_reason_code_payload_format_invalid":            0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_connack_sent_reason_code_protocol_error":                    0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_connack_sent_reason_code_qos_not_supported":                 0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_connack_sent_reason_code_quota_exceeded":                    0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_connack_sent_reason_code_retain_not_supported":              0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_connack_sent_reason_code_server_busy":                       0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_connack_sent_reason_code_server_moved":                      0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_connack_sent_reason_code_server_unavailable":                0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_connack_sent_reason_code_success":                           0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_connack_sent_reason_code_topic_name_invalid":                0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_connack_sent_reason_code_unspecified_error":                 0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_connack_sent_reason_code_unsupported_protocol_version":      0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_connack_sent_reason_code_use_another_server":                0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_connect_received":                                           0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_disconnect_received":                                        0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_disconnect_received_reason_code_administrative_action":      0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_disconnect_received_reason_code_disconnect_with_will_msg":   0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_disconnect_received_reason_code_impl_specific_error":        0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_disconnect_received_reason_code_malformed_packet":           0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_disconnect_received_reason_code_message_rate_too_high":      0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_disconnect_received_reason_code_normal_disconnect":          0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_disconnect_received_reason_code_packet_too_large":           0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_disconnect_received_reason_code_payload_format_invalid":     0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_disconnect_received_reason_code_protocol_error":             0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_disconnect_received_reason_code_quota_exceeded":             0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_disconnect_received_reason_code_receive_max_exceeded":       0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_disconnect_received_reason_code_topic_alias_invalid":        0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_disconnect_received_reason_code_topic_name_invalid":         0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_disconnect_received_reason_code_unspecified_error":          0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_disconnect_sent":                                            0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_disconnect_sent_reason_code_administrative_action":          0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_disconnect_sent_reason_code_connection_rate_exceeded":       0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_disconnect_sent_reason_code_impl_specific_error":            0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_disconnect_sent_reason_code_keep_alive_timeout":             0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_disconnect_sent_reason_code_malformed_packet":               0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_disconnect_sent_reason_code_max_connect_time":               0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_disconnect_sent_reason_code_message_rate_too_high":          0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_disconnect_sent_reason_code_normal_disconnect":              0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_disconnect_sent_reason_code_not_authorized":                 0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_disconnect_sent_reason_code_packet_too_large":               0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_disconnect_sent_reason_code_payload_format_invalid":         0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_disconnect_sent_reason_code_protocol_error":                 0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_disconnect_sent_reason_code_qos_not_supported":              0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_disconnect_sent_reason_code_quota_exceeded":                 0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_disconnect_sent_reason_code_receive_max_exceeded":           0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_disconnect_sent_reason_code_retain_not_supported":           0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_disconnect_sent_reason_code_server_busy":                    0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_disconnect_sent_reason_code_server_moved":                   0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_disconnect_sent_reason_code_server_shutting_down":           0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_disconnect_sent_reason_code_session_taken_over":             0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_disconnect_sent_reason_code_shared_subs_not_supported":      0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_disconnect_sent_reason_code_subscription_ids_not_supported": 0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_disconnect_sent_reason_code_topic_alias_invalid":            0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_disconnect_sent_reason_code_topic_filter_invalid":           0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_disconnect_sent_reason_code_topic_name_invalid":             0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_disconnect_sent_reason_code_unspecified_error":              0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_disconnect_sent_reason_code_use_another_server":             0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_disconnect_sent_reason_code_wildcard_subs_not_supported":    0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_invalid_msg_size_error":                                     0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_pingreq_received":                                           0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_pingresp_sent":                                              0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_puback_invalid_error":                                       0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_puback_received":                                            0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_puback_received_reason_code_impl_specific_error":            0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_puback_received_reason_code_no_matching_subscribers":        0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_puback_received_reason_code_not_authorized":                 0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_puback_received_reason_code_packet_id_in_use":               0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_puback_received_reason_code_payload_format_invalid":         0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_puback_received_reason_code_quota_exceeded":                 0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_puback_received_reason_code_success":                        0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_puback_received_reason_code_topic_name_invalid":             0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_puback_received_reason_code_unspecified_error":              0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_puback_sent":                                                0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_puback_sent_reason_code_impl_specific_error":                0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_puback_sent_reason_code_no_matching_subscribers":            0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_puback_sent_reason_code_not_authorized":                     0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_puback_sent_reason_code_packet_id_in_use":                   0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_puback_sent_reason_code_payload_format_invalid":             0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_puback_sent_reason_code_quota_exceeded":                     0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_puback_sent_reason_code_success":                            0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_puback_sent_reason_code_topic_name_invalid":                 0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_puback_sent_reason_code_unspecified_error":                  0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_pubcomp_invalid_error":                                      0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_pubcomp_received":                                           0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_pubcomp_received_reason_code_packet_id_not_found":           0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_pubcomp_received_reason_code_success":                       0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_pubcomp_sent":                                               0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_pubcomp_sent_reason_code_packet_id_not_found":               0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_pubcomp_sent_reason_code_success":                           0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_publish_auth_error":                                         0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_publish_error":                                              0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_publish_received":                                           0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_publish_sent":                                               0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_pubrec_received":                                            0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_pubrec_received_reason_code_impl_specific_error":            0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_pubrec_received_reason_code_no_matching_subscribers":        0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_pubrec_received_reason_code_not_authorized":                 0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_pubrec_received_reason_code_packet_id_in_use":               0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_pubrec_received_reason_code_payload_format_invalid":         0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_pubrec_received_reason_code_quota_exceeded":                 0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_pubrec_received_reason_code_success":                        0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_pubrec_received_reason_code_topic_name_invalid":             0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_pubrec_received_reason_code_unspecified_error":              0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_pubrec_sent":                                                0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_pubrec_sent_reason_code_impl_specific_error":                0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_pubrec_sent_reason_code_no_matching_subscribers":            0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_pubrec_sent_reason_code_not_authorized":                     0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_pubrec_sent_reason_code_packet_id_in_use":                   0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_pubrec_sent_reason_code_payload_format_invalid":             0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_pubrec_sent_reason_code_quota_exceeded":                     0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_pubrec_sent_reason_code_success":                            0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_pubrec_sent_reason_code_topic_name_invalid":                 0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_pubrec_sent_reason_code_unspecified_error":                  0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_pubrel_received":                                            0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_pubrel_received_reason_code_packet_id_not_found":            0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_pubrel_received_reason_code_success":                        0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_pubrel_sent":                                                0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_pubrel_sent_reason_code_packet_id_not_found":                0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_pubrel_sent_reason_code_success":                            0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_suback_sent":                                                0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_subscribe_auth_error":                                       0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_subscribe_error":                                            0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_subscribe_received":                                         0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_unsuback_sent":                                              0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_unsubscribe_error":                                          0,
				"node_VerneMQ@172.17.0.2_mqtt5_mqtt_unsubscribe_received":                                       0,
				"node_VerneMQ@172.17.0.2_netsplit_detected":                                                     0,
				"node_VerneMQ@172.17.0.2_netsplit_resolved":                                                     0,
				"node_VerneMQ@172.17.0.2_netsplit_unresolved":                                                   0,
				"node_VerneMQ@172.17.0.2_open_sockets":                                                          0,
				"node_VerneMQ@172.17.0.2_queue_initialized_from_storage":                                        0,
				"node_VerneMQ@172.17.0.2_queue_message_drop":                                                    0,
				"node_VerneMQ@172.17.0.2_queue_message_expired":                                                 0,
				"node_VerneMQ@172.17.0.2_queue_message_in":                                                      525722,
				"node_VerneMQ@172.17.0.2_queue_message_out":                                                     525721,
				"node_VerneMQ@172.17.0.2_queue_message_unhandled":                                               1,
				"node_VerneMQ@172.17.0.2_queue_processes":                                                       0,
				"node_VerneMQ@172.17.0.2_queue_setup":                                                           338948,
				"node_VerneMQ@172.17.0.2_queue_teardown":                                                        338948,
				"node_VerneMQ@172.17.0.2_queued_messages":                                                       0,
				"node_VerneMQ@172.17.0.2_retain_memory":                                                         11344,
				"node_VerneMQ@172.17.0.2_retain_messages":                                                       0,
				"node_VerneMQ@172.17.0.2_router_matches_local":                                                  525722,
				"node_VerneMQ@172.17.0.2_router_matches_remote":                                                 0,
				"node_VerneMQ@172.17.0.2_router_memory":                                                         12752,
				"node_VerneMQ@172.17.0.2_router_subscriptions":                                                  0,
				"node_VerneMQ@172.17.0.2_socket_close":                                                          338956,
				"node_VerneMQ@172.17.0.2_socket_close_timeout":                                                  0,
				"node_VerneMQ@172.17.0.2_socket_error":                                                          0,
				"node_VerneMQ@172.17.0.2_socket_open":                                                           338956,
				"node_VerneMQ@172.17.0.2_system_context_switches":                                               39088198,
				"node_VerneMQ@172.17.0.2_system_exact_reductions":                                               3854024620,
				"node_VerneMQ@172.17.0.2_system_gc_count":                                                       12189976,
				"node_VerneMQ@172.17.0.2_system_io_in":                                                          68998296,
				"node_VerneMQ@172.17.0.2_system_io_out":                                                         961001488,
				"node_VerneMQ@172.17.0.2_system_process_count":                                                  329,
				"node_VerneMQ@172.17.0.2_system_reductions":                                                     3857458067,
				"node_VerneMQ@172.17.0.2_system_run_queue":                                                      0,
				"node_VerneMQ@172.17.0.2_system_runtime":                                                        1775355,
				"node_VerneMQ@172.17.0.2_system_utilization":                                                    9,
				"node_VerneMQ@172.17.0.2_system_wallclock":                                                      163457858,
				"node_VerneMQ@172.17.0.2_system_words_reclaimed_by_gc":                                          7158470019,
				"node_VerneMQ@172.17.0.2_vm_memory_atom":                                                        768953,
				"node_VerneMQ@172.17.0.2_vm_memory_atom_used":                                                   755998,
				"node_VerneMQ@172.17.0.2_vm_memory_binary":                                                      1293672,
				"node_VerneMQ@172.17.0.2_vm_memory_code":                                                        11372082,
				"node_VerneMQ@172.17.0.2_vm_memory_ets":                                                         6065944,
				"node_VerneMQ@172.17.0.2_vm_memory_processes":                                                   8673288,
				"node_VerneMQ@172.17.0.2_vm_memory_processes_used":                                              8671232,
				"node_VerneMQ@172.17.0.2_vm_memory_system":                                                      27051848,
				"node_VerneMQ@172.17.0.2_vm_memory_total":                                                       35725136,
			},
		},
		"success on valid response ver 2.0.1": {
			prepare:         caseOkVer201,
			wantNumOfCharts: len(nodeChartsTmpl) + len(nodeMqtt4ChartsTmpl) + len(nodeMqtt5ChartsTmpl),
			wantMetrics: map[string]int64{
				"node_VerneMQ@10.10.10.20_active_mqtt_connections":                                               0,
				"node_VerneMQ@10.10.10.20_active_mqttws_connections":                                             0,
				"node_VerneMQ@10.10.10.20_bytes_received":                                                        0,
				"node_VerneMQ@10.10.10.20_bytes_sent":                                                            0,
				"node_VerneMQ@10.10.10.20_client_expired":                                                        0,
				"node_VerneMQ@10.10.10.20_cluster_bytes_dropped":                                                 0,
				"node_VerneMQ@10.10.10.20_cluster_bytes_received":                                                0,
				"node_VerneMQ@10.10.10.20_cluster_bytes_sent":                                                    0,
				"node_VerneMQ@10.10.10.20_mqtt4_client_keepalive_expired":                                        0,
				"node_VerneMQ@10.10.10.20_mqtt4_mqtt_connack_accepted_sent":                                      0,
				"node_VerneMQ@10.10.10.20_mqtt4_mqtt_connack_bad_credentials_sent":                               0,
				"node_VerneMQ@10.10.10.20_mqtt4_mqtt_connack_identifier_rejected_sent":                           0,
				"node_VerneMQ@10.10.10.20_mqtt4_mqtt_connack_not_authorized_sent":                                0,
				"node_VerneMQ@10.10.10.20_mqtt4_mqtt_connack_sent":                                               0,
				"node_VerneMQ@10.10.10.20_mqtt4_mqtt_connack_sent_return_code_bad_username_or_password":          0,
				"node_VerneMQ@10.10.10.20_mqtt4_mqtt_connack_sent_return_code_client_identifier_not_valid":       0,
				"node_VerneMQ@10.10.10.20_mqtt4_mqtt_connack_sent_return_code_not_authorized":                    0,
				"node_VerneMQ@10.10.10.20_mqtt4_mqtt_connack_sent_return_code_server_unavailable":                0,
				"node_VerneMQ@10.10.10.20_mqtt4_mqtt_connack_sent_return_code_success":                           0,
				"node_VerneMQ@10.10.10.20_mqtt4_mqtt_connack_sent_return_code_unsupported_protocol_version":      0,
				"node_VerneMQ@10.10.10.20_mqtt4_mqtt_connack_server_unavailable_sent":                            0,
				"node_VerneMQ@10.10.10.20_mqtt4_mqtt_connack_unacceptable_protocol_sent":                         0,
				"node_VerneMQ@10.10.10.20_mqtt4_mqtt_connect_received":                                           0,
				"node_VerneMQ@10.10.10.20_mqtt4_mqtt_disconnect_received":                                        0,
				"node_VerneMQ@10.10.10.20_mqtt4_mqtt_invalid_msg_size_error":                                     0,
				"node_VerneMQ@10.10.10.20_mqtt4_mqtt_pingreq_received":                                           0,
				"node_VerneMQ@10.10.10.20_mqtt4_mqtt_pingresp_sent":                                              0,
				"node_VerneMQ@10.10.10.20_mqtt4_mqtt_puback_invalid_error":                                       0,
				"node_VerneMQ@10.10.10.20_mqtt4_mqtt_puback_received":                                            0,
				"node_VerneMQ@10.10.10.20_mqtt4_mqtt_puback_sent":                                                0,
				"node_VerneMQ@10.10.10.20_mqtt4_mqtt_pubcomp_invalid_error":                                      0,
				"node_VerneMQ@10.10.10.20_mqtt4_mqtt_pubcomp_received":                                           0,
				"node_VerneMQ@10.10.10.20_mqtt4_mqtt_pubcomp_sent":                                               0,
				"node_VerneMQ@10.10.10.20_mqtt4_mqtt_publish_auth_error":                                         0,
				"node_VerneMQ@10.10.10.20_mqtt4_mqtt_publish_error":                                              0,
				"node_VerneMQ@10.10.10.20_mqtt4_mqtt_publish_received":                                           0,
				"node_VerneMQ@10.10.10.20_mqtt4_mqtt_publish_sent":                                               0,
				"node_VerneMQ@10.10.10.20_mqtt4_mqtt_pubrec_invalid_error":                                       0,
				"node_VerneMQ@10.10.10.20_mqtt4_mqtt_pubrec_received":                                            0,
				"node_VerneMQ@10.10.10.20_mqtt4_mqtt_pubrec_sent":                                                0,
				"node_VerneMQ@10.10.10.20_mqtt4_mqtt_pubrel_received":                                            0,
				"node_VerneMQ@10.10.10.20_mqtt4_mqtt_pubrel_sent":                                                0,
				"node_VerneMQ@10.10.10.20_mqtt4_mqtt_suback_sent":                                                0,
				"node_VerneMQ@10.10.10.20_mqtt4_mqtt_subscribe_auth_error":                                       0,
				"node_VerneMQ@10.10.10.20_mqtt4_mqtt_subscribe_error":                                            0,
				"node_VerneMQ@10.10.10.20_mqtt4_mqtt_subscribe_received":                                         0,
				"node_VerneMQ@10.10.10.20_mqtt4_mqtt_unsuback_sent":                                              0,
				"node_VerneMQ@10.10.10.20_mqtt4_mqtt_unsubscribe_error":                                          0,
				"node_VerneMQ@10.10.10.20_mqtt4_mqtt_unsubscribe_received":                                       0,
				"node_VerneMQ@10.10.10.20_mqtt5_client_keepalive_expired":                                        0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_auth_received":                                              0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_auth_received_reason_code_continue_authentication":          0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_auth_received_reason_code_reauthenticate":                   0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_auth_received_reason_code_success":                          0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_auth_sent":                                                  0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_auth_sent_reason_code_continue_authentication":              0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_auth_sent_reason_code_reauthenticate":                       0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_auth_sent_reason_code_success":                              0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_connack_sent":                                               0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_connack_sent_reason_code_bad_authentication_method":         0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_connack_sent_reason_code_bad_username_or_password":          0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_connack_sent_reason_code_banned":                            0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_connack_sent_reason_code_client_identifier_not_valid":       0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_connack_sent_reason_code_connection_rate_exceeded":          0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_connack_sent_reason_code_impl_specific_error":               0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_connack_sent_reason_code_malformed_packet":                  0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_connack_sent_reason_code_not_authorized":                    0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_connack_sent_reason_code_packet_too_large":                  0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_connack_sent_reason_code_payload_format_invalid":            0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_connack_sent_reason_code_protocol_error":                    0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_connack_sent_reason_code_qos_not_supported":                 0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_connack_sent_reason_code_quota_exceeded":                    0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_connack_sent_reason_code_retain_not_supported":              0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_connack_sent_reason_code_server_busy":                       0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_connack_sent_reason_code_server_moved":                      0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_connack_sent_reason_code_server_unavailable":                0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_connack_sent_reason_code_success":                           0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_connack_sent_reason_code_topic_name_invalid":                0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_connack_sent_reason_code_unspecified_error":                 0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_connack_sent_reason_code_unsupported_protocol_version":      0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_connack_sent_reason_code_use_another_server":                0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_connect_received":                                           0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_disconnect_received":                                        0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_disconnect_received_reason_code_administrative_action":      0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_disconnect_received_reason_code_disconnect_with_will_msg":   0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_disconnect_received_reason_code_impl_specific_error":        0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_disconnect_received_reason_code_malformed_packet":           0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_disconnect_received_reason_code_message_rate_too_high":      0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_disconnect_received_reason_code_normal_disconnect":          0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_disconnect_received_reason_code_packet_too_large":           0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_disconnect_received_reason_code_payload_format_invalid":     0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_disconnect_received_reason_code_protocol_error":             0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_disconnect_received_reason_code_quota_exceeded":             0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_disconnect_received_reason_code_receive_max_exceeded":       0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_disconnect_received_reason_code_topic_alias_invalid":        0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_disconnect_received_reason_code_topic_name_invalid":         0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_disconnect_received_reason_code_unspecified_error":          0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_disconnect_sent":                                            0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_disconnect_sent_reason_code_administrative_action":          0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_disconnect_sent_reason_code_connection_rate_exceeded":       0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_disconnect_sent_reason_code_impl_specific_error":            0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_disconnect_sent_reason_code_keep_alive_timeout":             0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_disconnect_sent_reason_code_malformed_packet":               0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_disconnect_sent_reason_code_max_connect_time":               0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_disconnect_sent_reason_code_message_rate_too_high":          0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_disconnect_sent_reason_code_normal_disconnect":              0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_disconnect_sent_reason_code_not_authorized":                 0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_disconnect_sent_reason_code_packet_too_large":               0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_disconnect_sent_reason_code_payload_format_invalid":         0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_disconnect_sent_reason_code_protocol_error":                 0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_disconnect_sent_reason_code_qos_not_supported":              0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_disconnect_sent_reason_code_quota_exceeded":                 0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_disconnect_sent_reason_code_receive_max_exceeded":           0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_disconnect_sent_reason_code_retain_not_supported":           0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_disconnect_sent_reason_code_server_busy":                    0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_disconnect_sent_reason_code_server_moved":                   0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_disconnect_sent_reason_code_server_shutting_down":           0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_disconnect_sent_reason_code_session_taken_over":             0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_disconnect_sent_reason_code_shared_subs_not_supported":      0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_disconnect_sent_reason_code_subscription_ids_not_supported": 0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_disconnect_sent_reason_code_topic_alias_invalid":            0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_disconnect_sent_reason_code_topic_filter_invalid":           0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_disconnect_sent_reason_code_topic_name_invalid":             0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_disconnect_sent_reason_code_unspecified_error":              0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_disconnect_sent_reason_code_use_another_server":             0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_disconnect_sent_reason_code_wildcard_subs_not_supported":    0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_invalid_msg_size_error":                                     0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_pingreq_received":                                           0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_pingresp_sent":                                              0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_puback_invalid_error":                                       0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_puback_received":                                            0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_puback_received_reason_code_impl_specific_error":            0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_puback_received_reason_code_no_matching_subscribers":        0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_puback_received_reason_code_not_authorized":                 0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_puback_received_reason_code_packet_id_in_use":               0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_puback_received_reason_code_payload_format_invalid":         0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_puback_received_reason_code_quota_exceeded":                 0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_puback_received_reason_code_success":                        0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_puback_received_reason_code_topic_name_invalid":             0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_puback_received_reason_code_unspecified_error":              0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_puback_sent":                                                0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_puback_sent_reason_code_impl_specific_error":                0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_puback_sent_reason_code_no_matching_subscribers":            0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_puback_sent_reason_code_not_authorized":                     0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_puback_sent_reason_code_packet_id_in_use":                   0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_puback_sent_reason_code_payload_format_invalid":             0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_puback_sent_reason_code_quota_exceeded":                     0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_puback_sent_reason_code_success":                            0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_puback_sent_reason_code_topic_name_invalid":                 0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_puback_sent_reason_code_unspecified_error":                  0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_pubcomp_invalid_error":                                      0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_pubcomp_received":                                           0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_pubcomp_received_reason_code_packet_id_not_found":           0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_pubcomp_received_reason_code_success":                       0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_pubcomp_sent":                                               0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_pubcomp_sent_reason_code_packet_id_not_found":               0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_pubcomp_sent_reason_code_success":                           0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_publish_auth_error":                                         0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_publish_error":                                              0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_publish_received":                                           0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_publish_sent":                                               0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_pubrec_received":                                            0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_pubrec_received_reason_code_impl_specific_error":            0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_pubrec_received_reason_code_no_matching_subscribers":        0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_pubrec_received_reason_code_not_authorized":                 0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_pubrec_received_reason_code_packet_id_in_use":               0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_pubrec_received_reason_code_payload_format_invalid":         0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_pubrec_received_reason_code_quota_exceeded":                 0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_pubrec_received_reason_code_success":                        0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_pubrec_received_reason_code_topic_name_invalid":             0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_pubrec_received_reason_code_unspecified_error":              0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_pubrec_sent":                                                0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_pubrec_sent_reason_code_impl_specific_error":                0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_pubrec_sent_reason_code_no_matching_subscribers":            0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_pubrec_sent_reason_code_not_authorized":                     0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_pubrec_sent_reason_code_packet_id_in_use":                   0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_pubrec_sent_reason_code_payload_format_invalid":             0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_pubrec_sent_reason_code_quota_exceeded":                     0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_pubrec_sent_reason_code_success":                            0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_pubrec_sent_reason_code_topic_name_invalid":                 0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_pubrec_sent_reason_code_unspecified_error":                  0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_pubrel_received":                                            0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_pubrel_received_reason_code_packet_id_not_found":            0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_pubrel_received_reason_code_success":                        0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_pubrel_sent":                                                0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_pubrel_sent_reason_code_packet_id_not_found":                0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_pubrel_sent_reason_code_success":                            0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_suback_sent":                                                0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_subscribe_auth_error":                                       0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_subscribe_error":                                            0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_subscribe_received":                                         0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_unsuback_sent":                                              0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_unsubscribe_error":                                          0,
				"node_VerneMQ@10.10.10.20_mqtt5_mqtt_unsubscribe_received":                                       0,
				"node_VerneMQ@10.10.10.20_netsplit_detected":                                                     0,
				"node_VerneMQ@10.10.10.20_netsplit_resolved":                                                     0,
				"node_VerneMQ@10.10.10.20_netsplit_unresolved":                                                   0,
				"node_VerneMQ@10.10.10.20_open_sockets":                                                          0,
				"node_VerneMQ@10.10.10.20_queue_initialized_from_storage":                                        0,
				"node_VerneMQ@10.10.10.20_queue_message_drop":                                                    0,
				"node_VerneMQ@10.10.10.20_queue_message_expired":                                                 0,
				"node_VerneMQ@10.10.10.20_queue_message_in":                                                      0,
				"node_VerneMQ@10.10.10.20_queue_message_out":                                                     0,
				"node_VerneMQ@10.10.10.20_queue_message_unhandled":                                               0,
				"node_VerneMQ@10.10.10.20_queue_processes":                                                       0,
				"node_VerneMQ@10.10.10.20_queue_setup":                                                           0,
				"node_VerneMQ@10.10.10.20_queue_teardown":                                                        0,
				"node_VerneMQ@10.10.10.20_queued_messages":                                                       0,
				"node_VerneMQ@10.10.10.20_retain_memory":                                                         15792,
				"node_VerneMQ@10.10.10.20_retain_messages":                                                       0,
				"node_VerneMQ@10.10.10.20_router_matches_local":                                                  0,
				"node_VerneMQ@10.10.10.20_router_matches_remote":                                                 0,
				"node_VerneMQ@10.10.10.20_router_memory":                                                         20224,
				"node_VerneMQ@10.10.10.20_router_subscriptions":                                                  0,
				"node_VerneMQ@10.10.10.20_socket_close":                                                          0,
				"node_VerneMQ@10.10.10.20_socket_close_timeout":                                                  0,
				"node_VerneMQ@10.10.10.20_socket_error":                                                          0,
				"node_VerneMQ@10.10.10.20_socket_open":                                                           0,
				"node_VerneMQ@10.10.10.20_system_context_switches":                                               3902972,
				"node_VerneMQ@10.10.10.20_system_exact_reductions":                                               340307987,
				"node_VerneMQ@10.10.10.20_system_gc_count":                                                       355236,
				"node_VerneMQ@10.10.10.20_system_io_in":                                                          30616669,
				"node_VerneMQ@10.10.10.20_system_io_out":                                                         1702500,
				"node_VerneMQ@10.10.10.20_system_process_count":                                                  465,
				"node_VerneMQ@10.10.10.20_system_reductions":                                                     340640218,
				"node_VerneMQ@10.10.10.20_system_run_queue":                                                      0,
				"node_VerneMQ@10.10.10.20_system_runtime":                                                        54249,
				"node_VerneMQ@10.10.10.20_system_utilization":                                                    5,
				"node_VerneMQ@10.10.10.20_system_wallclock":                                                      51082602,
				"node_VerneMQ@10.10.10.20_system_words_reclaimed_by_gc":                                          661531058,
				"node_VerneMQ@10.10.10.20_total_active_connections":                                              0,
				"node_VerneMQ@10.10.10.20_vm_memory_atom":                                                        639193,
				"node_VerneMQ@10.10.10.20_vm_memory_atom_used":                                                   634383,
				"node_VerneMQ@10.10.10.20_vm_memory_binary":                                                      729792,
				"node_VerneMQ@10.10.10.20_vm_memory_code":                                                        13546686,
				"node_VerneMQ@10.10.10.20_vm_memory_ets":                                                         6665000,
				"node_VerneMQ@10.10.10.20_vm_memory_processes":                                                   13754320,
				"node_VerneMQ@10.10.10.20_vm_memory_processes_used":                                              13751224,
				"node_VerneMQ@10.10.10.20_vm_memory_system":                                                      44839192,
				"node_VerneMQ@10.10.10.20_vm_memory_total":                                                       58593512,
			},
		},
		"fails on unexpected Prometheus response": {
			prepare:     caseUnexpectedPrometheusMetrics,
			wantMetrics: nil,
		},
		"fails on invalid data response": {
			prepare:     caseInvalidDataResponse,
			wantMetrics: nil,
		},
		"fails on connection refused": {
			prepare:     caseConnectionRefused,
			wantMetrics: nil,
		},
		"fails on 404 response": {
			prepare:     case404,
			wantMetrics: nil,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr, cleanup := test.prepare(t)
			defer cleanup()

			mx := collr.Collect(context.Background())

			require.Equal(t, test.wantMetrics, mx)

			if len(test.wantMetrics) > 0 {
				assert.Equal(t, test.wantNumOfCharts, len(*collr.Charts()), "want charts")

				module.TestMetricsHasAllChartsDims(t, collr.Charts(), mx)
			}
		})
	}
}

func caseOkVer201(t *testing.T) (*Collector, func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write(dataVer201Metrics)
		}))

	collr := New()
	collr.URL = srv.URL
	require.NoError(t, collr.Init(context.Background()))

	return collr, srv.Close
}

func caseOkVer1101(t *testing.T) (*Collector, func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write(dataVer1101Metrics)
		}))

	collr := New()
	collr.URL = srv.URL
	require.NoError(t, collr.Init(context.Background()))

	return collr, srv.Close
}

func caseUnexpectedPrometheusMetrics(t *testing.T) (*Collector, func()) {
	data := `
# HELP wmi_os_process_memory_limix_bytes OperatingSystem.MaxProcessMemorySize
# TYPE wmi_os_process_memory_limix_bytes gauge
wmi_os_process_memory_limix_bytes 1.40737488224256e+14
# HELP wmi_os_processes OperatingSystem.NumberOfProcesses
# TYPE wmi_os_processes gauge
wmi_os_processes 124
# HELP wmi_os_processes_limit OperatingSystem.MaxNumberOfProcesses
# TYPE wmi_os_processes_limit gauge
wmi_os_processes_limit 4.294967295e+09
`
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(data))
		}))

	collr := New()
	collr.URL = srv.URL
	require.NoError(t, collr.Init(context.Background()))

	return collr, srv.Close
}

func caseInvalidDataResponse(t *testing.T) (*Collector, func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("hello and\n goodbye"))
		}))
	collr := New()
	collr.URL = srv.URL
	require.NoError(t, collr.Init(context.Background()))

	return collr, srv.Close
}

func caseConnectionRefused(t *testing.T) (*Collector, func()) {
	t.Helper()
	collr := New()
	collr.URL = "http://127.0.0.1:65001"
	require.NoError(t, collr.Init(context.Background()))

	return collr, func() {}
}

func case404(t *testing.T) (*Collector, func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))

	collr := New()
	collr.URL = srv.URL
	require.NoError(t, collr.Init(context.Background()))

	return collr, srv.Close
}
