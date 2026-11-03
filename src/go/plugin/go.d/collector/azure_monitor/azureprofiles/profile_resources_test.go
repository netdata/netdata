// SPDX-License-Identifier: GPL-3.0-or-later

package azureprofiles

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
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
        - selector: power_state
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
	assert.Equal(t, "test_vm.power_state", profile.Template.Charts[1].Dimensions[0].Selector)
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
		"state id colliding with a metric series": {
			mutate: func(p *Profile) { p.ResourceStates[0].ID = "percentage_cpu_average" },
			want:   "collides with a metric series",
		},
		"state without options": {
			mutate: func(p *Profile) { p.ResourceStates[0].States = nil },
			want:   "'states' must contain at least one state",
		},
		"state option without a value": {
			mutate: func(p *Profile) { p.ResourceStates[0].States[0].Value = " " },
			want:   "'value' is required",
		},
		"property segment with an underscore, which would share a discovery column": {
			mutate: func(p *Profile) { p.ResourceLabels[0].Property = "properties.hardware_profile" },
			want:   "'property' must match",
		},
		"two states with the same value": {
			mutate: func(p *Profile) { p.ResourceStates[0].States[1].Value = "powerstate/RUNNING" },
			want:   `duplicate state value "powerstate/RUNNING"`,
		},
		"duplicate state id": {
			mutate: func(p *Profile) { p.ResourceStates = append(p.ResourceStates, p.ResourceStates[0]) },
			want:   `duplicate state id "power_state"`,
		},
		"chart referencing an undeclared state": {
			mutate: func(p *Profile) { p.Template.Charts[1].Dimensions[0].Selector = "undefined_state" },
			want:   "undefined_state",
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

func TestCatalog_ResourceProperties(t *testing.T) {
	dir := t.TempDir()
	appProfile := strings.NewReplacer(
		"Microsoft.Compute/virtualMachines", "Microsoft.Web/sites",
		"context_namespace: test_vm", "context_namespace: test_app",
		"properties.hardwareProfile.vmSize", "properties.siteConfig.linuxFxVersion",
	).Replace(resourceProfileYAML)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "test_vm.yaml"), []byte(resourceProfileYAML), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "test_app.yaml"), []byte(appProfile), 0o644))

	catalog, err := LoadFromDirs([]DirSpec{{Path: dir, IsStock: true}})
	require.NoError(t, err)

	assert.Equal(t, []string{
		"properties.extended.instanceView.powerState.code",
		"properties.hardwareProfile.vmSize",
		"properties.serverFarmId",
		"properties.siteConfig.linuxFxVersion",
	}, catalog.ResourceProperties())
	assert.Equal(t, []string{
		"properties.extended.instanceView.powerState.code",
		"properties.serverFarmId",
		"properties.siteConfig.linuxFxVersion",
	}, catalog.ResourceProperties("microsoft.web/sites"))
	assert.Empty(t, catalog.ResourceProperties("Microsoft.Sql/servers/databases"))
}

func TestProfile_RejectsUncompilableTemplate(t *testing.T) {
	profile := loadResourceProfile(t, func(p *Profile) { p.Template.Charts[0].Dimensions[0].Name = "" })
	require.ErrorContains(t, profile.Validate("test_vm", "test_vm"), "name inference requires inferable selector")
}

func TestProfile_StateDimensionNaming(t *testing.T) {
	for _, selector := range []string{"phase", "test_vm.phase", `phase{region="eastus"}`} {
		t.Run(selector, func(t *testing.T) {
			profile := loadResourceProfile(t, func(p *Profile) {
				p.ResourceStates[0].ID = "phase"
				p.Template.Charts[1].Dimensions[0].Selector = selector
				// Nested chart groups follow the same normalization contract.
				p.Template.Groups = []charttpl.Group{{Family: "State", Charts: p.Template.Charts[1:]}}
				p.Template.Charts = p.Template.Charts[:1]
			})
			require.NoError(t, profile.Validate("test_vm", "test_vm"))
			assert.Equal(t, "test_vm.phase", profile.Template.Groups[0].Charts[0].Dimensions[0].NameFromLabel)
		})
	}
	for _, field := range []string{"name", "name_from_label"} {
		t.Run("explicit "+field, func(t *testing.T) {
			profile := loadResourceProfile(t, func(p *Profile) {
				dim := &p.Template.Charts[1].Dimensions[0]
				if field == "name" {
					dim.Name = "custom"
				} else {
					dim.NameFromLabel = "resource_name"
				}
			})
			require.NoError(t, profile.Validate("test_vm", "test_vm"))
			dim := profile.Template.Charts[1].Dimensions[0]
			if field == "name" {
				assert.Equal(t, "custom", dim.Name)
				assert.Empty(t, dim.NameFromLabel)
			} else {
				assert.Equal(t, "resource_name", dim.NameFromLabel)
			}
		})
	}
}

func TestCatalog_UncompilableOverrideFallsBackToStock(t *testing.T) {
	stock, custom := t.TempDir(), t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(stock, "test_vm.yaml"), []byte(resourceProfileYAML), 0o644))
	invalid := strings.ReplaceAll(resourceProfileYAML, "          name: average\n", "")
	require.NoError(t, os.WriteFile(filepath.Join(custom, "test_vm.yaml"), []byte(invalid), 0o644))
	catalog, err := LoadFromDirs([]DirSpec{{Path: custom}, {Path: stock, IsStock: true}})
	require.NoError(t, err)
	profile, ok := catalog.Get("test_vm")
	require.True(t, ok)
	assert.Equal(t, "average", profile.Template.Charts[0].Dimensions[0].Name)
}
