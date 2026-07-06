// SPDX-License-Identifier: GPL-3.0-or-later

package cloudwatch

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSanitizeLabel(t *testing.T) {
	tests := map[string]string{
		"owner":                     "owner",
		"Name":                      "name",
		"Environment":               "environment",
		"aws:autoscaling:groupName": "aws_autoscaling_groupname",
		"app.kubernetes.io/team":    "app_kubernetes_io_team",
		"123abc":                    "123abc", // stays invalid (leading digit); resolveTagPlan skips it
	}
	for in, want := range tests {
		t.Run(in, func(t *testing.T) {
			assert.Equal(t, want, sanitizeLabel(in))
		})
	}
}

func TestResolveTagPlan(t *testing.T) {
	tests := map[string]struct {
		tags      []TagConfig
		dimLabels []string
		wantPlan  []resolvedTag
		wantWarn  int
	}{
		"empty allowlist disables": {
			tags:     nil,
			wantPlan: nil,
			wantWarn: 0,
		},
		"plain keys pass through": {
			tags:     []TagConfig{{Name: "owner"}, {Name: "project"}},
			wantPlan: []resolvedTag{{"owner", "owner"}, {"project", "project"}},
		},
		"Name sanitizes to name": {
			tags:     []TagConfig{{Name: "Name"}},
			wantPlan: []resolvedTag{{"Name", "name"}},
		},
		"rename overrides default": {
			tags:     []TagConfig{{Name: "Name", Rename: "instance_name"}},
			wantPlan: []resolvedTag{{"Name", "instance_name"}},
		},
		"region tag collides with reserved -> skipped": {
			tags:     []TagConfig{{Name: "region"}},
			wantPlan: nil,
			wantWarn: 1,
		},
		"region tag renamed -> kept": {
			tags:     []TagConfig{{Name: "region", Rename: "aws_region"}},
			wantPlan: []resolvedTag{{"region", "aws_region"}},
		},
		"collision with a profile dimension label -> skipped": {
			tags:      []TagConfig{{Name: "instance_id"}},
			dimLabels: []string{"instance_id"},
			wantPlan:  nil,
			wantWarn:  1,
		},
		"duplicate AWS key -> one kept": {
			tags:     []TagConfig{{Name: "owner"}, {Name: "owner"}},
			wantPlan: []resolvedTag{{"owner", "owner"}},
			wantWarn: 1,
		},
		"two keys sanitizing to the same label -> second skipped": {
			tags:     []TagConfig{{Name: "Name"}, {Name: "name"}},
			wantPlan: []resolvedTag{{"Name", "name"}},
			wantWarn: 1,
		},
		"invalid rename -> skipped": {
			tags:     []TagConfig{{Name: "foo", Rename: "Bad-Name"}},
			wantPlan: nil,
			wantWarn: 1,
		},
		"key sanitizing to an invalid label -> skipped": {
			tags:     []TagConfig{{Name: "123abc"}},
			wantPlan: nil,
			wantWarn: 1,
		},
		"empty name -> skipped": {
			tags:     []TagConfig{{Name: "  "}},
			wantPlan: nil,
			wantWarn: 1,
		},
		"survivors kept when mixed with skips": {
			tags:      []TagConfig{{Name: "owner"}, {Name: "region"}, {Name: "project"}},
			dimLabels: []string{"instance_id"},
			wantPlan:  []resolvedTag{{"owner", "owner"}, {"project", "project"}},
			wantWarn:  1,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			plan, warnings := resolveTagPlan(tc.tags, tc.dimLabels)
			assert.Equal(t, tc.wantPlan, plan)
			assert.Len(t, warnings, tc.wantWarn)
		})
	}
}
