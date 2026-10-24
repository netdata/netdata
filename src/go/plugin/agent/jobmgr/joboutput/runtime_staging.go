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

func (svnl *stagedVNodeLookup) Lookup(name string) (jobruntime.VnodeSnapshot, bool) {
	if svnl == nil || name == "" {
		return jobruntime.VnodeSnapshot{}, false
	}
	svnl.mu.Lock()
	if svnl.closed {
		svnl.mu.Unlock()
		return jobruntime.VnodeSnapshot{}, false
	}
	live := svnl.live
	if live == nil {
		initial := svnl.initial
		matches := name == svnl.name
		svnl.mu.Unlock()
		return initial, matches
	}
	svnl.mu.Unlock()
	return live(name)
}

func (svnl *stagedVNodeLookup) attach(
	live func(string) (jobruntime.VnodeSnapshot, bool),
) error {
	if svnl == nil {
		return nil
	}
	if live == nil {
		return errors.New("job output: staged vnode lookup has no live attachment")
	}
	svnl.mu.Lock()
	defer svnl.mu.Unlock()
	if svnl.closed || svnl.live != nil {
		return errors.New("job output: invalid staged vnode attachment")
	}
	svnl.live = live
	svnl.initial = jobruntime.VnodeSnapshot{}
	return nil
}

func (svnl *stagedVNodeLookup) close() {
	if svnl == nil {
		return
	}
	svnl.mu.Lock()
	svnl.closed = true
	svnl.live = nil
	svnl.initial = jobruntime.VnodeSnapshot{}
	svnl.mu.Unlock()
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

func (srs *stagedRuntimeService) RegisterComponent(config runtimecomp.ComponentConfig) error {
	if srs == nil || config.Name == "" {
		return errors.New("job output: invalid staged runtime component")
	}
	srs.mu.Lock()
	defer srs.mu.Unlock()
	if srs.closed {
		return errors.New("job output: staged runtime service is closed")
	}
	if srs.live != nil {
		if err := srs.live.RegisterComponent(config); err != nil {
			return err
		}
		srs.components[config.Name] = config
		return nil
	}
	if _, exists := srs.components[config.Name]; exists {
		return errors.New("job output: duplicate staged runtime component")
	}
	srs.components[config.Name] = config
	return nil
}

func (srs *stagedRuntimeService) UnregisterComponent(name string) {
	if srs == nil || name == "" {
		return
	}
	srs.mu.Lock()
	defer srs.mu.Unlock()
	if srs.live != nil {
		srs.live.UnregisterComponent(name)
	}
	delete(srs.components, name)
}

func (srs *stagedRuntimeService) QuarantineComponent(name string) {
	if srs == nil || name == "" {
		return
	}
	srs.mu.Lock()
	defer srs.mu.Unlock()
	if srs.live != nil {
		srs.live.QuarantineComponent(name)
	}
	delete(srs.components, name)
}

func (srs *stagedRuntimeService) FinalizeComponent(name string) {
	if srs == nil || name == "" {
		return
	}
	srs.mu.Lock()
	defer srs.mu.Unlock()
	if srs.live != nil {
		srs.live.FinalizeComponent(name)
	}
	delete(srs.components, name)
}

func (srs *stagedRuntimeService) RegisterProducer(name string, producer func() error) error {
	if srs == nil || name == "" || producer == nil {
		return errors.New("job output: invalid staged runtime producer")
	}
	srs.mu.Lock()
	defer srs.mu.Unlock()
	if srs.closed {
		return errors.New("job output: staged runtime service is closed")
	}
	if srs.live != nil {
		if err := srs.live.RegisterProducer(name, producer); err != nil {
			return err
		}
		srs.producers[name] = producer
		return nil
	}
	if _, exists := srs.producers[name]; exists {
		return errors.New("job output: duplicate staged runtime producer")
	}
	srs.producers[name] = producer
	return nil
}

func (srs *stagedRuntimeService) UnregisterProducer(name string) {
	if srs == nil || name == "" {
		return
	}
	srs.mu.Lock()
	defer srs.mu.Unlock()
	if srs.live != nil {
		srs.live.UnregisterProducer(name)
	}
	delete(srs.producers, name)
}

func (srs *stagedRuntimeService) attach(live runtimecomp.Service) (resultErr error) {
	if srs == nil {
		return nil
	}
	if live == nil {
		return errors.New("job output: staged runtime service has no live attachment")
	}
	srs.mu.Lock()
	defer srs.mu.Unlock()
	if srs.closed || srs.attached || srs.live != nil {
		return errors.New("job output: invalid staged runtime attachment")
	}
	componentNames := make([]string, 0, len(srs.components))
	for name := range srs.components {
		componentNames = append(componentNames, name)
	}
	producerNames := make([]string, 0, len(srs.producers))
	for name := range srs.producers {
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
		if err := live.RegisterComponent(srs.components[name]); err != nil {
			return err
		}
		registeredComponents = append(registeredComponents, name)
	}
	for _, name := range producerNames {
		if err := live.RegisterProducer(name, srs.producers[name]); err != nil {
			return err
		}
		registeredProducers = append(registeredProducers, name)
	}
	srs.live = live
	srs.attached = true
	return nil
}

func (srs *stagedRuntimeService) close() {
	if srs == nil {
		return
	}
	srs.mu.Lock()
	defer srs.mu.Unlock()
	if srs.closed {
		return
	}
	srs.closed = true
	if srs.live != nil {
		producerNames := make([]string, 0, len(srs.producers))
		for name := range srs.producers {
			producerNames = append(producerNames, name)
		}
		componentNames := make([]string, 0, len(srs.components))
		for name := range srs.components {
			componentNames = append(componentNames, name)
		}
		slices.Sort(producerNames)
		slices.Sort(componentNames)
		for _, name := range producerNames {
			srs.live.UnregisterProducer(name)
		}
		for _, name := range componentNames {
			srs.live.UnregisterComponent(name)
		}
	}
	clear(srs.producers)
	clear(srs.components)
	srs.live = nil
}
