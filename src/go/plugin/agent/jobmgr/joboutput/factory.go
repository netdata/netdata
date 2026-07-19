// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	secretresolver "github.com/netdata/netdata/go/plugins/plugin/agent/secrets/resolver"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/jobruntime"
	"github.com/netdata/netdata/go/plugins/plugin/framework/runtimecomp"
	"github.com/netdata/netdata/go/plugins/plugin/framework/vnoderegistry"
	"gopkg.in/yaml.v2"
)

type ModuleCatalog interface {
	Lookup(string) (collectorapi.Creator, bool)
}

type RuntimeJob interface {
	ManagedJob
	collectorapi.RuntimeJob

	AutoDetection(context.Context) error
	AutoDetectionManaged(context.Context) error
	CleanupRejected()
	Tick(int)
}

type PublishedJob struct {
	Identity lifecycle.ResourceIdentity
	Variant  JobVariant
	Job      RuntimeJob
}

type HandlerLifecycle interface {
	Publish() error
	CloseAndDrain(context.Context) error
	Cleanup(context.Context) error
}

type JobHooks interface {
	Prepare(PublishedJob) (HandlerLifecycle, error)
}

type FactoryConfig struct {
	PluginName string
	Modules    ModuleCatalog
	Tasks      *lifecycle.TaskSupervisor
	Frames     *lifecycle.FrameOwner
	Resolver   *secretresolver.AtomicResolver
	StoreScope secretresolver.AtomicScopeAcquirer
	Runtime    runtimecomp.Service
	Vnodes     *vnoderegistry.Registry
	Vnode      func(string) (jobruntime.VnodeSnapshot, bool)
	Hooks      JobHooks
	Scheduler  *Scheduler
	Observer   lifecycle.RuntimeObserver
	Logger     *logger.Logger
}

// Factory owns collector construction, validation, and transfer. It does not
// own current-job indexing or lifecycle state.
type Factory struct {
	config FactoryConfig
}

func NewFactory(config FactoryConfig) (*Factory, error) {
	if config.PluginName == "" ||
		config.Modules == nil ||
		config.Tasks == nil ||
		config.Frames == nil ||
		config.Resolver == nil ||
		config.StoreScope == nil ||
		config.Vnodes == nil ||
		config.Scheduler == nil {
		return nil, errors.New("job output: incomplete factory configuration")
	}
	if config.Logger == nil {
		config.Logger = logger.New().With(slog.String("component", "job factory"))
	}
	return &Factory{config: config}, nil
}

func (factory *Factory) ValidateConfig(
	ctx context.Context,
	config confgroup.Config,
) error {
	if factory == nil || ctx == nil || config == nil {
		return errors.New("job output: invalid factory validation")
	}
	creator, ok := factory.config.Modules.Lookup(config.Module())
	if !ok {
		return fmt.Errorf(
			"job output: module %q is not registered",
			config.Module(),
		)
	}
	if err := validateFactoryConfigIdentity(config, creator); err != nil {
		return err
	}
	if config.FunctionOnly() &&
		creator.SharedFunctions == nil &&
		creator.AgentFunctions == nil &&
		creator.InstanceFunctions == nil {
		return fmt.Errorf(
			"job output: function_only is set but module %q declares no Functions",
			config.Module(),
		)
	}
	if _, err := factory.lookupVNode(config); err != nil {
		return err
	}
	configModules, err := NewConfigModuleFactory(
		ConfigModuleFactoryConfig{
			Modules: factory.config.Modules, Resolver: factory.config.Resolver,
			StoreScope: factory.config.StoreScope,
			Logger:     factory.config.Logger,
		},
	)
	if err != nil {
		return err
	}
	return configModules.Validate(ctx, config)
}

func (factory *Factory) Build(
	ctx context.Context,
	config confgroup.Config,
	generation uint64,
) (ConstructedJob, error) {
	if factory == nil || ctx == nil || config == nil || generation == 0 {
		return ConstructedJob{}, errors.New("job output: invalid factory build")
	}
	creator, ok := factory.config.Modules.Lookup(config.Module())
	if !ok {
		return ConstructedJob{}, fmt.Errorf("job output: module %q is not registered", config.Module())
	}
	if err := validateFactoryConfigIdentity(config, creator); err != nil {
		return ConstructedJob{}, err
	}
	functionOnly := creator.FunctionOnly || config.FunctionOnly()
	hasFunctions := creator.SharedFunctions != nil ||
		creator.AgentFunctions != nil ||
		creator.InstanceFunctions != nil
	if functionOnly && !hasFunctions {
		return ConstructedJob{}, fmt.Errorf(
			"job output: function_only is set but module %q declares no Functions",
			config.Module(),
		)
	}
	vnode, err := factory.lookupVNode(config)
	if err != nil {
		return ConstructedJob{}, err
	}
	identity := lifecycle.ResourceIdentity{
		ID: config.FullName(), Generation: generation,
	}
	var job RuntimeJob
	var variant JobVariant
	if creator.CreateV2 != nil {
		job, err = factory.buildV2(ctx, config, creator, functionOnly, vnode)
		variant = JobVariantV2
	} else {
		job, err = factory.buildV1(ctx, config, creator, functionOnly, vnode)
		variant = JobVariantV1
	}
	if err != nil {
		return ConstructedJob{}, err
	}
	cleanup := &factoryJobCleanup{job: job}
	if err := job.AutoDetectionManaged(ctx); err != nil {
		return ConstructedJob{}, errors.Join(
			err,
			cleanup.reject(context.WithoutCancel(ctx)),
		)
	}
	constructed, err := NewManagedJob(
		variant,
		job,
		factory.config.Tasks,
		identity,
		factory.config.Scheduler,
	)
	if err != nil {
		return ConstructedJob{}, errors.Join(
			err,
			cleanup.reject(context.WithoutCancel(ctx)),
		)
	}
	constructed.CollectorCleanup = cleanup.reject
	if hasFunctions && factory.config.Hooks == nil {
		return ConstructedJob{}, errors.Join(
			errors.New("job output: function-bearing job has no handler lifecycle"),
			disposeConstructed(context.WithoutCancel(ctx), constructed),
		)
	}
	if hooks := factory.config.Hooks; hasFunctions {
		published := PublishedJob{Identity: identity, Variant: variant, Job: job}
		handlers, prepareErr := callPrepareHandlers(hooks, published)
		if prepareErr != nil {
			return ConstructedJob{}, errors.Join(
				prepareErr,
				disposeHandlers(context.WithoutCancel(ctx), handlers),
				disposeConstructed(context.WithoutCancel(ctx), constructed),
			)
		}
		if handlers == nil {
			return ConstructedJob{}, errors.Join(
				errors.New("job output: nil prepared handler lifecycle"),
				disposeConstructed(context.WithoutCancel(ctx), constructed),
			)
		}
		constructed.Handlers = handlers
	}
	constructed.CollectorCleanup = cleanup.final
	constructed.Observer = factory.config.Observer
	return constructed, nil
}

func (factory *Factory) Prepare(
	ctx context.Context,
	config confgroup.Config,
	identity lifecycle.ResourceIdentity,
	permit lifecycle.LongLivedPermit,
) (lifecycle.PreparedResource, error) {
	if factory == nil || ctx == nil || config == nil ||
		!identity.Valid() ||
		identity.ID != config.FullName() {
		return nil, errors.New("job output: invalid factory preparation")
	}
	return PrepareJob(
		ctx,
		identity.ID,
		identity.Generation,
		permit,
		func(buildCtx context.Context) (ConstructedJob, error) {
			return factory.Build(
				buildCtx,
				config,
				identity.Generation,
			)
		},
	)
}

type factoryJobCleanup struct {
	once sync.Once
	job  RuntimeJob
	err  error
}

func (cleanup *factoryJobCleanup) reject(context.Context) error {
	cleanup.once.Do(func() {
		cleanup.err = callJobLifecycle("rejected collector Cleanup", func() error {
			cleanup.job.CleanupRejected()
			return nil
		})
	})
	return cleanup.err
}

func (cleanup *factoryJobCleanup) final(context.Context) error {
	cleanup.once.Do(func() {
		cleanup.err = callJobLifecycle("collector Cleanup", func() error {
			cleanup.job.Cleanup()
			return nil
		})
	})
	return cleanup.err
}

func callPrepareHandlers(
	hooks JobHooks,
	published PublishedJob,
) (handlers HandlerLifecycle, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			handlers = nil
			err = fmt.Errorf(
				"%w in handler preparation: %v",
				lifecycle.ErrTaskPanic,
				recovered,
			)
		}
	}()
	return hooks.Prepare(published)
}

func disposeHandlers(ctx context.Context, handlers HandlerLifecycle) error {
	if handlers == nil {
		return nil
	}
	return errors.Join(
		callJobLifecycle("prepared handler close/drain", func() error {
			return handlers.CloseAndDrain(ctx)
		}),
		callJobLifecycle("prepared handler Cleanup", func() error {
			return handlers.Cleanup(ctx)
		}),
	)
}

func (factory *Factory) buildV1(
	ctx context.Context,
	config confgroup.Config,
	creator collectorapi.Creator,
	functionOnly bool,
	vnode jobruntime.VnodeSnapshot,
) (job RuntimeJob, err error) {
	if creator.Create == nil {
		return nil, fmt.Errorf("job output: module %q has no V1 creator", config.Module())
	}
	var module collectorapi.CollectorV1
	defer func() {
		if recovered := recover(); recovered != nil {
			job = nil
			err = fmt.Errorf(
				"%w in V1 construction: %v",
				lifecycle.ErrTaskPanic,
				recovered,
			)
		}
		if err != nil && module != nil {
			err = errors.Join(
				err,
				callFactoryModuleCleanup(ctx, module.Cleanup),
			)
		}
	}()
	module = creator.Create()
	if module == nil {
		return nil, fmt.Errorf("job output: module %q returned a nil V1 collector", config.Module())
	}
	if named, ok := module.(interface{ SetJobName(string) }); ok {
		named.SetJobName(config.Name())
	}
	if err := factory.applyConfig(ctx, config, module); err != nil {
		return nil, err
	}
	jobConfig := jobruntime.JobConfig{
		PluginName: factory.config.PluginName, Name: config.Name(),
		ModuleName: config.Module(), FullName: config.FullName(),
		Source: factoryLogSource(config), Module: module,
		Labels: factoryLabels(config), Out: FrameWriter{Owner: factory.config.Frames},
		UpdateEvery: config.UpdateEvery(), AutoDetectEvery: config.AutoDetectionRetry(),
		Priority: config.Priority(), IsStock: config.SourceType() == confgroup.TypeStock,
		FunctionOnly: functionOnly,
	}
	if vnode.Vnode != nil {
		jobConfig.Vnode = *vnode.Vnode.Copy()
		jobConfig.VnodeName = config.Vnode()
		jobConfig.VnodeRevision = vnode.Revision
		jobConfig.VnodeMetadataRevision = vnode.MetadataRevision
		jobConfig.VnodeLookup = factory.config.Vnode
	}
	return jobruntime.NewJob(jobConfig), nil
}

func (factory *Factory) buildV2(
	ctx context.Context,
	config confgroup.Config,
	creator collectorapi.Creator,
	functionOnly bool,
	vnode jobruntime.VnodeSnapshot,
) (job RuntimeJob, err error) {
	var module collectorapi.CollectorV2
	defer func() {
		if recovered := recover(); recovered != nil {
			job = nil
			err = fmt.Errorf(
				"%w in V2 construction: %v",
				lifecycle.ErrTaskPanic,
				recovered,
			)
		}
		if err != nil && module != nil {
			err = errors.Join(
				err,
				callFactoryModuleCleanup(ctx, module.Cleanup),
			)
		}
	}()
	module = creator.CreateV2()
	if module == nil {
		return nil, fmt.Errorf("job output: module %q returned a nil V2 collector", config.Module())
	}
	if named, ok := module.(interface{ SetJobName(string) }); ok {
		named.SetJobName(config.Name())
	}
	if err := factory.applyConfig(ctx, config, module); err != nil {
		return nil, err
	}
	jobConfig := jobruntime.JobV2Config{
		PluginName: factory.config.PluginName, Name: config.Name(),
		ModuleName: config.Module(), FullName: config.FullName(),
		Source: factoryLogSource(config), Module: module,
		Labels: factoryLabels(config), Out: FrameWriter{Owner: factory.config.Frames},
		UpdateEvery: config.UpdateEvery(), AutoDetectEvery: config.AutoDetectionRetry(),
		IsStock: config.SourceType() == confgroup.TypeStock, FunctionOnly: functionOnly,
		RuntimeService: factory.config.Runtime, VnodeRegistry: factory.config.Vnodes,
	}
	if vnode.Vnode != nil {
		jobConfig.Vnode = *vnode.Vnode.Copy()
		jobConfig.VnodeName = config.Vnode()
		jobConfig.VnodeRevision = vnode.Revision
		jobConfig.VnodeMetadataRevision = vnode.MetadataRevision
		jobConfig.VnodeLookup = factory.config.Vnode
	}
	return jobruntime.NewJobV2(jobConfig), nil
}

func callFactoryModuleCleanup(
	ctx context.Context,
	cleanup func(context.Context),
) error {
	return callJobLifecycle("construction collector Cleanup", func() error {
		cleanup(context.WithoutCancel(ctx))
		return nil
	})
}

func (factory *Factory) applyConfig(
	ctx context.Context,
	config confgroup.Config,
	module any,
) error {
	resolveCtx := logger.ContextWithLogger(
		ctx,
		factory.config.Logger.With(
			slog.String("collector", config.Module()),
			slog.String("job", config.Name()),
		),
	)
	resolved, err := factory.config.Resolver.Resolve(
		resolveCtx,
		map[string]any(config),
		factory.config.StoreScope,
	)
	if err != nil {
		return fmt.Errorf("job output: resolving configuration secrets: %w", err)
	}
	payload, err := yaml.Marshal(resolved)
	if err != nil {
		return fmt.Errorf("job output: marshaling configuration: %w", err)
	}
	if len(payload) > secretresolver.MaximumAtomicResolvedBytes {
		return errors.New("job output: serialized configuration exceeds maximum size")
	}
	if err := yaml.Unmarshal(payload, module); err != nil {
		return fmt.Errorf("job output: applying configuration: %w", err)
	}
	return nil
}

func (factory *Factory) lookupVNode(
	config confgroup.Config,
) (jobruntime.VnodeSnapshot, error) {
	if config.Vnode() == "" {
		return jobruntime.VnodeSnapshot{}, nil
	}
	if factory.config.Vnode == nil {
		return jobruntime.VnodeSnapshot{}, fmt.Errorf(
			"job output: vnode %q is unavailable",
			config.Vnode(),
		)
	}
	vnode, ok := factory.config.Vnode(config.Vnode())
	if !ok || vnode.Vnode == nil {
		return jobruntime.VnodeSnapshot{}, fmt.Errorf(
			"job output: vnode %q is not registered",
			config.Vnode(),
		)
	}
	return vnode, nil
}

func validateFactoryConfigIdentity(
	config confgroup.Config,
	creator collectorapi.Creator,
) error {
	if creator.InstancePolicy != collectorapi.InstancePolicySingle ||
		config.Name() == config.Module() {
		return nil
	}
	return fmt.Errorf(
		"job output: single-instance module %q requires config name %q, got %q",
		config.Module(),
		config.Module(),
		config.Name(),
	)
}

func factoryLogSource(config confgroup.Config) string {
	if config.SourceType() != "" && config.SourceType() == config.Provider() {
		return config.SourceType()
	}
	return fmt.Sprintf("%s/%s", config.SourceType(), config.Provider())
}

func factoryLabels(config confgroup.Config) map[string]string {
	labels := make(map[string]string)
	for name, value := range config.Labels() {
		name, nameOK := name.(string)
		value, valueOK := value.(string)
		if nameOK && valueOK {
			labels[name] = value
		}
	}
	return labels
}
