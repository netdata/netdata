// SPDX-License-Identifier: GPL-3.0-or-later

package promsemantics

import (
	"fmt"
	"slices"
	"strings"
)

func entityIdentityLabels(identity EntityIdentity) []string {
	count := len(identity.Required) + len(identity.Optional)
	for _, alternative := range identity.Alternatives {
		count += len(alternative)
	}
	out := make([]string, 0, count)
	out = append(out, identity.Required...)
	for _, alternative := range identity.Alternatives {
		out = append(out, alternative...)
	}
	return append(out, identity.Optional...)
}

func selectedLabelAlternative(alternatives [][]string, labels map[string]string) (int, error) {
	selected := -1
	for index, alternative := range alternatives {
		active := false
		complete := true
		for _, label := range alternative {
			present := strings.TrimSpace(labels[label]) != ""
			active = active || present
			complete = complete && present
		}
		if !active {
			continue
		}
		if !complete || selected != -1 {
			return -1, fmt.Errorf("exactly one complete label alternative must be present and all others absent or blank")
		}
		selected = index
	}
	if selected == -1 {
		return -1, fmt.Errorf("exactly one complete label alternative must be present and all others absent or blank")
	}
	return selected, nil
}

func canonicalLabelAlternatives(alternatives [][]string) []string {
	out := make([]string, 0, len(alternatives))
	for _, alternative := range alternatives {
		labels := slices.Clone(alternative)
		slices.Sort(labels)
		out = append(out, strings.Join(labels, "\x00"))
	}
	slices.Sort(out)
	return out
}

func cloneLabelPresenceConstraints(
	constraints map[string]LabelPresenceConstraint,
) map[string]LabelPresenceConstraint {
	if constraints == nil {
		return nil
	}
	out := make(map[string]LabelPresenceConstraint, len(constraints))
	for id, constraint := range constraints {
		constraint.Evidence = slices.Clone(constraint.Evidence)
		alternatives := constraint.Alternatives
		constraint.Alternatives = make([][]string, len(alternatives))
		for index, alternative := range alternatives {
			constraint.Alternatives[index] = slices.Clone(alternative)
		}
		out[id] = constraint
	}
	return out
}
