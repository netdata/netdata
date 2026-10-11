// SPDX-License-Identifier: GPL-3.0-or-later

package composition

import (
	"errors"
	"fmt"
	"sync"
)

func processDependencySources(services []ProcessService) []DependencySource {
	var sources []DependencySource
	for _, service := range services {
		if source, ok := service.(DependencySource); ok {
			sources = append(sources, source)
		}
	}
	return sources
}

// Bound at run startup, closed after its jobs and activation attempts have joined.
// Each run has distinct subscriptions even though the source survives restarts.
type dependencySubscriptions struct {
	mu          sync.Mutex
	closed      bool
	done        chan struct{}
	unsubscribe []func()
	err         error
}

func (s *dependencySubscriptions) bind(sources []DependencySource, notify func(string, string)) (err error) {
	defer func() {
		if value := recover(); value != nil {
			err = fmt.Errorf("dependency subscription panicked: %v", value)
		}
		if err != nil {
			err = errors.Join(err, s.close())
		}
	}()
	for _, source := range sources {
		s.mu.Lock()
		closed := s.closed
		s.mu.Unlock()
		if closed {
			return errors.New("dependency subscriptions closed")
		}
		if unsubscribe := source.SubscribeDependencies(notify); unsubscribe != nil {
			s.mu.Lock()
			closed = s.closed
			if !closed {
				s.unsubscribe = append(s.unsubscribe, unsubscribe)
			}
			s.mu.Unlock()
			if closed {
				// Registration may return after the run's finalizer has finished.
				return errors.Join(errors.New("dependency subscriptions closed"), unsubscribeDependency(unsubscribe))
			}
		}
	}
	return nil
}
func (s *dependencySubscriptions) close() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	if s.closed {
		done := s.done
		s.mu.Unlock()
		<-done
		return s.err
	}
	s.closed = true
	s.done = make(chan struct{})
	unsubscribe := s.unsubscribe
	s.unsubscribe = nil
	s.mu.Unlock()

	// Provider code must run outside the subscription owner's lock.
	var err error
	for _, fn := range unsubscribe {
		err = errors.Join(err, unsubscribeDependency(fn))
	}
	s.err = err
	close(s.done)
	return err
}
func unsubscribeDependency(unsubscribe func()) (err error) {
	defer func() {
		if value := recover(); value != nil {
			err = fmt.Errorf("dependency unsubscribe panicked: %v", value)
		}
	}()
	unsubscribe()
	return nil
}
