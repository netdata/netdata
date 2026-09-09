// SPDX-License-Identifier: GPL-3.0-or-later

package promvalidation

import (
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/prometheus/promprofiles"
)

// profileValidationContext keeps one supplied profile's production policy and
// public report slot together throughout composed checks.
type profileValidationContext struct {
	profile  promprofiles.Profile
	report   *profileReport
	composed bool
}

func newProfileValidationContexts(staged stagedValidationInputs, r *Report) []profileValidationContext {
	contexts := make([]profileValidationContext, 0, len(staged.profiles))
	for index, profile := range staged.profiles {
		report := &r.Profiles.Candidate
		if index > 0 {
			report = &r.Profiles.Supports[index-1]
		}
		contexts = append(contexts, profileValidationContext{
			profile:  profile,
			report:   report,
			composed: len(staged.profiles) > 1,
		})
	}
	return contexts
}

func (c profileValidationContext) path(path string) string {
	if !c.composed {
		return path
	}
	return fmt.Sprintf("profiles[%s].%s", c.profile.Name, path)
}
