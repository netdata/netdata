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
	Modules    ModuleCatalog                      // registry of module creators to look up by name
	Resolver   *secretresolver.AtomicResolver     // resolves secret references in a config
	StoreScope secretresolver.AtomicScopeAcquirer // acquires the reader scope for secret resolution
}

type configModule interface {
	GetBase() *collectorapi.Base
	Init(context.Context) error
	Check(context.Context) error
	Cleanup(context.Context)
	Configuration() any
}

// ConfigModuleFactory builds short-lived throwaway collector modules used to
// validate a config and render its effective configuration. Every constructed
// module is cleaned before the call returns.
type ConfigModuleFactory struct {
	config ConfigModuleFactoryConfig
	logger *logger.Logger
}

func NewConfigModuleFactory(config ConfigModuleFactoryConfig) (*ConfigModuleFactory, error) {
	if config.Modules == nil || config.Resolver == nil || config.StoreScope == nil {
		return nil, errors.New("job output: incomplete config-module factory configuration")
	}
	return &ConfigModuleFactory{
		config: config,
		logger: logger.New().With(slog.String("component", "config module factory")),
	}, nil
}

func (cmf *ConfigModuleFactory) Configuration(
	ctx context.Context,
	config confgroup.Config,
) (payload []byte, err error) {
	if ctx == nil || config == nil {
		return nil, errors.New("job output: invalid config-module configuration request")
	}
	probe, err := cmf.construct(config.Module())
	if err != nil {
		return nil, err
	}
	defer func() {
		err = errors.Join(err, probe.cleanup(context.WithoutCancel(ctx)))
	}()
	if err = applyConfigModuleRaw(config, probe.module); err != nil {
		return nil, fmt.Errorf("job output: applying raw configuration: %w", err)
	}
	value := probe.module.Configuration()
	if value == nil {
		return nil, errors.New("job output: collector does not provide configuration")
	}
	payload, err = json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("job output: marshaling configuration: %w", err)
	}
	return payload, nil
}

func (cmf *ConfigModuleFactory) Test(ctx context.Context, config confgroup.Config) (err error) {
	if ctx == nil || config == nil {
		return errors.New("job output: invalid config-module test")
	}
	probe, err := cmf.construct(config.Module())
	if err != nil {
		return err
	}
	defer func() {
		err = errors.Join(err, probe.cleanup(context.WithoutCancel(ctx)))
	}()
	if named, ok := probe.module.(interface{ SetJobName(string) }); ok {
		named.SetJobName(config.Name())
	}
	if err := cmf.applyResolved(ctx, config, probe.module); err != nil {
		return err
	}
	probe.module.GetBase().Logger = cmf.logger.With(
		slog.String("collector", config.Module()),
		slog.String("job", config.Name()),
	)
	if err := probe.module.Init(ctx); err != nil {
		return fmt.Errorf("job output: collector initialization: %w", err)
	}
	if err := probe.module.Check(ctx); err != nil {
		return fmt.Errorf("job output: collector check: %w", err)
	}
	return nil
}

func (cmf *ConfigModuleFactory) Validate(ctx context.Context, config confgroup.Config) (err error) {
	if ctx == nil || config == nil {
		return errors.New("job output: invalid config-module validation")
	}
	probe, err := cmf.construct(config.Module())
	if err != nil {
		return err
	}
	defer func() {
		err = errors.Join(err, probe.cleanup(context.WithoutCancel(ctx)))
	}()
	if named, ok := probe.module.(interface{ SetJobName(string) }); ok {
		named.SetJobName(config.Name())
	}
	return cmf.applyResolved(ctx, config, probe.module)
}

func (cmf *ConfigModuleFactory) construct(module string) (probe constructedConfigModule, err error) {
	if cmf == nil || module == "" {
		return constructedConfigModule{}, errors.New("job output: invalid config-module construction")
	}
	creator, ok := cmf.config.Modules.Lookup(module)
	if !ok {
		return constructedConfigModule{}, fmt.Errorf("job output: module %q is not registered", module)
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("%w in config-module construction: %v", lifecycle.ErrTaskPanic, recovered)
		}
	}()
	var constructed configModule
	if creator.CreateV2 != nil {
		constructed = creator.CreateV2()
	} else if creator.Create != nil {
		constructed = creator.Create()
	} else {
		return constructedConfigModule{}, fmt.Errorf("job output: module %q has no collector creator", module)
	}
	if constructed == nil {
		return constructedConfigModule{}, fmt.Errorf("job output: module %q returned a nil config module", module)
	}
	return constructedConfigModule{
		module: constructed,
	}, nil
}

func (cmf *ConfigModuleFactory) applyResolved(ctx context.Context, config confgroup.Config, module any) error {
	resolveCtx := logger.ContextWithLogger(
		ctx,
		cmf.logger.With(slog.String("collector", config.Module()), slog.String("job", config.Name())),
	)
	resolved, err := cmf.config.Resolver.Resolve(resolveCtx, map[string]any(config), cmf.config.StoreScope)
	if err != nil {
		return fmt.Errorf("job output: resolving configuration secrets: %w", err)
	}
	payload, err := yaml.Marshal(resolved)
	if err != nil {
		return fmt.Errorf("job output: marshaling resolved configuration: %w", err)
	}
	if len(payload) > secretresolver.MaximumAtomicResolvedBytes {
		return errors.New("job output: serialized configuration exceeds maximum size")
	}
	if err := yaml.Unmarshal(payload, module); err != nil {
		return fmt.Errorf("job output: applying resolved configuration: %w", err)
	}
	return nil
}

type constructedConfigModule struct {
	module configModule
	once   sync.Once
	err    error
}

func (ccm *constructedConfigModule) cleanup(ctx context.Context) error {
	if ccm == nil || ccm.module == nil {
		return nil
	}
	ccm.once.Do(func() {
		ccm.err = callJobLifecycle(
			"config-module Cleanup",
			func() error {
				ccm.module.Cleanup(ctx)
				return nil
			},
		)
	})
	return ccm.err
}

func applyConfigModuleRaw(config confgroup.Config, module configModule) error {
	payload, err := yaml.Marshal(config)
	if err != nil {
		return err
	}
	return yaml.Unmarshal(payload, module)
}
