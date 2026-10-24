// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"maps"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/diagnostics"
)

func normalMetric(value ddsnmp.Metric) diagnostics.NormalMetric {
	result := diagnostics.NormalMetric{
		Name:        value.Name,
		Description: value.Description,
		Family:      value.Family,
		Unit:        value.Unit,
		ChartType:   value.ChartType,
		MetricType:  value.MetricType,
		StaticTags:  maps.Clone(value.StaticTags),
		Tags:        maps.Clone(value.Tags),
		Table:       value.Table,
		Value:       value.Value,
		MultiValue:  maps.Clone(value.MultiValue),
		IsTable:     value.IsTable,
		IsVirtual:   value.IsVirtual,
	}
	if value.Profile != nil {
		result.Profile = value.Profile.Source
	}
	return result
}

func normalLicense(value licenseRow) diagnostics.NormalLicense {
	result := diagnostics.NormalLicense{
		ID:                   value.ID,
		StructuralID:         value.StructuralID,
		Source:               value.Source,
		Table:                value.Table,
		Name:                 value.Name,
		Feature:              value.Feature,
		Component:            value.Component,
		Type:                 value.Type,
		Impact:               value.Impact,
		StateRaw:             value.StateRaw,
		StateSeverity:        value.StateSeverity,
		HasState:             value.HasState,
		StateBucket:          string(value.StateBucket),
		ExpiryTS:             value.ExpiryTS,
		HasExpiry:            value.HasExpiry,
		AuthorizationExpiry:  value.AuthorizationExpiry,
		HasAuthorizationTime: value.HasAuthorizationTime,
		CertificateExpiry:    value.CertificateExpiry,
		HasCertificateTime:   value.HasCertificateTime,
		GraceExpiry:          value.GraceExpiry,
		HasGraceTime:         value.HasGraceTime,
		Usage:                value.Usage,
		HasUsage:             value.HasUsage,
		Capacity:             value.Capacity,
		HasCapacity:          value.HasCapacity,
		Available:            value.Available,
		HasAvailable:         value.HasAvailable,
		UsagePercent:         value.UsagePercent,
		HasUsagePct:          value.HasUsagePct,
		IsUnlimited:          value.IsUnlimited,
		IsPerpetual:          value.IsPerpetual,
		ExpirySource:         value.ExpirySource,
		AuthSource:           value.AuthSource,
		CertSource:           value.CertSource,
		GraceSource:          value.GraceSource,
	}
	return result
}

func normalBGPPeer(value *bgpPeerEntry) diagnostics.NormalBGPPeer {
	result := diagnostics.NormalBGPPeer{
		Key:                  value.key,
		Scope:                value.scope,
		Source:               value.source,
		Tags:                 maps.Clone(value.tags),
		Stale:                value.stale,
		LastUpdate:           value.lastUpdate,
		LastFailure:          value.lastFailure,
		AdminStatus:          value.adminStatus,
		State:                value.state,
		PreviousState:        value.previousState,
		EstablishedUptime:    value.establishedUptime,
		LastReceivedUpdate:   value.lastReceivedUpdate,
		EstablishedCount:     value.establishedCount,
		DownTransitions:      value.downTransitions,
		UpTransitions:        value.upTransitions,
		Flaps:                value.flaps,
		LastErrorCode:        value.lastErrorCode,
		LastErrorSubcode:     value.lastErrorSubcode,
		LastErrorText:        value.lastErrorText,
		LastDownReason:       value.lastDownReason,
		LastRecvNotify:       value.lastRecvNotify,
		LastSentNotify:       value.lastSentNotify,
		GracefulRestart:      value.gracefulRestart,
		UnavailabilityReason: value.unavailabilityReason,
		UpdateCounts:         maps.Clone(value.updateCounts),
		MessageCounts:        maps.Clone(value.messageCounts),
		NotificationCounts:   maps.Clone(value.notificationCounts),
		RouteRefreshCounts:   maps.Clone(value.routeRefreshCounts),
		OpenCounts:           maps.Clone(value.openCounts),
		KeepaliveCounts:      maps.Clone(value.keepaliveCounts),
		RouteCounts:          maps.Clone(value.routeCounts),
		RouteTotals:          maps.Clone(value.routeTotals),
		RouteLimits:          maps.Clone(value.routeLimits),
		RouteLimitThresholds: maps.Clone(value.routeLimitThresholds),
	}
	return result
}
