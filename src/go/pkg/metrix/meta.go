// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

func metricKindPublic(kind metricKind) MetricKind {
	switch kind {
	case kindGauge:
		return MetricKindGauge
	case kindCounter:
		return MetricKindCounter
	case kindHistogram:
		return MetricKindHistogram
	case kindSummary:
		return MetricKindSummary
	case kindStateSet:
		return MetricKindStateSet
	default:
		return MetricKindUnknown
	}
}

func baseSeriesMeta(desc *instrumentDescriptor) SeriesMeta {
	kind := MetricKindUnknown
	if desc != nil {
		kind = metricKindPublic(desc.kind)
	}
	return SeriesMeta{
		Kind:        kind,
		SourceKind:  kind,
		FlattenRole: FlattenRoleNone,
	}
}

func ensureSeriesMeta(desc *instrumentDescriptor, meta *SeriesMeta) {
	if meta == nil {
		return
	}
	if desc == nil {
		return
	}
	if meta.Kind == MetricKindUnknown {
		meta.Kind = metricKindPublic(desc.kind)
	}
	if meta.SourceKind == MetricKindUnknown {
		meta.SourceKind = meta.Kind
	}
}

func flattenedSeriesMeta(base SeriesMeta, kind MetricKind, sourceKind MetricKind, role FlattenRole) SeriesMeta {
	base.Kind = kind
	base.SourceKind = sourceKind
	base.FlattenRole = role
	return base
}
