// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ddprofiledefinition

import "errors"

// ValidateEnrichProfile validates a profile and normalizes it.
func ValidateEnrichProfile(p *ProfileDefinition) error {
	p.Normalize()

	errs := []error{
		validateEnrichSelector(p),
		validateEnrichMetadata(p.Metadata),
		validateEnrichSysobjectIDMetadata(p.SysobjectIDMetadata),
		validateEnrichMetrics(p.Metrics),
		validateEnrichTopology(p.Topology),
		validateEnrichLicensing(p.Licensing),
		validateEnrichBGP(p.BGP),
		validateEnrichGlobalMetricTags(p.MetricTags),
		validateEnrichVirtualMetrics(p.Metrics, p.Topology, p.VirtualMetrics),
	}

	return errors.Join(errs...)
}
