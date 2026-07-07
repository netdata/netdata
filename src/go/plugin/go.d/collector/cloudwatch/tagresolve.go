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
// (e.g. a leading digit); resolveTagPlan validates it and skips on failure.
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

// resolveTagPlan turns the job's tag allowlist into the ordered set of
// (AWS key -> label) that survive for a profile whose identity dimension labels are
// dimLabels. It is NON-FATAL: a bad or colliding entry is skipped and described in
// warnings, never an error (tags are optional enrichment).
//
// An entry is skipped when its target label:
//   - is empty or fails the label-key regex (after sanitize, when not renamed);
//   - equals a reserved identity label (account_id, region) or one of the profile's
//     dimension labels — which would duplicate an emitted label key and panic metrix;
//   - duplicates an already-surviving tag's label; or
//   - repeats an AWS key already seen in this allowlist.
//
// The surviving plan is the same for every instance of the profile; only the tag
// VALUES differ per resource, applied at cache-build time.
func resolveTagPlan(tags []TagConfig, dimLabels []string) (plan []resolvedTag, warnings []string) {
	if len(tags) == 0 {
		return nil, nil
	}

	reserved := map[string]string{"account_id": "reserved", "region": "reserved"}
	for _, d := range dimLabels {
		reserved[d] = "dimension"
	}

	seenAWS := make(map[string]struct{}, len(tags))
	seenLabel := make(map[string]struct{}, len(tags))

	for _, t := range tags {
		awsKey := t.Name
		if strings.TrimSpace(awsKey) == "" {
			warnings = append(warnings, "skipped a tag entry with an empty name")
			continue
		}
		if _, dup := seenAWS[awsKey]; dup {
			warnings = append(warnings, fmt.Sprintf("skipped duplicate tag %q", awsKey))
			continue
		}
		seenAWS[awsKey] = struct{}{}

		label := t.Rename
		if label == "" {
			label = sanitizeLabel(awsKey)
		}
		if !labelKeyRe.MatchString(label) {
			warnings = append(warnings, fmt.Sprintf("skipped tag %q: label %q is not a valid label key (rename it)", awsKey, label))
			continue
		}
		if kind, ok := reserved[label]; ok {
			warnings = append(warnings, fmt.Sprintf("skipped tag %q: label %q collides with a %s label (rename it)", awsKey, label, kind))
			continue
		}
		if _, dup := seenLabel[label]; dup {
			warnings = append(warnings, fmt.Sprintf("skipped tag %q: label %q duplicates another tag's label (rename it)", awsKey, label))
			continue
		}
		seenLabel[label] = struct{}{}

		plan = append(plan, resolvedTag{awsKey: awsKey, label: label})
	}
	return plan, warnings
}
