// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ddprofiledefinition

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
)

func validateEnrichSelector(p *ProfileDefinition) error {
	var errs []error

	// If the new selector is absent but legacy sysobjectid exists, migrate it.
	if len(p.Selector) == 0 && len(p.SysObjectIDs) > 0 {
		p.Selector = SelectorSpec{
			{
				SysObjectID: SelectorIncludeExclude{
					Include: slices.Clone(p.SysObjectIDs), // legacy -> include
				},
			},
		}
	}

	// Validate every rule
	for i := range p.Selector {
		r := &p.Selector[i]

		// Validate regex syntax for sysObjectID includes/excludes
		for j, pat := range r.SysObjectID.Include {
			if _, err := regexp.Compile(pat); err != nil {
				errs = append(
					errs,
					fmt.Errorf("selector[%d].sysObjectID.include[%d]: invalid regex %q: %v", i, j, pat, err),
				)
			}
		}
		for j, pat := range r.SysObjectID.Exclude {
			if _, err := regexp.Compile(pat); err != nil {
				errs = append(
					errs,
					fmt.Errorf("selector[%d].sysObjectID.exclude[%d]: invalid regex %q: %v", i, j, pat, err),
				)
			}
		}
	}

	return errors.Join(errs...)
}
