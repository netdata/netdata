// SPDX-License-Identifier: GPL-3.0-or-later

package azureprofiles

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

const resourceProfileYAML = `
display_name: Test VM
resource_type: Microsoft.Compute/virtualMachines
metrics:
  - id: percentage_cpu
    azure_name: Percentage CPU
    series:
      - aggregation: average
        kind: gauge
resource_labels:
  - name: vm_size
    property: properties.hardwareProfile.vmSize
  - name: plan
    property: properties.serverFarmId
    value: arm_name
resource_states:
  - id: power_state
    property: properties.extended.instanceView.powerState.code
    states:
      - name: running
        value: PowerState/running
      - name: deallocated
        value: PowerState/deallocated
template:
  family: Test
  context_namespace: test_vm
  charts:
    - id: test_vm_cpu
      title: CPU
      context: cpu
      units: percentage
      dimensions:
        - selector: percentage_cpu_average
          name: average
    - id: test_vm_power_state
      title: Power State
      context: power_state
      units: state
      dimensions:
        - selector: power_state_running
          name: running
        - selector: power_state_deallocated
          name: deallocated
`

func loadResourceProfile(t *testing.T, mutate func(*Profile)) Profile {
	t.Helper()
	var profile Profile
	require.NoError(t, yaml.Unmarshal([]byte(resourceProfileYAML), &profile))
	if mutate != nil {
		mutate(&profile)
	}
	require.NoError(t, profile.Normalize("test_vm"))
	return profile
}

func TestProfile_ResourceLabelsAndStatesAreValid(t *testing.T) {
	profile := loadResourceProfile(t, nil)

	require.NoError(t, profile.Validate("test_vm", "test_vm"))
	assert.Equal(t, []string{
		"properties.hardwareProfile.vmSize",
		"properties.serverFarmId",
		"properties.extended.instanceView.powerState.code",
	}, profile.ResourceProperties())
	assert.Equal(t, "test_vm.power_state_running", profile.Template.Charts[1].Dimensions[0].Selector)
}

func TestProfile_ResourceLabelsAndStatesValidation(t *testing.T) {
	tests := map[string]struct {
		mutate func(*Profile)
		want   string
	}{
		"label name reserved by the collector": {
			mutate: func(p *Profile) { p.ResourceLabels[0].Name = "region" },
			want:   `'name' "region" is reserved`,
		},
		"duplicate label name": {
			mutate: func(p *Profile) { p.ResourceLabels[1].Name = "vm_size" },
			want:   `duplicate label name "vm_size"`,
		},
		"property outside the resource properties": {
			mutate: func(p *Profile) { p.ResourceLabels[0].Property = "tags.owner" },
			want:   "'property' must match",
		},
		"property with a query injection": {
			mutate: func(p *Profile) { p.ResourceLabels[0].Property = "properties.a) | take 1 | extend x=(1" },
			want:   "'property' must match",
		},
		"unknown label value format": {
			mutate: func(p *Profile) { p.ResourceLabels[1].Value = "upper" },
			want:   "'value' must be one of",
		},
		"state id colliding with a metric id": {
			mutate: func(p *Profile) { p.ResourceStates[0].ID = "percentage_cpu" },
			want:   "collides with a metric id",
		},
		"state without options": {
			mutate: func(p *Profile) { p.ResourceStates[0].States = nil },
			want:   "'states' must contain at least one state",
		},
		"state option without a value": {
			mutate: func(p *Profile) { p.ResourceStates[0].States[0].Value = " " },
			want:   "'value' is required",
		},
		"chart referencing an undeclared state": {
			mutate: func(p *Profile) { p.ResourceStates[0].States = p.ResourceStates[0].States[:1] },
			want:   "power_state_deallocated",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			profile := loadResourceProfile(t, tc.mutate)
			err := profile.Validate("test_vm", "test_vm")
			require.Error(t, err)
			assert.ErrorContains(t, err, tc.want)
		})
	}
}

func TestNormalizeResourceLabelValue(t *testing.T) {
	assert.Equal(t, ResourceLabelValueRaw, NormalizeResourceLabelValue(""))
	assert.Equal(t, ResourceLabelValueRaw, NormalizeResourceLabelValue("RAW"))
	assert.Equal(t, ResourceLabelValueARMName, NormalizeResourceLabelValue(" arm_name "))
	assert.Empty(t, NormalizeResourceLabelValue("upper"))
}

func TestResourcePropertyColumn(t *testing.T) {
	assert.Equal(t, "nd_properties_hardwareProfile_vmSize", ResourcePropertyColumn("properties.hardwareProfile.vmSize"))
}
