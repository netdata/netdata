// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/runtimemgr"
)

// RuntimeComponentConfig registers one internal/runtime metrics component.
// It aliases runtimemgr.ComponentConfig for backward compatibility.
type RuntimeComponentConfig = runtimemgr.ComponentConfig

func (m *Manager) RegisterRuntimeComponent(cfg RuntimeComponentConfig) error {
	if m == nil {
		return fmt.Errorf("jobmgr: nil manager")
	}
	if m.runtimeService == nil {
		return fmt.Errorf("jobmgr: runtime service is not initialized")
	}
	return m.runtimeService.RegisterComponent(cfg)
}

func (m *Manager) UnregisterRuntimeComponent(name string) {
	if m == nil || m.runtimeService == nil {
		return
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return
	}
	m.runtimeService.UnregisterComponent(name)
}
