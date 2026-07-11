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
		"123abc":                    "123abc", // stays invalid; config validation requires an explicit valid label
	}
	for in, want := range tests {
		t.Run(in, func(t *testing.T) {
			assert.Equal(t, want, sanitizeLabel(in))
		})
	}
}

func TestResolveTagPlan(t *testing.T) {
	tests := map[string]struct {
		tags      []ResourceTagLabelConfig
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
			tags:     []ResourceTagLabelConfig{{Key: "owner"}, {Key: "project"}},
			wantPlan: []resolvedTag{{"owner", "owner"}, {"project", "project"}},
		},
		"Name sanitizes to name": {
			tags:     []ResourceTagLabelConfig{{Key: "Name"}},
			wantPlan: []resolvedTag{{"Name", "name"}},
		},
		"rename overrides default": {
			tags:     []ResourceTagLabelConfig{{Key: "Name", Label: "instance_name"}},
			wantPlan: []resolvedTag{{"Name", "instance_name"}},
		},
		"region tag collides with reserved -> skipped": {
			tags:     []ResourceTagLabelConfig{{Key: "region"}},
			wantPlan: nil,
			wantWarn: 1,
		},
		"region tag renamed -> kept": {
			tags:     []ResourceTagLabelConfig{{Key: "region", Label: "aws_region"}},
			wantPlan: []resolvedTag{{"region", "aws_region"}},
		},
		"collision with a profile dimension label -> skipped": {
			tags:      []ResourceTagLabelConfig{{Key: "instance_id"}},
			dimLabels: []string{"instance_id"},
			wantPlan:  nil,
			wantWarn:  1,
		},
		"survivors kept when mixed with skips": {
			tags:      []ResourceTagLabelConfig{{Key: "owner"}, {Key: "region"}, {Key: "project"}},
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
