// SPDX-License-Identifier: GPL-3.0-or-later

package runtimemgr

import (
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"

	"github.com/netdata/netdata/go/plugins/logger"
)

// Service owns runtime/internal metrics components and their cadence-driven
// chart emission job.
type Service struct {
	*logger.Logger

	mu         sync.Mutex
	pluginName string
	out        io.Writer
	started    bool

	registry  *componentRegistry
	job       *runtimeMetricsJob
	producers []runtimeProducer
}

func New(log *logger.Logger) *Service {
	if log == nil {
		log = logger.New().With(slog.String("component", "runtime metrics service"))
	}
	return &Service{
		Logger:   log,
		registry: newComponentRegistry(),
	}
}

// Start initializes the runtime metrics service and starts the runtime emitter
// job. Calling Start multiple times is safe.
func (s *Service) Start(pluginName string, out io.Writer) {
	if s == nil {
		return
	}

	s.mu.Lock()
	if p := strings.TrimSpace(pluginName); p != "" {
		s.pluginName = p
	}
	if out != nil {
		s.out = out
	}
	if s.out == nil {
		s.out = io.Discard
	}
	if s.started {
		s.mu.Unlock()
		return
	}

	s.bootstrapDefaults()

	s.job = newRuntimeMetricsJob(
		s.out,
		s.registry,
		s.Logger.With(slog.String("component", "runtime metrics job")),
	)
	job := s.job
	s.started = true
	s.mu.Unlock()

	go job.Start()
}

// Stop terminates the runtime emitter job. Calling Stop multiple times is safe.
func (s *Service) Stop() {
	if s == nil {
		return
	}

	s.mu.Lock()
	if !s.started {
		s.mu.Unlock()
		return
	}
	job := s.job
	s.job = nil
	s.started = false
	s.mu.Unlock()

	if job != nil {
		job.Stop()
	}
}

// Tick advances runtime producers and runtime emitter job on scheduler cadence.
func (s *Service) Tick(clock int) {
	if s == nil {
		return
	}

	s.mu.Lock()
	if !s.started {
		s.mu.Unlock()
		return
	}
	producers := append([]runtimeProducer(nil), s.producers...)
	job := s.job
	s.mu.Unlock()

	for _, producer := range producers {
		if producer == nil {
			continue
		}
		if err := producer.Tick(); err != nil {
			s.Warningf("runtime producer %q tick failed: %v", producer.Name(), err)
		}
	}
	if job != nil {
		job.Tick(clock)
	}
}

func (s *Service) RegisterComponent(cfg ComponentConfig) error {
	if s == nil {
		return fmt.Errorf("runtimemgr: nil service")
	}

	s.mu.Lock()
	pluginName := firstNotEmpty(s.pluginName, "go.d")
	s.mu.Unlock()

	spec, err := normalizeComponent(cfg, pluginName)
	if err != nil {
		return err
	}
	s.registry.upsert(spec)
	return nil
}

func (s *Service) UnregisterComponent(name string) {
	if s == nil {
		return
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return
	}
	s.registry.remove(name)
}
