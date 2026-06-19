// SPDX-License-Identifier: GPL-3.0-or-later

package policy

// RunModePolicy defines runtime-mode behavior gates injected from composition root.
type RunModePolicy struct {
	IsTerminal               bool
	AutoEnableDiscovered     bool
	UseFileStatusPersistence bool
	EnableServiceDiscovery   bool
	EnableRuntimeCharts      bool
}

// Agent returns the shared defaults for long-lived agent processes.
func Agent(isTerminal bool) RunModePolicy {
	return RunModePolicy{
		IsTerminal:               isTerminal,
		AutoEnableDiscovered:     isTerminal,
		UseFileStatusPersistence: !isTerminal,
		EnableServiceDiscovery:   !isTerminal,
		EnableRuntimeCharts:      !isTerminal,
	}
}

// FunctionCLI returns the shared defaults for one-shot function execution.
func FunctionCLI() RunModePolicy {
	return RunModePolicy{
		IsTerminal:               false,
		AutoEnableDiscovered:     true,
		UseFileStatusPersistence: true,
		EnableServiceDiscovery:   false,
		EnableRuntimeCharts:      false,
	}
}
