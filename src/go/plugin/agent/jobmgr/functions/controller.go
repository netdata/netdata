// SPDX-License-Identifier: GPL-3.0-or-later

package functions

import (
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"sync"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

type Controller struct {
	mu sync.Mutex // guards all fields

	epoch       uint64 // run generation this controller belongs to
	attempts    jobmgr.ProcessAttemptAuthority
	modules     collectorapi.Registry       // collector module registry
	catalog     *Catalog                    // the route catalog it mutates
	mutations   jobmgr.FunctionMutationPort // kernel Function-mutation port
	publication *Publication                // external FUNCTION/FUNCTION_DEL registration set

	plans              map[string]controllerModulePlan     // per-module route plans
	jobs               map[string]map[string]controllerJob // live jobs by module then job name
	groups             map[string]*controllerGroup         // method-generation groups by signature key
	routes             map[string]controllerRoute          // controller's view of published routes
	initialNames       map[string]struct{}                 // immutable initial-route public names
	initialGenerations []*HandlerGenerationDeclaration     // initial handler generations retained for cleanup
	version            uint64                              // controller version
	nextID             uint64                              // next controller-assigned id
	activated          bool                                // Activate has published the external snapshot
	draining           bool                                // shutdown draining has begun
	terminated         bool                                // controller fully torn down
	dirty              error                               // sticky poison error
}

type controllerJob struct {
	identity lifecycle.ResourceIdentity
	bundle   *functionBundle
	methods  []funcapi.FunctionConfig
}

type controllerModulePlan struct {
	agent       []funcapi.FunctionConfig
	shared      []funcapi.FunctionConfig
	agentBundle *functionBundle
	owner       *modulePlanOwner
}

type controllerGroup struct {
	key        string                     // group key (module + content signature)
	module     string                     // owning module
	signature  string                     // content signature of the grouped methods
	generation *methodGeneration          // the method generation backing the group
	routes     map[string]controllerRoute // routes published for this group
}

type controllerRoute struct {
	module      string
	declaration Declaration
	publication PublicationRecord
}

// InitialRoute installs an immutable private catalog route. External
// publication is reserved for controller-managed collector Functions.
type InitialRoute struct {
	Declaration Declaration
}

type JobHandle struct {
	mu sync.Mutex // guards all fields

	controller *Controller                // owning controller
	identity   lifecycle.ResourceIdentity // job identity (id + generation)
	bundle     *functionBundle            // stable job handler bundle
	methods    []funcapi.FunctionConfig   // staged instance Function declarations
	published  bool                       // job Functions are published
	detached   bool                       // job routes are withdrawn from the run catalog
	closed     bool                       // job publication closed and draining
}

// StagedJobHandle owns collector-created Function state without retaining a
// run controller or resource generation.
type StagedJobHandle struct {
	mu sync.Mutex

	job      collectorapi.RuntimeJob
	bundle   *functionBundle
	methods  []funcapi.FunctionConfig
	consumed bool
}

// JobStager is the run-detached capability used by process-owned collector
// candidates. It contains immutable creator and shared-method snapshots only.
type JobStager struct {
	modules  collectorapi.Registry
	shared   map[string][]funcapi.FunctionConfig
	attempts jobmgr.ProcessAttemptAuthority
	epoch    uint64
}

func (c *Controller) JobStager() (*JobStager, error) {
	if c == nil {
		return nil, errors.New("jobmgr Function controller: nil job stager")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	modules := make(collectorapi.Registry, len(c.modules))
	shared := make(map[string][]funcapi.FunctionConfig, len(c.plans))
	for name, creator := range c.modules {
		modules[name] = creator
		shared[name] = slices.Clone(c.plans[name].shared)
	}
	return &JobStager{
		modules:  modules,
		shared:   shared,
		attempts: c.attempts,
		epoch:    c.epoch,
	}, nil
}

func (js *JobStager) StageJob(
	job collectorapi.RuntimeJob,
) (*StagedJobHandle, error) {
	if js == nil || job == nil || job.FullName() == "" ||
		job.ModuleName() == "" || job.Name() == "" ||
		js.attempts == nil || js.epoch == 0 {
		return nil, errors.New("jobmgr Function controller: invalid job preparation")
	}
	creator, ok := js.modules.Lookup(job.ModuleName())
	if !ok {
		return nil, errors.New("jobmgr Function controller: job module is not registered")
	}
	methods, err := callInstanceFunctions(creator, job)
	if err != nil {
		return nil, err
	}
	methods, err = validateConfiguredMethods(job.ModuleName(), methods)
	if err != nil {
		return nil, err
	}
	shared := slices.Clone(js.shared[job.ModuleName()])
	allMethods := append(shared, methods...)
	bundle, err := newJobFunctionBundle(job.ModuleName(), creator, job, allMethods)
	if err != nil {
		return nil, err
	}
	if err := bundle.bindContainment(
		js.attempts,
		js.epoch,
		jobFunctionAttemptKey(js.epoch, job.FullName()),
		candidateFunctionResource(job.FullName()),
	); err != nil {
		bundle.retire()
		return nil, joinRetainedBundleCleanup(
			err,
			bundle.wait(context.Background()),
		)
	}
	return &StagedJobHandle{
		job:     job,
		bundle:  bundle,
		methods: methods,
	}, nil
}

func (c *Controller) AttachJob(
	identity lifecycle.ResourceIdentity,
	staged *StagedJobHandle,
) (*JobHandle, error) {
	if c == nil || !identity.Valid() || staged == nil {
		return nil, errors.New("jobmgr Function controller: invalid job attachment")
	}
	staged.mu.Lock()
	defer staged.mu.Unlock()
	if staged.consumed ||
		staged.job == nil ||
		staged.bundle == nil ||
		identity.ID != staged.job.FullName() {
		return nil, errors.New("jobmgr Function controller: stale job attachment")
	}
	staged.consumed = true
	handle := &JobHandle{
		controller: c,
		identity:   identity,
		bundle:     staged.bundle,
		methods:    staged.methods,
	}
	staged.job = nil
	staged.bundle = nil
	staged.methods = nil
	return handle, nil
}

func (sjh *StagedJobHandle) CloseAndDrain(ctx context.Context) error {
	if sjh == nil || ctx == nil {
		return errors.New("jobmgr Function controller: invalid staged job close")
	}
	sjh.mu.Lock()
	if sjh.consumed {
		sjh.mu.Unlock()
		return nil
	}
	sjh.consumed = true
	bundle := sjh.bundle
	sjh.job = nil
	sjh.bundle = nil
	sjh.methods = nil
	sjh.mu.Unlock()
	bundle.retire()
	return bundle.wait(ctx)
}

func (jh *JobHandle) Publish() error {
	if jh == nil {
		return errors.New("jobmgr Function controller: nil job handle")
	}
	jh.mu.Lock()
	defer jh.mu.Unlock()
	if jh.published || jh.detached || jh.closed {
		return errors.New("jobmgr Function controller: invalid job-handle publication")
	}
	if err := jh.controller.publishJob(context.Background(), jh.identity, jh.bundle, jh.methods); err != nil {
		return err
	}
	jh.published = true
	return nil
}

func (jh *JobHandle) Detach(ctx context.Context) error {
	if jh == nil || ctx == nil {
		return errors.New("jobmgr Function controller: invalid job-handle detach")
	}
	jh.mu.Lock()
	defer jh.mu.Unlock()
	if jh.closed || jh.detached {
		return nil
	}
	if jh.published {
		if err := jh.controller.closeAndDrainJob(ctx, jh.identity, jh.bundle); err != nil {
			return err
		}
		jh.published = false
	}
	jh.detached = true
	return nil
}

func (jh *JobHandle) Finalize(ctx context.Context) error {
	if jh == nil || ctx == nil {
		return errors.New("jobmgr Function controller: invalid job-handle finalization")
	}
	jh.mu.Lock()
	defer jh.mu.Unlock()
	if jh.closed {
		return nil
	}
	if !jh.detached {
		if jh.published {
			if err := jh.controller.closeAndDrainJob(ctx, jh.identity, jh.bundle); err != nil {
				return err
			}
			jh.published = false
		}
		jh.detached = true
	}
	jh.bundle.retire()
	if err := jh.bundle.wait(ctx); err != nil {
		return err
	}
	jh.closed = true
	return nil
}

func (jh *JobHandle) CloseAndDrain(ctx context.Context) error {
	if err := jh.Detach(ctx); err != nil {
		return err
	}
	return jh.Finalize(ctx)
}

func NewContainedController(
	ctx context.Context,
	epoch uint64,
	attempts jobmgr.ProcessAttemptAuthority,
	modules collectorapi.Registry,
	initial ...InitialRoute,
) (*Controller, *Catalog, error) {
	plans, err := prepareContainedModulePlans(ctx, epoch, attempts, modules)
	if err != nil {
		return nil, nil, err
	}
	controller, catalog, err := newControllerWithPlans(epoch, attempts, modules, plans, initial...)
	if err != nil {
		return nil, nil, err
	}
	return controller, catalog, nil
}

func newControllerWithPlans(
	epoch uint64,
	attempts jobmgr.ProcessAttemptAuthority,
	modules collectorapi.Registry,
	plans map[string]controllerModulePlan,
	initial ...InitialRoute,
) (*Controller, *Catalog, error) {
	if epoch == 0 || attempts == nil || modules == nil || len(plans) != len(modules) {
		return nil, nil, errors.Join(
			errors.New("jobmgr Function controller: invalid staged construction"),
			cleanupControllerModulePlans(plans),
		)
	}
	controller := &Controller{
		epoch:        epoch,
		attempts:     attempts,
		modules:      make(collectorapi.Registry, len(modules)),
		plans:        plans,
		jobs:         make(map[string]map[string]controllerJob),
		groups:       make(map[string]*controllerGroup),
		routes:       make(map[string]controllerRoute),
		initialNames: make(map[string]struct{}, len(initial)),
		version:      1,
	}
	names := make([]string, 0, len(modules))
	for name, creator := range modules {
		if plans[name].agentBundle == nil {
			return nil, nil, errors.Join(
				errors.New("jobmgr Function controller: missing staged module plan"),
				cleanupControllerModulePlans(plans),
			)
		}
		controller.modules[name] = creator
		names = append(names, name)
	}
	cleanupConstruction := func(cause error) error {
		return errors.Join(
			cause,
			controller.cleanupUnpublishedGroups(context.Background(), controller.groups),
			controller.cleanupModuleBundles(context.Background()),
			controller.cleanupInitialRoutes(context.Background(), controller.initialGenerations),
		)
	}
	slices.Sort(names)
	for _, module := range names {
		group, err := controller.buildAgentGroup(module, controller.modules[module])
		if err != nil {
			return nil, nil, cleanupConstruction(err)
		}
		if group != nil {
			controller.groups[group.key] = group
		}
	}
	routes, err := indexControllerRoutes(controller.groups)
	if err != nil {
		return nil, nil, cleanupConstruction(err)
	}
	declarations := make([]Declaration, 0, len(routes)+len(initial))
	initialGenerations := make(map[*HandlerGenerationDeclaration]struct{}, len(initial))
	for _, route := range initial {
		if err := validateDeclaration(route.Declaration); err != nil {
			return nil, nil, cleanupConstruction(err)
		}
		if _, exists := routes[route.Declaration.PublicName]; exists {
			return nil, nil, cleanupConstruction(
				errors.New("jobmgr Function controller: initial route collides with collector Function"),
			)
		}
		controller.initialNames[route.Declaration.PublicName] = struct{}{}
		if _, exists := initialGenerations[route.Declaration.Generation]; !exists {
			initialGenerations[route.Declaration.Generation] = struct{}{}
			controller.initialGenerations = append(controller.initialGenerations, route.Declaration.Generation)
		}
		declarations = append(declarations, route.Declaration)
	}
	for _, name := range sortedControllerRouteNames(routes) {
		declarations = append(declarations, routes[name].declaration)
	}
	catalog, err := NewCatalog(declarations)
	if err != nil {
		return nil, nil, cleanupConstruction(err)
	}
	controller.routes = routes
	controller.catalog = catalog
	return controller, catalog, nil
}

func cleanupControllerModulePlans(plans map[string]controllerModulePlan) (result error) {
	for _, plan := range plans {
		if plan.owner != nil {
			plan.owner.Release()
			continue
		}
		if plan.agentBundle != nil {
			plan.agentBundle.retire()
			result = joinRetainedBundleCleanup(
				result,
				plan.agentBundle.wait(context.Background()),
			)
		}
	}
	return result
}

func (c *Controller) Bind(mutations jobmgr.FunctionMutationPort, publication *Publication) error {
	if c == nil || mutations == nil || publication == nil {
		return errors.New("jobmgr Function controller: invalid binding")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.mutations != nil || c.publication != nil || c.activated || c.draining || c.terminated || c.dirty != nil {
		return errors.New("jobmgr Function controller: duplicate binding")
	}
	c.mutations = mutations
	c.publication = publication
	return nil
}

// AbortConstruction cleans handler generations that never became externally
// visible. A private kernel may already hold the catalog and mutation port, but
// its loop must not have started.
func (c *Controller) AbortConstruction(ctx context.Context) error {
	if c == nil {
		return nil
	}
	if ctx == nil {
		return errors.New("jobmgr Function controller: nil construction abort context")
	}
	c.mu.Lock()
	if c.activated || c.draining || c.terminated {
		c.mu.Unlock()
		return errors.New("jobmgr Function controller: construction abort after activation")
	}
	c.terminated = true
	groups := c.groups
	initial := c.initialGenerations
	c.groups = nil
	c.routes = nil
	c.initialGenerations = nil
	c.mu.Unlock()
	return errors.Join(
		c.cleanupUnpublishedGroups(ctx, groups),
		c.cleanupModuleBundles(ctx),
		c.cleanupInitialRoutes(ctx, initial),
	)
}

func (c *Controller) Activate() error {
	if c == nil {
		return errors.New("jobmgr Function controller: nil activation")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.mutations == nil || c.publication == nil || c.activated || c.draining || c.terminated || c.dirty != nil {
		return errors.New("jobmgr Function controller: invalid activation")
	}
	records := c.publicationRecordsLocked(c.routes)
	changes := make([]PublicationChange, 0, len(records))
	for index := range records {
		record := records[index]
		changes = append(changes, PublicationChange{
			Name:   record.Name,
			Record: &record,
		})
	}
	if err := c.publication.ApplyInitialSnapshot(c.epoch, c.version, changes); err != nil {
		c.dirty = errors.Join(c.dirty, err)
		return err
	}
	c.activated = true
	return nil
}

func (c *Controller) publishJob(
	ctx context.Context,
	identity lifecycle.ResourceIdentity,
	bundle *functionBundle,
	methods []funcapi.FunctionConfig,
) error {
	if ctx == nil || !identity.Valid() || bundle == nil || bundle.job == nil ||
		identity.ID != bundle.job.FullName() ||
		bundle.job.ModuleName() == "" ||
		bundle.job.Name() == "" {
		return errors.New("jobmgr Function controller: invalid job publication")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.usableLocked(); err != nil {
		return err
	}
	job := bundle.job
	creator, ok := c.modules.Lookup(job.ModuleName())
	if !ok {
		return errors.New("jobmgr Function controller: job module is not registered")
	}
	moduleJobs := c.jobs[job.ModuleName()]
	if moduleJobs == nil {
		moduleJobs = make(map[string]controllerJob)
		c.jobs[job.ModuleName()] = moduleJobs
	}
	if _, exists := moduleJobs[job.Name()]; exists {
		return errors.New("jobmgr Function controller: job is already published")
	}
	moduleJobs[job.Name()] = controllerJob{
		identity: identity,
		bundle:   bundle,
		methods:  slices.Clone(methods),
	}
	if _, err := c.reconcileModuleLocked(ctx, job.ModuleName(), creator); err != nil {
		if c.dirty == nil {
			delete(moduleJobs, job.Name())
			if len(moduleJobs) == 0 {
				delete(c.jobs, job.ModuleName())
			}
		}
		return err
	}
	return nil
}

func (c *Controller) closeAndDrainJob(
	ctx context.Context,
	identity lifecycle.ResourceIdentity,
	bundle *functionBundle,
) error {
	if ctx == nil || !identity.Valid() || bundle == nil || bundle.job == nil {
		return errors.New("jobmgr Function controller: invalid job close")
	}
	job := bundle.job
	c.mu.Lock()
	if c.draining {
		moduleJobs := c.jobs[job.ModuleName()]
		current, exists := moduleJobs[job.Name()]
		if !exists {
			c.mu.Unlock()
			return nil
		}
		if current.identity != identity || current.bundle != bundle {
			c.mu.Unlock()
			return errors.New("jobmgr Function controller: stale draining job close")
		}
		delete(moduleJobs, job.Name())
		if len(moduleJobs) == 0 {
			delete(c.jobs, job.ModuleName())
		}
		retired := make([]*methodGeneration, 0, len(c.groups))
		for _, group := range c.groups {
			if generationReferencesJob(group.generation, job) {
				retired = append(retired, group.generation)
			}
		}
		c.mu.Unlock()
		for _, generation := range retired {
			if err := generation.wait(ctx); err != nil {
				return err
			}
		}
		return nil
	}
	if err := c.usableLocked(); err != nil {
		c.mu.Unlock()
		return err
	}
	moduleJobs := c.jobs[job.ModuleName()]
	current, exists := moduleJobs[job.Name()]
	if !exists {
		c.mu.Unlock()
		return nil
	}
	if current.identity != identity || current.bundle != bundle {
		c.mu.Unlock()
		return errors.New("jobmgr Function controller: stale job close")
	}
	creator := c.modules[job.ModuleName()]
	delete(moduleJobs, job.Name())
	if len(moduleJobs) == 0 {
		delete(c.jobs, job.ModuleName())
	}
	retired, err := c.reconcileModuleLocked(ctx, job.ModuleName(), creator)
	if err != nil {
		if c.dirty == nil {
			moduleJobs = c.jobs[job.ModuleName()]
			if moduleJobs == nil {
				moduleJobs = make(map[string]controllerJob)
				c.jobs[job.ModuleName()] = moduleJobs
			}
			moduleJobs[job.Name()] = current
		}
		c.mu.Unlock()
		return err
	}
	c.mu.Unlock()
	for _, generation := range retired {
		if generationReferencesJob(generation, job) {
			if err := generation.wait(ctx); err != nil {
				return err
			}
		}
	}
	return nil
}

func (c *Controller) ReconcileModule(ctx context.Context, module string) error {
	if c == nil || ctx == nil || module == "" {
		return errors.New("jobmgr Function controller: invalid module reconcile")
	}
	c.mu.Lock()
	if err := c.usableLocked(); err != nil {
		c.mu.Unlock()
		return err
	}
	creator, ok := c.modules.Lookup(module)
	if !ok {
		c.mu.Unlock()
		return errors.New("jobmgr Function controller: module is not registered")
	}
	pollable := false
	if bundle := c.plans[module].agentBundle; bundle != nil {
		pollable = pollable || bundle.pollable
	}
	for _, job := range c.jobs[module] {
		pollable = pollable || job.bundle.pollable
	}
	if !pollable {
		c.mu.Unlock()
		return nil
	}
	bundles := make([]*functionBundle, 0, len(c.jobs[module])+1)
	if bundle := c.plans[module].agentBundle; bundle != nil && bundle.pollable {
		bundles = append(bundles, bundle)
	}
	for _, job := range c.jobs[module] {
		if job.bundle.pollable {
			bundles = append(bundles, job.bundle)
		}
	}
	c.mu.Unlock()

	for _, bundle := range bundles {
		poll, err := bundle.startAvailabilityPoll()
		if errors.Is(err, jobmgr.ErrProcessAttemptBusy) ||
			errors.Is(err, jobmgr.ErrProcessAttemptStopped) {
			continue
		}
		if err != nil {
			return err
		}
		if poll.attempt == nil {
			continue
		}
		go c.finishAvailabilityPoll(module, creator, poll)
	}
	return nil
}

func (c *Controller) finishAvailabilityPoll(
	module string,
	creator collectorapi.Creator,
	poll functionAvailabilityPoll,
) {
	if err := poll.attempt.Await(context.Background()); err != nil {
		return
	}
	result := <-poll.workerResult
	if result.err != nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.usableLocked() != nil {
		return
	}
	if !poll.bundle.commitAvailability(result.availability) {
		return
	}
	_, _ = c.reconcileModuleLocked(context.Background(), module, creator)
}

func (c *Controller) Stop(epoch uint64) error {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	if epoch != c.epoch || !c.draining || c.terminated {
		c.mu.Unlock()
		return errors.New("jobmgr Function controller: invalid stop")
	}
	c.terminated = true
	dirty := c.dirty
	c.mu.Unlock()
	return errors.Join(dirty, c.cleanupModuleBundles(context.Background()))
}

// BeginShutdown withdraws external routes before CommandKernel closes the
// catalog. Job handles may still close afterward; in that mode they wait for
// catalog-owned generation cleanup without submitting another mutation.
func (c *Controller) BeginShutdown(epoch uint64) error {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if epoch != c.epoch {
		return errors.New("jobmgr Function controller: invalid shutdown generation")
	}
	if c.terminated {
		return errors.New("jobmgr Function controller: shutdown after stop")
	}
	if c.draining {
		return c.dirty
	}
	c.draining = true
	if c.publication == nil {
		c.dirty = errors.Join(c.dirty, errors.New("jobmgr Function controller: shutdown before binding"))
		return c.dirty
	}
	c.dirty = errors.Join(c.dirty, c.publication.Stop(epoch))
	return c.dirty
}

func (c *Controller) usableLocked() error {
	if c.dirty != nil {
		return c.dirty
	}
	if !c.activated || c.draining || c.terminated || c.mutations == nil || c.publication == nil {
		return errors.New("jobmgr Function controller: not active")
	}
	return nil
}

func (c *Controller) reconcileModuleLocked(
	ctx context.Context,
	module string,
	creator collectorapi.Creator,
) ([]*methodGeneration, error) {
	unpublished := make(map[string]*controllerGroup)
	cleanupUnpublished := true
	defer func() {
		if cleanupUnpublished {
			_ = c.cleanupUnpublishedGroups(context.WithoutCancel(ctx), unpublished)
		}
	}()
	desired, err := c.buildModuleGroups(module, creator, unpublished)
	if err != nil {
		cleanupUnpublished = false
		return nil, errors.Join(err, c.cleanupUnpublishedGroups(context.WithoutCancel(ctx), unpublished))
	}

	nextGroups := make(map[string]*controllerGroup, len(c.groups)+len(desired))
	for key, group := range c.groups {
		if group.module != module {
			nextGroups[key] = group
		}
	}
	maps.Copy(nextGroups, desired)
	nextRoutes, err := indexControllerRoutes(nextGroups)
	if err != nil {
		return nil, err
	}
	for name := range c.initialNames {
		if _, exists := nextRoutes[name]; exists {
			return nil, errors.New("jobmgr Function controller: collector Function collides with initial route")
		}
	}
	routeChanges := controllerRouteChanges(c.routes, nextRoutes)
	if len(routeChanges) == 0 {
		c.groups = nextGroups
		cleanupUnpublished = false
		_ = c.cleanupUnpublishedGroups(context.WithoutCancel(ctx), unpublished)
		return nil, nil
	}
	mutation, err := c.catalog.NewMutation(c.version, routeChanges)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = mutation.Discard()
	}()
	retired := retiredMethodGenerations(c.groups, nextGroups)
	publicationChanges := controllerPublicationChanges(c.routes, nextRoutes)
	expectedVersion := c.version + 1
	transitionCtx := context.WithoutCancel(ctx)
	err = c.publication.ApplyTransition(
		c.epoch,
		expectedVersion,
		publicationChanges,
		func() error {
			return c.mutations.QuiesceFunctions(ctx, mutation)
		},
		func() error {
			version, mutationErr := c.mutations.CommitFunctions(transitionCtx, mutation)
			if mutationErr != nil {
				return mutationErr
			}
			// The catalog commit transfers ownership of every new generation
			// and retires the predecessors. No later publication failure may
			// roll that transition back or clean catalog-owned handlers.
			c.version = version
			c.groups = nextGroups
			c.routes = nextRoutes
			cleanupUnpublished = false
			if version != expectedVersion {
				return errors.New("jobmgr Function controller: mutation version mismatch")
			}
			return nil
		},
		func() error {
			return c.mutations.AbortFunctions(transitionCtx, mutation)
		},
	)
	err = errors.Join(err, mutation.Discard())
	if err != nil {
		c.dirty = errors.Join(c.dirty, err)
		return nil, err
	}
	return retired, nil
}

func (c *Controller) buildModuleGroups(
	module string,
	creator collectorapi.Creator,
	unpublished map[string]*controllerGroup,
) (map[string]*controllerGroup, error) {
	desired := make(map[string]*controllerGroup)
	add := func(group *controllerGroup, err error) error {
		if err != nil {
			return err
		}
		if group != nil {
			desired[group.key] = group
			if c.groups[group.key] != group {
				unpublished[group.key] = group
			}
		}
		return nil
	}
	if err := add(c.buildAgentGroup(module, creator)); err != nil {
		return nil, err
	}
	if err := add(c.buildSharedGroup(module, creator)); err != nil {
		return nil, err
	}
	jobs := c.jobs[module]
	names := slices.Sorted(maps.Keys(jobs))
	for _, name := range names {
		if err := add(c.buildInstanceGroup(module, creator, jobs[name])); err != nil {
			return nil, err
		}
	}
	return desired, nil
}

func (c *Controller) buildAgentGroup(module string, creator collectorapi.Creator) (*controllerGroup, error) {
	if creator.AgentFunctions == nil {
		return nil, nil
	}
	methods := c.availableAgentMethods(module)
	return c.buildGroup(
		module+"/agent",
		module,
		methodGenerationAgent,
		creator,
		methods,
		map[string]*functionBundle{"": c.plans[module].agentBundle},
		true,
	)
}

func (c *Controller) buildSharedGroup(module string, creator collectorapi.Creator) (*controllerGroup, error) {
	if creator.SharedFunctions == nil || len(c.jobs[module]) == 0 {
		return nil, nil
	}
	bundles := make(map[string]*functionBundle, len(c.jobs[module]))
	for name, job := range c.jobs[module] {
		bundles[name] = job.bundle
	}
	return c.buildGroup(
		module+"/shared",
		module,
		methodGenerationShared,
		creator,
		c.plans[module].shared,
		bundles,
		true,
	)
}

func (c *Controller) buildInstanceGroup(
	module string,
	creator collectorapi.Creator,
	job controllerJob,
) (*controllerGroup, error) {
	if creator.InstanceFunctions == nil {
		return nil, nil
	}
	return c.buildGroup(
		module+"/instance/"+job.bundle.job.Name(),
		module,
		methodGenerationInstance,
		creator,
		job.methods,
		map[string]*functionBundle{job.bundle.job.Name(): job.bundle},
		false,
	)
}

func (c *Controller) buildGroup(
	key string,
	module string,
	kind methodGenerationKind,
	creator collectorapi.Creator,
	methods []funcapi.FunctionConfig,
	bundles map[string]*functionBundle,
	aliases bool,
) (*controllerGroup, error) {
	var err error
	methods, err = validateConfiguredMethods(module, methods)
	if err != nil {
		return nil, err
	}
	if len(methods) == 0 {
		return nil, nil
	}
	signature, err := controllerGroupSignature(kind, methods, bundles)
	if err != nil {
		return nil, err
	}
	if current := c.groups[key]; current != nil && current.signature == signature {
		return current, nil
	}
	c.nextID++
	if c.nextID == 0 {
		return nil, errors.New("jobmgr Function controller: generation wrapped")
	}
	id := module + "/" + strconv.FormatUint(c.nextID, 10)
	generation, err := newMethodGeneration(id, module, kind, creator, methods, bundles)
	if err != nil {
		return nil, err
	}
	declaration := generation.declaration()
	group := &controllerGroup{
		key:        key,
		module:     module,
		signature:  signature,
		generation: generation,
		routes:     make(map[string]controllerRoute),
	}
	for _, method := range methods {
		names := []string{funcapi.FunctionName(module, method)}
		if aliases {
			names = funcapi.FunctionNames(module, method)
		}
		for _, name := range names {
			if !validFunctionName(name) {
				_ = generation.cleanup(context.Background())
				return nil, errors.New("jobmgr Function controller: invalid public name")
			}
			if _, exists := group.routes[name]; exists {
				_ = generation.cleanup(context.Background())
				return nil, errors.New("jobmgr Function controller: duplicate public name")
			}
			group.routes[name] = controllerRoute{
				module: module,
				declaration: Declaration{
					ID:                  method.ID,
					Generation:          declaration,
					PublicName:          name,
					RawPayload:          method.RawRequest,
					CooperativeCancel:   true,
					CooperativeDeadline: true,
				},
				publication: methodPublicationRecord(name, module, method, c.nextID),
			}
		}
	}
	return group, nil
}

func (c *Controller) availableAgentMethods(module string) []funcapi.FunctionConfig {
	current := c.groups[module+"/agent"]
	methods := make([]funcapi.FunctionConfig, 0, len(c.plans[module].agent))
	for _, method := range c.plans[module].agent {
		published := false
		if current != nil && current.generation != nil {
			_, published = current.generation.methods[method.ID]
		}
		if published ||
			method.Available == nil ||
			c.plans[module].agentBundle.available(method.ID) {
			methods = append(methods, method)
		}
	}
	return methods
}

func validateConfiguredMethods(module string, methods []funcapi.FunctionConfig) ([]funcapi.FunctionConfig, error) {
	valid := make([]funcapi.FunctionConfig, 0, len(methods))
	seen := make(map[string]struct{}, len(methods))
	for _, method := range methods {
		if method.ID == "" {
			return nil, errors.New("jobmgr Function controller: empty method ID")
		}
		if _, exists := seen[method.ID]; exists {
			return nil, errors.New("jobmgr Function controller: duplicate method ID")
		}
		seen[method.ID] = struct{}{}
		if !validQuotedProtocolField(method.Help) {
			return nil, errors.New("jobmgr Function controller: invalid Function help")
		}
		if !validQuotedProtocolField(method.Tags) {
			return nil, errors.New("jobmgr Function controller: invalid Function tags")
		}
		publicNames := make(map[string]struct{}, len(method.Aliases)+1)
		primary := funcapi.FunctionName(module, method)
		publicNames[primary] = struct{}{}
		for _, alias := range method.Aliases {
			if alias == "" {
				return nil, errors.New("jobmgr Function controller: empty Function alias")
			}
			if _, exists := publicNames[alias]; exists {
				return nil, errors.New("jobmgr Function controller: duplicate Function alias")
			}
			publicNames[alias] = struct{}{}
		}
		valid = append(valid, method)
	}
	slices.SortFunc(valid, func(a, b funcapi.FunctionConfig) int {
		return cmp.Compare(a.ID, b.ID)
	})
	return valid, nil
}

func controllerGroupSignature(
	kind methodGenerationKind,
	methods []funcapi.FunctionConfig,
	bundles map[string]*functionBundle,
) (string, error) {
	digest := sha256.New()
	_ = binary.Write(digest, binary.BigEndian, uint8(kind))
	for _, method := range methods {
		writeDigestString(digest, method.ID)
		writeDigestString(digest, method.FunctionName)
		writeDigestString(digest, method.Name)
		writeDigestUint64(digest, uint64(method.UpdateEvery))
		writeDigestString(digest, method.Help)
		writeDigestBool(digest, method.RequireCloud)
		writeDigestString(digest, method.Tags)
		writeDigestString(digest, method.ResponseType)
		writeDigestBool(digest, method.RawRequest)
		for _, alias := range method.Aliases {
			writeDigestString(digest, alias)
		}
		for _, parameter := range method.RequiredParams {
			writeDigestString(digest, parameter.ID)
			writeDigestString(digest, parameter.Name)
			writeDigestString(digest, parameter.Help)
			writeDigestUint64(digest, uint64(parameter.Selection))
			writeDigestBool(digest, parameter.UniqueView)
			for _, option := range parameter.Options {
				writeDigestString(digest, option.ID)
				writeDigestString(digest, option.Name)
				writeDigestBool(digest, option.Default)
				writeDigestBool(digest, option.Disabled)
				writeDigestString(digest, option.Column)
				if option.Sort != nil {
					writeDigestString(digest, option.Sort.String())
				} else {
					writeDigestString(digest, "")
				}
			}
		}
		presentation, err := json.Marshal(method.Presentation())
		if err != nil {
			return "", fmt.Errorf("jobmgr Function controller: method %q presentation: %w", method.ID, err)
		}
		writeDigestString(digest, string(presentation))
	}
	names := slices.Sorted(maps.Keys(bundles))
	for _, name := range names {
		bundle := bundles[name]
		writeDigestString(digest, name)
		if bundle.job != nil {
			writeDigestString(digest, bundle.job.FullName())
		} else {
			writeDigestString(digest, "")
		}
		if kind != methodGenerationAgent {
			for _, method := range methods {
				writeDigestBool(digest, bundle.available(method.ID))
			}
		}
	}
	return fmt.Sprintf("%x", digest.Sum(nil)), nil
}

func methodPublicationRecord(
	name string,
	module string,
	method funcapi.FunctionConfig,
	generation uint64,
) PublicationRecord {
	help := method.Help
	if help == "" {
		help = fmt.Sprintf("%s %s data function", module, method.ID)
	}
	access := "0x0000"
	if method.RequireCloud {
		access = "0x0013"
	}
	tags := method.Tags
	if tags == "" {
		tags = "top"
	}
	return PublicationRecord{
		Name:       name,
		Generation: generation,
		Timeout:    60,
		Help:       help,
		Tags:       tags,
		Access:     access,
		Priority:   100,
		Version:    3,
	}
}

func indexControllerRoutes(groups map[string]*controllerGroup) (map[string]controllerRoute, error) {
	routes := make(map[string]controllerRoute)
	keys := slices.Sorted(maps.Keys(groups))
	for _, key := range keys {
		group := groups[key]
		for name, route := range group.routes {
			if current, exists := routes[name]; exists {
				return nil, fmt.Errorf(
					"jobmgr Function controller: public name %q collides between %s and %s",
					name,
					current.module,
					route.module,
				)
			}
			routes[name] = route
		}
	}
	return routes, nil
}

// controllerChanges walks the changed route names between current and next,
// emitting removed(name) for a route that disappeared and present(name, route)
// otherwise. It backs the route and publication change builders below.
func controllerChanges[T any](
	current map[string]controllerRoute,
	next map[string]controllerRoute,
	removed func(name string) T,
	present func(name string, route controllerRoute) T,
) []T {
	names := controllerChangedRouteNames(current, next)
	changes := make([]T, 0, len(names))
	for _, name := range names {
		route, exists := next[name]
		if !exists {
			changes = append(changes, removed(name))
			continue
		}
		changes = append(changes, present(name, route))
	}
	return changes
}

func controllerRouteChanges(current map[string]controllerRoute, next map[string]controllerRoute) []RouteChange {
	return controllerChanges(current, next,
		func(name string) RouteChange {
			return RouteChange{
				PublicName: name,
			}
		},
		func(name string, route controllerRoute) RouteChange {
			declaration := route.declaration
			return RouteChange{
				PublicName:  name,
				Declaration: &declaration,
			}
		},
	)
}

func controllerPublicationChanges(
	current map[string]controllerRoute,
	next map[string]controllerRoute,
) []PublicationChange {
	return controllerChanges(current, next,
		func(name string) PublicationChange {
			return PublicationChange{
				Name: name,
			}
		},
		func(name string, route controllerRoute) PublicationChange {
			record := route.publication
			return PublicationChange{
				Name:   name,
				Record: &record,
			}
		},
	)
}

func controllerChangedRouteNames(current map[string]controllerRoute, next map[string]controllerRoute) []string {
	changed := make(map[string]struct{})
	for name, currentRoute := range current {
		nextRoute, exists := next[name]
		if !exists ||
			currentRoute.publication != nextRoute.publication ||
			currentRoute.declaration.Generation != nextRoute.declaration.Generation {
			changed[name] = struct{}{}
		}
	}
	for name := range next {
		if _, exists := current[name]; !exists {
			changed[name] = struct{}{}
		}
	}
	return slices.Sorted(maps.Keys(changed))
}

func sortedControllerRouteNames(routes map[string]controllerRoute) []string {
	return slices.Sorted(maps.Keys(routes))
}

func (c *Controller) publicationRecordsLocked(routes map[string]controllerRoute) []PublicationRecord {
	names := sortedControllerRouteNames(routes)
	records := make([]PublicationRecord, 0, len(names))
	for _, name := range names {
		records = append(records, routes[name].publication)
	}
	return records
}

func (c *Controller) cleanupInitialRoutes(
	ctx context.Context,
	generations []*HandlerGenerationDeclaration,
) (err error) {
	for _, generation := range generations {
		if generation == nil || generation.Cleanup == nil {
			continue
		}
		func() {
			defer func() {
				if recovered := recover(); recovered != nil {
					err = errors.Join(
						err,
						fmt.Errorf("jobmgr Function controller: initial route cleanup panic: %v", recovered),
					)
				}
			}()
			err = errors.Join(err, generation.Cleanup(ctx))
		}()
	}
	return err
}

func (c *Controller) cleanupModuleBundles(ctx context.Context) (result error) {
	if c == nil {
		return nil
	}
	seen := make(map[*functionBundle]struct{})
	var bundles []*functionBundle
	c.mu.Lock()
	for _, plan := range c.plans {
		if plan.agentBundle != nil {
			if _, ok := seen[plan.agentBundle]; !ok {
				seen[plan.agentBundle] = struct{}{}
				bundles = append(bundles, plan.agentBundle)
			}
		}
	}
	for _, jobs := range c.jobs {
		for _, job := range jobs {
			if job.bundle != nil {
				if _, ok := seen[job.bundle]; !ok {
					seen[job.bundle] = struct{}{}
					bundles = append(bundles, job.bundle)
				}
			}
		}
	}
	c.mu.Unlock()
	for _, bundle := range bundles {
		var owner *modulePlanOwner
		for _, plan := range c.plans {
			if plan.agentBundle == bundle {
				owner = plan.owner
				break
			}
		}
		if owner != nil {
			owner.Release()
			continue
		}
		bundle.retire()
		result = errors.Join(result, bundle.wait(ctx))
	}
	return result
}

func retiredMethodGenerations(
	current map[string]*controllerGroup,
	next map[string]*controllerGroup,
) []*methodGeneration {
	seen := make(map[*methodGeneration]struct{})
	var retired []*methodGeneration
	for key, group := range current {
		if next[key] == group {
			continue
		}
		if _, exists := seen[group.generation]; exists {
			continue
		}
		seen[group.generation] = struct{}{}
		retired = append(retired, group.generation)
	}
	return retired
}

func generationReferencesJob(generation *methodGeneration, job collectorapi.RuntimeJob) bool {
	if generation == nil || job == nil {
		return false
	}
	for _, bundle := range generation.bundles {
		if bundle.job == job {
			return true
		}
	}
	return false
}

func (c *Controller) cleanupUnpublishedGroups(ctx context.Context, groups map[string]*controllerGroup) (err error) {
	seen := make(map[*methodGeneration]struct{})
	for _, group := range groups {
		if group == nil || group.generation == nil {
			continue
		}
		if _, exists := seen[group.generation]; exists {
			continue
		}
		seen[group.generation] = struct{}{}
		err = errors.Join(err, group.generation.cleanup(ctx))
	}
	return err
}
