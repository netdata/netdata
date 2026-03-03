// SPDX-License-Identifier: GPL-3.0-or-later

package functions

import (
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/framework/runtimecomp"
)

const functionsRuntimeComponentName = "functions.manager"

func (m *Manager) SetRuntimeService(service runtimecomp.Service) {
	if m == nil {
		return
	}
	m.runtimeService = service
}

func (m *Manager) registerRuntimeComponent() error {
	if m == nil || m.runtimeService == nil || m.runtimeComponentRegistered {
		return nil
	}
	if m.runtimeStore == nil {
		return fmt.Errorf("nil runtime store")
	}

	componentName := strings.TrimSpace(m.runtimeComponentName)
	if componentName == "" {
		componentName = functionsRuntimeComponentName
	}

	cfg := runtimecomp.ComponentConfig{
		Name:        componentName,
		Store:       m.runtimeStore,
		UpdateEvery: 1,
		Autogen: runtimecomp.AutogenPolicy{
			Enabled: true,
		},
		Module:  "functions",
		JobName: "manager",
		JobLabels: map[string]string{
			"component": "functions_manager",
		},
	}
	if err := m.runtimeService.RegisterComponent(cfg); err != nil {
		return err
	}

	m.runtimeComponentName = componentName
	m.runtimeComponentRegistered = true
	return nil
}

func (m *Manager) unregisterRuntimeComponent() {
	if m == nil || m.runtimeService == nil || !m.runtimeComponentRegistered {
		return
	}
	m.runtimeService.UnregisterComponent(m.runtimeComponentName)
	m.runtimeComponentRegistered = false
}
