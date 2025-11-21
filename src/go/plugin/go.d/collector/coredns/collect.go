// SPDX-License-Identifier: GPL-3.0-or-later

package coredns

import (
	"errors"
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/prometheus"
	"github.com/netdata/netdata/go/plugins/pkg/stm"

	"github.com/blang/semver/v4"
)

const (
	metricPanicCountTotal169orOlder         = "coredns_panic_count_total"
	metricRequestCountTotal169orOlder       = "coredns_dns_request_count_total"
	metricRequestTypeCountTotal169orOlder   = "coredns_dns_request_type_count_total"
	metricResponseRcodeCountTotal169orOlder = "coredns_dns_response_rcode_count_total"

	metricPanicCountTotal170orNewer         = "coredns_panics_total"
	metricRequestCountTotal170orNewer       = "coredns_dns_requests_total"
	metricRequestTypeCountTotal170orNewer   = "coredns_dns_requests_total"
	metricResponseRcodeCountTotal170orNewer = "coredns_dns_responses_total"
)

var (
	empty                  = ""
	dropped                = "dropped"
	emptyServerReplaceName = "empty"
	rootZoneReplaceName    = "root"
	version169             = semver.MustParse("1.6.9")
)

type requestMetricsNames struct {
	panicCountTotal string
	// true for all metrics below:
	// - if none of server block matches 'server' tag is "", empty server has only one zone - dropped.
	//   example:
	//   coredns_dns_requests_total{family="1",proto="udp",server="",zone="dropped"} 1 for
	// - dropped requests are added to both dropped and corresponding zone
	//   example:
	//   coredns_dns_requests_total{family="1",proto="udp",server="dns://:53",zone="dropped"} 2
	//   coredns_dns_requests_total{family="1",proto="udp",server="dns://:53",zone="ya.ru."} 2
	requestCountTotal       string
	requestTypeCountTotal   string
	responseRcodeCountTotal string
}

func (c *Collector) collect() (map[string]int64, error) {
	raw, err := c.prom.ScrapeSeries()

	if err != nil {
		return nil, err
	}

	mx := newMetrics()

	// some metric names are different depending on the version
	// update them once
	if !c.skipVersionCheck {
		c.updateVersionDependentMetrics(raw)
		c.skipVersionCheck = true
	}

	//we can only get these metrics if we know the server version
	if c.version == nil {
		return nil, errors.New("unable to determine server version")
	}

	c.collectPanic(mx, raw)
	c.collectSummaryRequests(mx, raw)
	c.collectSummaryRequestsPerType(mx, raw)
	c.collectSummaryResponsesPerRcode(mx, raw)

	if c.perServerMatcher != nil {
		c.collectPerServerRequests(mx, raw)
		//c.collectPerServerRequestsDuration(mx, raw)
		c.collectPerServerRequestPerType(mx, raw)
		c.collectPerServerResponsePerRcode(mx, raw)
	}

	if c.perZoneMatcher != nil {
		c.collectPerZoneRequests(mx, raw)
		//c.collectPerZoneRequestsDuration(mx, raw)
		c.collectPerZoneRequestsPerType(mx, raw)
		c.collectPerZoneResponsesPerRcode(mx, raw)
	}

	return stm.ToMap(mx), nil
}

func (c *Collector) updateVersionDependentMetrics(raw prometheus.Series) {
	version := c.parseVersion(raw)
	if version == nil {
		return
	}
	c.version = version
	if c.version.LTE(version169) {
		c.metricNames.panicCountTotal = metricPanicCountTotal169orOlder
		c.metricNames.requestCountTotal = metricRequestCountTotal169orOlder
		c.metricNames.requestTypeCountTotal = metricRequestTypeCountTotal169orOlder
		c.metricNames.responseRcodeCountTotal = metricResponseRcodeCountTotal169orOlder
	} else {
		c.metricNames.panicCountTotal = metricPanicCountTotal170orNewer
		c.metricNames.requestCountTotal = metricRequestCountTotal170orNewer
		c.metricNames.requestTypeCountTotal = metricRequestTypeCountTotal170orNewer
		c.metricNames.responseRcodeCountTotal = metricResponseRcodeCountTotal170orNewer
	}
}

func (c *Collector) parseVersion(raw prometheus.Series) *semver.Version {
	var versionStr string
	for _, metric := range raw.FindByName("coredns_build_info") {
		versionStr = metric.Labels.Get("version")
	}
	if versionStr == "" {
		c.Error("cannot find version string in metrics")
		return nil
	}

	version, err := semver.Make(versionStr)
	if err != nil {
		c.Errorf("failed to find server version: %v", err)
		return nil
	}
	return &version
}

func (c *Collector) collectPanic(mx *metrics, raw prometheus.Series) {
	mx.Panic.Set(raw.FindByName(c.metricNames.panicCountTotal).Max())
}

func (c *Collector) collectSummaryRequests(mx *metrics, raw prometheus.Series) {
	for _, metric := range raw.FindByName(c.metricNames.requestCountTotal) {
		var (
			family = metric.Labels.Get("family")
			proto  = metric.Labels.Get("proto")
			server = metric.Labels.Get("server")
			zone   = metric.Labels.Get("zone")
			value  = metric.Value
		)

		if family == empty || proto == empty || zone == empty {
			continue
		}

		if server == empty {
			mx.NoZoneDropped.Add(value)
		}

		setRequestPerStatus(&mx.Summary.Request, value, server, zone)

		if zone == dropped && server != empty {
			continue
		}

		mx.Summary.Request.Total.Add(value)
		setRequestPerIPFamily(&mx.Summary.Request, value, family)
		setRequestPerProto(&mx.Summary.Request, value, proto)
	}
}

//func (cd *Collector) collectSummaryRequestsDuration(mx *metrics, raw prometheus.Series) {
//	for _, metric := range raw.FindByName(metricRequestDurationSecondsBucket) {
//		var (
//			server = metric.Labels.Get("server")
//			zone   = metric.Labels.Get("zone")
//			le     = metric.Labels.Get("le")
//			value  = metric.Value
//		)
//
//		if zone == empty || zone == dropped && server != empty || le == empty {
//			continue
//		}
//
//		setRequestDuration(&mx.Summary.RequestConfig, value, le)
//	}
//	processRequestDuration(&mx.Summary.RequestConfig)
//}

func (c *Collector) collectSummaryRequestsPerType(mx *metrics, raw prometheus.Series) {
	for _, metric := range raw.FindByName(c.metricNames.requestTypeCountTotal) {
		var (
			server = metric.Labels.Get("server")
			typ    = metric.Labels.Get("type")
			zone   = metric.Labels.Get("zone")
			value  = metric.Value
		)

		if typ == empty || zone == empty || zone == dropped && server != empty {
			continue
		}

		setRequestPerType(&mx.Summary.Request, value, typ)
	}
}

func (c *Collector) collectSummaryResponsesPerRcode(mx *metrics, raw prometheus.Series) {
	for _, metric := range raw.FindByName(c.metricNames.responseRcodeCountTotal) {
		var (
			rcode  = metric.Labels.Get("rcode")
			server = metric.Labels.Get("server")
			zone   = metric.Labels.Get("zone")
			value  = metric.Value
		)

		if rcode == empty || zone == empty || zone == dropped && server != empty {
			continue
		}

		setResponsePerRcode(&mx.Summary.Response, value, rcode)
	}
}

// Per Server

func (c *Collector) collectPerServerRequests(mx *metrics, raw prometheus.Series) {
	for _, metric := range raw.FindByName(c.metricNames.requestCountTotal) {
		var (
			family = metric.Labels.Get("family")
			proto  = metric.Labels.Get("proto")
			server = metric.Labels.Get("server")
			zone   = metric.Labels.Get("zone")
			value  = metric.Value
		)

		if family == empty || proto == empty || zone == empty {
			continue
		}

		if !c.perServerMatcher.MatchString(server) {
			continue
		}

		if server == empty {
			server = emptyServerReplaceName
		}

		if !c.collectedServers[server] {
			c.addNewServerCharts(server)
			c.collectedServers[server] = true
		}

		if _, ok := mx.PerServer[server]; !ok {
			mx.PerServer[server] = &requestResponse{}
		}

		srv := mx.PerServer[server]

		setRequestPerStatus(&srv.Request, value, server, zone)

		if zone == dropped && server != emptyServerReplaceName {
			continue
		}

		srv.Request.Total.Add(value)
		setRequestPerIPFamily(&srv.Request, value, family)
		setRequestPerProto(&srv.Request, value, proto)
	}
}

//func (cd *Collector) collectPerServerRequestsDuration(mx *metrics, raw prometheus.Series) {
//	for _, metric := range raw.FindByName(metricRequestDurationSecondsBucket) {
//		var (
//			server = metric.Labels.Get("server")
//			zone   = metric.Labels.Get("zone")
//			le     = metric.Labels.Get("le")
//			value  = metric.Value
//		)
//
//		if zone == empty || zone == dropped && server != empty || le == empty {
//			continue
//		}
//
//		if !cd.perServerMatcher.MatchString(server) {
//			continue
//		}
//
//		if server == empty {
//			server = emptyServerReplaceName
//		}
//
//		if !cd.collectedServers[server] {
//			cd.addNewServerCharts(server)
//			cd.collectedServers[server] = true
//		}
//
//		if _, ok := mx.PerServer[server]; !ok {
//			mx.PerServer[server] = &requestResponse{}
//		}
//
//		setRequestDuration(&mx.PerServer[server].RequestConfig, value, le)
//	}
//	for _, s := range mx.PerServer {
//		processRequestDuration(&s.RequestConfig)
//	}
//}

func (c *Collector) collectPerServerRequestPerType(mx *metrics, raw prometheus.Series) {
	for _, metric := range raw.FindByName(c.metricNames.requestTypeCountTotal) {
		var (
			server = metric.Labels.Get("server")
			typ    = metric.Labels.Get("type")
			zone   = metric.Labels.Get("zone")
			value  = metric.Value
		)

		if typ == empty || zone == empty || zone == dropped && server != empty {
			continue
		}

		if !c.perServerMatcher.MatchString(server) {
			continue
		}

		if server == empty {
			server = emptyServerReplaceName
		}

		if !c.collectedServers[server] {
			c.addNewServerCharts(server)
			c.collectedServers[server] = true
		}

		if _, ok := mx.PerServer[server]; !ok {
			mx.PerServer[server] = &requestResponse{}
		}

		setRequestPerType(&mx.PerServer[server].Request, value, typ)
	}
}

func (c *Collector) collectPerServerResponsePerRcode(mx *metrics, raw prometheus.Series) {
	for _, metric := range raw.FindByName(c.metricNames.responseRcodeCountTotal) {
		var (
			rcode  = metric.Labels.Get("rcode")
			server = metric.Labels.Get("server")
			zone   = metric.Labels.Get("zone")
			value  = metric.Value
		)

		if rcode == empty || zone == empty || zone == dropped && server != empty {
			continue
		}

		if !c.perServerMatcher.MatchString(server) {
			continue
		}

		if server == empty {
			server = emptyServerReplaceName
		}

		if !c.collectedServers[server] {
			c.addNewServerCharts(server)
			c.collectedServers[server] = true
		}

		if _, ok := mx.PerServer[server]; !ok {
			mx.PerServer[server] = &requestResponse{}
		}

		setResponsePerRcode(&mx.PerServer[server].Response, value, rcode)
	}
}

// Per Zone

func (c *Collector) collectPerZoneRequests(mx *metrics, raw prometheus.Series) {
	for _, metric := range raw.FindByName(c.metricNames.requestCountTotal) {
		var (
			family = metric.Labels.Get("family")
			proto  = metric.Labels.Get("proto")
			zone   = metric.Labels.Get("zone")
			value  = metric.Value
		)

		if family == empty || proto == empty || zone == empty {
			continue
		}

		if !c.perZoneMatcher.MatchString(zone) {
			continue
		}

		if zone == "." {
			zone = rootZoneReplaceName
		}

		if !c.collectedZones[zone] {
			c.addNewZoneCharts(zone)
			c.collectedZones[zone] = true
		}

		if _, ok := mx.PerZone[zone]; !ok {
			mx.PerZone[zone] = &requestResponse{}
		}

		zoneMX := mx.PerZone[zone]
		zoneMX.Request.Total.Add(value)
		setRequestPerIPFamily(&zoneMX.Request, value, family)
		setRequestPerProto(&zoneMX.Request, value, proto)
	}
}

//func (cd *Collector) collectPerZoneRequestsDuration(mx *metrics, raw prometheus.Series) {
//	for _, metric := range raw.FindByName(metricRequestDurationSecondsBucket) {
//		var (
//			zone  = metric.Labels.Get("zone")
//			le    = metric.Labels.Get("le")
//			value = metric.Value
//		)
//
//		if zone == empty || le == empty {
//			continue
//		}
//
//		if !cd.perZoneMatcher.MatchString(zone) {
//			continue
//		}
//
//		if zone == "." {
//			zone = rootZoneReplaceName
//		}
//
//		if !cd.collectedZones[zone] {
//			cd.addNewZoneCharts(zone)
//			cd.collectedZones[zone] = true
//		}
//
//		if _, ok := mx.PerZone[zone]; !ok {
//			mx.PerZone[zone] = &requestResponse{}
//		}
//
//		setRequestDuration(&mx.PerZone[zone].RequestConfig, value, le)
//	}
//	for _, s := range mx.PerZone {
//		processRequestDuration(&s.RequestConfig)
//	}
//}

func (c *Collector) collectPerZoneRequestsPerType(mx *metrics, raw prometheus.Series) {
	for _, metric := range raw.FindByName(c.metricNames.requestTypeCountTotal) {
		var (
			typ   = metric.Labels.Get("type")
			zone  = metric.Labels.Get("zone")
			value = metric.Value
		)

		if typ == empty || zone == empty {
			continue
		}

		if !c.perZoneMatcher.MatchString(zone) {
			continue
		}

		if zone == "." {
			zone = rootZoneReplaceName
		}

		if !c.collectedZones[zone] {
			c.addNewZoneCharts(zone)
			c.collectedZones[zone] = true
		}

		if _, ok := mx.PerZone[zone]; !ok {
			mx.PerZone[zone] = &requestResponse{}
		}

		setRequestPerType(&mx.PerZone[zone].Request, value, typ)
	}
}

func (c *Collector) collectPerZoneResponsesPerRcode(mx *metrics, raw prometheus.Series) {
	for _, metric := range raw.FindByName(c.metricNames.responseRcodeCountTotal) {
		var (
			rcode = metric.Labels.Get("rcode")
			zone  = metric.Labels.Get("zone")
			value = metric.Value
		)

		if rcode == empty || zone == empty {
			continue
		}

		if !c.perZoneMatcher.MatchString(zone) {
			continue
		}

		if zone == "." {
			zone = rootZoneReplaceName
		}

		if !c.collectedZones[zone] {
			c.addNewZoneCharts(zone)
			c.collectedZones[zone] = true
		}

		if _, ok := mx.PerZone[zone]; !ok {
			mx.PerZone[zone] = &requestResponse{}
		}

		setResponsePerRcode(&mx.PerZone[zone].Response, value, rcode)
	}
}

// ---

func setRequestPerIPFamily(mx *request, value float64, family string) {
	switch family {
	case "1":
		mx.PerIPFamily.IPv4.Add(value)
	case "2":
		mx.PerIPFamily.IPv6.Add(value)
	}
}

func setRequestPerProto(mx *request, value float64, proto string) {
	switch proto {
	case "udp":
		mx.PerProto.UDP.Add(value)
	case "tcp":
		mx.PerProto.TCP.Add(value)
	}
}

func setRequestPerStatus(mx *request, value float64, server, zone string) {
	switch zone {
	default:
		mx.PerStatus.Processed.Add(value)
	case "dropped":
		mx.PerStatus.Dropped.Add(value)
		if server == empty || server == emptyServerReplaceName {
			return
		}
		mx.PerStatus.Processed.Sub(value)
	}
}

func setRequestPerType(mx *request, value float64, typ string) {
	switch typ {
	default:
		mx.PerType.Other.Add(value)
	case "A":
		mx.PerType.A.Add(value)
	case "AAAA":
		mx.PerType.AAAA.Add(value)
	case "MX":
		mx.PerType.MX.Add(value)
	case "SOA":
		mx.PerType.SOA.Add(value)
	case "CNAME":
		mx.PerType.CNAME.Add(value)
	case "PTR":
		mx.PerType.PTR.Add(value)
	case "TXT":
		mx.PerType.TXT.Add(value)
	case "NS":
		mx.PerType.NS.Add(value)
	case "DS":
		mx.PerType.DS.Add(value)
	case "DNSKEY":
		mx.PerType.DNSKEY.Add(value)
	case "RRSIG":
		mx.PerType.RRSIG.Add(value)
	case "NSEC":
		mx.PerType.NSEC.Add(value)
	case "NSEC3":
		mx.PerType.NSEC3.Add(value)
	case "IXFR":
		mx.PerType.IXFR.Add(value)
	case "ANY":
		mx.PerType.ANY.Add(value)
	}
}

func setResponsePerRcode(mx *response, value float64, rcode string) {
	mx.Total.Add(value)

	switch rcode {
	default:
		mx.PerRcode.Other.Add(value)
	case "NOERROR":
		mx.PerRcode.NOERROR.Add(value)
	case "FORMERR":
		mx.PerRcode.FORMERR.Add(value)
	case "SERVFAIL":
		mx.PerRcode.SERVFAIL.Add(value)
	case "NXDOMAIN":
		mx.PerRcode.NXDOMAIN.Add(value)
	case "NOTIMP":
		mx.PerRcode.NOTIMP.Add(value)
	case "REFUSED":
		mx.PerRcode.REFUSED.Add(value)
	case "YXDOMAIN":
		mx.PerRcode.YXDOMAIN.Add(value)
	case "YXRRSET":
		mx.PerRcode.YXRRSET.Add(value)
	case "NXRRSET":
		mx.PerRcode.NXRRSET.Add(value)
	case "NOTAUTH":
		mx.PerRcode.NOTAUTH.Add(value)
	case "NOTZONE":
		mx.PerRcode.NOTZONE.Add(value)
	case "BADSIG":
		mx.PerRcode.BADSIG.Add(value)
	case "BADKEY":
		mx.PerRcode.BADKEY.Add(value)
	case "BADTIME":
		mx.PerRcode.BADTIME.Add(value)
	case "BADMODE":
		mx.PerRcode.BADMODE.Add(value)
	case "BADNAME":
		mx.PerRcode.BADNAME.Add(value)
	case "BADALG":
		mx.PerRcode.BADALG.Add(value)
	case "BADTRUNC":
		mx.PerRcode.BADTRUNC.Add(value)
	case "BADCOOKIE":
		mx.PerRcode.BADCOOKIE.Add(value)
	}
}

//func setRequestDuration(mx *request, value float64, le string) {
//	switch le {
//	case "0.00025":
//		mx.Duration.LE000025.Add(value)
//	case "0.0005":
//		mx.Duration.LE00005.Add(value)
//	case "0.001":
//		mx.Duration.LE0001.Add(value)
//	case "0.002":
//		mx.Duration.LE0002.Add(value)
//	case "0.004":
//		mx.Duration.LE0004.Add(value)
//	case "0.008":
//		mx.Duration.LE0008.Add(value)
//	case "0.016":
//		mx.Duration.LE0016.Add(value)
//	case "0.032":
//		mx.Duration.LE0032.Add(value)
//	case "0.064":
//		mx.Duration.LE0064.Add(value)
//	case "0.128":
//		mx.Duration.LE0128.Add(value)
//	case "0.256":
//		mx.Duration.LE0256.Add(value)
//	case "0.512":
//		mx.Duration.LE0512.Add(value)
//	case "1.024":
//		mx.Duration.LE1024.Add(value)
//	case "2.048":
//		mx.Duration.LE2048.Add(value)
//	case "4.096":
//		mx.Duration.LE4096.Add(value)
//	case "8.192":
//		mx.Duration.LE8192.Add(value)
//	case "+Inf":
//		mx.Duration.LEInf.Add(value)
//	}
//}

//func processRequestDuration(mx *request) {
//	mx.Duration.LEInf.Sub(mx.Duration.LE8192.Value())
//	mx.Duration.LE8192.Sub(mx.Duration.LE4096.Value())
//	mx.Duration.LE4096.Sub(mx.Duration.LE2048.Value())
//	mx.Duration.LE2048.Sub(mx.Duration.LE1024.Value())
//	mx.Duration.LE1024.Sub(mx.Duration.LE0512.Value())
//	mx.Duration.LE0512.Sub(mx.Duration.LE0256.Value())
//	mx.Duration.LE0256.Sub(mx.Duration.LE0128.Value())
//	mx.Duration.LE0128.Sub(mx.Duration.LE0064.Value())
//	mx.Duration.LE0064.Sub(mx.Duration.LE0032.Value())
//	mx.Duration.LE0032.Sub(mx.Duration.LE0016.Value())
//	mx.Duration.LE0016.Sub(mx.Duration.LE0008.Value())
//	mx.Duration.LE0008.Sub(mx.Duration.LE0004.Value())
//	mx.Duration.LE0004.Sub(mx.Duration.LE0002.Value())
//	mx.Duration.LE0002.Sub(mx.Duration.LE0001.Value())
//	mx.Duration.LE0001.Sub(mx.Duration.LE00005.Value())
//	mx.Duration.LE00005.Sub(mx.Duration.LE000025.Value())
//}

// ---

func (c *Collector) addNewServerCharts(name string) {
	charts := serverCharts.Copy()
	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, "server", name)
		chart.Title = fmt.Sprintf(chart.Title, "Server", name)
		chart.Fam = fmt.Sprintf(chart.Fam, "server", name)

		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, name)
		}
	}
	_ = c.charts.Add(*charts...)
}

func (c *Collector) addNewZoneCharts(name string) {
	charts := zoneCharts.Copy()
	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, "zone", name)
		chart.Title = fmt.Sprintf(chart.Title, "Zone", name)
		chart.Fam = fmt.Sprintf(chart.Fam, "zone", name)
		chart.Ctx = strings.Replace(chart.Ctx, "coredns.server_", "coredns.zone_", 1)

		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, name)
		}
	}
	_ = c.charts.Add(*charts...)
}
