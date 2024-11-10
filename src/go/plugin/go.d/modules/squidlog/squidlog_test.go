// SPDX-License-Identifier: GPL-3.0-or-later

package squidlog

import (
	"bytes"
	"os"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/logs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	dataConfigJSON, _ = os.ReadFile("testdata/config.json")
	dataConfigYAML, _ = os.ReadFile("testdata/config.yaml")

	dataNativeFormatAccessLog, _ = os.ReadFile("testdata/access.log")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON":            dataConfigJSON,
		"dataConfigYAML":            dataConfigYAML,
		"dataNativeFormatAccessLog": dataNativeFormatAccessLog,
	} {
		require.NotNil(t, data, name)
	}
}

func TestSquidLog_ConfigurationSerialize(t *testing.T) {
	module.TestConfigurationSerialize(t, &SquidLog{}, dataConfigJSON, dataConfigYAML)
}

func TestNew(t *testing.T) {
	assert.Implements(t, (*module.Module)(nil), New())
}

func TestSquidLog_Init(t *testing.T) {
	squidlog := New()

	assert.NoError(t, squidlog.Init())
}

func TestSquidLog_Check(t *testing.T) {
}

func TestSquidLog_Check_ErrorOnCreatingLogReaderNoLogFile(t *testing.T) {
	squid := New()
	defer squid.Cleanup()
	squid.Path = "testdata/not_exists.log"
	require.NoError(t, squid.Init())

	assert.Error(t, squid.Check())
}

func TestSquid_Check_ErrorOnCreatingParserUnknownFormat(t *testing.T) {
	squid := New()
	defer squid.Cleanup()
	squid.Path = "testdata/unknown.log"
	require.NoError(t, squid.Init())

	assert.Error(t, squid.Check())
}

func TestSquid_Check_ErrorOnCreatingParserZeroKnownFields(t *testing.T) {
	squid := New()
	defer squid.Cleanup()
	squid.Path = "testdata/access.log"
	squid.ParserConfig.CSV.Format = "$one $two"
	require.NoError(t, squid.Init())

	assert.Error(t, squid.Check())
}

func TestSquidLog_Charts(t *testing.T) {
	assert.Nil(t, New().Charts())

	squid := prepareSquidCollect(t)
	assert.NotNil(t, squid.Charts())

}

func TestSquidLog_Cleanup(t *testing.T) {
	New().Cleanup()
}

func TestSquidLog_Collect(t *testing.T) {
	squid := prepareSquidCollect(t)

	expected := map[string]int64{
		"bytes_sent":                                         6827357,
		"cache_error_tag_ABORTED":                            326,
		"cache_handling_tag_CF":                              154,
		"cache_handling_tag_CLIENT":                          172,
		"cache_load_source_tag_MEM":                          172,
		"cache_object_tag_NEGATIVE":                          308,
		"cache_object_tag_STALE":                             172,
		"cache_result_code_NONE":                             158,
		"cache_result_code_TCP_CF_NEGATIVE_NEGATIVE_ABORTED": 154,
		"cache_result_code_UDP_CLIENT_STALE_MEM_ABORTED":     172,
		"cache_transport_tag_NONE":                           158,
		"cache_transport_tag_TCP":                            154,
		"cache_transport_tag_UDP":                            172,
		"hier_code_HIER_CACHE_DIGEST_HIT":                    128,
		"hier_code_HIER_NO_CACHE_DIGEST_DIRECT":              130,
		"hier_code_HIER_PARENT_HIT":                          106,
		"hier_code_HIER_SINGLE_PARENT":                       120,
		"http_resp_0xx":                                      51,
		"http_resp_1xx":                                      45,
		"http_resp_2xx":                                      62,
		"http_resp_3xx":                                      120,
		"http_resp_4xx":                                      112,
		"http_resp_5xx":                                      46,
		"http_resp_6xx":                                      48,
		"http_resp_code_0":                                   51,
		"http_resp_code_100":                                 45,
		"http_resp_code_200":                                 62,
		"http_resp_code_300":                                 58,
		"http_resp_code_304":                                 62,
		"http_resp_code_400":                                 56,
		"http_resp_code_401":                                 56,
		"http_resp_code_500":                                 46,
		"http_resp_code_603":                                 48,
		"mime_type_application":                              52,
		"mime_type_audio":                                    56,
		"mime_type_font":                                     44,
		"mime_type_image":                                    50,
		"mime_type_message":                                  44,
		"mime_type_model":                                    62,
		"mime_type_multipart":                                61,
		"mime_type_text":                                     61,
		"mime_type_video":                                    54,
		"req_method_COPY":                                    84,
		"req_method_GET":                                     70,
		"req_method_HEAD":                                    59,
		"req_method_OPTIONS":                                 99,
		"req_method_POST":                                    74,
		"req_method_PURGE":                                   98,
		"req_type_bad":                                       56,
		"req_type_error":                                     94,
		"req_type_redirect":                                  58,
		"req_type_success":                                   276,
		"requests":                                           500,
		"resp_time_avg":                                      3015931,
		"resp_time_count":                                    484,
		"resp_time_max":                                      4988000,
		"resp_time_min":                                      1002000,
		"resp_time_sum":                                      1459711000,
		"server_address_2001:db8:2ce:a":                      79,
		"server_address_2001:db8:2ce:b":                      89,
		"server_address_203.0.113.100":                       67,
		"server_address_203.0.113.200":                       70,
		"server_address_content-gateway":                     87,
		"uniq_clients":                                       5,
		"unmatched":                                          16,
	}

	collected := squid.Collect()

	assert.Equal(t, expected, collected)
	testCharts(t, squid, collected)
}

func TestSquidLog_Collect_ReturnOldDataIfNothingRead(t *testing.T) {
	squid := prepareSquidCollect(t)

	expected := map[string]int64{
		"bytes_sent":                                         6827357,
		"cache_error_tag_ABORTED":                            326,
		"cache_handling_tag_CF":                              154,
		"cache_handling_tag_CLIENT":                          172,
		"cache_load_source_tag_MEM":                          172,
		"cache_object_tag_NEGATIVE":                          308,
		"cache_object_tag_STALE":                             172,
		"cache_result_code_NONE":                             158,
		"cache_result_code_TCP_CF_NEGATIVE_NEGATIVE_ABORTED": 154,
		"cache_result_code_UDP_CLIENT_STALE_MEM_ABORTED":     172,
		"cache_transport_tag_NONE":                           158,
		"cache_transport_tag_TCP":                            154,
		"cache_transport_tag_UDP":                            172,
		"hier_code_HIER_CACHE_DIGEST_HIT":                    128,
		"hier_code_HIER_NO_CACHE_DIGEST_DIRECT":              130,
		"hier_code_HIER_PARENT_HIT":                          106,
		"hier_code_HIER_SINGLE_PARENT":                       120,
		"http_resp_0xx":                                      51,
		"http_resp_1xx":                                      45,
		"http_resp_2xx":                                      62,
		"http_resp_3xx":                                      120,
		"http_resp_4xx":                                      112,
		"http_resp_5xx":                                      46,
		"http_resp_6xx":                                      48,
		"http_resp_code_0":                                   51,
		"http_resp_code_100":                                 45,
		"http_resp_code_200":                                 62,
		"http_resp_code_300":                                 58,
		"http_resp_code_304":                                 62,
		"http_resp_code_400":                                 56,
		"http_resp_code_401":                                 56,
		"http_resp_code_500":                                 46,
		"http_resp_code_603":                                 48,
		"mime_type_application":                              52,
		"mime_type_audio":                                    56,
		"mime_type_font":                                     44,
		"mime_type_image":                                    50,
		"mime_type_message":                                  44,
		"mime_type_model":                                    62,
		"mime_type_multipart":                                61,
		"mime_type_text":                                     61,
		"mime_type_video":                                    54,
		"req_method_COPY":                                    84,
		"req_method_GET":                                     70,
		"req_method_HEAD":                                    59,
		"req_method_OPTIONS":                                 99,
		"req_method_POST":                                    74,
		"req_method_PURGE":                                   98,
		"req_type_bad":                                       56,
		"req_type_error":                                     94,
		"req_type_redirect":                                  58,
		"req_type_success":                                   276,
		"requests":                                           500,
		"resp_time_avg":                                      0,
		"resp_time_count":                                    0,
		"resp_time_max":                                      0,
		"resp_time_min":                                      0,
		"resp_time_sum":                                      0,
		"server_address_2001:db8:2ce:a":                      79,
		"server_address_2001:db8:2ce:b":                      89,
		"server_address_203.0.113.100":                       67,
		"server_address_203.0.113.200":                       70,
		"server_address_content-gateway":                     87,
		"uniq_clients":                                       0,
		"unmatched":                                          16,
	}

	_ = squid.Collect()

	mx := squid.Collect()

	assert.Equal(t, expected, mx)

	testCharts(t, squid, mx)
}

func testCharts(t *testing.T, squidlog *SquidLog, mx map[string]int64) {
	t.Helper()
	ensureChartsDynamicDimsCreated(t, squidlog)
	module.TestMetricsHasAllChartsDims(t, squidlog.Charts(), mx)
}

func ensureChartsDynamicDimsCreated(t *testing.T, squid *SquidLog) {
	ensureDynamicDimsCreated(t, squid, cacheCodeChart.ID, pxCacheCode, squid.mx.CacheCode)
	ensureDynamicDimsCreated(t, squid, cacheCodeTransportTagChart.ID, pxTransportTag, squid.mx.CacheCodeTransportTag)
	ensureDynamicDimsCreated(t, squid, cacheCodeHandlingTagChart.ID, pxHandlingTag, squid.mx.CacheCodeHandlingTag)
	ensureDynamicDimsCreated(t, squid, cacheCodeObjectTagChart.ID, pxObjectTag, squid.mx.CacheCodeObjectTag)
	ensureDynamicDimsCreated(t, squid, cacheCodeLoadSourceTagChart.ID, pxSourceTag, squid.mx.CacheCodeLoadSourceTag)
	ensureDynamicDimsCreated(t, squid, cacheCodeErrorTagChart.ID, pxErrorTag, squid.mx.CacheCodeErrorTag)
	ensureDynamicDimsCreated(t, squid, httpRespCodesChart.ID, pxHTTPCode, squid.mx.HTTPRespCode)
	ensureDynamicDimsCreated(t, squid, reqMethodChart.ID, pxReqMethod, squid.mx.ReqMethod)
	ensureDynamicDimsCreated(t, squid, hierCodeChart.ID, pxHierCode, squid.mx.HierCode)
	ensureDynamicDimsCreated(t, squid, serverAddrChart.ID, pxSrvAddr, squid.mx.Server)
	ensureDynamicDimsCreated(t, squid, mimeTypeChart.ID, pxMimeType, squid.mx.MimeType)
}

func ensureDynamicDimsCreated(t *testing.T, squid *SquidLog, chartID, dimPrefix string, data metrix.CounterVec) {
	chart := squid.Charts().Get(chartID)
	assert.NotNilf(t, chart, "chart '%s' is not created", chartID)
	if chart == nil {
		return
	}
	for v := range data {
		id := dimPrefix + v
		assert.Truef(t, chart.HasDim(id), "chart '%s' has no dim for '%s', expected '%s'", chart.ID, v, id)
	}
}

func prepareSquidCollect(t *testing.T) *SquidLog {
	t.Helper()
	squid := New()
	squid.Path = "testdata/access.log"
	require.NoError(t, squid.Init())
	require.NoError(t, squid.Check())
	defer squid.Cleanup()

	p, err := logs.NewCSVParser(squid.ParserConfig.CSV, bytes.NewReader(dataNativeFormatAccessLog))
	require.NoError(t, err)
	squid.parser = p
	return squid
}

// generateLogs is used to populate 'testdata/access.log'
//func generateLogs(w io.Writer, num int) error {
//	var (
//		client    = []string{"localhost", "203.0.113.1", "203.0.113.2", "2001:db8:2ce:1", "2001:db8:2ce:2"}
//		cacheCode = []string{"TCP_CF_NEGATIVE_NEGATIVE_ABORTED", "UDP_CLIENT_STALE_MEM_ABORTED", "NONE"}
//		httpCode  = []string{"000", "100", "200", "300", "304", "400", "401", "500", "603"}
//		method    = []string{"GET", "HEAD", "POST", "COPY", "PURGE", "OPTIONS"}
//		hierCode  = []string{"HIER_PARENT_HIT", "HIER_SINGLE_PARENT", "HIER_CACHE_DIGEST_HIT", "HIER_NO_CACHE_DIGEST_DIRECT"}
//		server    = []string{"content-gateway", "203.0.113.100", "203.0.113.200", "2001:db8:2ce:a", "2001:db8:2ce:b", "-"}
//		mimeType  = []string{"application", "audio", "font", "image", "message", "model", "multipart", "video", "text"}
//	)
//
//	r := rand.New(rand.NewSource(time.Now().UnixNano()))
//	randFromString := func(s []string) string { return s[r.Intn(len(s))] }
//	randInt := func(min, max int) int { return r.Intn(max-min) + min }
//
//	var line string
//	for i := 0; i < num; i++ {
//		unmatched := randInt(1, 100) > 95
//		if i > 0 && unmatched {
//			line = "Unmatched! The rat the cat the dog chased killed ate the malt!\n"
//		} else {
//			// 1576177221.686     0 ::1 TCP_MISS/200 1621 GET cache_object://localhost/counters - HIER_NONE/- text/plain
//			line = fmt.Sprintf(
//				"1576177221.686     %d %s %s/%s %d %s cache_object://localhost/counters - %s/%s %s/plain\n",
//				randInt(1000, 5000),
//				randFromString(client),
//				randFromString(cacheCode),
//				randFromString(httpCode),
//				randInt(9000, 19000),
//				randFromString(method),
//				randFromString(hierCode),
//				randFromString(server),
//				randFromString(mimeType),
//			)
//		}
//		_, err := fmt.Fprint(w, line)
//		if err != nil {
//			return err
//		}
//	}
//	return nil
//}
