// SPDX-License-Identifier: GPL-3.0-or-later

package cloudwatch

import (
	"fmt"
	"regexp"
	"strings"
)

// labelKeyRe is the Netdata label-key contract (matches cwprofiles' identity-id
// rule): lowercase start, then lowercase alphanumerics/underscore.
var labelKeyRe = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// resolvedTag maps one surviving AWS tag key to the Netdata label it emits.
type resolvedTag struct {
	awsKey string // exact AWS tag key (case-sensitive)
	label  string // Netdata label key (valid, collision-free)
}

// sanitizeLabel derives a default label from an AWS tag key: lowercase, with every
// character outside [a-z0-9_] replaced by '_'. The result may still be invalid
// (e.g. a leading digit); configuration validation rejects such a derived label.
func sanitizeLabel(key string) string {
	var b strings.Builder
	b.Grow(len(key))
	for _, r := range strings.ToLower(key) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	return b.String()
}

// resolveTagPlan turns the job's validated tag-label configuration into the ordered set of
// (AWS key -> label) that survive for a profile whose identity dimension labels are
// dimLabels. Profile-specific collisions are non-fatal: the entry is skipped and
// described in warnings because tag labels are optional enrichment.
//
// Job-level validation has already rejected duplicate AWS keys and emitted labels.
// At this profile-specific stage, an entry is skipped only when its target label
// equals a reserved identity label (account_id, region) or one of the profile's
// dimension labels, which would duplicate an emitted label key and panic metrix.
//
// The surviving plan is the same for every instance of the profile; only the tag
// VALUES differ per resource, applied at cache-build time.
func resolveTagPlan(tags []ResourceTagLabelConfig, dimLabels []string) (plan []resolvedTag, warnings []string) {
	if len(tags) == 0 {
		return nil, nil
	}

	reserved := map[string]string{"account_id": "reserved", "region": "reserved"}
	for _, d := range dimLabels {
		reserved[d] = "dimension"
	}

	for _, t := range tags {
		awsKey := t.Key
		label := t.Label
		if label == "" {
			label = sanitizeLabel(awsKey)
		}
		if kind, ok := reserved[label]; ok {
			warnings = append(warnings, fmt.Sprintf("skipped tag %q: label %q collides with a %s label (rename it)", awsKey, label, kind))
			continue
		}
		plan = append(plan, resolvedTag{awsKey: awsKey, label: label})
	}
	return plan, warnings
}
