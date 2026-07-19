// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	secretresolver "github.com/netdata/netdata/go/plugins/plugin/agent/secrets/resolver"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"gopkg.in/yaml.v2"
)

type ConfigModuleFactoryConfig struct {
	Modules    ModuleCatalog
	Resolver   *secretresolver.AtomicResolver
	StoreScope secretresolver.AtomicScopeAcquirer
	Logger     *logger.Logger
}

type configModule interface {
	GetBase() *collectorapi.Base
	Init(context.Context) error
	Check(context.Context) error
	Cleanup(context.Context)
	Configuration() any
}

// ConfigModuleFactory owns short-lived V2-over-V1 configuration probes.
// Every constructed module is cleaned before the call returns.
type ConfigModuleFactory struct {
	config ConfigModuleFactoryConfig
}

func NewConfigModuleFactory(
	config ConfigModuleFactoryConfig,
) (*ConfigModuleFactory, error) {
	if config.Modules == nil ||
		config.Resolver == nil ||
		config.StoreScope == nil {
		return nil, errors.New(
			"job output: incomplete config-module factory configuration",
		)
	}
	if config.Logger == nil {
		config.Logger = logger.New().With(
			slog.String("component", "config module factory"),
		)
	}
	return &ConfigModuleFactory{config: config}, nil
}

func (factory *ConfigModuleFactory) Configuration(
	ctx context.Context,
	config confgroup.Config,
) (payload []byte, err error) {
	if ctx == nil || config == nil {
		return nil, errors.New(
			"job output: invalid config-module configuration request",
		)
	}
	attempt, err := factory.construct(config.Module())
	if err != nil {
		return nil, err
	}
	defer func() {
		err = errors.Join(
			err,
			attempt.cleanup(context.WithoutCancel(ctx)),
		)
	}()
	if err = applyConfigModuleRaw(config, attempt.module); err != nil {
		return nil, fmt.Errorf(
			"job output: applying raw configuration: %w",
			err,
		)
	}
	value := attempt.module.Configuration()
	if value == nil {
		return nil, errors.New(
			"job output: collector does not provide configuration",
		)
	}
	payload, err = json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf(
			"job output: marshaling configuration: %w",
			err,
		)
	}
	return payload, nil
}

func (factory *ConfigModuleFactory) Test(
	ctx context.Context,
	config confgroup.Config,
) (err error) {
	if ctx == nil || config == nil {
		return errors.New("job output: invalid config-module test")
	}
	attempt, err := factory.construct(config.Module())
	if err != nil {
		return err
	}
	defer func() {
		err = errors.Join(
			err,
			attempt.cleanup(context.WithoutCancel(ctx)),
		)
	}()
	if named, ok := attempt.module.(interface{ SetJobName(string) }); ok {
		named.SetJobName(config.Name())
	}
	if err := factory.applyResolved(ctx, config, attempt.module); err != nil {
		return err
	}
	attempt.module.GetBase().Logger = factory.config.Logger.With(
		slog.String("collector", config.Module()),
		slog.String("job", config.Name()),
	)
	if err := attempt.module.Init(ctx); err != nil {
		return fmt.Errorf("job output: collector initialization: %w", err)
	}
	if err := attempt.module.Check(ctx); err != nil {
		return fmt.Errorf("job output: collector check: %w", err)
	}
	return nil
}

func (factory *ConfigModuleFactory) Validate(
	ctx context.Context,
	config confgroup.Config,
) (err error) {
	if ctx == nil || config == nil {
		return errors.New("job output: invalid config-module validation")
	}
	attempt, err := factory.construct(config.Module())
	if err != nil {
		return err
	}
	defer func() {
		err = errors.Join(
			err,
			attempt.cleanup(context.WithoutCancel(ctx)),
		)
	}()
	if named, ok := attempt.module.(interface{ SetJobName(string) }); ok {
		named.SetJobName(config.Name())
	}
	return factory.applyResolved(ctx, config, attempt.module)
}

func (factory *ConfigModuleFactory) construct(
	module string,
) (attempt configModuleAttempt, err error) {
	if factory == nil || module == "" {
		return configModuleAttempt{},
			errors.New("job output: invalid config-module construction")
	}
	creator, ok := factory.config.Modules.Lookup(module)
	if !ok {
		return configModuleAttempt{},
			fmt.Errorf("job output: module %q is not registered", module)
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf(
				"%w in config-module construction: %v",
				lifecycle.ErrTaskPanic,
				recovered,
			)
		}
	}()
	var constructed configModule
	if creator.CreateV2 != nil {
		constructed = creator.CreateV2()
	} else if creator.Create != nil {
		constructed = creator.Create()
	} else {
		return configModuleAttempt{},
			fmt.Errorf(
				"job output: module %q has no collector creator",
				module,
			)
	}
	if constructed == nil {
		return configModuleAttempt{},
			fmt.Errorf(
				"job output: module %q returned a nil config module",
				module,
			)
	}
	return configModuleAttempt{module: constructed}, nil
}

func (factory *ConfigModuleFactory) applyResolved(
	ctx context.Context,
	config confgroup.Config,
	module configModule,
) error {
	resolved, err := factory.config.Resolver.Resolve(
		ctx,
		map[string]any(config),
		factory.config.StoreScope,
	)
	if err != nil {
		return fmt.Errorf(
			"job output: resolving configuration secrets: %w",
			err,
		)
	}
	payload, err := yaml.Marshal(resolved)
	if err != nil {
		return fmt.Errorf(
			"job output: marshaling resolved configuration: %w",
			err,
		)
	}
	if len(payload) > secretresolver.MaximumAtomicResolvedBytes {
		return errors.New(
			"job output: serialized configuration exceeds maximum size",
		)
	}
	if err := yaml.Unmarshal(payload, module); err != nil {
		return fmt.Errorf(
			"job output: applying resolved configuration: %w",
			err,
		)
	}
	return nil
}

type configModuleAttempt struct {
	module configModule
	once   sync.Once
	err    error
}

func (attempt *configModuleAttempt) cleanup(ctx context.Context) error {
	if attempt == nil || attempt.module == nil {
		return nil
	}
	attempt.once.Do(func() {
		attempt.err = callJobLifecycle(
			"config-module Cleanup",
			func() error {
				attempt.module.Cleanup(ctx)
				return nil
			},
		)
	})
	return attempt.err
}

func applyConfigModuleRaw(
	config confgroup.Config,
	module configModule,
) error {
	payload, err := yaml.Marshal(config)
	if err != nil {
		return err
	}
	return yaml.Unmarshal(payload, module)
}
