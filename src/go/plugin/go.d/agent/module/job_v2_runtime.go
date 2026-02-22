// SPDX-License-Identifier: GPL-3.0-or-later

package module

import (
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/runtimecomp"
)

func (j *JobV2) registerRuntimeComponent() error {
	if j == nil || j.runtimeService == nil || j.runtimeComponentRegistered {
		return nil
	}
	if j.engine == nil {
		return fmt.Errorf("nil engine")
	}
	store := j.engine.RuntimeStore()
	if store == nil {
		return fmt.Errorf("nil runtime store")
	}

	componentName := j.runtimeComponentName
	if componentName == "" {
		componentName = j.buildRuntimeComponentName()
	}

	updateEvery := j.updateEvery
	if updateEvery <= 0 {
		updateEvery = 1
	}

	cfg := runtimecomp.ComponentConfig{
		Name:        componentName,
		Store:       store,
		UpdateEvery: updateEvery,
		Autogen: runtimecomp.AutogenPolicy{
			Enabled: true,
		},
		Plugin:    j.pluginName,
		Module:    "chartengine",
		JobName:   firstNonEmpty(strings.TrimSpace(j.name), strings.TrimSpace(j.fullName)),
		JobLabels: j.runtimeComponentLabels(),
	}
	if err := j.runtimeService.RegisterComponent(cfg); err != nil {
		return err
	}
	j.runtimeComponentName = componentName
	j.runtimeComponentRegistered = true
	return nil
}

func (j *JobV2) unregisterRuntimeComponent() {
	if j == nil || j.runtimeService == nil || !j.runtimeComponentRegistered {
		return
	}
	j.runtimeService.UnregisterComponent(j.runtimeComponentName)
	j.runtimeComponentRegistered = false
}

func (j *JobV2) runtimeComponentLabels() map[string]string {
	labels := map[string]string{
		"_collect_module": j.moduleName,
	}
	if v, ok := j.labels["instance"]; ok && strings.TrimSpace(v) != "" {
		labels["collector_instance"] = v
	}
	return labels
}

func (j *JobV2) buildRuntimeComponentName() string {
	plugin := sanitizeRuntimeComponentPart(firstNonEmpty(j.pluginName, "go.d"))
	fullName := sanitizeRuntimeComponentPart(firstNonEmpty(j.fullName, j.name, "job"))
	return fmt.Sprintf("chartengine.%s.%s", plugin, fullName)
}

func sanitizeRuntimeComponentPart(name string) string {
	replacer := strings.NewReplacer("/", "_", "\\", "_", " ", "_", ":", "_", "*", "_", "?", "_", "\"", "_", "<", "_", ">", "_", "|", "_")
	return strings.TrimSpace(replacer.Replace(name))
}

func firstNonEmpty(items ...string) string {
	for _, item := range items {
		if item = strings.TrimSpace(item); item != "" {
			return item
		}
	}
	return ""
}
