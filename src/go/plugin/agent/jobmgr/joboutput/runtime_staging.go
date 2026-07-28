// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"errors"
	"slices"
	"sync"

	"github.com/netdata/netdata/go/plugins/plugin/framework/jobruntime"
	"github.com/netdata/netdata/go/plugins/plugin/framework/runtimecomp"
)

type stagedVNodeLookup struct {
	mu sync.Mutex

	name    string
	initial jobruntime.VnodeSnapshot
	live    func(string) (jobruntime.VnodeSnapshot, bool)
	closed  bool
}

func newStagedVNodeLookup(name string, initial jobruntime.VnodeSnapshot) *stagedVNodeLookup {
	return &stagedVNodeLookup{
		name:    name,
		initial: initial,
	}
}

func (lookup *stagedVNodeLookup) Lookup(name string) (jobruntime.VnodeSnapshot, bool) {
	if lookup == nil || name == "" {
		return jobruntime.VnodeSnapshot{}, false
	}
	lookup.mu.Lock()
	if lookup.closed {
		lookup.mu.Unlock()
		return jobruntime.VnodeSnapshot{}, false
	}
	live := lookup.live
	if live == nil {
		initial := lookup.initial
		matches := name == lookup.name
		lookup.mu.Unlock()
		return initial, matches
	}
	lookup.mu.Unlock()
	return live(name)
}

func (lookup *stagedVNodeLookup) attach(
	live func(string) (jobruntime.VnodeSnapshot, bool),
) error {
	if lookup == nil {
		return nil
	}
	if live == nil {
		return errors.New("job output: staged vnode lookup has no live attachment")
	}
	lookup.mu.Lock()
	defer lookup.mu.Unlock()
	if lookup.closed || lookup.live != nil {
		return errors.New("job output: invalid staged vnode attachment")
	}
	lookup.live = live
	lookup.initial = jobruntime.VnodeSnapshot{}
	return nil
}

func (lookup *stagedVNodeLookup) close() {
	if lookup == nil {
		return
	}
	lookup.mu.Lock()
	lookup.closed = true
	lookup.live = nil
	lookup.initial = jobruntime.VnodeSnapshot{}
	lookup.mu.Unlock()
}

// stagedRuntimeService gives V2 Init/Check a private runtime capability.
// Registrations become live only when attachment succeeds.
type stagedRuntimeService struct {
	mu sync.Mutex

	components map[string]runtimecomp.ComponentConfig
	producers  map[string]func() error
	live       runtimecomp.Service
	attached   bool
	closed     bool
}

func newStagedRuntimeService() *stagedRuntimeService {
	return &stagedRuntimeService{
		components: make(map[string]runtimecomp.ComponentConfig),
		producers:  make(map[string]func() error),
	}
}

func (service *stagedRuntimeService) RegisterComponent(config runtimecomp.ComponentConfig) error {
	if service == nil || config.Name == "" {
		return errors.New("job output: invalid staged runtime component")
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	if service.closed {
		return errors.New("job output: staged runtime service is closed")
	}
	if service.live != nil {
		if err := service.live.RegisterComponent(config); err != nil {
			return err
		}
		service.components[config.Name] = config
		return nil
	}
	if _, exists := service.components[config.Name]; exists {
		return errors.New("job output: duplicate staged runtime component")
	}
	service.components[config.Name] = config
	return nil
}

func (service *stagedRuntimeService) UnregisterComponent(name string) {
	if service == nil || name == "" {
		return
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	if service.live != nil {
		service.live.UnregisterComponent(name)
	}
	delete(service.components, name)
}

func (service *stagedRuntimeService) QuarantineComponent(name string) {
	if service == nil || name == "" {
		return
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	if service.live != nil {
		service.live.QuarantineComponent(name)
	}
	delete(service.components, name)
}

func (service *stagedRuntimeService) FinalizeComponent(name string) {
	if service == nil || name == "" {
		return
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	if service.live != nil {
		service.live.FinalizeComponent(name)
	}
	delete(service.components, name)
}

func (service *stagedRuntimeService) RegisterProducer(name string, producer func() error) error {
	if service == nil || name == "" || producer == nil {
		return errors.New("job output: invalid staged runtime producer")
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	if service.closed {
		return errors.New("job output: staged runtime service is closed")
	}
	if service.live != nil {
		if err := service.live.RegisterProducer(name, producer); err != nil {
			return err
		}
		service.producers[name] = producer
		return nil
	}
	if _, exists := service.producers[name]; exists {
		return errors.New("job output: duplicate staged runtime producer")
	}
	service.producers[name] = producer
	return nil
}

func (service *stagedRuntimeService) UnregisterProducer(name string) {
	if service == nil || name == "" {
		return
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	if service.live != nil {
		service.live.UnregisterProducer(name)
	}
	delete(service.producers, name)
}

func (service *stagedRuntimeService) attach(live runtimecomp.Service) (resultErr error) {
	if service == nil {
		return nil
	}
	if live == nil {
		return errors.New("job output: staged runtime service has no live attachment")
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	if service.closed || service.attached || service.live != nil {
		return errors.New("job output: invalid staged runtime attachment")
	}
	componentNames := make([]string, 0, len(service.components))
	for name := range service.components {
		componentNames = append(componentNames, name)
	}
	producerNames := make([]string, 0, len(service.producers))
	for name := range service.producers {
		producerNames = append(producerNames, name)
	}
	slices.Sort(componentNames)
	slices.Sort(producerNames)
	var registeredComponents []string
	var registeredProducers []string
	defer func() {
		if resultErr == nil {
			return
		}
		for index := len(registeredProducers) - 1; index >= 0; index-- {
			live.UnregisterProducer(registeredProducers[index])
		}
		for index := len(registeredComponents) - 1; index >= 0; index-- {
			live.UnregisterComponent(registeredComponents[index])
		}
	}()
	for _, name := range componentNames {
		if err := live.RegisterComponent(service.components[name]); err != nil {
			return err
		}
		registeredComponents = append(registeredComponents, name)
	}
	for _, name := range producerNames {
		if err := live.RegisterProducer(name, service.producers[name]); err != nil {
			return err
		}
		registeredProducers = append(registeredProducers, name)
	}
	service.live = live
	service.attached = true
	return nil
}

func (service *stagedRuntimeService) close() {
	if service == nil {
		return
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	if service.closed {
		return
	}
	service.closed = true
	if service.live != nil {
		producerNames := make([]string, 0, len(service.producers))
		for name := range service.producers {
			producerNames = append(producerNames, name)
		}
		componentNames := make([]string, 0, len(service.components))
		for name := range service.components {
			componentNames = append(componentNames, name)
		}
		slices.Sort(producerNames)
		slices.Sort(componentNames)
		for _, name := range producerNames {
			service.live.UnregisterProducer(name)
		}
		for _, name := range componentNames {
			service.live.UnregisterComponent(name)
		}
	}
	clear(service.producers)
	clear(service.components)
	service.live = nil
}
