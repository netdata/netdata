// SPDX-License-Identifier: GPL-3.0-or-later

package bind

import (
	"context"
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

	dataServerStatsJSON, _ = os.ReadFile("testdata/query-server.json")
	dataServerStatsXML, _  = os.ReadFile("testdata/query-server.xml")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON":      dataConfigJSON,
		"dataConfigYAML":      dataConfigYAML,
		"dataServerStatsJSON": dataServerStatsJSON,
		"dataServerStatsXML":  dataServerStatsXML,
	} {
		require.NotNil(t, data, name)

	}
}

func TestCollector_ConfigurationSerialize(t *testing.T) {
	module.TestConfigurationSerialize(t, &Collector{}, dataConfigJSON, dataConfigYAML)
}

func TestCollector_Cleanup(t *testing.T) { New().Cleanup(context.Background()) }

func TestCollector_Init(t *testing.T) {
	// OK
	collr := New()
	assert.NoError(t, collr.Init(context.Background()))
	assert.NotNil(t, collr.bindAPIClient)

	//NG
	collr = New()
	collr.URL = ""
	assert.Error(t, collr.Init(context.Background()))
}

func TestCollector_Check(t *testing.T) {
	ts := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/json/v1/server" {
					_, _ = w.Write(dataServerStatsJSON)
				}
			}))
	defer ts.Close()

	collr := New()
	collr.URL = ts.URL + "/json/v1"

	require.NoError(t, collr.Init(context.Background()))
	require.NoError(t, collr.Check(context.Background()))
}

func TestCollector_CheckNG(t *testing.T) {
	collr := New()

	collr.URL = "http://127.0.0.1:38001/xml/v3"
	require.NoError(t, collr.Init(context.Background()))
	assert.Error(t, collr.Check(context.Background()))
}

func TestCollector_Charts(t *testing.T) {
	assert.NotNil(t, New().Charts())
}

func TestCollector_CollectJSON(t *testing.T) {
	ts := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/json/v1/server" {
					_, _ = w.Write(dataServerStatsJSON)
				}
			}))
	defer ts.Close()

	collr := New()
	collr.URL = ts.URL + "/json/v1"
	collr.PermitView = "*"

	require.NoError(t, collr.Init(context.Background()))
	require.NoError(t, collr.Check(context.Background()))

	expected := map[string]int64{
		"_default_Queryv4":         4503685324,
		"_default_NSEC":            53193,
		"_default_NSEC3PARAM":      993,
		"_default_ANY":             5149356,
		"QryFORMERR":               8,
		"CookieMatch":              125065,
		"A6":                       538255,
		"MAILA":                    44,
		"ExpireOpt":                195,
		"CNAME":                    534171,
		"TYPE115":                  285,
		"_default_RESERVED0":       19,
		"_default_ClientCookieOut": 3790767469,
		"_default_CookieClientOk":  297765763,
		"QryFailure":               225786697,
		"TYPE127":                  1,
		"_default_GlueFetchv4":     110619519,
		"_default_Queryv6":         291939086,
		"UPDATE":                   18836,
		"RESERVED0":                13705,
		"_default_CacheHits":       405229520524,
		"Requestv6":                155,
		"QryTCP":                   4226324,
		"RESERVED15":               0,
		"QUERY":                    36766967932,
		"EUI64":                    627,
		"_default_NXDOMAIN":        1245990908,
		"_default_REFUSED":         106664780,
		"_default_EUI64":           2087,
		"QrySERVFAIL":              219515158,
		"QryRecursion":             3666523564,
		"MX":                       1483690,
		"DNSKEY":                   143483,
		"_default_TYPE115":         112,
		"_default_Others":          813,
		"_default_CacheMisses":     127371,
		"RateDropped":              219,
		"NAPTR":                    109959,
		"NSEC":                     81,
		"AAAA":                     3304112238,
		"_default_QryRTT500":       2071767970,
		"_default_TYPE127":         2,
		"_default_A6":              556692,
		"QryAuthAns":               440508475,
		"RecursClients":            74,
		"XfrRej":                   97,
		"LOC":                      52,
		"CookieIn":                 1217208,
		"RRSIG":                    25192,
		"_default_LOC":             21,
		"ReqBadEDNSVer":            450,
		"MG":                       4,
		"_default_GlueFetchv6":     121100044,
		"_default_HINFO":           1,
		"IQUERY":                   199,
		"_default_BadCookieRcode":  14779,
		"AuthQryRej":               148023,
		"QrySuccess":               28766465065,
		"SRV":                      27637747,
		"TYPE223":                  2,
		"CookieNew":                1058677,
		"_default_QryRTT10":        628295,
		"_default_ServerCookieOut": 364811250,
		"RESERVED11":               3,
		"_default_CookieIn":        298084581,
		"_default_DS":              973892,
		"_bind_CacheHits":          0,
		"STATUS":                   35546,
		"TLSA":                     297,
		"_default_SERVFAIL":        6523360,
		"_default_GlueFetchv4Fail": 3949012,
		"_default_NULL":            3548,
		"UpdateRej":                15661,
		"RESERVED10":               5,
		"_default_EDNS0Fail":       3982564,
		"_default_DLV":             20418,
		"ANY":                      298451299,
		"_default_GlueFetchv6Fail": 91728801,
		"_default_RP":              134,
		"_default_AAAA":            817525939,
		"X25":                      2,
		"NS":                       5537956,
		"_default_NumFetch":        100,
		"_default_DNSKEY":          182224,
		"QryUDP":                   36455909449,
		"QryReferral":              1152155,
		"QryNXDOMAIN":              5902446156,
		"TruncatedResp":            25882799,
		"DNAME":                    1,
		"DLV":                      37676,
		"_default_FORMERR":         3827518,
		"_default_RRSIG":           191628,
		"RecQryRej":                225638588,
		"QryDropped":               52141050,
		"Response":                 36426730232,
		"RESERVED14":               0,
		"_default_SPF":             16521,
		"_default_DNAME":           6,
		"Requestv4":                36767496594,
		"CookieNoMatch":            33466,
		"RESERVED9":                0,
		"_default_QryRTT800":       2709649,
		"_default_QryRTT1600":      455315,
		"_default_OtherError":      1426431,
		"_default_MX":              1575795,
		"QryNoauthAns":             35538538399,
		"NSIDOpt":                  81,
		"ReqTCP":                   4234792,
		"SOA":                      3860272,
		"RESERVED8":                0,
		"RESERVED13":               8,
		"MAILB":                    42,
		"AXFR":                     105,
		"QryNxrrset":               1308983498,
		"SPF":                      2872,
		"PTR":                      693769261,
		"_default_Responsev4":      4169576370,
		"_default_QryRTT100":       2086168894,
		"_default_Retry":           783763680,
		"_default_SRV":             3848459,
		"QryDuplicate":             288617636,
		"ECSOpt":                   8742938,
		"A":                        32327037206,
		"DS":                       1687895,
		"RESERVED12":               1,
		"_default_QryRTT1600+":     27639,
		"_default_TXT":             43595113,
		"_default_CDS":             251,
		"RESERVED6":                7401,
		"RESERVED3":                2,
		"_default_Truncated":       14015078,
		"_default_NextItem":        1788902,
		"_default_Responsev6":      151,
		"_default_QueryTimeout":    335575100,
		"_default_A":               3673673090,
		"ReqEdns0":                 532104182,
		"OtherOpt":                 3425542,
		"NULL":                     3604,
		"HINFO":                    9,
		"_default_SOA":             1326766,
		"_default_NAPTR":           30685,
		"_default_PTR":             208067284,
		"_default_CNAME":           38153754,
		"RespEDNS0":                527991455,
		"RESERVED7":                0,
		"TXT":                      100045556,
		"_default_Lame":            1975334,
		"_bind_CacheMisses":        509,
		"IXFR":                     33,
		"_default_NS":              675609,
		"_default_AFSDB":           5,
		"NOTIFY":                   390443,
		"Others":                   74006,
	}

	assert.Equal(t, expected, collr.Collect(context.Background()))
	assert.Len(t, *collr.charts, 17)
}

func TestCollector_CollectXML3(t *testing.T) {
	ts := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/xml/v3/server" {
					_, _ = w.Write(dataServerStatsXML)
				}
			}))
	defer ts.Close()

	collr := New()
	collr.PermitView = "*"
	collr.URL = ts.URL + "/xml/v3"

	require.NoError(t, collr.Init(context.Background()))
	require.NoError(t, collr.Check(context.Background()))

	expected := map[string]int64{
		"_bind_CookieClientOk":     0,
		"_bind_ValNegOk":           0,
		"_bind_GlueFetchv4Fail":    0,
		"_bind_ValFail":            0,
		"RateSlipped":              0,
		"_default_ValFail":         0,
		"_default_TYPE127":         2,
		"TLSA":                     299,
		"_default_FORMERR":         3831796,
		"_default_ValNegOk":        0,
		"_default_RRSIG":           191877,
		"_default_CacheHits":       405816752908,
		"CookieBadTime":            0,
		"RESERVED14":               0,
		"_default_SPF":             16563,
		"RESERVED3":                2,
		"NS":                       5545011,
		"QrySERVFAIL":              219790234,
		"UPDATE":                   18839,
		"_default_NAPTR":           30706,
		"RESERVED13":               8,
		"_default_CookieIn":        298556974,
		"_bind_Retry":              0,
		"_default_SOA":             1327966,
		"_bind_Truncated":          0,
		"RESERVED6":                7401,
		"_default_CookieClientOk":  298237641,
		"_default_QueryTimeout":    336165169,
		"SPF":                      2887,
		"_default_DNAME":           6,
		"_bind_Lame":               0,
		"QryUDP":                   36511992002,
		"NOTIFY":                   390521,
		"DNAME":                    1,
		"DS":                       1688561,
		"_default_OtherError":      1464741,
		"_default_Retry":           784916992,
		"_default_TXT":             43650696,
		"QryBADCOOKIE":             0,
		"RespEDNS0":                528451140,
		"TXT":                      100195931,
		"OtherOpt":                 3431439,
		"_default_HINFO":           1,
		"RESERVED0":                13705,
		"_bind_CacheHits":          0,
		"ReqTCP":                   4241537,
		"RespTSIG":                 0,
		"RESERVED11":               3,
		"_default_QryRTT100":       2087797539,
		"_default_REFUSED":         106782830,
		"_bind_SERVFAIL":           0,
		"X25":                      2,
		"_default_RP":              134,
		"QryDuplicate":             289518897,
		"CookieNoMatch":            34013,
		"_default_BadCookieRcode":  15399,
		"_default_CacheMisses":     127371,
		"_bind_Mismatch":           0,
		"_default_ServerCookieOut": 365308714,
		"_bind_QryRTT500":          0,
		"RPZRewrites":              0,
		"A":                        32377004350,
		"_default_NextItem":        1790135,
		"_default_MX":              1576150,
		"_bind_REFUSED":            0,
		"_bind_ZoneQuota":          0,
		"_default_ServerQuota":     0,
		"_default_ANY":             5149916,
		"_default_EUI64":           2087,
		"_default_QueryCurUDP":     0,
		"RESERVED7":                0,
		"IXFR":                     33,
		"_default_Queryv4":         4509791268,
		"_default_GlueFetchv4":     110749701,
		"_default_TYPE115":         112,
		"_bind_QueryAbort":         0,
		"UpdateReqFwd":             0,
		"_default_NSEC3PARAM":      995,
		"_bind_NextItem":           0,
		"RecursClients":            64,
		"QryReferral":              1152178,
		"QryFORMERR":               8,
		"CookieIn":                 1220424,
		"NSIDOpt":                  81,
		"MAILA":                    44,
		"TYPE223":                  2,
		"RRSIG":                    25193,
		"UpdateBadPrereq":          0,
		"UpdateRej":                15661,
		"QryAuthAns":               440885288,
		"_default_PTR":             208337408,
		"_default_Others":          813,
		"_default_NS":              676773,
		"_bind_GlueFetchv4":        0,
		"QryNoauthAns":             35593104164,
		"QryRecursion":             3671792792,
		"_default_ClientCookieOut": 3795901994,
		"_bind_BadEDNSVersion":     0,
		"ReqEdns0":                 532586114,
		"RateDropped":              230,
		"_default_ValOk":           0,
		"CNAME":                    535141,
		"AuthQryRej":               148159,
		"RESERVED10":               5,
		"_default_QueryCurTCP":     0,
		"_bind_Queryv4":            0,
		"_bind_CacheMisses":        509,
		"ExpireOpt":                195,
		"XfrRej":                   97,
		"_default_DNSKEY":          182399,
		"RecQryRej":                225832466,
		"NSEC":                     81,
		"_default_Responsev4":      4175093103,
		"_bind_ValOk":              0,
		"_bind_QueryCurTCP":        0,
		"Requestv4":                36823884979,
		"DNSKEY":                   143600,
		"_default_LOC":             21,
		"UpdateRespFwd":            0,
		"AXFR":                     105,
		"_bind_CookieIn":           0,
		"_default_QryRTT1600":      455849,
		"_bind_BadCookieRcode":     0,
		"QryNXDOMAIN":              5911582433,
		"ReqSIG0":                  0,
		"QUERY":                    36823356081,
		"NULL":                     3606,
		"_default_Lame":            1979599,
		"_default_DS":              974240,
		"SRV":                      27709732,
		"_bind_QuerySockFail":      0,
		"MG":                       4,
		"_default_QryRTT800":       2712733,
		"_bind_QryRTT1600+":        0,
		"DNS64":                    0,
		"_default_Truncated":       14028716,
		"_default_QryRTT10":        629577,
		"_default_SERVFAIL":        6533579,
		"_default_AFSDB":           5,
		"STATUS":                   35585,
		"Response":                 36482142477,
		"KeyTagOpt":                0,
		"_default_Mismatch":        0,
		"Requestv6":                156,
		"LOC":                      52,
		"_bind_NXDOMAIN":           0,
		"PTR":                      694347710,
		"_default_NSEC":            53712,
		"_bind_QryRTT100":          0,
		"RESERVED8":                0,
		"DLV":                      37712,
		"HINFO":                    9,
		"_default_AAAA":            818803359,
		"QryNXRedirRLookup":        0,
		"TYPE127":                  1,
		"_default_EDNS0Fail":       3987571,
		"_default_CDS":             251,
		"_bind_ServerCookieOut":    0,
		"_bind_QueryCurUDP":        0,
		"_bind_GlueFetchv6Fail":    0,
		"UpdateFail":               0,
		"_default_ZoneQuota":       0,
		"_default_QuerySockFail":   0,
		"_default_GlueFetchv6Fail": 91852240,
		"RespSIG0":                 0,
		"_default_GlueFetchv4Fail": 3964627,
		"_bind_Responsev6":         0,
		"_default_GlueFetchv6":     121268854,
		"_default_Queryv6":         292282376,
		"TruncatedResp":            25899017,
		"ReqTSIG":                  0,
		"_default_BadEDNSVersion":  0,
		"_bind_NumFetch":           0,
		"RESERVED12":               1,
		"_default_Responsev6":      152,
		"_default_SRV":             3855156,
		"ANY":                      298567781,
		"_default_CNAME":           38213966,
		"_bind_ClientCookieOut":    0,
		"NAPTR":                    109998,
		"_default_QryRTT500":       2075608518,
		"_default_A6":              558874,
		"_bind_OtherError":         0,
		"CookieMatch":              125340,
		"_default_QryRTT1600+":     27681,
		"_default_DLV":             20468,
		"_default_NULL":            3554,
		"_bind_Queryv6":            0,
		"_bind_QueryTimeout":       0,
		"_bind_ValAttempt":         0,
		"RESERVED9":                0,
		"A6":                       539773,
		"MX":                       1484497,
		"QrySuccess":               28810069822,
		"XfrReqDone":               0,
		"RESERVED15":               0,
		"MAILB":                    42,
		"Others":                   74007,
		"_bind_ServerQuota":        0,
		"_bind_EDNS0Fail":          0,
		"QryNxrrset":               1311185019,
		"QryFailure":               225980711,
		"ReqBadSIG":                0,
		"UpdateFwdFail":            0,
		"ECSOpt":                   8743959,
		"QryDropped":               52215943,
		"EUI64":                    627,
		"_default_ValAttempt":      0,
		"_default_A":               3678445415,
		"_bind_QryRTT800":          0,
		"_default_NXDOMAIN":        1247746765,
		"_default_RESERVED0":       19,
		"_default_NumFetch":        62,
		"_bind_Responsev4":         0,
		"_bind_QryRTT1600":         0,
		"CookieNew":                1061071,
		"ReqBadEDNSVer":            450,
		"TYPE115":                  285,
		"_bind_FORMERR":            0,
		"SOA":                      3863889,
		"_bind_QryRTT10":           0,
		"CookieBadSize":            0,
		"_bind_GlueFetchv6":        0,
		"QryNXRedir":               0,
		"AAAA":                     3309600766,
		"_default_QueryAbort":      0,
		"QryTCP":                   4233061,
		"UpdateDone":               0,
		"IQUERY":                   199,
	}

	assert.Equal(t, expected, collr.Collect(context.Background()))
	assert.Len(t, *collr.charts, 20)
}

func TestCollector_InvalidData(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("hello and goodbye"))
	}))
	defer ts.Close()

	collr := New()
	collr.URL = ts.URL + "/json/v1"
	require.NoError(t, collr.Init(context.Background()))
	assert.Error(t, collr.Check(context.Background()))
}

func TestCollector_404(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
	}))
	defer ts.Close()

	collr := New()
	collr.URL = ts.URL + "/json/v1"
	require.NoError(t, collr.Init(context.Background()))
	assert.Error(t, collr.Check(context.Background()))
}
