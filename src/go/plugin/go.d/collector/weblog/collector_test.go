// SPDX-License-Identifier: GPL-3.0-or-later

package weblog

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/logs"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/metrix"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	dataConfigJSON, _ = os.ReadFile("testdata/config.json")
	dataConfigYAML, _ = os.ReadFile("testdata/config.yaml")

	dataCommonLog, _          = os.ReadFile("testdata/common.log")
	dataFullLog, _            = os.ReadFile("testdata/full.log")
	dataCustomLog, _          = os.ReadFile("testdata/custom.log")
	dataCustomTimeFieldLog, _ = os.ReadFile("testdata/custom_time_fields.log")
	dataIISLog, _             = os.ReadFile("testdata/u_ex221107.log")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON":         dataConfigJSON,
		"dataConfigYAML":         dataConfigYAML,
		"dataCommonLog":          dataCommonLog,
		"dataFullLog":            dataFullLog,
		"dataCustomLog":          dataCustomLog,
		"dataCustomTimeFieldLog": dataCustomTimeFieldLog,
		"dataIISLog":             dataIISLog,
	} {
		require.NotNil(t, data, name)
	}
}

func TestCollector_ConfigurationSerialize(t *testing.T) {
	module.TestConfigurationSerialize(t, &Collector{}, dataConfigJSON, dataConfigYAML)
}

func TestCollector_Init(t *testing.T) {
	collr := New()

	assert.NoError(t, collr.Init(context.Background()))
}

func TestCollector_Init_ErrorOnCreatingURLPatterns(t *testing.T) {
	collr := New()
	collr.URLPatterns = []userPattern{{Match: "* !*"}}

	assert.Error(t, collr.Init(context.Background()))
}

func TestCollector_Init_ErrorOnCreatingCustomFields(t *testing.T) {
	collr := New()
	collr.CustomFields = []customField{{Patterns: []userPattern{{Name: "p1", Match: "* !*"}}}}

	assert.Error(t, collr.Init(context.Background()))
}

func TestCollector_Check(t *testing.T) {
	collr := New()
	defer collr.Cleanup(context.Background())
	collr.Path = "testdata/common.log"
	require.NoError(t, collr.Init(context.Background()))

	assert.NoError(t, collr.Check(context.Background()))
}

func TestCollector_Check_ErrorOnCreatingLogReaderNoLogFile(t *testing.T) {
	collr := New()
	defer collr.Cleanup(context.Background())
	collr.Path = "testdata/not_exists.log"
	require.NoError(t, collr.Init(context.Background()))

	assert.Error(t, collr.Check(context.Background()))
}

func TestCollector_Check_ErrorOnCreatingParserUnknownFormat(t *testing.T) {
	collr := New()
	defer collr.Cleanup(context.Background())
	collr.Path = "testdata/custom.log"
	require.NoError(t, collr.Init(context.Background()))

	assert.Error(t, collr.Check(context.Background()))
}

func TestCollector_Check_ErrorOnCreatingParserEmptyLine(t *testing.T) {
	collr := New()
	defer collr.Cleanup(context.Background())
	collr.Path = "testdata/custom.log"
	collr.ParserConfig.LogType = logs.TypeCSV
	collr.ParserConfig.CSV.Format = "$one $two"
	require.NoError(t, collr.Init(context.Background()))

	assert.Error(t, collr.Check(context.Background()))
}

func TestCollector_Charts(t *testing.T) {
	collr := New()
	defer collr.Cleanup(context.Background())
	collr.Path = "testdata/common.log"
	require.NoError(t, collr.Init(context.Background()))
	require.NoError(t, collr.Check(context.Background()))

	assert.NotNil(t, collr.Charts())
}

func TestCollector_Cleanup(t *testing.T) {
	New().Cleanup(context.Background())
}

func TestCollector_Collect(t *testing.T) {
	collr := prepareWebLogCollectFull(t)

	//mx := collr.Collect()
	//l := make([]string, 0)
	//for k := range mx {
	//	l = append(l, k)
	//}
	//sort.Strings(l)
	//for _, value := range l {
	//	fmt.Println(fmt.Sprintf("\"%s\": %d,", value, mx[value]))
	//}

	expected := map[string]int64{
		"bytes_received":                                          1374096,
		"bytes_sent":                                              1373185,
		"custom_field_drink_beer":                                 221,
		"custom_field_drink_wine":                                 231,
		"custom_field_side_dark":                                  231,
		"custom_field_side_light":                                 221,
		"custom_time_field_random_time_field_time_avg":            230,
		"custom_time_field_random_time_field_time_count":          452,
		"custom_time_field_random_time_field_time_hist_bucket_1":  452,
		"custom_time_field_random_time_field_time_hist_bucket_10": 452,
		"custom_time_field_random_time_field_time_hist_bucket_11": 452,
		"custom_time_field_random_time_field_time_hist_bucket_2":  452,
		"custom_time_field_random_time_field_time_hist_bucket_3":  452,
		"custom_time_field_random_time_field_time_hist_bucket_4":  452,
		"custom_time_field_random_time_field_time_hist_bucket_5":  452,
		"custom_time_field_random_time_field_time_hist_bucket_6":  452,
		"custom_time_field_random_time_field_time_hist_bucket_7":  452,
		"custom_time_field_random_time_field_time_hist_bucket_8":  452,
		"custom_time_field_random_time_field_time_hist_bucket_9":  452,
		"custom_time_field_random_time_field_time_hist_count":     452,
		"custom_time_field_random_time_field_time_hist_sum":       103960,
		"custom_time_field_random_time_field_time_max":            230,
		"custom_time_field_random_time_field_time_min":            230,
		"custom_time_field_random_time_field_time_sum":            103960,
		"req_http_scheme":                                         218,
		"req_https_scheme":                                        234,
		"req_ipv4":                                                275,
		"req_ipv6":                                                177,
		"req_method_GET":                                          156,
		"req_method_HEAD":                                         150,
		"req_method_POST":                                         146,
		"req_port_80":                                             96,
		"req_port_81":                                             100,
		"req_port_82":                                             84,
		"req_port_83":                                             85,
		"req_port_84":                                             87,
		"req_proc_time_avg":                                       244,
		"req_proc_time_count":                                     402,
		"req_proc_time_hist_bucket_1":                             402,
		"req_proc_time_hist_bucket_10":                            402,
		"req_proc_time_hist_bucket_11":                            402,
		"req_proc_time_hist_bucket_2":                             402,
		"req_proc_time_hist_bucket_3":                             402,
		"req_proc_time_hist_bucket_4":                             402,
		"req_proc_time_hist_bucket_5":                             402,
		"req_proc_time_hist_bucket_6":                             402,
		"req_proc_time_hist_bucket_7":                             402,
		"req_proc_time_hist_bucket_8":                             402,
		"req_proc_time_hist_bucket_9":                             402,
		"req_proc_time_hist_count":                                402,
		"req_proc_time_hist_sum":                                  98312,
		"req_proc_time_max":                                       497,
		"req_proc_time_min":                                       2,
		"req_proc_time_sum":                                       98312,
		"req_ssl_cipher_suite_AES256-SHA":                         101,
		"req_ssl_cipher_suite_DHE-RSA-AES256-SHA":                 111,
		"req_ssl_cipher_suite_ECDHE-RSA-AES256-SHA":               127,
		"req_ssl_cipher_suite_PSK-RC4-SHA":                        113,
		"req_ssl_proto_SSLv2":                                     74,
		"req_ssl_proto_SSLv3":                                     57,
		"req_ssl_proto_TLSv1":                                     76,
		"req_ssl_proto_TLSv1.1":                                   87,
		"req_ssl_proto_TLSv1.2":                                   73,
		"req_ssl_proto_TLSv1.3":                                   85,
		"req_type_bad":                                            49,
		"req_type_error":                                          0,
		"req_type_redirect":                                       119,
		"req_type_success":                                        284,
		"req_unmatched":                                           48,
		"req_url_ptn_com":                                         120,
		"req_url_ptn_net":                                         116,
		"req_url_ptn_not_match":                                   0,
		"req_url_ptn_org":                                         113,
		"req_version_1.1":                                         168,
		"req_version_2":                                           143,
		"req_version_2.0":                                         141,
		"req_vhost_198.51.100.1":                                  81,
		"req_vhost_2001:db8:1ce::1":                               100,
		"req_vhost_localhost":                                     102,
		"req_vhost_test.example.com":                              87,
		"req_vhost_test.example.org":                              82,
		"requests":                                                500,
		"resp_1xx":                                                110,
		"resp_2xx":                                                128,
		"resp_3xx":                                                119,
		"resp_4xx":                                                95,
		"resp_5xx":                                                0,
		"resp_code_100":                                           60,
		"resp_code_101":                                           50,
		"resp_code_200":                                           58,
		"resp_code_201":                                           70,
		"resp_code_300":                                           58,
		"resp_code_301":                                           61,
		"resp_code_400":                                           49,
		"resp_code_401":                                           46,
		"uniq_ipv4":                                               3,
		"uniq_ipv6":                                               2,
		"upstream_resp_time_avg":                                  255,
		"upstream_resp_time_count":                                452,
		"upstream_resp_time_hist_bucket_1":                        452,
		"upstream_resp_time_hist_bucket_10":                       452,
		"upstream_resp_time_hist_bucket_11":                       452,
		"upstream_resp_time_hist_bucket_2":                        452,
		"upstream_resp_time_hist_bucket_3":                        452,
		"upstream_resp_time_hist_bucket_4":                        452,
		"upstream_resp_time_hist_bucket_5":                        452,
		"upstream_resp_time_hist_bucket_6":                        452,
		"upstream_resp_time_hist_bucket_7":                        452,
		"upstream_resp_time_hist_bucket_8":                        452,
		"upstream_resp_time_hist_bucket_9":                        452,
		"upstream_resp_time_hist_count":                           452,
		"upstream_resp_time_hist_sum":                             115615,
		"upstream_resp_time_max":                                  497,
		"upstream_resp_time_min":                                  7,
		"upstream_resp_time_sum":                                  115615,
		"url_ptn_com_bytes_received":                              379864,
		"url_ptn_com_bytes_sent":                                  372669,
		"url_ptn_com_req_method_GET":                              38,
		"url_ptn_com_req_method_HEAD":                             39,
		"url_ptn_com_req_method_POST":                             43,
		"url_ptn_com_req_proc_time_avg":                           209,
		"url_ptn_com_req_proc_time_count":                         105,
		"url_ptn_com_req_proc_time_max":                           495,
		"url_ptn_com_req_proc_time_min":                           5,
		"url_ptn_com_req_proc_time_sum":                           22010,
		"url_ptn_com_resp_code_100":                               12,
		"url_ptn_com_resp_code_101":                               15,
		"url_ptn_com_resp_code_200":                               13,
		"url_ptn_com_resp_code_201":                               26,
		"url_ptn_com_resp_code_300":                               16,
		"url_ptn_com_resp_code_301":                               12,
		"url_ptn_com_resp_code_400":                               13,
		"url_ptn_com_resp_code_401":                               13,
		"url_ptn_net_bytes_received":                              349988,
		"url_ptn_net_bytes_sent":                                  339867,
		"url_ptn_net_req_method_GET":                              51,
		"url_ptn_net_req_method_HEAD":                             33,
		"url_ptn_net_req_method_POST":                             32,
		"url_ptn_net_req_proc_time_avg":                           254,
		"url_ptn_net_req_proc_time_count":                         104,
		"url_ptn_net_req_proc_time_max":                           497,
		"url_ptn_net_req_proc_time_min":                           10,
		"url_ptn_net_req_proc_time_sum":                           26510,
		"url_ptn_net_resp_code_100":                               16,
		"url_ptn_net_resp_code_101":                               12,
		"url_ptn_net_resp_code_200":                               16,
		"url_ptn_net_resp_code_201":                               14,
		"url_ptn_net_resp_code_300":                               14,
		"url_ptn_net_resp_code_301":                               17,
		"url_ptn_net_resp_code_400":                               14,
		"url_ptn_net_resp_code_401":                               13,
		"url_ptn_not_match_bytes_received":                        0,
		"url_ptn_not_match_bytes_sent":                            0,
		"url_ptn_not_match_req_proc_time_avg":                     0,
		"url_ptn_not_match_req_proc_time_count":                   0,
		"url_ptn_not_match_req_proc_time_max":                     0,
		"url_ptn_not_match_req_proc_time_min":                     0,
		"url_ptn_not_match_req_proc_time_sum":                     0,
		"url_ptn_org_bytes_received":                              331836,
		"url_ptn_org_bytes_sent":                                  340095,
		"url_ptn_org_req_method_GET":                              29,
		"url_ptn_org_req_method_HEAD":                             46,
		"url_ptn_org_req_method_POST":                             38,
		"url_ptn_org_req_proc_time_avg":                           260,
		"url_ptn_org_req_proc_time_count":                         102,
		"url_ptn_org_req_proc_time_max":                           497,
		"url_ptn_org_req_proc_time_min":                           2,
		"url_ptn_org_req_proc_time_sum":                           26599,
		"url_ptn_org_resp_code_100":                               15,
		"url_ptn_org_resp_code_101":                               11,
		"url_ptn_org_resp_code_200":                               20,
		"url_ptn_org_resp_code_201":                               16,
		"url_ptn_org_resp_code_300":                               10,
		"url_ptn_org_resp_code_301":                               19,
		"url_ptn_org_resp_code_400":                               13,
		"url_ptn_org_resp_code_401":                               9,
	}

	mx := collr.Collect(context.Background())
	assert.Equal(t, expected, mx)
	testCharts(t, collr, mx)
}

func TestCollector_Collect_CommonLogFormat(t *testing.T) {
	collr := prepareWebLogCollectCommon(t)

	expected := map[string]int64{
		"bytes_received":                    0,
		"bytes_sent":                        1388056,
		"req_http_scheme":                   0,
		"req_https_scheme":                  0,
		"req_ipv4":                          283,
		"req_ipv6":                          173,
		"req_method_GET":                    159,
		"req_method_HEAD":                   143,
		"req_method_POST":                   154,
		"req_proc_time_avg":                 0,
		"req_proc_time_count":               0,
		"req_proc_time_hist_bucket_1":       0,
		"req_proc_time_hist_bucket_10":      0,
		"req_proc_time_hist_bucket_11":      0,
		"req_proc_time_hist_bucket_2":       0,
		"req_proc_time_hist_bucket_3":       0,
		"req_proc_time_hist_bucket_4":       0,
		"req_proc_time_hist_bucket_5":       0,
		"req_proc_time_hist_bucket_6":       0,
		"req_proc_time_hist_bucket_7":       0,
		"req_proc_time_hist_bucket_8":       0,
		"req_proc_time_hist_bucket_9":       0,
		"req_proc_time_hist_count":          0,
		"req_proc_time_hist_sum":            0,
		"req_proc_time_max":                 0,
		"req_proc_time_min":                 0,
		"req_proc_time_sum":                 0,
		"req_type_bad":                      54,
		"req_type_error":                    0,
		"req_type_redirect":                 122,
		"req_type_success":                  280,
		"req_unmatched":                     44,
		"req_version_1.1":                   155,
		"req_version_2":                     147,
		"req_version_2.0":                   154,
		"requests":                          500,
		"resp_1xx":                          130,
		"resp_2xx":                          100,
		"resp_3xx":                          122,
		"resp_4xx":                          104,
		"resp_5xx":                          0,
		"resp_code_100":                     80,
		"resp_code_101":                     50,
		"resp_code_200":                     43,
		"resp_code_201":                     57,
		"resp_code_300":                     70,
		"resp_code_301":                     52,
		"resp_code_400":                     54,
		"resp_code_401":                     50,
		"uniq_ipv4":                         3,
		"uniq_ipv6":                         2,
		"upstream_resp_time_avg":            0,
		"upstream_resp_time_count":          0,
		"upstream_resp_time_hist_bucket_1":  0,
		"upstream_resp_time_hist_bucket_10": 0,
		"upstream_resp_time_hist_bucket_11": 0,
		"upstream_resp_time_hist_bucket_2":  0,
		"upstream_resp_time_hist_bucket_3":  0,
		"upstream_resp_time_hist_bucket_4":  0,
		"upstream_resp_time_hist_bucket_5":  0,
		"upstream_resp_time_hist_bucket_6":  0,
		"upstream_resp_time_hist_bucket_7":  0,
		"upstream_resp_time_hist_bucket_8":  0,
		"upstream_resp_time_hist_bucket_9":  0,
		"upstream_resp_time_hist_count":     0,
		"upstream_resp_time_hist_sum":       0,
		"upstream_resp_time_max":            0,
		"upstream_resp_time_min":            0,
		"upstream_resp_time_sum":            0,
	}

	mx := collr.Collect(context.Background())
	assert.Equal(t, expected, mx)
	testCharts(t, collr, mx)
}

func TestCollector_Collect_CustomLogs(t *testing.T) {
	collr := prepareWebLogCollectCustom(t)

	expected := map[string]int64{
		"bytes_received":                    0,
		"bytes_sent":                        0,
		"custom_field_drink_beer":           52,
		"custom_field_drink_wine":           40,
		"custom_field_side_dark":            46,
		"custom_field_side_light":           46,
		"req_http_scheme":                   0,
		"req_https_scheme":                  0,
		"req_ipv4":                          0,
		"req_ipv6":                          0,
		"req_proc_time_avg":                 0,
		"req_proc_time_count":               0,
		"req_proc_time_hist_bucket_1":       0,
		"req_proc_time_hist_bucket_10":      0,
		"req_proc_time_hist_bucket_11":      0,
		"req_proc_time_hist_bucket_2":       0,
		"req_proc_time_hist_bucket_3":       0,
		"req_proc_time_hist_bucket_4":       0,
		"req_proc_time_hist_bucket_5":       0,
		"req_proc_time_hist_bucket_6":       0,
		"req_proc_time_hist_bucket_7":       0,
		"req_proc_time_hist_bucket_8":       0,
		"req_proc_time_hist_bucket_9":       0,
		"req_proc_time_hist_count":          0,
		"req_proc_time_hist_sum":            0,
		"req_proc_time_max":                 0,
		"req_proc_time_min":                 0,
		"req_proc_time_sum":                 0,
		"req_type_bad":                      0,
		"req_type_error":                    0,
		"req_type_redirect":                 0,
		"req_type_success":                  0,
		"req_unmatched":                     8,
		"requests":                          100,
		"resp_1xx":                          0,
		"resp_2xx":                          0,
		"resp_3xx":                          0,
		"resp_4xx":                          0,
		"resp_5xx":                          0,
		"uniq_ipv4":                         0,
		"uniq_ipv6":                         0,
		"upstream_resp_time_avg":            0,
		"upstream_resp_time_count":          0,
		"upstream_resp_time_hist_bucket_1":  0,
		"upstream_resp_time_hist_bucket_10": 0,
		"upstream_resp_time_hist_bucket_11": 0,
		"upstream_resp_time_hist_bucket_2":  0,
		"upstream_resp_time_hist_bucket_3":  0,
		"upstream_resp_time_hist_bucket_4":  0,
		"upstream_resp_time_hist_bucket_5":  0,
		"upstream_resp_time_hist_bucket_6":  0,
		"upstream_resp_time_hist_bucket_7":  0,
		"upstream_resp_time_hist_bucket_8":  0,
		"upstream_resp_time_hist_bucket_9":  0,
		"upstream_resp_time_hist_count":     0,
		"upstream_resp_time_hist_sum":       0,
		"upstream_resp_time_max":            0,
		"upstream_resp_time_min":            0,
		"upstream_resp_time_sum":            0,
	}

	mx := collr.Collect(context.Background())
	assert.Equal(t, expected, mx)
	testCharts(t, collr, mx)
}

func TestCollector_Collect_CustomTimeFieldsLogs(t *testing.T) {
	collr := prepareWebLogCollectCustomTimeFields(t)

	expected := map[string]int64{
		"bytes_received":                              0,
		"bytes_sent":                                  0,
		"custom_time_field_time1_time_avg":            224,
		"custom_time_field_time1_time_count":          72,
		"custom_time_field_time1_time_hist_bucket_1":  72,
		"custom_time_field_time1_time_hist_bucket_10": 72,
		"custom_time_field_time1_time_hist_bucket_11": 72,
		"custom_time_field_time1_time_hist_bucket_2":  72,
		"custom_time_field_time1_time_hist_bucket_3":  72,
		"custom_time_field_time1_time_hist_bucket_4":  72,
		"custom_time_field_time1_time_hist_bucket_5":  72,
		"custom_time_field_time1_time_hist_bucket_6":  72,
		"custom_time_field_time1_time_hist_bucket_7":  72,
		"custom_time_field_time1_time_hist_bucket_8":  72,
		"custom_time_field_time1_time_hist_bucket_9":  72,
		"custom_time_field_time1_time_hist_count":     72,
		"custom_time_field_time1_time_hist_sum":       16152,
		"custom_time_field_time1_time_max":            431,
		"custom_time_field_time1_time_min":            121,
		"custom_time_field_time1_time_sum":            16152,
		"custom_time_field_time2_time_avg":            255,
		"custom_time_field_time2_time_count":          72,
		"custom_time_field_time2_time_hist_bucket_1":  72,
		"custom_time_field_time2_time_hist_bucket_10": 72,
		"custom_time_field_time2_time_hist_bucket_11": 72,
		"custom_time_field_time2_time_hist_bucket_2":  72,
		"custom_time_field_time2_time_hist_bucket_3":  72,
		"custom_time_field_time2_time_hist_bucket_4":  72,
		"custom_time_field_time2_time_hist_bucket_5":  72,
		"custom_time_field_time2_time_hist_bucket_6":  72,
		"custom_time_field_time2_time_hist_bucket_7":  72,
		"custom_time_field_time2_time_hist_bucket_8":  72,
		"custom_time_field_time2_time_hist_bucket_9":  72,
		"custom_time_field_time2_time_hist_count":     72,
		"custom_time_field_time2_time_hist_sum":       18360,
		"custom_time_field_time2_time_max":            321,
		"custom_time_field_time2_time_min":            123,
		"custom_time_field_time2_time_sum":            18360,
		"req_http_scheme":                             0,
		"req_https_scheme":                            0,
		"req_ipv4":                                    0,
		"req_ipv6":                                    0,
		"req_proc_time_avg":                           0,
		"req_proc_time_count":                         0,
		"req_proc_time_hist_bucket_1":                 0,
		"req_proc_time_hist_bucket_10":                0,
		"req_proc_time_hist_bucket_11":                0,
		"req_proc_time_hist_bucket_2":                 0,
		"req_proc_time_hist_bucket_3":                 0,
		"req_proc_time_hist_bucket_4":                 0,
		"req_proc_time_hist_bucket_5":                 0,
		"req_proc_time_hist_bucket_6":                 0,
		"req_proc_time_hist_bucket_7":                 0,
		"req_proc_time_hist_bucket_8":                 0,
		"req_proc_time_hist_bucket_9":                 0,
		"req_proc_time_hist_count":                    0,
		"req_proc_time_hist_sum":                      0,
		"req_proc_time_max":                           0,
		"req_proc_time_min":                           0,
		"req_proc_time_sum":                           0,
		"req_type_bad":                                0,
		"req_type_error":                              0,
		"req_type_redirect":                           0,
		"req_type_success":                            0,
		"req_unmatched":                               0,
		"requests":                                    72,
		"resp_1xx":                                    0,
		"resp_2xx":                                    0,
		"resp_3xx":                                    0,
		"resp_4xx":                                    0,
		"resp_5xx":                                    0,
		"uniq_ipv4":                                   0,
		"uniq_ipv6":                                   0,
		"upstream_resp_time_avg":                      0,
		"upstream_resp_time_count":                    0,
		"upstream_resp_time_hist_bucket_1":            0,
		"upstream_resp_time_hist_bucket_10":           0,
		"upstream_resp_time_hist_bucket_11":           0,
		"upstream_resp_time_hist_bucket_2":            0,
		"upstream_resp_time_hist_bucket_3":            0,
		"upstream_resp_time_hist_bucket_4":            0,
		"upstream_resp_time_hist_bucket_5":            0,
		"upstream_resp_time_hist_bucket_6":            0,
		"upstream_resp_time_hist_bucket_7":            0,
		"upstream_resp_time_hist_bucket_8":            0,
		"upstream_resp_time_hist_bucket_9":            0,
		"upstream_resp_time_hist_count":               0,
		"upstream_resp_time_hist_sum":                 0,
		"upstream_resp_time_max":                      0,
		"upstream_resp_time_min":                      0,
		"upstream_resp_time_sum":                      0,
	}

	mx := collr.Collect(context.Background())
	assert.Equal(t, expected, mx)
	testCharts(t, collr, mx)
}

func TestCollector_Collect_CustomNumericFieldsLogs(t *testing.T) {
	collr := prepareWebLogCollectCustomNumericFields(t)

	expected := map[string]int64{
		"bytes_received": 0,
		"bytes_sent":     0,
		"custom_numeric_field_numeric1_summary_avg":   224,
		"custom_numeric_field_numeric1_summary_count": 72,
		"custom_numeric_field_numeric1_summary_max":   431,
		"custom_numeric_field_numeric1_summary_min":   121,
		"custom_numeric_field_numeric1_summary_sum":   16152,
		"custom_numeric_field_numeric2_summary_avg":   255,
		"custom_numeric_field_numeric2_summary_count": 72,
		"custom_numeric_field_numeric2_summary_max":   321,
		"custom_numeric_field_numeric2_summary_min":   123,
		"custom_numeric_field_numeric2_summary_sum":   18360,
		"req_http_scheme":                   0,
		"req_https_scheme":                  0,
		"req_ipv4":                          0,
		"req_ipv6":                          0,
		"req_proc_time_avg":                 0,
		"req_proc_time_count":               0,
		"req_proc_time_hist_bucket_1":       0,
		"req_proc_time_hist_bucket_10":      0,
		"req_proc_time_hist_bucket_11":      0,
		"req_proc_time_hist_bucket_2":       0,
		"req_proc_time_hist_bucket_3":       0,
		"req_proc_time_hist_bucket_4":       0,
		"req_proc_time_hist_bucket_5":       0,
		"req_proc_time_hist_bucket_6":       0,
		"req_proc_time_hist_bucket_7":       0,
		"req_proc_time_hist_bucket_8":       0,
		"req_proc_time_hist_bucket_9":       0,
		"req_proc_time_hist_count":          0,
		"req_proc_time_hist_sum":            0,
		"req_proc_time_max":                 0,
		"req_proc_time_min":                 0,
		"req_proc_time_sum":                 0,
		"req_type_bad":                      0,
		"req_type_error":                    0,
		"req_type_redirect":                 0,
		"req_type_success":                  0,
		"req_unmatched":                     0,
		"requests":                          72,
		"resp_1xx":                          0,
		"resp_2xx":                          0,
		"resp_3xx":                          0,
		"resp_4xx":                          0,
		"resp_5xx":                          0,
		"uniq_ipv4":                         0,
		"uniq_ipv6":                         0,
		"upstream_resp_time_avg":            0,
		"upstream_resp_time_count":          0,
		"upstream_resp_time_hist_bucket_1":  0,
		"upstream_resp_time_hist_bucket_10": 0,
		"upstream_resp_time_hist_bucket_11": 0,
		"upstream_resp_time_hist_bucket_2":  0,
		"upstream_resp_time_hist_bucket_3":  0,
		"upstream_resp_time_hist_bucket_4":  0,
		"upstream_resp_time_hist_bucket_5":  0,
		"upstream_resp_time_hist_bucket_6":  0,
		"upstream_resp_time_hist_bucket_7":  0,
		"upstream_resp_time_hist_bucket_8":  0,
		"upstream_resp_time_hist_bucket_9":  0,
		"upstream_resp_time_hist_count":     0,
		"upstream_resp_time_hist_sum":       0,
		"upstream_resp_time_max":            0,
		"upstream_resp_time_min":            0,
		"upstream_resp_time_sum":            0,
	}

	mx := collr.Collect(context.Background())

	assert.Equal(t, expected, mx)
	testCharts(t, collr, mx)
}

func TestCollector_IISLogs(t *testing.T) {
	collr := prepareWebLogCollectIISFields(t)

	expected := map[string]int64{
		"bytes_received":                    0,
		"bytes_sent":                        0,
		"req_http_scheme":                   0,
		"req_https_scheme":                  0,
		"req_ipv4":                          38,
		"req_ipv6":                          114,
		"req_method_GET":                    152,
		"req_port_80":                       152,
		"req_proc_time_avg":                 5,
		"req_proc_time_count":               152,
		"req_proc_time_hist_bucket_1":       133,
		"req_proc_time_hist_bucket_10":      145,
		"req_proc_time_hist_bucket_11":      146,
		"req_proc_time_hist_bucket_2":       133,
		"req_proc_time_hist_bucket_3":       133,
		"req_proc_time_hist_bucket_4":       133,
		"req_proc_time_hist_bucket_5":       133,
		"req_proc_time_hist_bucket_6":       133,
		"req_proc_time_hist_bucket_7":       133,
		"req_proc_time_hist_bucket_8":       138,
		"req_proc_time_hist_bucket_9":       143,
		"req_proc_time_hist_count":          152,
		"req_proc_time_hist_sum":            799,
		"req_proc_time_max":                 256,
		"req_proc_time_min":                 0,
		"req_proc_time_sum":                 799,
		"req_type_bad":                      42,
		"req_type_error":                    0,
		"req_type_redirect":                 0,
		"req_type_success":                  110,
		"req_unmatched":                     16,
		"req_vhost_127.0.0.1":               38,
		"req_vhost_::1":                     114,
		"requests":                          168,
		"resp_1xx":                          0,
		"resp_2xx":                          99,
		"resp_3xx":                          11,
		"resp_4xx":                          42,
		"resp_5xx":                          0,
		"resp_code_200":                     99,
		"resp_code_304":                     11,
		"resp_code_404":                     42,
		"uniq_ipv4":                         1,
		"uniq_ipv6":                         1,
		"upstream_resp_time_avg":            0,
		"upstream_resp_time_count":          0,
		"upstream_resp_time_hist_bucket_1":  0,
		"upstream_resp_time_hist_bucket_10": 0,
		"upstream_resp_time_hist_bucket_11": 0,
		"upstream_resp_time_hist_bucket_2":  0,
		"upstream_resp_time_hist_bucket_3":  0,
		"upstream_resp_time_hist_bucket_4":  0,
		"upstream_resp_time_hist_bucket_5":  0,
		"upstream_resp_time_hist_bucket_6":  0,
		"upstream_resp_time_hist_bucket_7":  0,
		"upstream_resp_time_hist_bucket_8":  0,
		"upstream_resp_time_hist_bucket_9":  0,
		"upstream_resp_time_hist_count":     0,
		"upstream_resp_time_hist_sum":       0,
		"upstream_resp_time_max":            0,
		"upstream_resp_time_min":            0,
		"upstream_resp_time_sum":            0,
	}

	mx := collr.Collect(context.Background())
	assert.Equal(t, expected, mx)
}

func testCharts(t *testing.T, c *Collector, mx map[string]int64) {
	testVhostChart(t, c)
	testPortChart(t, c)
	testSchemeChart(t, c)
	testClientCharts(t, c)
	testHTTPMethodChart(t, c)
	testURLPatternChart(t, c)
	testHTTPVersionChart(t, c)
	testRespCodeCharts(t, c)
	testBandwidthChart(t, c)
	testReqProcTimeCharts(t, c)
	testUpsRespTimeCharts(t, c)
	testSSLProtoChart(t, c)
	testSSLCipherSuiteChart(t, c)
	testURLPatternStatsCharts(t, c)
	testCustomFieldCharts(t, c)
	testCustomTimeFieldCharts(t, c)
	testCustomNumericFieldCharts(t, c)

	module.TestMetricsHasAllChartsDims(t, c.Charts(), mx)
}

func testVhostChart(t *testing.T, c *Collector) {
	if len(c.mx.ReqVhost) == 0 {
		assert.Falsef(t, c.Charts().Has(reqByVhost.ID), "chart '%s' is created", reqByVhost.ID)
		return
	}

	chart := c.Charts().Get(reqByVhost.ID)
	assert.NotNilf(t, chart, "chart '%s' is not created", reqByVhost.ID)
	if chart == nil {
		return
	}
	for v := range c.mx.ReqVhost {
		id := "req_vhost_" + v
		assert.Truef(t, chart.HasDim(id), "chart '%s' has no dim for '%s' vhost, expected '%s'", chart.ID, v, id)
	}
}

func testPortChart(t *testing.T, c *Collector) {
	if len(c.mx.ReqPort) == 0 {
		assert.Falsef(t, c.Charts().Has(reqByPort.ID), "chart '%s' is created", reqByPort.ID)
		return
	}

	chart := c.Charts().Get(reqByPort.ID)
	assert.NotNilf(t, chart, "chart '%s' is not created", reqByPort.ID)
	if chart == nil {
		return
	}
	for v := range c.mx.ReqPort {
		id := "req_port_" + v
		assert.Truef(t, chart.HasDim(id), "chart '%s' has no dim for '%s' port, expected '%s'", chart.ID, v, id)
	}
}

func testSchemeChart(t *testing.T, c *Collector) {
	if c.mx.ReqHTTPScheme.Value() == 0 && c.mx.ReqHTTPSScheme.Value() == 0 {
		assert.Falsef(t, c.Charts().Has(reqByScheme.ID), "chart '%s' is created", reqByScheme.ID)
	} else {
		assert.Truef(t, c.Charts().Has(reqByScheme.ID), "chart '%s' is not created", reqByScheme.ID)
	}
}

func testClientCharts(t *testing.T, c *Collector) {
	if c.mx.ReqIPv4.Value() == 0 && c.mx.ReqIPv6.Value() == 0 {
		assert.Falsef(t, c.Charts().Has(reqByIPProto.ID), "chart '%s' is created", reqByIPProto.ID)
	} else {
		assert.Truef(t, c.Charts().Has(reqByIPProto.ID), "chart '%s' is not created", reqByIPProto.ID)
	}

	if c.mx.UniqueIPv4.Value() == 0 && c.mx.UniqueIPv6.Value() == 0 {
		assert.Falsef(t, c.Charts().Has(uniqIPsCurPoll.ID), "chart '%s' is created", uniqIPsCurPoll.ID)
	} else {
		assert.Truef(t, c.Charts().Has(uniqIPsCurPoll.ID), "chart '%s' is not created", uniqIPsCurPoll.ID)
	}
}

func testHTTPMethodChart(t *testing.T, c *Collector) {
	if len(c.mx.ReqMethod) == 0 {
		assert.Falsef(t, c.Charts().Has(reqByMethod.ID), "chart '%s' is created", reqByMethod.ID)
		return
	}

	chart := c.Charts().Get(reqByMethod.ID)
	assert.NotNilf(t, chart, "chart '%s' is not created", reqByMethod.ID)
	if chart == nil {
		return
	}
	for v := range c.mx.ReqMethod {
		id := "req_method_" + v
		assert.Truef(t, chart.HasDim(id), "chart '%s' has no dim for '%s' method, expected '%s'", chart.ID, v, id)
	}
}

func testURLPatternChart(t *testing.T, c *Collector) {
	if isEmptyCounterVec(c.mx.ReqURLPattern) {
		assert.Falsef(t, c.Charts().Has(reqByURLPattern.ID), "chart '%s' is created", reqByURLPattern.ID)
		return
	}

	chart := c.Charts().Get(reqByURLPattern.ID)
	assert.NotNilf(t, chart, "chart '%s' is not created", reqByURLPattern.ID)
	if chart == nil {
		return
	}
	for v := range c.mx.ReqURLPattern {
		id := "req_url_ptn_" + v
		assert.True(t, chart.HasDim(id), "chart '%s' has no dim for '%s' pattern, expected '%s'", chart.ID, v, id)
	}
}

func testHTTPVersionChart(t *testing.T, c *Collector) {
	if len(c.mx.ReqVersion) == 0 {
		assert.Falsef(t, c.Charts().Has(reqByVersion.ID), "chart '%s' is created", reqByVersion.ID)
		return
	}

	chart := c.Charts().Get(reqByVersion.ID)
	assert.NotNilf(t, chart, "chart '%s' is not created", reqByVersion.ID)
	if chart == nil {
		return
	}
	for v := range c.mx.ReqVersion {
		id := "req_version_" + v
		assert.Truef(t, chart.HasDim(id), "chart '%s' has no dim for '%s' version, expected '%s'", chart.ID, v, id)
	}
}

func testRespCodeCharts(t *testing.T, c *Collector) {
	if isEmptyCounterVec(c.mx.RespCode) {
		for _, id := range []string{
			respCodes.ID,
			respCodes1xx.ID,
			respCodes2xx.ID,
			respCodes3xx.ID,
			respCodes4xx.ID,
			respCodes5xx.ID,
		} {
			assert.Falsef(t, c.Charts().Has(id), "chart '%s' is created", id)
		}
		return
	}

	if !c.GroupRespCodes {
		chart := c.Charts().Get(respCodes.ID)
		assert.NotNilf(t, chart, "chart '%s' is not created", respCodes.ID)
		if chart == nil {
			return
		}
		for v := range c.mx.RespCode {
			id := "resp_code_" + v
			assert.Truef(t, chart.HasDim(id), "chart '%s' has no dim for '%s' code, expected '%s'", chart.ID, v, id)
		}
		return
	}

	findCodes := func(class string) (codes []string) {
		for v := range c.mx.RespCode {
			if v[:1] == class {
				codes = append(codes, v)
			}
		}
		return codes
	}

	var n int
	ids := []string{
		respCodes1xx.ID,
		respCodes2xx.ID,
		respCodes3xx.ID,
		respCodes4xx.ID,
		respCodes5xx.ID,
	}
	for i, chartID := range ids {
		class := strconv.Itoa(i + 1)
		codes := findCodes(class)
		n += len(codes)
		chart := c.Charts().Get(chartID)
		assert.NotNilf(t, chart, "chart '%s' is not created", chartID)
		if chart == nil {
			return
		}
		for _, v := range codes {
			id := "resp_code_" + v
			assert.Truef(t, chart.HasDim(id), "chart '%s' has no dim for '%s' code, expected '%s'", chartID, v, id)
		}
	}
	assert.Equal(t, len(c.mx.RespCode), n)
}

func testBandwidthChart(t *testing.T, c *Collector) {
	if c.mx.BytesSent.Value() == 0 && c.mx.BytesReceived.Value() == 0 {
		assert.Falsef(t, c.Charts().Has(bandwidth.ID), "chart '%s' is created", bandwidth.ID)
	} else {
		assert.Truef(t, c.Charts().Has(bandwidth.ID), "chart '%s' is not created", bandwidth.ID)
	}
}

func testReqProcTimeCharts(t *testing.T, c *Collector) {
	if isEmptySummary(c.mx.ReqProcTime) {
		assert.Falsef(t, c.Charts().Has(reqProcTime.ID), "chart '%s' is created", reqProcTime.ID)
	} else {
		assert.Truef(t, c.Charts().Has(reqProcTime.ID), "chart '%s' is not created", reqProcTime.ID)
	}

	if isEmptyHistogram(c.mx.ReqProcTimeHist) {
		assert.Falsef(t, c.Charts().Has(reqProcTimeHist.ID), "chart '%s' is created", reqProcTimeHist.ID)
	} else {
		assert.Truef(t, c.Charts().Has(reqProcTimeHist.ID), "chart '%s' is not created", reqProcTimeHist.ID)
	}
}

func testUpsRespTimeCharts(t *testing.T, c *Collector) {
	if isEmptySummary(c.mx.UpsRespTime) {
		assert.Falsef(t, c.Charts().Has(upsRespTime.ID), "chart '%s' is created", upsRespTime.ID)
	} else {
		assert.Truef(t, c.Charts().Has(upsRespTime.ID), "chart '%s' is not created", upsRespTime.ID)
	}

	if isEmptyHistogram(c.mx.UpsRespTimeHist) {
		assert.Falsef(t, c.Charts().Has(upsRespTimeHist.ID), "chart '%s' is created", upsRespTimeHist.ID)
	} else {
		assert.Truef(t, c.Charts().Has(upsRespTimeHist.ID), "chart '%s' is not created", upsRespTimeHist.ID)
	}
}

func testSSLProtoChart(t *testing.T, c *Collector) {
	if len(c.mx.ReqSSLProto) == 0 {
		assert.Falsef(t, c.Charts().Has(reqBySSLProto.ID), "chart '%s' is created", reqBySSLProto.ID)
		return
	}

	chart := c.Charts().Get(reqBySSLProto.ID)
	assert.NotNilf(t, chart, "chart '%s' is not created", reqBySSLProto.ID)
	if chart == nil {
		return
	}
	for v := range c.mx.ReqSSLProto {
		id := "req_ssl_proto_" + v
		assert.Truef(t, chart.HasDim(id), "chart '%s' has no dim for '%s' ssl proto, expected '%s'", chart.ID, v, id)
	}
}

func testSSLCipherSuiteChart(t *testing.T, c *Collector) {
	if len(c.mx.ReqSSLCipherSuite) == 0 {
		assert.Falsef(t, c.Charts().Has(reqBySSLCipherSuite.ID), "chart '%s' is created", reqBySSLCipherSuite.ID)
		return
	}

	chart := c.Charts().Get(reqBySSLCipherSuite.ID)
	assert.NotNilf(t, chart, "chart '%s' is not created", reqBySSLCipherSuite.ID)
	if chart == nil {
		return
	}
	for v := range c.mx.ReqSSLCipherSuite {
		id := "req_ssl_cipher_suite_" + v
		assert.Truef(t, chart.HasDim(id), "chart '%s' has no dim for '%s' ssl cipher suite, expected '%s'", chart.ID, v, id)
	}
}

func testURLPatternStatsCharts(t *testing.T, collr *Collector) {
	for _, p := range collr.URLPatterns {
		chartID := fmt.Sprintf(urlPatternRespCodes.ID, p.Name)

		if isEmptyCounterVec(collr.mx.RespCode) {
			assert.Falsef(t, collr.Charts().Has(chartID), "chart '%s' is created", chartID)
			continue
		}

		chart := collr.Charts().Get(chartID)
		assert.NotNilf(t, chart, "chart '%s' is not created", chartID)
		if chart == nil {
			continue
		}

		stats, ok := collr.mx.URLPatternStats[p.Name]
		assert.Truef(t, ok, "url pattern '%s' has no metric in w.mx.URLPatternStats", p.Name)
		if !ok {
			continue
		}
		for v := range stats.RespCode {
			id := fmt.Sprintf("url_ptn_%s_resp_code_%s", p.Name, v)
			assert.Truef(t, chart.HasDim(id), "chart '%s' has no dim for '%s' code, expected '%s'", chartID, v, id)
		}
	}

	for _, p := range collr.URLPatterns {
		id := fmt.Sprintf(urlPatternReqMethods.ID, p.Name)
		if isEmptyCounterVec(collr.mx.ReqMethod) {
			assert.Falsef(t, collr.Charts().Has(id), "chart '%s' is created", id)
			continue
		}

		chart := collr.Charts().Get(id)
		assert.NotNilf(t, chart, "chart '%s' is not created", id)
		if chart == nil {
			continue
		}

		stats, ok := collr.mx.URLPatternStats[p.Name]
		assert.Truef(t, ok, "url pattern '%s' has no metric in w.mx.URLPatternStats", p.Name)
		if !ok {
			continue
		}
		for v := range stats.ReqMethod {
			dimID := fmt.Sprintf("url_ptn_%s_req_method_%s", p.Name, v)
			assert.Truef(t, chart.HasDim(dimID), "chart '%s' has no dim for '%s' method, expected '%s'", id, v, dimID)
		}
	}

	for _, p := range collr.URLPatterns {
		id := fmt.Sprintf(urlPatternBandwidth.ID, p.Name)
		if collr.mx.BytesSent.Value() == 0 && collr.mx.BytesReceived.Value() == 0 {
			assert.Falsef(t, collr.Charts().Has(id), "chart '%s' is created", id)
		} else {
			assert.Truef(t, collr.Charts().Has(id), "chart '%s' is not created", id)
		}
	}

	for _, p := range collr.URLPatterns {
		id := fmt.Sprintf(urlPatternReqProcTime.ID, p.Name)
		if isEmptySummary(collr.mx.ReqProcTime) {
			assert.Falsef(t, collr.Charts().Has(id), "chart '%s' is created", id)
		} else {
			assert.Truef(t, collr.Charts().Has(id), "chart '%s' is not created", id)
		}
	}
}

func testCustomFieldCharts(t *testing.T, c *Collector) {
	for _, cf := range c.CustomFields {
		id := fmt.Sprintf(reqByCustomFieldPattern.ID, cf.Name)
		chart := c.Charts().Get(id)
		assert.NotNilf(t, chart, "chart '%s' is not created", id)
		if chart == nil {
			continue
		}

		for _, p := range cf.Patterns {
			id := fmt.Sprintf("custom_field_%s_%s", cf.Name, p.Name)
			assert.True(t, chart.HasDim(id), "chart '%s' has no dim for '%s' pattern, expected '%s'", chart.ID, p, id)
		}
	}
}

func testCustomTimeFieldCharts(t *testing.T, c *Collector) {
	for _, cf := range c.CustomTimeFields {
		id := fmt.Sprintf(reqByCustomTimeField.ID, cf.Name)
		chart := c.Charts().Get(id)
		assert.NotNilf(t, chart, "chart '%s' is not created", id)
		if chart == nil {
			continue
		}
		dimMinID := fmt.Sprintf("custom_time_field_%s_time_min", cf.Name)
		assert.True(t, chart.HasDim(dimMinID), "chart '%s' has no dim for '%s' name, expected '%s'", chart.ID, cf.Name, dimMinID)

		dimMaxID := fmt.Sprintf("custom_time_field_%s_time_min", cf.Name)
		assert.True(t, chart.HasDim(dimMaxID), "chart '%s' has no dim for '%s' name, expected '%s'", chart.ID, cf.Name, dimMaxID)

		dimAveID := fmt.Sprintf("custom_time_field_%s_time_min", cf.Name)
		assert.True(t, chart.HasDim(dimAveID), "chart '%s' has no dim for '%s' name, expected '%s'", chart.ID, cf.Name, dimAveID)
	}
}

func testCustomNumericFieldCharts(t *testing.T, c *Collector) {
	for _, cf := range c.CustomNumericFields {
		id := fmt.Sprintf(customNumericFieldSummaryChartTmpl.ID, cf.Name)
		chart := c.Charts().Get(id)
		assert.NotNilf(t, chart, "chart '%s' is not created", id)
		if chart == nil {
			continue
		}
		dimMinID := fmt.Sprintf("custom_numeric_field_%s_summary_min", cf.Name)
		assert.True(t, chart.HasDim(dimMinID), "chart '%s' has no dim for '%s' name, expected '%s'", chart.ID, cf.Name, dimMinID)

		dimMaxID := fmt.Sprintf("custom_numeric_field_%s_summary_min", cf.Name)
		assert.True(t, chart.HasDim(dimMaxID), "chart '%s' has no dim for '%s' name, expected '%s'", chart.ID, cf.Name, dimMaxID)

		dimAveID := fmt.Sprintf("custom_numeric_field_%s_summary_min", cf.Name)
		assert.True(t, chart.HasDim(dimAveID), "chart '%s' has no dim for '%s' name, expected '%s'", chart.ID, cf.Name, dimAveID)
	}
}

var (
	emptySummary   = newWebLogSummary()
	emptyHistogram = metrix.NewHistogram(metrix.DefBuckets)
)

func isEmptySummary(s metrix.Summary) bool     { return reflect.DeepEqual(s, emptySummary) }
func isEmptyHistogram(h metrix.Histogram) bool { return reflect.DeepEqual(h, emptyHistogram) }

func isEmptyCounterVec(cv metrix.CounterVec) bool {
	for _, c := range cv {
		if c.Value() > 0 {
			return false
		}
	}
	return true
}

func prepareWebLogCollectFull(t *testing.T) *Collector {
	t.Helper()
	format := strings.Join([]string{
		"$host:$server_port",
		"$remote_addr",
		"-",
		"-",
		"$time_local",
		`"$request"`,
		"$status",
		"$body_bytes_sent",
		"$request_length",
		"$request_time",
		"$upstream_response_time",
		"$scheme",
		"$ssl_protocol",
		"$ssl_cipher",
		"$side",
		"$drink",
		"$random_time_field",
	}, " ")

	cfg := Config{
		ParserConfig: logs.ParserConfig{
			LogType: logs.TypeCSV,
			CSV: logs.CSVConfig{
				FieldsPerRecord:  -1,
				Delimiter:        " ",
				TrimLeadingSpace: false,
				Format:           format,
				CheckField:       checkCSVFormatField,
			},
		},
		Path:        "testdata/full.log",
		ExcludePath: "",
		URLPatterns: []userPattern{
			{Name: "com", Match: "~ com$"},
			{Name: "org", Match: "~ org$"},
			{Name: "net", Match: "~ net$"},
			{Name: "not_match", Match: "* !*"},
		},
		CustomFields: []customField{
			{
				Name: "side",
				Patterns: []userPattern{
					{Name: "dark", Match: "= dark"},
					{Name: "light", Match: "= light"},
				},
			},
			{
				Name: "drink",
				Patterns: []userPattern{
					{Name: "beer", Match: "= beer"},
					{Name: "wine", Match: "= wine"},
				},
			},
		},
		CustomTimeFields: []customTimeField{
			{
				Name:      "random_time_field",
				Histogram: metrix.DefBuckets,
			},
		},
		Histogram:      metrix.DefBuckets,
		GroupRespCodes: true,
	}
	collr := New()
	collr.Config = cfg
	require.NoError(t, collr.Init(context.Background()))
	require.NoError(t, collr.Check(context.Background()))
	defer collr.Cleanup(context.Background())

	p, err := logs.NewCSVParser(collr.ParserConfig.CSV, bytes.NewReader(dataFullLog))
	require.NoError(t, err)
	collr.parser = p
	return collr
}

func prepareWebLogCollectCommon(t *testing.T) *Collector {
	t.Helper()
	format := strings.Join([]string{
		"$remote_addr",
		"-",
		"-",
		"$time_local",
		`"$request"`,
		"$status",
		"$body_bytes_sent",
	}, " ")

	cfg := Config{
		ParserConfig: logs.ParserConfig{
			LogType: logs.TypeCSV,
			CSV: logs.CSVConfig{
				FieldsPerRecord:  -1,
				Delimiter:        " ",
				TrimLeadingSpace: false,
				Format:           format,
				CheckField:       checkCSVFormatField,
			},
		},
		Path:           "testdata/common.log",
		ExcludePath:    "",
		URLPatterns:    nil,
		CustomFields:   nil,
		Histogram:      nil,
		GroupRespCodes: false,
	}

	collr := New()
	collr.Config = cfg
	require.NoError(t, collr.Init(context.Background()))
	require.NoError(t, collr.Check(context.Background()))
	defer collr.Cleanup(context.Background())

	p, err := logs.NewCSVParser(collr.ParserConfig.CSV, bytes.NewReader(dataCommonLog))
	require.NoError(t, err)
	collr.parser = p
	return collr
}

func prepareWebLogCollectCustom(t *testing.T) *Collector {
	t.Helper()
	format := strings.Join([]string{
		"$side",
		"$drink",
	}, " ")

	cfg := Config{
		ParserConfig: logs.ParserConfig{
			LogType: logs.TypeCSV,
			CSV: logs.CSVConfig{
				FieldsPerRecord:  2,
				Delimiter:        " ",
				TrimLeadingSpace: false,
				Format:           format,
				CheckField:       checkCSVFormatField,
			},
		},
		CustomFields: []customField{
			{
				Name: "side",
				Patterns: []userPattern{
					{Name: "dark", Match: "= dark"},
					{Name: "light", Match: "= light"},
				},
			},
			{
				Name: "drink",
				Patterns: []userPattern{
					{Name: "beer", Match: "= beer"},
					{Name: "wine", Match: "= wine"},
				},
			},
		},
		Path:           "testdata/custom.log",
		ExcludePath:    "",
		URLPatterns:    nil,
		Histogram:      nil,
		GroupRespCodes: false,
	}
	collr := New()
	collr.Config = cfg
	require.NoError(t, collr.Init(context.Background()))
	require.NoError(t, collr.Check(context.Background()))
	defer collr.Cleanup(context.Background())

	p, err := logs.NewCSVParser(collr.ParserConfig.CSV, bytes.NewReader(dataCustomLog))
	require.NoError(t, err)
	collr.parser = p
	return collr
}

func prepareWebLogCollectCustomTimeFields(t *testing.T) *Collector {
	t.Helper()
	format := strings.Join([]string{
		"$time1",
		"$time2",
	}, " ")

	cfg := Config{
		ParserConfig: logs.ParserConfig{
			LogType: logs.TypeCSV,
			CSV: logs.CSVConfig{
				FieldsPerRecord:  2,
				Delimiter:        " ",
				TrimLeadingSpace: false,
				Format:           format,
				CheckField:       checkCSVFormatField,
			},
		},
		CustomTimeFields: []customTimeField{
			{
				Name:      "time1",
				Histogram: metrix.DefBuckets,
			},
			{
				Name:      "time2",
				Histogram: metrix.DefBuckets,
			},
		},
		Path:           "testdata/custom_time_fields.log",
		ExcludePath:    "",
		URLPatterns:    nil,
		Histogram:      nil,
		GroupRespCodes: false,
	}
	collr := New()
	collr.Config = cfg
	require.NoError(t, collr.Init(context.Background()))
	require.NoError(t, collr.Check(context.Background()))
	defer collr.Cleanup(context.Background())

	p, err := logs.NewCSVParser(collr.ParserConfig.CSV, bytes.NewReader(dataCustomTimeFieldLog))
	require.NoError(t, err)
	collr.parser = p
	return collr
}

func prepareWebLogCollectCustomNumericFields(t *testing.T) *Collector {
	t.Helper()
	format := strings.Join([]string{
		"$numeric1",
		"$numeric2",
	}, " ")

	cfg := Config{
		ParserConfig: logs.ParserConfig{
			LogType: logs.TypeCSV,
			CSV: logs.CSVConfig{
				FieldsPerRecord:  2,
				Delimiter:        " ",
				TrimLeadingSpace: false,
				Format:           format,
				CheckField:       checkCSVFormatField,
			},
		},
		CustomNumericFields: []customNumericField{
			{
				Name:  "numeric1",
				Units: "bytes",
			},
			{
				Name:  "numeric2",
				Units: "requests",
			},
		},
		Path:           "testdata/custom_time_fields.log",
		ExcludePath:    "",
		URLPatterns:    nil,
		Histogram:      nil,
		GroupRespCodes: false,
	}
	collr := New()
	collr.Config = cfg
	require.NoError(t, collr.Init(context.Background()))
	require.NoError(t, collr.Check(context.Background()))
	defer collr.Cleanup(context.Background())

	p, err := logs.NewCSVParser(collr.ParserConfig.CSV, bytes.NewReader(dataCustomTimeFieldLog))
	require.NoError(t, err)
	collr.parser = p
	return collr
}

func prepareWebLogCollectIISFields(t *testing.T) *Collector {
	t.Helper()
	format := strings.Join([]string{
		"-",               // date
		"-",               // time
		"$host",           // s-ip
		"$request_method", // cs-method
		"$request_uri",    // cs-uri-stem
		"-",               // cs-uri-query
		"$server_port",    // s-port
		"-",               // cs-username
		"$remote_addr",    // c-ip
		"-",               // cs(User-Agent)
		"-",               // cs(Referer)
		"$status",         // sc-status
		"-",               // sc-substatus
		"-",               // sc-win32-status
		"$request_time",   // time-taken
	}, " ")
	cfg := Config{
		ParserConfig: logs.ParserConfig{
			LogType: logs.TypeCSV,
			CSV: logs.CSVConfig{
				// Users can define number of fields
				FieldsPerRecord:  -1,
				Delimiter:        " ",
				TrimLeadingSpace: false,
				Format:           format,
				CheckField:       checkCSVFormatField,
			},
		},
		Path:           "testdata/u_ex221107.log",
		ExcludePath:    "",
		URLPatterns:    nil,
		Histogram:      nil,
		GroupRespCodes: false,
	}

	collr := New()
	collr.Config = cfg
	require.NoError(t, collr.Init(context.Background()))
	require.NoError(t, collr.Check(context.Background()))
	defer collr.Cleanup(context.Background())

	p, err := logs.NewCSVParser(collr.ParserConfig.CSV, bytes.NewReader(dataIISLog))
	require.NoError(t, err)
	collr.parser = p
	return collr
}

// generateLogs is used to populate 'testdata/full.log'
//func generateLogs(w io.Writer, num int) error {
//	var (
//		vhost     = []string{"localhost", "test.example.com", "test.example.org", "198.51.100.1", "2001:db8:1ce::1"}
//		scheme    = []string{"http", "https"}
//		client    = []string{"localhost", "203.0.113.1", "203.0.113.2", "2001:db8:2ce:1", "2001:db8:2ce:2"}
//		method    = []string{"GET", "HEAD", "POST"}
//		url       = []string{"example.other", "example.com", "example.org", "example.net"}
//		version   = []string{"1.1", "2", "2.0"}
//		status    = []int{100, 101, 200, 201, 300, 301, 400, 401} // no 5xx on purpose
//		sslProto  = []string{"TLSv1", "TLSv1.1", "TLSv1.2", "TLSv1.3", "SSLv2", "SSLv3"}
//		sslCipher = []string{"ECDHE-RSA-AES256-SHA", "DHE-RSA-AES256-SHA", "AES256-SHA", "PSK-RC4-SHA"}
//
//		customField1 = []string{"dark", "light"}
//		customField2 = []string{"beer", "wine"}
//	)
//
//	var line string
//	for i := 0; i < num; i++ {
//		unmatched := randInt(1, 100) > 90
//		if unmatched {
//			line = "Unmatched! The rat the cat the dog chased killed ate the malt!\n"
//		} else {
//			// test.example.com:80 203.0.113.1 - - "GET / HTTP/1.1" 200 1674 2674 3674 4674 http TLSv1 AES256-SHA dark beer
//			line = fmt.Sprintf(
//				"%s:%d %s - - [22/Mar/2009:09:30:31 +0100] \"%s /%s HTTP/%s\" %d %d %d %d %d %s %s %s %s %s\n",
//				randFromString(vhost),
//				randInt(80, 85),
//				randFromString(client),
//				randFromString(method),
//				randFromString(url),
//				randFromString(version),
//				randFromInt(status),
//				randInt(1000, 5000),
//				randInt(1000, 5000),
//				randInt(1, 500),
//				randInt(1, 500),
//				randFromString(scheme),
//				randFromString(sslProto),
//				randFromString(sslCipher),
//				randFromString(customField1),
//				randFromString(customField2),
//			)
//		}
//		_, err := fmt.Fprint(w, line)
//		if err != nil {
//			return err
//		}
//	}
//	return nil
//}
//
//var r = rand.New(rand.NewSource(time.Now().UnixNano()))
//
//func randFromString(s []string) string { return s[r.Intn(len(s))] }
//func randFromInt(s []int) int          { return s[r.Intn(len(s))] }
//func randInt(min, max int) int         { return r.Intn(max-min) + min }
