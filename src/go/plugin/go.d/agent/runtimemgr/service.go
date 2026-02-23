// SPDX-License-Identifier: GPL-3.0-or-later

package runtimemgr

import (
	"fmt"
	"io"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/ticker"
	"github.com/netdata/netdata/go/plugins/plugin/framework/runtimecomp"
)

// Service owns runtime/internal metrics components and their cadence-driven
// chart emission job.
type Service struct {
	*logger.Logger

	mu         sync.Mutex
	pluginName string
	out        io.Writer
	started    bool
	tickEvery  time.Duration

	registry  *componentRegistry
	job       *runtimeMetricsJob
	producers map[string]runtimeProducer
	tkStop    chan struct{}
	tkDone    chan struct{}
}

func New(log *logger.Logger) *Service {
	if log == nil {
		log = logger.New().With(slog.String("component", "runtime metrics service"))
	}
	return &Service{
		Logger:    log,
		registry:  newComponentRegistry(),
		producers: make(map[string]runtimeProducer),
		tickEvery: time.Second,
	}
}

// SetTickEvery updates the service scheduler interval. Non-positive values are
// ignored. Changing this value while running does not affect the current ticker
// and applies on next Start.
func (s *Service) SetTickEvery(interval time.Duration) {
	if s == nil || interval <= 0 {
		return
	}
	s.mu.Lock()
	s.tickEvery = interval
	s.mu.Unlock()
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

	s.job = newRuntimeMetricsJob(
		s.out,
		s.registry,
		s.Logger.With(slog.String("component", "runtime metrics job")),
	)
	job := s.job
	stopCh := make(chan struct{})
	doneCh := make(chan struct{})
	interval := s.tickEvery
	if interval <= 0 {
		interval = time.Second
	}
	s.tkStop = stopCh
	s.tkDone = doneCh
	s.started = true
	s.mu.Unlock()

	go job.Start()
	go s.runTicker(interval, stopCh, doneCh)
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
	stopCh := s.tkStop
	doneCh := s.tkDone
	s.job = nil
	s.tkStop = nil
	s.tkDone = nil
	s.started = false
	s.mu.Unlock()

	if stopCh != nil {
		close(stopCh)
	}
	if doneCh != nil {
		<-doneCh
	}
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
	producers := make([]runtimeProducer, 0, len(s.producers))
	for _, producer := range s.producers {
		producers = append(producers, producer)
	}
	sort.Slice(producers, func(i, j int) bool {
		return producers[i].Name() < producers[j].Name()
	})
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

func (s *Service) runTicker(interval time.Duration, stop <-chan struct{}, done chan<- struct{}) {
	defer close(done)

	tk := ticker.New(interval)
	defer tk.Stop()

	for {
		select {
		case <-stop:
			return
		case clock := <-tk.C:
			s.Tick(clock)
		}
	}
}

func (s *Service) RegisterComponent(cfg runtimecomp.ComponentConfig) error {
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

func (s *Service) RegisterProducer(name string, tickFn func() error) error {
	if s == nil {
		return fmt.Errorf("runtimemgr: nil service")
	}

	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("runtimemgr: runtime producer name is required")
	}
	if tickFn == nil {
		return fmt.Errorf("runtimemgr: runtime producer %q tick function is required", name)
	}

	s.mu.Lock()
	if s.producers == nil {
		s.producers = make(map[string]runtimeProducer)
	}
	s.producers[name] = runtimeProducerFunc{
		name: name,
		tick: tickFn,
	}
	s.mu.Unlock()
	return nil
}

func (s *Service) UnregisterProducer(name string) {
	if s == nil {
		return
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return
	}
	s.mu.Lock()
	delete(s.producers, name)
	s.mu.Unlock()
}
