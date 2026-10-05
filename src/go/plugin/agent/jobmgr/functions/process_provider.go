// SPDX-License-Identifier: GPL-3.0-or-later

package functions

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
)

// functionOwner separates collector and process identities even when their
// supplied names are identical. Neither owner can replace the other's groups.
type functionOwner struct {
	name    string
	process bool
}

type controllerGroupKey struct {
	owner functionOwner
	scope string
}

// ValidateProcessProviders checks fixed identity and callback requirements
// without calling provider code. Factories run only inside generation containment.
func ValidateProcessProviders(providers []funcapi.ProcessFunctionProvider) error {
	seen := make(map[string]struct{}, len(providers))
	for _, provider := range providers {
		if provider.ID == "" || provider.Functions == nil || provider.NewHandler == nil {
			return errors.New("jobmgr Function controller: incomplete process Function provider")
		}
		if _, ok := seen[provider.ID]; ok {
			return fmt.Errorf("jobmgr Function controller: duplicate process Function provider %q", provider.ID)
		}
		seen[provider.ID] = struct{}{}
	}
	return nil
}

func buildProcessProviderPlan(provider funcapi.ProcessFunctionProvider) (controllerModulePlan, error) {
	methods, err := callModuleFunctions("ProcessFunctionProvider.Functions", provider.Functions)
	if err != nil {
		return controllerModulePlan{}, err
	}
	methods, err = validateConfiguredMethods(provider.ID, methods)
	if err != nil {
		return controllerModulePlan{}, err
	}
	bundle, err := newFunctionBundle(functionBundleAgent, provider.ID, provider.NewHandler, nil, methods)
	if err != nil {
		return controllerModulePlan{}, err
	}
	return controllerModulePlan{
		agent:       methods,
		agentBundle: bundle,
	}, nil
}

// ReconcileProcessProviders uses the process tick, independently of job scheduling.
func (c *Controller) ReconcileProcessProviders(ctx context.Context) error {
	if c == nil || ctx == nil {
		return errors.New("jobmgr Function controller: invalid process reconciliation")
	}
	var result error
	// The owner set is immutable after construction.
	for _, owner := range c.processOwners {
		result = errors.Join(result, c.reconcileOwner(ctx, owner))
	}
	return result
}

// Reserve every declared provider name, including initially unavailable methods.
// Later instance Functions also check these reservations before publication.
func (c *Controller) validateProcessNames(initial []InitialRoute) error {
	c.processNames = make(map[string]functionOwner)
	for owner, plan := range c.plans {
		if !owner.process {
			continue
		}
		for _, method := range plan.agent {
			for _, name := range funcapi.FunctionNames(owner.name, method) {
				if !validFunctionName(name) {
					return errors.New("jobmgr Function controller: invalid process Function public name")
				}
				if _, exists := c.processNames[name]; exists {
					return fmt.Errorf("jobmgr Function controller: duplicate process Function public name %q", name)
				}
				c.processNames[name] = owner
			}
		}
	}
	for _, route := range initial {
		if _, exists := c.processNames[route.Declaration.PublicName]; exists {
			return errors.New("jobmgr Function controller: process Function collides with initial route")
		}
	}
	for owner, plan := range c.plans {
		if owner.process {
			continue
		}
		for _, methods := range [][]funcapi.FunctionConfig{plan.agent, plan.shared} {
			for _, method := range methods {
				for _, name := range funcapi.FunctionNames(owner.name, method) {
					if _, exists := c.processNames[name]; exists {
						return errors.New(
							"jobmgr Function controller: process Function collides with collector Function",
						)
					}
				}
			}
		}
	}
	return nil
}

func (c *Controller) ownerJobs(owner functionOwner) map[string]controllerJob {
	if owner.process {
		return nil
	}
	return c.jobs[owner.name]
}

func sortedProcessOwners(plans map[functionOwner]controllerModulePlan) []functionOwner {
	var owners []functionOwner
	for owner := range plans {
		if owner.process {
			owners = append(owners, owner)
		}
	}
	slices.SortFunc(owners, func(a, b functionOwner) int { return strings.Compare(a.name, b.name) })
	return owners
}
