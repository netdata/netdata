// SPDX-License-Identifier: GPL-3.0-or-later

package schedulers

import (
	"fmt"
	"maps"
	"sync"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/pkg/runtime"
)

// Definition describes a configured scheduler instance.
type Definition struct {
	Name           string
	Workers        int
	QueueSize      int
	Labels         map[string]string
	LoggingEnabled bool
	Logging        runtime.OTLPEmitterConfig
	Builtin        bool
}

type manager struct {
	mu    sync.RWMutex
	defs  map[string]Definition
	hosts map[string]*runtimeHost
}

var defaultManager = newManager()

func defaultDefinition() Definition {
	return Definition{
		Name:           "default",
		Workers:        50,
		QueueSize:      128,
		Labels:         nil,
		LoggingEnabled: true,
		Logging: runtime.OTLPEmitterConfig{
			Endpoint: runtime.DefaultOTLPEndpoint,
			Timeout:  runtime.DefaultOTLPTimeout,
			UseTLS:   false,
			Headers:  map[string]string{},
		},
		Builtin: true,
	}
}

func newManager() *manager {
	m := &manager{
		defs:  make(map[string]Definition),
		hosts: make(map[string]*runtimeHost),
	}
	// Seed with default scheduler definition.
	m.defs["default"] = defaultDefinition()
	return m
}

// ApplyDefinition registers or updates a scheduler definition and ensures its runtime host exists.
func ApplyDefinition(def Definition, log *logger.Logger) error {
	return defaultManager.applyDefinition(def, log)
}

// RemoveDefinition deletes a scheduler definition (and stops its runtime) if no jobs remain.
func RemoveDefinition(name string) error {
	return defaultManager.removeDefinition(name)
}

// Get returns a scheduler definition by name.
func Get(name string) (Definition, bool) {
	return defaultManager.get(name)
}

// All returns every registered definition.
func All() []Definition {
	return defaultManager.all()
}

// JobHandle represents a job registered with a scheduler.
type JobHandle struct {
	scheduler string
	jobID     string
}

// AttachJob registers a job to a scheduler.
func AttachJob(name string, reg runtime.JobRegistration, log *logger.Logger) (*JobHandle, error) {
	return defaultManager.attachJob(name, reg, log)
}

// DetachJob removes a job from its scheduler.
func DetachJob(handle *JobHandle) {
	defaultManager.detachJob(handle)
}

// CollectMetrics returns runtime metrics for the given scheduler.
func CollectMetrics(name string) map[string]int64 {
	return defaultManager.collectMetrics(name)
}

func (m *manager) applyDefinition(def Definition, log *logger.Logger) error {
	if def.Name == "" {
		return fmt.Errorf("scheduler name is required")
	}
	norm := normalizeDefinition(def)
	m.mu.Lock()
	old := m.hosts[norm.Name]
	m.mu.Unlock()
	if old == nil {
		host, err := newRuntimeHost(norm, log)
		if err != nil {
			return err
		}
		m.mu.Lock()
		m.defs[norm.Name] = norm
		m.hosts[norm.Name] = host
		m.mu.Unlock()
		return nil
	}
	jobs := old.snapshotJobs()
	newHost, err := newRuntimeHost(norm, log)
	if err != nil {
		return err
	}
	if err := newHost.restoreJobs(jobs); err != nil {
		newHost.stop()
		return err
	}
	old.stop()
	m.mu.Lock()
	m.defs[norm.Name] = norm
	m.hosts[norm.Name] = newHost
	m.mu.Unlock()
	return nil
}

func (m *manager) removeDefinition(name string) error {
	if name == "" {
		return fmt.Errorf("scheduler name is required")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	host := m.hosts[name]
	if host != nil && host.jobCount() > 0 {
		return fmt.Errorf("scheduler '%s' still has %d jobs", name, host.jobCount())
	}
	if host != nil {
		host.stop()
		delete(m.hosts, name)
	}
	if name == "default" {
		m.defs[name] = defaultDefinition()
		return nil
	}
	delete(m.defs, name)
	return nil
}

func (m *manager) get(name string) (Definition, bool) {
	m.mu.RLock()
	def, ok := m.defs[name]
	m.mu.RUnlock()
	return def, ok
}

func (m *manager) all() []Definition {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Definition, 0, len(m.defs))
	for _, def := range m.defs {
		out = append(out, def)
	}
	return out
}

func (m *manager) attachJob(name string, reg runtime.JobRegistration, log *logger.Logger) (*JobHandle, error) {
	m.mu.RLock()
	host := m.hosts[name]
	m.mu.RUnlock()
	if host == nil {
		m.mu.Lock()
		defer m.mu.Unlock()
		var ok bool
		host, ok = m.hosts[name]
		if !ok {
			def, exists := m.defs[name]
			if !exists {
				return nil, fmt.Errorf("scheduler '%s' not defined", name)
			}
			newHost, err := newRuntimeHost(def, log)
			if err != nil {
				return nil, err
			}
			host = newHost
			m.hosts[name] = host
		}
	}
	jobID, err := host.attach(reg)
	if err != nil {
		return nil, err
	}
	return &JobHandle{scheduler: name, jobID: jobID}, nil
}

func (m *manager) detachJob(handle *JobHandle) {
	if handle == nil {
		return
	}
	m.mu.RLock()
	host := m.hosts[handle.scheduler]
	m.mu.RUnlock()
	if host == nil {
		return
	}
	host.detach(handle.jobID)
}

func (m *manager) collectMetrics(name string) map[string]int64 {
	m.mu.RLock()
	host := m.hosts[name]
	m.mu.RUnlock()
	if host == nil {
		return nil
	}
	return host.collectMetrics()
}

func normalizeDefinition(def Definition) Definition {
	if def.Workers <= 0 {
		def.Workers = 50
	}
	if def.QueueSize <= 0 {
		def.QueueSize = 128
	}
	if def.Logging.Headers == nil {
		def.Logging.Headers = make(map[string]string)
	} else {
		def.Logging.Headers = maps.Clone(def.Logging.Headers)
	}
	if def.Labels != nil {
		def.Labels = maps.Clone(def.Labels)
	}
	return def
}
