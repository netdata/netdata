// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
)

func validateCollectorInstanceIdentity(module, name string, policy collectorapi.InstancePolicy) error {
	if policy != collectorapi.InstancePolicySingle {
		return nil
	}
	if name == module {
		return nil
	}
	return fmt.Errorf("single-instance collector %s must use config name %q, got %q", module, module, name)
}

func validateCollectorConfigIdentity(cfg confgroup.Config, creator collectorapi.Creator) error {
	return validateCollectorInstanceIdentity(cfg.Module(), cfg.Name(), creator.InstancePolicy)
}

func (m *Manager) collectorInstancePolicy(module string) collectorapi.InstancePolicy {
	creator, ok := m.modules.Lookup(module)
	if !ok {
		return collectorapi.InstancePolicyPerJob
	}
	return creator.InstancePolicy
}

func (m *Manager) isSingleInstanceCollector(module string) bool {
	return m.collectorInstancePolicy(module) == collectorapi.InstancePolicySingle
}

func (m *Manager) validateDyncfgCollectorIdentity(module, name string) error {
	return validateCollectorInstanceIdentity(module, name, m.collectorInstancePolicy(module))
}
