// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"

func failAcquisitionRoute(route *AcquisitionRouteReport, class AcquisitionFailureClass) {
	if route != nil {
		route.Outcome, route.FailureClass = AcquisitionRouteOutcomeFailed, class
	}
}

func rejectAcquisitionRoute(route *AcquisitionRouteReport) {
	if route != nil {
		route.Rejected++
		route.Outcome, route.FailureClass = AcquisitionRouteOutcomeRejected, AcquisitionFailureClassProcessing
	}
}

func startLicenseScalarRoute(route *AcquisitionRouteReport, oids, missing []string) {
	if route == nil {
		return
	}
	route.Missing = uint64(len(missing))
	if len(oids) > 0 {
		route.Source = AcquisitionRouteSourceGET
	} else if len(missing) > 0 {
		route.Source = AcquisitionRouteSourceCache
	}
}

func finishLicenseScalarRoute(route *AcquisitionRouteReport, requested []string, missing map[string]bool) {
	if route == nil {
		return
	}
	for _, oid := range requested {
		if missing[oid] {
			route.Missing++
		}
	}
	setAcquisitionRouteCounts(route, 1, route.Values, route.Rejected)
}

func (c *acquisitionProfileCollection) addLicenseReference(route *AcquisitionRouteReport, row string) {
	if c == nil || route == nil {
		return
	}
	c.addValueReference(&c.licenseValueReferences, AcquisitionValueReference{RouteOrdinal: route.Ordinal, RowIndex: row, RowOrdinal: uint32(route.Values)})
	route.Values++
}

func firstLicenseRouteOID(cfg ddprofiledefinition.LicensingConfig) string {
	var first string
	add := func(oid string) {
		if oid = trimOID(oid); oid != "" && (first == "" || oid < first) {
			first = oid
		}
	}
	forEachLicenseValue(cfg, func(value ddprofiledefinition.LicenseValueConfig) { add(licenseValueSymbol(value).OID) })
	for _, tag := range cfg.MetricTags {
		add(tag.Symbol.OID)
	}
	return first
}

func (ctx licenseValueContext) record(cfg ddprofiledefinition.LicenseValueConfig, oid, reason string) {
	ctx.processing.record(licenseValueSymbol(cfg).Name, oid, reason)
}
