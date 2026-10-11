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
	once        sync.Once
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
		if unsubscribe := source.SubscribeDependencies(notify); unsubscribe != nil {
			s.unsubscribe = append(s.unsubscribe, unsubscribe)
		}
	}
	return nil
}
func (s *dependencySubscriptions) close() error {
	if s == nil {
		return nil
	}
	s.once.Do(func() {
		for _, unsubscribe := range s.unsubscribe {
			s.err = errors.Join(s.err, unsubscribeDependency(unsubscribe))
		}
		s.unsubscribe = nil
	})
	return s.err
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
