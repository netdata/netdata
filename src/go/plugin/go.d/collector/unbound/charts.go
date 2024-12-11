// SPDX-License-Identifier: GPL-3.0-or-later

package unbound

import (
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

type (
	// Charts is an alias for module.Charts
	Charts = module.Charts
	// Chart is an alias for module.Charts
	Chart = module.Chart
	// Dims is an alias for module.Dims
	Dims = module.Dims
	// Dim is an alias for module.Dim
	Dim = module.Dim
)

const (
	prioQueries = module.Priority + iota
	prioIPRateLimitedQueries
	prioQueryType
	prioQueryClass
	prioQueryOpCode
	prioQueryFlag
	prioDNSCryptQueries

	prioRecurReplies
	prioReplyRCode

	prioRecurTime

	prioCache
	prioCachePercentage
	prioCachePrefetch
	prioCacheExpired
	prioZeroTTL
	prioCacheCount

	prioReqListUsage
	prioReqListCurUsage
	prioReqListJostle

	prioTCPUsage

	prioMemCache
	prioMemMod
	prioMemStreamWait
	prioUptime

	prioThread
)

func charts(cumulative bool) *Charts {
	return &Charts{
		makeIncrIf(queriesChart.Copy(), cumulative),
		makeIncrIf(ipRateLimitedQueriesChart.Copy(), cumulative),
		makeIncrIf(cacheChart.Copy(), cumulative),
		makePercOfIncrIf(cachePercentageChart.Copy(), cumulative),
		makeIncrIf(prefetchChart.Copy(), cumulative),
		makeIncrIf(expiredChart.Copy(), cumulative),
		makeIncrIf(zeroTTLChart.Copy(), cumulative),
		makeIncrIf(dnsCryptChart.Copy(), cumulative),
		makeIncrIf(recurRepliesChart.Copy(), cumulative),
		recurTimeChart.Copy(),
		reqListUsageChart.Copy(),
		reqListCurUsageChart.Copy(),
		makeIncrIf(reqListJostleChart.Copy(), cumulative),
		tcpUsageChart.Copy(),
		uptimeChart.Copy(),
	}
}

func extendedCharts(cumulative bool) *Charts {
	return &Charts{
		memCacheChart.Copy(),
		memModChart.Copy(),
		memStreamWaitChart.Copy(),
		cacheCountChart.Copy(),
		makeIncrIf(queryTypeChart.Copy(), cumulative),
		makeIncrIf(queryClassChart.Copy(), cumulative),
		makeIncrIf(queryOpCodeChart.Copy(), cumulative),
		makeIncrIf(queryFlagChart.Copy(), cumulative),
		makeIncrIf(answerRCodeChart.Copy(), cumulative),
	}
}

func threadCharts(thread string, cumulative bool) *Charts {
	charts := charts(cumulative)
	_ = charts.Remove(uptimeChart.ID)

	for i, chart := range *charts {
		convertTotalChartToThread(chart, thread, prioThread+i)
	}
	return charts
}

func convertTotalChartToThread(chart *Chart, thread string, priority int) {
	chart.ID = fmt.Sprintf("%s_%s", thread, chart.ID)
	chart.Title = fmt.Sprintf("%s %s",
		cases.Title(language.English, cases.Compact).String(thread),
		chart.Title,
	)
	chart.Fam = thread + "_stats"
	chart.Ctx = "thread_" + chart.Ctx
	chart.Priority = priority
	for _, dim := range chart.Dims {
		dim.ID = strings.Replace(dim.ID, "total", thread, 1)
	}
}

// Common stats charts
// see https://nlnetlabs.nl/documentation/unbound/unbound-control for the stats provided by unbound-control
var (
	queriesChart = Chart{
		ID:       "queries",
		Title:    "Received Queries",
		Units:    "queries",
		Fam:      "queries",
		Ctx:      "unbound.queries",
		Priority: prioQueries,
		Dims: Dims{
			{ID: "total.num.queries", Name: "queries"},
		},
	}
	ipRateLimitedQueriesChart = Chart{
		ID:       "queries_ip_ratelimited",
		Title:    "Rate Limited Queries",
		Units:    "queries",
		Fam:      "queries",
		Ctx:      "unbound.queries_ip_ratelimited",
		Priority: prioIPRateLimitedQueries,
		Dims: Dims{
			{ID: "total.num.queries_ip_ratelimited", Name: "ratelimited"},
		},
	}
	// ifdef USE_DNSCRYPT
	dnsCryptChart = Chart{
		ID:       "dnscrypt_queries",
		Title:    "DNSCrypt Queries",
		Units:    "queries",
		Fam:      "queries",
		Ctx:      "unbound.dnscrypt_queries",
		Priority: prioDNSCryptQueries,
		Dims: Dims{
			{ID: "total.num.dnscrypt.crypted", Name: "crypted"},
			{ID: "total.num.dnscrypt.cert", Name: "cert"},
			{ID: "total.num.dnscrypt.cleartext", Name: "cleartext"},
			{ID: "total.num.dnscrypt.malformed", Name: "malformed"},
		},
	}
	cacheChart = Chart{
		ID:       "cache",
		Title:    "Cache Statistics",
		Units:    "events",
		Fam:      "cache",
		Ctx:      "unbound.cache",
		Type:     module.Stacked,
		Priority: prioCache,
		Dims: Dims{
			{ID: "total.num.cachehits", Name: "hits"},
			{ID: "total.num.cachemiss", Name: "miss"},
		},
	}
	cachePercentageChart = Chart{
		ID:       "cache_percentage",
		Title:    "Cache Statistics Percentage",
		Units:    "percentage",
		Fam:      "cache",
		Ctx:      "unbound.cache_percentage",
		Type:     module.Stacked,
		Priority: prioCachePercentage,
		Dims: Dims{
			{ID: "total.num.cachehits", Name: "hits", Algo: module.PercentOfAbsolute},
			{ID: "total.num.cachemiss", Name: "miss", Algo: module.PercentOfAbsolute},
		},
	}
	prefetchChart = Chart{
		ID:       "cache_prefetch",
		Title:    "Cache Prefetches",
		Units:    "prefetches",
		Fam:      "cache",
		Ctx:      "unbound.prefetch",
		Priority: prioCachePrefetch,
		Dims: Dims{
			{ID: "total.num.prefetch", Name: "prefetches"},
		},
	}
	expiredChart = Chart{
		ID:       "cache_expired",
		Title:    "Replies Served From Expired Cache",
		Units:    "replies",
		Fam:      "cache",
		Ctx:      "unbound.expired",
		Priority: prioCacheExpired,
		Dims: Dims{
			{ID: "total.num.expired", Name: "expired"},
		},
	}
	zeroTTLChart = Chart{
		ID:       "zero_ttl_replies",
		Title:    "Replies Served From Expired Cache",
		Units:    "replies",
		Fam:      "cache",
		Ctx:      "unbound.zero_ttl_replies",
		Priority: prioZeroTTL,
		Dims: Dims{
			{ID: "total.num.zero_ttl", Name: "zero_ttl"},
		},
	}
	recurRepliesChart = Chart{
		ID:       "recursive_replies",
		Title:    "Replies That Needed Recursive Processing",
		Units:    "replies",
		Fam:      "replies",
		Ctx:      "unbound.recursive_replies",
		Priority: prioRecurReplies,
		Dims: Dims{
			{ID: "total.num.recursivereplies", Name: "recursive"},
		},
	}
	recurTimeChart = Chart{
		ID:       "recursion_time",
		Title:    "Time Spent On Recursive Processing",
		Units:    "milliseconds",
		Fam:      "recursion timings",
		Ctx:      "unbound.recursion_time",
		Priority: prioRecurTime,
		Dims: Dims{
			{ID: "total.recursion.time.avg", Name: "avg"},
			{ID: "total.recursion.time.median", Name: "median"},
		},
	}
	reqListUsageChart = Chart{
		ID:       "request_list_usage",
		Title:    "Request List Usage",
		Units:    "queries",
		Fam:      "request list",
		Ctx:      "unbound.request_list_usage",
		Priority: prioReqListUsage,
		Dims: Dims{
			{ID: "total.requestlist.avg", Name: "avg", Div: 1000},
			{ID: "total.requestlist.max", Name: "max"}, // all time max in cumulative mode, never resets
		},
	}
	reqListCurUsageChart = Chart{
		ID:       "current_request_list_usage",
		Title:    "Current Request List Usage",
		Units:    "queries",
		Fam:      "request list",
		Ctx:      "unbound.current_request_list_usage",
		Type:     module.Area,
		Priority: prioReqListCurUsage,
		Dims: Dims{
			{ID: "total.requestlist.current.all", Name: "all"},
			{ID: "total.requestlist.current.user", Name: "user"},
		},
	}
	reqListJostleChart = Chart{
		ID:       "request_list_jostle_list",
		Title:    "Request List Jostle List Events",
		Units:    "queries",
		Fam:      "request list",
		Ctx:      "unbound.request_list_jostle_list",
		Priority: prioReqListJostle,
		Dims: Dims{
			{ID: "total.requestlist.overwritten", Name: "overwritten"},
			{ID: "total.requestlist.exceeded", Name: "dropped"},
		},
	}
	tcpUsageChart = Chart{
		ID:       "tcpusage",
		Title:    "TCP Handler Buffers",
		Units:    "buffers",
		Fam:      "tcp buffers",
		Ctx:      "unbound.tcpusage",
		Priority: prioTCPUsage,
		Dims: Dims{
			{ID: "total.tcpusage", Name: "usage"},
		},
	}
	uptimeChart = Chart{
		ID:       "uptime",
		Title:    "Uptime",
		Units:    "seconds",
		Fam:      "uptime",
		Ctx:      "unbound.uptime",
		Priority: prioUptime,
		Dims: Dims{
			{ID: "time.up", Name: "time"},
		},
	}
)

// Extended stats charts
var (
	// TODO: do not add dnscrypt stuff by default?
	memCacheChart = Chart{
		ID:       "cache_memory",
		Title:    "Cache Memory",
		Units:    "KB",
		Fam:      "mem",
		Ctx:      "unbound.cache_memory",
		Type:     module.Stacked,
		Priority: prioMemCache,
		Dims: Dims{
			{ID: "mem.cache.message", Name: "message", Div: 1024},
			{ID: "mem.cache.rrset", Name: "rrset", Div: 1024},
			{ID: "mem.cache.dnscrypt_nonce", Name: "dnscrypt_nonce", Div: 1024},                 // ifdef USE_DNSCRYPT
			{ID: "mem.cache.dnscrypt_shared_secret", Name: "dnscrypt_shared_secret", Div: 1024}, // ifdef USE_DNSCRYPT
		},
	}
	// TODO: do not add subnet and ipsecmod stuff by default?
	memModChart = Chart{
		ID:       "mod_memory",
		Title:    "Module Memory",
		Units:    "KB",
		Fam:      "mem",
		Ctx:      "unbound.mod_memory",
		Type:     module.Stacked,
		Priority: prioMemMod,
		Dims: Dims{
			{ID: "mem.mod.iterator", Name: "iterator", Div: 1024},
			{ID: "mem.mod.respip", Name: "respip", Div: 1024},
			{ID: "mem.mod.validator", Name: "validator", Div: 1024},
			{ID: "mem.mod.subnet", Name: "subnet", Div: 1024},  // ifdef CLIENT_SUBNET
			{ID: "mem.mod.ipsecmod", Name: "ipsec", Div: 1024}, // ifdef USE_IPSECMOD
		},
	}
	memStreamWaitChart = Chart{
		ID:       "mem_stream_wait",
		Title:    "TCP and TLS Stream Waif Buffer Memory",
		Units:    "KB",
		Fam:      "mem",
		Ctx:      "unbound.mem_streamwait",
		Priority: prioMemStreamWait,
		Dims: Dims{
			{ID: "mem.streamwait", Name: "streamwait", Div: 1024},
		},
	}
	// NOTE: same family as for cacheChart
	cacheCountChart = Chart{
		ID:       "cache_count",
		Title:    "Cache Items Count",
		Units:    "items",
		Fam:      "cache",
		Ctx:      "unbound.cache_count",
		Type:     module.Stacked,
		Priority: prioCacheCount,
		Dims: Dims{
			{ID: "infra.cache.count", Name: "infra"},
			{ID: "key.cache.count", Name: "key"},
			{ID: "msg.cache.count", Name: "msg"},
			{ID: "rrset.cache.count", Name: "rrset"},
			{ID: "dnscrypt_nonce.cache.count", Name: "dnscrypt_nonce"},
			{ID: "dnscrypt_shared_secret.cache.count", Name: "shared_secret"},
		},
	}
	queryTypeChart = Chart{
		ID:       "queries_by_type",
		Title:    "Queries By Type",
		Units:    "queries",
		Fam:      "queries",
		Ctx:      "unbound.type_queries",
		Type:     module.Stacked,
		Priority: prioQueryType,
	}
	queryClassChart = Chart{
		ID:       "queries_by_class",
		Title:    "Queries By Class",
		Units:    "queries",
		Fam:      "queries",
		Ctx:      "unbound.class_queries",
		Type:     module.Stacked,
		Priority: prioQueryClass,
	}
	queryOpCodeChart = Chart{
		ID:       "queries_by_opcode",
		Title:    "Queries By OpCode",
		Units:    "queries",
		Fam:      "queries",
		Ctx:      "unbound.opcode_queries",
		Type:     module.Stacked,
		Priority: prioQueryOpCode,
	}
	queryFlagChart = Chart{
		ID:       "queries_by_flag",
		Title:    "Queries By Flag",
		Units:    "queries",
		Fam:      "queries",
		Ctx:      "unbound.flag_queries",
		Type:     module.Stacked,
		Priority: prioQueryFlag,
		Dims: Dims{
			{ID: "num.query.flags.QR", Name: "QR"},
			{ID: "num.query.flags.AA", Name: "AA"},
			{ID: "num.query.flags.TC", Name: "TC"},
			{ID: "num.query.flags.RD", Name: "RD"},
			{ID: "num.query.flags.RA", Name: "RA"},
			{ID: "num.query.flags.Z", Name: "Z"},
			{ID: "num.query.flags.AD", Name: "AD"},
			{ID: "num.query.flags.CD", Name: "CD"},
		},
	}
	answerRCodeChart = Chart{
		ID:       "replies_by_rcode",
		Title:    "Replies By RCode",
		Units:    "replies",
		Fam:      "replies",
		Ctx:      "unbound.rcode_answers",
		Type:     module.Stacked,
		Priority: prioReplyRCode,
	}
)

func (c *Collector) updateCharts() {
	if len(c.curCache.threads) > 1 {
		for v := range c.curCache.threads {
			if !c.cache.threads[v] {
				c.cache.threads[v] = true
				c.addThreadCharts(v)
			}
		}
	}
	// 0-6 rcodes always included
	if hasExtendedData := len(c.curCache.answerRCode) > 0; !hasExtendedData {
		return
	}

	if !c.extChartsCreated {
		charts := extendedCharts(c.Cumulative)
		if err := c.Charts().Add(*charts...); err != nil {
			c.Warningf("add extended charts: %v", err)
		}
		c.extChartsCreated = true
	}

	for v := range c.curCache.queryType {
		if !c.cache.queryType[v] {
			c.cache.queryType[v] = true
			c.addDimToQueryTypeChart(v)
		}
	}
	for v := range c.curCache.queryClass {
		if !c.cache.queryClass[v] {
			c.cache.queryClass[v] = true
			c.addDimToQueryClassChart(v)
		}
	}
	for v := range c.curCache.queryOpCode {
		if !c.cache.queryOpCode[v] {
			c.cache.queryOpCode[v] = true
			c.addDimToQueryOpCodeChart(v)
		}
	}
	for v := range c.curCache.answerRCode {
		if !c.cache.answerRCode[v] {
			c.cache.answerRCode[v] = true
			c.addDimToAnswerRcodeChart(v)
		}
	}
}

func (c *Collector) addThreadCharts(thread string) {
	charts := threadCharts(thread, c.Cumulative)
	if err := c.Charts().Add(*charts...); err != nil {
		c.Warningf("add '%s' charts: %v", thread, err)
	}
}

func (c *Collector) addDimToQueryTypeChart(typ string) {
	c.addDimToChart(queryTypeChart.ID, "num.query.type."+typ, typ)
}
func (c *Collector) addDimToQueryClassChart(class string) {
	c.addDimToChart(queryClassChart.ID, "num.query.class."+class, class)
}
func (c *Collector) addDimToQueryOpCodeChart(opcode string) {
	c.addDimToChart(queryOpCodeChart.ID, "num.query.opcode."+opcode, opcode)
}
func (c *Collector) addDimToAnswerRcodeChart(rcode string) {
	c.addDimToChart(answerRCodeChart.ID, "num.answer.rcode."+rcode, rcode)
}

func (c *Collector) addDimToChart(chartID, dimID, dimName string) {
	chart := c.Charts().Get(chartID)
	if chart == nil {
		c.Warningf("add '%s' dim: couldn't find '%s' chart", dimID, chartID)
		return
	}
	dim := &Dim{ID: dimID, Name: dimName}
	if c.Cumulative {
		dim.Algo = module.Incremental
	}
	if err := chart.AddDim(dim); err != nil {
		c.Warningf("add '%s' dim: %v", dimID, err)
		return
	}
	chart.MarkNotCreated()
}

func makeIncrIf(chart *Chart, do bool) *Chart {
	if !do {
		return chart
	}
	chart.Units += "/s"
	for _, d := range chart.Dims {
		d.Algo = module.Incremental
	}
	return chart
}

func makePercOfIncrIf(chart *Chart, do bool) *Chart {
	if !do {
		return chart
	}
	for _, d := range chart.Dims {
		d.Algo = module.PercentOfIncremental
	}
	return chart
}
