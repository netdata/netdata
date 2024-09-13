// SPDX-License-Identifier: GPL-3.0-or-later

package vernemq

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	dataConfigJSON, _ = os.ReadFile("testdata/config.json")
	dataConfigYAML, _ = os.ReadFile("testdata/config.yaml")

	dataVer1101MQTTv5Metrics, _ = os.ReadFile("testdata/metrics-v1.10.1-mqtt5.txt")
	dataUnexpectedMetrics, _    = os.ReadFile("testdata/non_vernemq.txt")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON":           dataConfigJSON,
		"dataConfigYAML":           dataConfigYAML,
		"dataVer1101MQTTv5Metrics": dataVer1101MQTTv5Metrics,
		"dataUnexpectedMetrics":    dataUnexpectedMetrics,
	} {
		require.NotNil(t, data, name)
	}
}

func TestVerneMQ_ConfigurationSerialize(t *testing.T) {
	module.TestConfigurationSerialize(t, &VerneMQ{}, dataConfigJSON, dataConfigYAML)
}

func TestVerneMQ_Init(t *testing.T) {
	verneMQ := prepareVerneMQ()

	assert.NoError(t, verneMQ.Init())
}

func TestVerneMQ_Init_ReturnsFalseIfURLIsNotSet(t *testing.T) {
	verneMQ := prepareVerneMQ()
	verneMQ.URL = ""

	assert.Error(t, verneMQ.Init())
}

func TestVerneMQ_Init_ReturnsFalseIfClientWrongTLSCA(t *testing.T) {
	verneMQ := prepareVerneMQ()
	verneMQ.ClientConfig.TLSConfig.TLSCA = "testdata/tls"

	assert.Error(t, verneMQ.Init())
}

func TestVerneMQ_Check(t *testing.T) {
	verneMQ, srv := prepareClientServerV1101(t)
	defer srv.Close()

	assert.NoError(t, verneMQ.Check())
}

func TestVerneMQ_Check_ReturnsFalseIfConnectionRefused(t *testing.T) {
	verneMQ := prepareVerneMQ()
	require.NoError(t, verneMQ.Init())

	assert.Error(t, verneMQ.Check())
}

func TestVerneMQ_Check_ReturnsFalseIfMetricsAreNotVerneMQ(t *testing.T) {
	verneMQ, srv := prepareClientServerNotVerneMQ(t)
	defer srv.Close()
	require.NoError(t, verneMQ.Init())

	assert.Error(t, verneMQ.Check())
}

func TestVerneMQ_Charts(t *testing.T) {
	assert.NotNil(t, New().Charts())
}

func TestVerneMQ_Cleanup(t *testing.T) {
	assert.NotPanics(t, New().Cleanup)
}

func TestVerneMQ_Collect(t *testing.T) {
	verneMQ, srv := prepareClientServerV1101(t)
	defer srv.Close()

	mx := verneMQ.Collect()

	assert.Equal(t, v1101ExpectedMetrics, mx)

	module.TestMetricsHasAllChartsDims(t, verneMQ.Charts(), mx)
}

func TestVerneMQ_Collect_ReturnsNilIfConnectionRefused(t *testing.T) {
	verneMQ := prepareVerneMQ()
	require.NoError(t, verneMQ.Init())

	assert.Nil(t, verneMQ.Collect())
}

func TestVerneMQ_Collect_ReturnsNilIfMetricsAreNotVerneMQ(t *testing.T) {
	verneMQ, srv := prepareClientServerNotVerneMQ(t)
	defer srv.Close()

	assert.Nil(t, verneMQ.Collect())
}

func TestVerneMQ_Collect_ReturnsNilIfReceiveInvalidResponse(t *testing.T) {
	verneMQ, ts := prepareClientServerInvalid(t)
	defer ts.Close()

	assert.Nil(t, verneMQ.Collect())
}

func TestVerneMQ_Collect_ReturnsNilIfReceiveResponse404(t *testing.T) {
	verneMQ, ts := prepareClientServerResponse404(t)
	defer ts.Close()

	assert.Nil(t, verneMQ.Collect())
}

func prepareVerneMQ() *VerneMQ {
	verneMQ := New()
	verneMQ.URL = "http://127.0.0.1:38001/metrics"
	return verneMQ
}

func prepareClientServerV1101(t *testing.T) (*VerneMQ, *httptest.Server) {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write(dataVer1101MQTTv5Metrics)
		}))

	verneMQ := New()
	verneMQ.URL = ts.URL
	require.NoError(t, verneMQ.Init())

	return verneMQ, ts
}

func prepareClientServerNotVerneMQ(t *testing.T) (*VerneMQ, *httptest.Server) {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write(dataUnexpectedMetrics)
		}))

	verneMQ := New()
	verneMQ.URL = ts.URL
	require.NoError(t, verneMQ.Init())

	return verneMQ, ts
}

func prepareClientServerInvalid(t *testing.T) (*VerneMQ, *httptest.Server) {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("hello and\n goodbye"))
		}))

	verneMQ := New()
	verneMQ.URL = ts.URL
	require.NoError(t, verneMQ.Init())

	return verneMQ, ts
}

func prepareClientServerResponse404(t *testing.T) (*VerneMQ, *httptest.Server) {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))

	verneMQ := New()
	verneMQ.URL = ts.URL
	require.NoError(t, verneMQ.Init())
	return verneMQ, ts
}

var v1101ExpectedMetrics = map[string]int64{
	"bytes_received":                                          36796908,
	"bytes_sent":                                              23361693,
	"client_keepalive_expired":                                1,
	"cluster_bytes_dropped":                                   0,
	"cluster_bytes_received":                                  0,
	"cluster_bytes_sent":                                      0,
	"mqtt_auth_received":                                      0,
	"mqtt_auth_received_continue_authentication":              0,
	"mqtt_auth_received_reauthenticate":                       0,
	"mqtt_auth_received_success":                              0,
	"mqtt_auth_received_v_5":                                  0,
	"mqtt_auth_received_v_5_continue_authentication":          0,
	"mqtt_auth_received_v_5_reauthenticate":                   0,
	"mqtt_auth_received_v_5_success":                          0,
	"mqtt_auth_sent":                                          0,
	"mqtt_auth_sent_continue_authentication":                  0,
	"mqtt_auth_sent_reauthenticate":                           0,
	"mqtt_auth_sent_success":                                  0,
	"mqtt_auth_sent_v_5":                                      0,
	"mqtt_auth_sent_v_5_continue_authentication":              0,
	"mqtt_auth_sent_v_5_reauthenticate":                       0,
	"mqtt_auth_sent_v_5_success":                              0,
	"mqtt_connack_sent":                                       338956,
	"mqtt_connack_sent_bad_authentication_method":             0,
	"mqtt_connack_sent_bad_username_or_password":              4,
	"mqtt_connack_sent_banned":                                0,
	"mqtt_connack_sent_client_identifier_not_valid":           0,
	"mqtt_connack_sent_connection_rate_exceeded":              0,
	"mqtt_connack_sent_impl_specific_error":                   0,
	"mqtt_connack_sent_malformed_packet":                      0,
	"mqtt_connack_sent_not_authorized":                        4,
	"mqtt_connack_sent_packet_too_large":                      0,
	"mqtt_connack_sent_payload_format_invalid":                0,
	"mqtt_connack_sent_protocol_error":                        0,
	"mqtt_connack_sent_qos_not_supported":                     0,
	"mqtt_connack_sent_quota_exceeded":                        0,
	"mqtt_connack_sent_retain_not_supported":                  0,
	"mqtt_connack_sent_server_busy":                           0,
	"mqtt_connack_sent_server_moved":                          0,
	"mqtt_connack_sent_server_unavailable":                    0,
	"mqtt_connack_sent_success":                               338948,
	"mqtt_connack_sent_topic_name_invalid":                    0,
	"mqtt_connack_sent_unspecified_error":                     0,
	"mqtt_connack_sent_unsupported_protocol_version":          0,
	"mqtt_connack_sent_use_another_server":                    0,
	"mqtt_connack_sent_v_4":                                   338956,
	"mqtt_connack_sent_v_4_bad_username_or_password":          4,
	"mqtt_connack_sent_v_4_client_identifier_not_valid":       0,
	"mqtt_connack_sent_v_4_not_authorized":                    4,
	"mqtt_connack_sent_v_4_server_unavailable":                0,
	"mqtt_connack_sent_v_4_success":                           338948,
	"mqtt_connack_sent_v_4_unsupported_protocol_version":      0,
	"mqtt_connack_sent_v_5":                                   0,
	"mqtt_connack_sent_v_5_bad_authentication_method":         0,
	"mqtt_connack_sent_v_5_bad_username_or_password":          0,
	"mqtt_connack_sent_v_5_banned":                            0,
	"mqtt_connack_sent_v_5_client_identifier_not_valid":       0,
	"mqtt_connack_sent_v_5_connection_rate_exceeded":          0,
	"mqtt_connack_sent_v_5_impl_specific_error":               0,
	"mqtt_connack_sent_v_5_malformed_packet":                  0,
	"mqtt_connack_sent_v_5_not_authorized":                    0,
	"mqtt_connack_sent_v_5_packet_too_large":                  0,
	"mqtt_connack_sent_v_5_payload_format_invalid":            0,
	"mqtt_connack_sent_v_5_protocol_error":                    0,
	"mqtt_connack_sent_v_5_qos_not_supported":                 0,
	"mqtt_connack_sent_v_5_quota_exceeded":                    0,
	"mqtt_connack_sent_v_5_retain_not_supported":              0,
	"mqtt_connack_sent_v_5_server_busy":                       0,
	"mqtt_connack_sent_v_5_server_moved":                      0,
	"mqtt_connack_sent_v_5_server_unavailable":                0,
	"mqtt_connack_sent_v_5_success":                           0,
	"mqtt_connack_sent_v_5_topic_name_invalid":                0,
	"mqtt_connack_sent_v_5_unspecified_error":                 0,
	"mqtt_connack_sent_v_5_unsupported_protocol_version":      0,
	"mqtt_connack_sent_v_5_use_another_server":                0,
	"mqtt_connect_received":                                   338956,
	"mqtt_connect_received_v_4":                               338956,
	"mqtt_connect_received_v_5":                               0,
	"mqtt_disconnect_received":                                107,
	"mqtt_disconnect_received_administrative_action":          0,
	"mqtt_disconnect_received_disconnect_with_will_msg":       0,
	"mqtt_disconnect_received_impl_specific_error":            0,
	"mqtt_disconnect_received_malformed_packet":               0,
	"mqtt_disconnect_received_message_rate_too_high":          0,
	"mqtt_disconnect_received_normal_disconnect":              0,
	"mqtt_disconnect_received_packet_too_large":               0,
	"mqtt_disconnect_received_payload_format_invalid":         0,
	"mqtt_disconnect_received_protocol_error":                 0,
	"mqtt_disconnect_received_quota_exceeded":                 0,
	"mqtt_disconnect_received_receive_max_exceeded":           0,
	"mqtt_disconnect_received_topic_alias_invalid":            0,
	"mqtt_disconnect_received_topic_name_invalid":             0,
	"mqtt_disconnect_received_unspecified_error":              0,
	"mqtt_disconnect_received_v_4":                            107,
	"mqtt_disconnect_received_v_5":                            0,
	"mqtt_disconnect_received_v_5_administrative_action":      0,
	"mqtt_disconnect_received_v_5_disconnect_with_will_msg":   0,
	"mqtt_disconnect_received_v_5_impl_specific_error":        0,
	"mqtt_disconnect_received_v_5_malformed_packet":           0,
	"mqtt_disconnect_received_v_5_message_rate_too_high":      0,
	"mqtt_disconnect_received_v_5_normal_disconnect":          0,
	"mqtt_disconnect_received_v_5_packet_too_large":           0,
	"mqtt_disconnect_received_v_5_payload_format_invalid":     0,
	"mqtt_disconnect_received_v_5_protocol_error":             0,
	"mqtt_disconnect_received_v_5_quota_exceeded":             0,
	"mqtt_disconnect_received_v_5_receive_max_exceeded":       0,
	"mqtt_disconnect_received_v_5_topic_alias_invalid":        0,
	"mqtt_disconnect_received_v_5_topic_name_invalid":         0,
	"mqtt_disconnect_received_v_5_unspecified_error":          0,
	"mqtt_disconnect_sent":                                    0,
	"mqtt_disconnect_sent_administrative_action":              0,
	"mqtt_disconnect_sent_connection_rate_exceeded":           0,
	"mqtt_disconnect_sent_impl_specific_error":                0,
	"mqtt_disconnect_sent_keep_alive_timeout":                 0,
	"mqtt_disconnect_sent_malformed_packet":                   0,
	"mqtt_disconnect_sent_max_connect_time":                   0,
	"mqtt_disconnect_sent_message_rate_too_high":              0,
	"mqtt_disconnect_sent_normal_disconnect":                  0,
	"mqtt_disconnect_sent_not_authorized":                     0,
	"mqtt_disconnect_sent_packet_too_large":                   0,
	"mqtt_disconnect_sent_payload_format_invalid":             0,
	"mqtt_disconnect_sent_protocol_error":                     0,
	"mqtt_disconnect_sent_qos_not_supported":                  0,
	"mqtt_disconnect_sent_quota_exceeded":                     0,
	"mqtt_disconnect_sent_receive_max_exceeded":               0,
	"mqtt_disconnect_sent_retain_not_supported":               0,
	"mqtt_disconnect_sent_server_busy":                        0,
	"mqtt_disconnect_sent_server_moved":                       0,
	"mqtt_disconnect_sent_server_shutting_down":               0,
	"mqtt_disconnect_sent_session_taken_over":                 0,
	"mqtt_disconnect_sent_shared_subs_not_supported":          0,
	"mqtt_disconnect_sent_subscription_ids_not_supported":     0,
	"mqtt_disconnect_sent_topic_alias_invalid":                0,
	"mqtt_disconnect_sent_topic_filter_invalid":               0,
	"mqtt_disconnect_sent_topic_name_invalid":                 0,
	"mqtt_disconnect_sent_unspecified_error":                  0,
	"mqtt_disconnect_sent_use_another_server":                 0,
	"mqtt_disconnect_sent_v_5":                                0,
	"mqtt_disconnect_sent_v_5_administrative_action":          0,
	"mqtt_disconnect_sent_v_5_connection_rate_exceeded":       0,
	"mqtt_disconnect_sent_v_5_impl_specific_error":            0,
	"mqtt_disconnect_sent_v_5_keep_alive_timeout":             0,
	"mqtt_disconnect_sent_v_5_malformed_packet":               0,
	"mqtt_disconnect_sent_v_5_max_connect_time":               0,
	"mqtt_disconnect_sent_v_5_message_rate_too_high":          0,
	"mqtt_disconnect_sent_v_5_normal_disconnect":              0,
	"mqtt_disconnect_sent_v_5_not_authorized":                 0,
	"mqtt_disconnect_sent_v_5_packet_too_large":               0,
	"mqtt_disconnect_sent_v_5_payload_format_invalid":         0,
	"mqtt_disconnect_sent_v_5_protocol_error":                 0,
	"mqtt_disconnect_sent_v_5_qos_not_supported":              0,
	"mqtt_disconnect_sent_v_5_quota_exceeded":                 0,
	"mqtt_disconnect_sent_v_5_receive_max_exceeded":           0,
	"mqtt_disconnect_sent_v_5_retain_not_supported":           0,
	"mqtt_disconnect_sent_v_5_server_busy":                    0,
	"mqtt_disconnect_sent_v_5_server_moved":                   0,
	"mqtt_disconnect_sent_v_5_server_shutting_down":           0,
	"mqtt_disconnect_sent_v_5_session_taken_over":             0,
	"mqtt_disconnect_sent_v_5_shared_subs_not_supported":      0,
	"mqtt_disconnect_sent_v_5_subscription_ids_not_supported": 0,
	"mqtt_disconnect_sent_v_5_topic_alias_invalid":            0,
	"mqtt_disconnect_sent_v_5_topic_filter_invalid":           0,
	"mqtt_disconnect_sent_v_5_topic_name_invalid":             0,
	"mqtt_disconnect_sent_v_5_unspecified_error":              0,
	"mqtt_disconnect_sent_v_5_use_another_server":             0,
	"mqtt_disconnect_sent_v_5_wildcard_subs_not_supported":    0,
	"mqtt_disconnect_sent_wildcard_subs_not_supported":        0,
	"mqtt_invalid_msg_size_error":                             0,
	"mqtt_invalid_msg_size_error_v_4":                         0,
	"mqtt_invalid_msg_size_error_v_5":                         0,
	"mqtt_pingreq_received":                                   205,
	"mqtt_pingreq_received_v_4":                               205,
	"mqtt_pingreq_received_v_5":                               0,
	"mqtt_pingresp_sent":                                      205,
	"mqtt_pingresp_sent_v_4":                                  205,
	"mqtt_pingresp_sent_v_5":                                  0,
	"mqtt_puback_invalid_error":                               0,
	"mqtt_puback_invalid_error_v_4":                           0,
	"mqtt_puback_invalid_error_v_5":                           0,
	"mqtt_puback_received":                                    525694,
	"mqtt_puback_received_impl_specific_error":                0,
	"mqtt_puback_received_no_matching_subscribers":            0,
	"mqtt_puback_received_not_authorized":                     0,
	"mqtt_puback_received_packet_id_in_use":                   0,
	"mqtt_puback_received_payload_format_invalid":             0,
	"mqtt_puback_received_quota_exceeded":                     0,
	"mqtt_puback_received_success":                            0,
	"mqtt_puback_received_topic_name_invalid":                 0,
	"mqtt_puback_received_unspecified_error":                  0,
	"mqtt_puback_received_v_4":                                525694,
	"mqtt_puback_received_v_5":                                0,
	"mqtt_puback_received_v_5_impl_specific_error":            0,
	"mqtt_puback_received_v_5_no_matching_subscribers":        0,
	"mqtt_puback_received_v_5_not_authorized":                 0,
	"mqtt_puback_received_v_5_packet_id_in_use":               0,
	"mqtt_puback_received_v_5_payload_format_invalid":         0,
	"mqtt_puback_received_v_5_quota_exceeded":                 0,
	"mqtt_puback_received_v_5_success":                        0,
	"mqtt_puback_received_v_5_topic_name_invalid":             0,
	"mqtt_puback_received_v_5_unspecified_error":              0,
	"mqtt_puback_sent":                                        537068,
	"mqtt_puback_sent_impl_specific_error":                    0,
	"mqtt_puback_sent_no_matching_subscribers":                0,
	"mqtt_puback_sent_not_authorized":                         0,
	"mqtt_puback_sent_packet_id_in_use":                       0,
	"mqtt_puback_sent_payload_format_invalid":                 0,
	"mqtt_puback_sent_quota_exceeded":                         0,
	"mqtt_puback_sent_success":                                0,
	"mqtt_puback_sent_topic_name_invalid":                     0,
	"mqtt_puback_sent_unspecified_error":                      0,
	"mqtt_puback_sent_v_4":                                    537068,
	"mqtt_puback_sent_v_5":                                    0,
	"mqtt_puback_sent_v_5_impl_specific_error":                0,
	"mqtt_puback_sent_v_5_no_matching_subscribers":            0,
	"mqtt_puback_sent_v_5_not_authorized":                     0,
	"mqtt_puback_sent_v_5_packet_id_in_use":                   0,
	"mqtt_puback_sent_v_5_payload_format_invalid":             0,
	"mqtt_puback_sent_v_5_quota_exceeded":                     0,
	"mqtt_puback_sent_v_5_success":                            0,
	"mqtt_puback_sent_v_5_topic_name_invalid":                 0,
	"mqtt_puback_sent_v_5_unspecified_error":                  0,
	"mqtt_pubcomp_invalid_error":                              0,
	"mqtt_pubcomp_invalid_error_v_4":                          0,
	"mqtt_pubcomp_invalid_error_v_5":                          0,
	"mqtt_pubcomp_received":                                   0,
	"mqtt_pubcomp_received_packet_id_not_found":               0,
	"mqtt_pubcomp_received_success":                           0,
	"mqtt_pubcomp_received_v_4":                               0,
	"mqtt_pubcomp_received_v_5":                               0,
	"mqtt_pubcomp_received_v_5_packet_id_not_found":           0,
	"mqtt_pubcomp_received_v_5_success":                       0,
	"mqtt_pubcomp_sent":                                       0,
	"mqtt_pubcomp_sent_packet_id_not_found":                   0,
	"mqtt_pubcomp_sent_success":                               0,
	"mqtt_pubcomp_sent_v_4":                                   0,
	"mqtt_pubcomp_sent_v_5":                                   0,
	"mqtt_pubcomp_sent_v_5_packet_id_not_found":               0,
	"mqtt_pubcomp_sent_v_5_success":                           0,
	"mqtt_publish_auth_error":                                 0,
	"mqtt_publish_auth_error_v_4":                             0,
	"mqtt_publish_auth_error_v_5":                             0,
	"mqtt_publish_error":                                      0,
	"mqtt_publish_error_v_4":                                  0,
	"mqtt_publish_error_v_5":                                  0,
	"mqtt_publish_received":                                   537088,
	"mqtt_publish_received_v_4":                               537088,
	"mqtt_publish_received_v_5":                               0,
	"mqtt_publish_sent":                                       525721,
	"mqtt_publish_sent_v_4":                                   525721,
	"mqtt_publish_sent_v_5":                                   0,
	"mqtt_pubrec_invalid_error":                               0,
	"mqtt_pubrec_invalid_error_v_4":                           0,
	"mqtt_pubrec_received":                                    0,
	"mqtt_pubrec_received_impl_specific_error":                0,
	"mqtt_pubrec_received_no_matching_subscribers":            0,
	"mqtt_pubrec_received_not_authorized":                     0,
	"mqtt_pubrec_received_packet_id_in_use":                   0,
	"mqtt_pubrec_received_payload_format_invalid":             0,
	"mqtt_pubrec_received_quota_exceeded":                     0,
	"mqtt_pubrec_received_success":                            0,
	"mqtt_pubrec_received_topic_name_invalid":                 0,
	"mqtt_pubrec_received_unspecified_error":                  0,
	"mqtt_pubrec_received_v_4":                                0,
	"mqtt_pubrec_received_v_5":                                0,
	"mqtt_pubrec_received_v_5_impl_specific_error":            0,
	"mqtt_pubrec_received_v_5_no_matching_subscribers":        0,
	"mqtt_pubrec_received_v_5_not_authorized":                 0,
	"mqtt_pubrec_received_v_5_packet_id_in_use":               0,
	"mqtt_pubrec_received_v_5_payload_format_invalid":         0,
	"mqtt_pubrec_received_v_5_quota_exceeded":                 0,
	"mqtt_pubrec_received_v_5_success":                        0,
	"mqtt_pubrec_received_v_5_topic_name_invalid":             0,
	"mqtt_pubrec_received_v_5_unspecified_error":              0,
	"mqtt_pubrec_sent":                                        0,
	"mqtt_pubrec_sent_impl_specific_error":                    0,
	"mqtt_pubrec_sent_no_matching_subscribers":                0,
	"mqtt_pubrec_sent_not_authorized":                         0,
	"mqtt_pubrec_sent_packet_id_in_use":                       0,
	"mqtt_pubrec_sent_payload_format_invalid":                 0,
	"mqtt_pubrec_sent_quota_exceeded":                         0,
	"mqtt_pubrec_sent_success":                                0,
	"mqtt_pubrec_sent_topic_name_invalid":                     0,
	"mqtt_pubrec_sent_unspecified_error":                      0,
	"mqtt_pubrec_sent_v_4":                                    0,
	"mqtt_pubrec_sent_v_5":                                    0,
	"mqtt_pubrec_sent_v_5_impl_specific_error":                0,
	"mqtt_pubrec_sent_v_5_no_matching_subscribers":            0,
	"mqtt_pubrec_sent_v_5_not_authorized":                     0,
	"mqtt_pubrec_sent_v_5_packet_id_in_use":                   0,
	"mqtt_pubrec_sent_v_5_payload_format_invalid":             0,
	"mqtt_pubrec_sent_v_5_quota_exceeded":                     0,
	"mqtt_pubrec_sent_v_5_success":                            0,
	"mqtt_pubrec_sent_v_5_topic_name_invalid":                 0,
	"mqtt_pubrec_sent_v_5_unspecified_error":                  0,
	"mqtt_pubrel_received":                                    0,
	"mqtt_pubrel_received_packet_id_not_found":                0,
	"mqtt_pubrel_received_success":                            0,
	"mqtt_pubrel_received_v_4":                                0,
	"mqtt_pubrel_received_v_5":                                0,
	"mqtt_pubrel_received_v_5_packet_id_not_found":            0,
	"mqtt_pubrel_received_v_5_success":                        0,
	"mqtt_pubrel_sent":                                        0,
	"mqtt_pubrel_sent_packet_id_not_found":                    0,
	"mqtt_pubrel_sent_success":                                0,
	"mqtt_pubrel_sent_v_4":                                    0,
	"mqtt_pubrel_sent_v_5":                                    0,
	"mqtt_pubrel_sent_v_5_packet_id_not_found":                0,
	"mqtt_pubrel_sent_v_5_success":                            0,
	"mqtt_suback_sent":                                        122,
	"mqtt_suback_sent_v_4":                                    122,
	"mqtt_suback_sent_v_5":                                    0,
	"mqtt_subscribe_auth_error":                               0,
	"mqtt_subscribe_auth_error_v_4":                           0,
	"mqtt_subscribe_auth_error_v_5":                           0,
	"mqtt_subscribe_error":                                    0,
	"mqtt_subscribe_error_v_4":                                0,
	"mqtt_subscribe_error_v_5":                                0,
	"mqtt_subscribe_received":                                 122,
	"mqtt_subscribe_received_v_4":                             122,
	"mqtt_subscribe_received_v_5":                             0,
	"mqtt_unsuback_sent":                                      108,
	"mqtt_unsuback_sent_v_4":                                  108,
	"mqtt_unsuback_sent_v_5":                                  0,
	"mqtt_unsubscribe_error":                                  0,
	"mqtt_unsubscribe_error_v_4":                              0,
	"mqtt_unsubscribe_error_v_5":                              0,
	"mqtt_unsubscribe_received":                               108,
	"mqtt_unsubscribe_received_v_4":                           108,
	"mqtt_unsubscribe_received_v_5":                           0,
	"netsplit_detected":                                       0,
	"netsplit_resolved":                                       0,
	"netsplit_unresolved":                                     0,
	"open_sockets":                                            0,
	"queue_initialized_from_storage":                          0,
	"queue_message_drop":                                      0,
	"queue_message_expired":                                   0,
	"queue_message_in":                                        525722,
	"queue_message_out":                                       525721,
	"queue_message_unhandled":                                 1,
	"queue_processes":                                         0,
	"queue_setup":                                             338948,
	"queue_teardown":                                          338948,
	"retain_memory":                                           11344,
	"retain_messages":                                         0,
	"router_matches_local":                                    525722,
	"router_matches_remote":                                   0,
	"router_memory":                                           12752,
	"router_subscriptions":                                    0,
	"socket_close":                                            338956,
	"socket_close_timeout":                                    0,
	"socket_error":                                            0,
	"socket_open":                                             338956,
	"system_context_switches":                                 39088198,
	"system_gc_count":                                         12189976,
	"system_io_in":                                            68998296,
	"system_io_out":                                           961001488,
	"system_process_count":                                    329,
	"system_reductions":                                       3857458067,
	"system_run_queue":                                        0,
	"system_utilization":                                      9,
	"system_utilization_scheduler_1":                          34,
	"system_utilization_scheduler_2":                          8,
	"system_utilization_scheduler_3":                          14,
	"system_utilization_scheduler_4":                          19,
	"system_utilization_scheduler_5":                          0,
	"system_utilization_scheduler_6":                          0,
	"system_utilization_scheduler_7":                          0,
	"system_utilization_scheduler_8":                          0,
	"system_wallclock":                                        163457858,
	"system_words_reclaimed_by_gc":                            7158470019,
	"vm_memory_processes":                                     8673288,
	"vm_memory_system":                                        27051848,
}
