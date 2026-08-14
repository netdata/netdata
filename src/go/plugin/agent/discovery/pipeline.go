// SPDX-License-Identifier: GPL-3.0-or-later

package discovery

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
)

const pipelineSendEvery = 2 * time.Second

// PipelineGeneration owns one run's frozen providers and aggregation state.
type PipelineGeneration struct {
	mu sync.Mutex

	providers map[string]*pipelineProvider
	enabled   []string
	disabled  []string
	sendEvery time.Duration
	run       bool
}

type pipelineProvider struct {
	discoverer Discoverer
	updates    chan []*confgroup.Group
	started    bool
}

type PipelineApply func(
	context.Context,
	[]*confgroup.Group,
) error

func NewPipelineGeneration(
	config PipelineConfig,
) (*PipelineGeneration, error) {
	if err := validatePipelineConfig(config); err != nil {
		return nil, fmt.Errorf(
			"discovery pipeline config validation: %w",
			err,
		)
	}
	generation := &PipelineGeneration{
		providers: make(
			map[string]*pipelineProvider,
			config.Providers.Len(),
		),
		sendEvery: pipelineSendEvery,
	}
	for _, name := range config.Providers.Names() {
		factory, ok := config.Providers.Lookup(name)
		if !ok {
			return nil, errors.New(
				"discovery pipeline: frozen provider disappeared",
			)
		}
		discoverer, enabled, err := buildPipelineProvider(
			factory,
			config.BuildContext,
		)
		if err != nil {
			return nil, fmt.Errorf(
				"discovery pipeline provider %q: %w",
				name,
				err,
			)
		}
		if !enabled {
			generation.disabled = append(generation.disabled, name)
			continue
		}
		if discoverer == nil {
			return nil, fmt.Errorf(
				"discovery pipeline provider %q: enabled with nil discoverer",
				name,
			)
		}
		generation.providers[name] = &pipelineProvider{
			discoverer: discoverer,
			updates:    make(chan []*confgroup.Group),
		}
		generation.enabled = append(generation.enabled, name)
	}
	if len(generation.enabled) == 0 {
		return nil, errors.New(
			"discovery pipeline: zero enabled providers",
		)
	}
	return generation, nil
}

func buildPipelineProvider(
	factory ProviderFactory,
	ctx BuildContext,
) (
	discoverer Discoverer,
	enabled bool,
	err error,
) {
	defer func() {
		if recovered := recover(); recovered != nil {
			discoverer = nil
			enabled = false
			err = fmt.Errorf(
				"provider factory panic: %v",
				recovered,
			)
		}
	}()
	return factory.Build(ctx)
}

func (generation *PipelineGeneration) ProviderNames() []string {
	if generation == nil {
		return nil
	}
	return append([]string(nil), generation.enabled...)
}

func (generation *PipelineGeneration) DisabledProviderNames() []string {
	if generation == nil {
		return nil
	}
	return append([]string(nil), generation.disabled...)
}

func (generation *PipelineGeneration) RunProvider(
	ctx context.Context,
	name string,
) error {
	if generation == nil || ctx == nil || name == "" {
		return errors.New("discovery pipeline: invalid provider run")
	}
	generation.mu.Lock()
	provider := generation.providers[name]
	if provider == nil || provider.started {
		generation.mu.Unlock()
		return errors.New(
			"discovery pipeline: unknown or already started provider",
		)
	}
	provider.started = true
	generation.mu.Unlock()
	provider.discoverer.Run(ctx, provider.updates)
	return nil
}

func (generation *PipelineGeneration) Run(
	ctx context.Context,
	apply PipelineApply,
) error {
	if generation == nil || ctx == nil || apply == nil {
		return errors.New("discovery pipeline: invalid aggregation run")
	}
	generation.mu.Lock()
	if generation.run {
		generation.mu.Unlock()
		return errors.New(
			"discovery pipeline: aggregation already started",
		)
	}
	generation.run = true
	providers := make([]*pipelineProvider, 0, len(generation.enabled))
	for _, name := range generation.enabled {
		providers = append(providers, generation.providers[name])
	}
	sendEvery := generation.sendEvery
	generation.mu.Unlock()

	ticker := time.NewTicker(sendEvery)
	defer ticker.Stop()
	cases := make([]reflect.SelectCase, 2, len(providers)+2)
	cases[0] = reflect.SelectCase{
		Dir:  reflect.SelectRecv,
		Chan: reflect.ValueOf(ctx.Done()),
	}
	cases[1] = reflect.SelectCase{
		Dir:  reflect.SelectRecv,
		Chan: reflect.ValueOf(ticker.C),
	}
	for _, provider := range providers {
		cases = append(cases, reflect.SelectCase{
			Dir:  reflect.SelectRecv,
			Chan: reflect.ValueOf(provider.updates),
		})
	}

	cache := newCache()
	pending := false
	sent := false
	for {
		index, value, ok := reflect.Select(cases)
		switch index {
		case 0:
			return nil
		case 1:
			if pending {
				if err := apply(ctx, cache.groups()); err != nil {
					if ctx.Err() != nil {
						return nil
					}
					return fmt.Errorf(
						"discovery pipeline apply: %w",
						err,
					)
				}
				cache.reset()
				pending = false
			}
		default:
			if !ok {
				cases[index].Chan = reflect.Value{}
				continue
			}
			groups, valid := value.Interface().([]*confgroup.Group)
			if !valid {
				return errors.New(
					"discovery pipeline: invalid provider update",
				)
			}
			cache.update(groups)
			pending = true
			if !sent {
				if err := apply(ctx, cache.groups()); err != nil {
					if ctx.Err() != nil {
						return nil
					}
					return fmt.Errorf(
						"discovery pipeline apply: %w",
						err,
					)
				}
				cache.reset()
				pending = false
				sent = true
			}
		}
	}
}
