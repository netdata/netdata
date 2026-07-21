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

	epoch       uint64                      // run generation this controller belongs to
	modules     collectorapi.Registry       // collector module registry
	catalog     *Catalog                    // the route catalog it mutates
	mutations   jobmgr.FunctionMutationPort // kernel Function-mutation port
	publication *Publication                // external FUNCTION/FUNCTION_DEL registration set

	plans            map[string]controllerModulePlan         // per-module route plans
	jobs             map[string]map[string]controllerJob     // live jobs by module then job name
	groups           map[string]*controllerGroup             // method-generation groups by signature key
	routes           map[string]controllerRoute              // controller's view of published routes
	fixed            map[string]PublicationRecord            // initial/fixed publication records
	fixedGenerations []*HandlerGenerationDeclaration         // initial/fixed handler-generation declarations (for cleanup)
	availability     map[string]controllerModuleAvailability // per-module availability state
	version          uint64                                  // controller version
	nextID           uint64                                  // next controller-assigned id
	activated        bool                                    // Activate has run (initial snapshot published)
	draining         bool                                    // shutdown draining has begun
	terminated       bool                                    // controller fully torn down
	dirty            error                                   // sticky poison error
}

type controllerJob struct {
	identity lifecycle.ResourceIdentity
	job      collectorapi.RuntimeJob
	methods  []funcapi.FunctionConfig
}

type controllerModulePlan struct {
	agent  []funcapi.FunctionConfig
	shared []funcapi.FunctionConfig
}

type controllerModuleAvailability struct {
	probes []controllerAvailabilityProbe
}

type controllerAvailabilityProbe struct {
	methodID string                  // method whose availability this probe tracks
	job      collectorapi.RuntimeJob // backing job for a job-scoped method; nil for an agent-level method
	agent    func() bool             // agent-level availability predicate (job nil); nil means always available
	observed bool                    // availability captured at build time, compared against the live value to detect change
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

type InitialRoute struct {
	Declaration Declaration
	Publication PublicationRecord
}

type JobHandle struct {
	mu sync.Mutex // guards all fields

	controller *Controller                // owning controller
	identity   lifecycle.ResourceIdentity // job identity (id + generation)
	job        collectorapi.RuntimeJob    // the runtime job whose Functions are published
	published  bool                       // job Functions are published
	closed     bool                       // job publication closed and draining
	cleaned    bool                       // job cleanup complete
}

func (c *Controller) PrepareJob(
	identity lifecycle.ResourceIdentity,
	job collectorapi.RuntimeJob,
) (*JobHandle, error) {
	if c == nil || !identity.Valid() || job == nil ||
		identity.ID != job.FullName() || job.ModuleName() == "" || job.Name() == "" {
		return nil, errors.New("jobmgr Function controller: invalid job preparation")
	}
	if _, ok := c.modules.Lookup(job.ModuleName()); !ok {
		return nil, errors.New("jobmgr Function controller: job module is not registered")
	}
	return &JobHandle{controller: c, identity: identity, job: job}, nil
}

func (jh *JobHandle) Publish() error {
	if jh == nil {
		return errors.New("jobmgr Function controller: nil job handle")
	}
	jh.mu.Lock()
	defer jh.mu.Unlock()
	if jh.published || jh.closed || jh.cleaned {
		return errors.New("jobmgr Function controller: invalid job-handle publication")
	}
	if err := jh.controller.PublishJob(
		context.Background(),
		jh.identity,
		jh.job,
	); err != nil {
		return err
	}
	jh.published = true
	return nil
}

func (jh *JobHandle) CloseAndDrain(ctx context.Context) error {
	if jh == nil || ctx == nil {
		return errors.New("jobmgr Function controller: invalid job-handle close")
	}
	jh.mu.Lock()
	defer jh.mu.Unlock()
	if jh.closed {
		return nil
	}
	if jh.cleaned {
		return errors.New("jobmgr Function controller: close after cleanup")
	}
	if jh.published {
		if err := jh.controller.CloseAndDrainJob(ctx, jh.identity, jh.job); err != nil {
			return err
		}
	}
	jh.closed = true
	return nil
}

func (jh *JobHandle) Cleanup(ctx context.Context) error {
	if jh == nil || ctx == nil {
		return errors.New("jobmgr Function controller: invalid job-handle cleanup")
	}
	jh.mu.Lock()
	defer jh.mu.Unlock()
	if jh.cleaned {
		return nil
	}
	if !jh.closed {
		return errors.New("jobmgr Function controller: cleanup before close")
	}
	// Catalog-owned cleanup plans execute the exact MethodHandler cleanup before
	// CloseAndDrain returns. This acknowledgement makes the per-job lifecycle
	// explicit without re-resolving the job through a second owner.
	jh.cleaned = true
	return nil
}

func NewController(
	epoch uint64,
	modules collectorapi.Registry,
	initial ...InitialRoute,
) (*Controller, *Catalog, error) {
	if epoch == 0 || modules == nil {
		return nil, nil, errors.New("jobmgr Function controller: invalid construction")
	}
	controller := &Controller{
		epoch:        epoch,
		modules:      make(collectorapi.Registry, len(modules)),
		plans:        make(map[string]controllerModulePlan, len(modules)),
		jobs:         make(map[string]map[string]controllerJob),
		groups:       make(map[string]*controllerGroup),
		routes:       make(map[string]controllerRoute),
		fixed:        make(map[string]PublicationRecord),
		availability: make(map[string]controllerModuleAvailability, len(modules)),
		version:      1,
	}
	names := make([]string, 0, len(modules))
	for name, creator := range modules {
		controller.modules[name] = creator
		names = append(names, name)
	}
	cleanupConstruction := func(cause error) error {
		return errors.Join(
			cause,
			controller.cleanupUnpublishedGroups(
				context.Background(),
				controller.groups,
			),
			controller.cleanupInitialRoutes(
				context.Background(),
				controller.fixedGenerations,
			),
		)
	}
	slices.Sort(names)
	for _, module := range names {
		creator := controller.modules[module]
		var plan controllerModulePlan
		if creator.AgentFunctions != nil {
			plan.agent = slices.Clone(creator.AgentFunctions())
		}
		if creator.SharedFunctions != nil {
			plan.shared = slices.Clone(creator.SharedFunctions())
		}
		controller.plans[module] = plan
		group, err := controller.buildAgentGroup(module, controller.modules[module])
		if err != nil {
			return nil, nil, cleanupConstruction(err)
		}
		if group != nil {
			controller.groups[group.key] = group
		}
		controller.refreshModuleAvailabilityLocked(module)
	}
	routes, err := indexControllerRoutes(controller.groups)
	if err != nil {
		return nil, nil, cleanupConstruction(err)
	}
	declarations := make(
		[]Declaration,
		0,
		len(routes)+len(initial),
	)
	fixedGenerations := make(
		map[*HandlerGenerationDeclaration]struct{},
		len(initial),
	)
	for _, route := range initial {
		if err := validateInitialRoute(route); err != nil {
			return nil, nil, cleanupConstruction(err)
		}
		if _, exists := routes[route.Declaration.PublicName]; exists {
			return nil, nil, cleanupConstruction(
				errors.New(
					"jobmgr Function controller: initial route collides with collector Function",
				),
			)
		}
		if current, exists := controller.fixed[route.Publication.Name]; exists &&
			current != route.Publication {
			return nil, nil, cleanupConstruction(
				errors.New(
					"jobmgr Function controller: initial routes disagree on publication",
				),
			)
		}
		controller.fixed[route.Publication.Name] = route.Publication
		if _, exists := fixedGenerations[route.Declaration.Generation]; !exists {
			fixedGenerations[route.Declaration.Generation] = struct{}{}
			controller.fixedGenerations = append(
				controller.fixedGenerations,
				route.Declaration.Generation,
			)
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

func (c *Controller) Bind(
	mutations jobmgr.FunctionMutationPort,
	publication *Publication,
) error {
	if c == nil || mutations == nil || publication == nil {
		return errors.New("jobmgr Function controller: invalid binding")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.mutations != nil || c.publication != nil ||
		c.activated || c.draining || c.terminated || c.dirty != nil {
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
	if c.activated ||
		c.draining ||
		c.terminated {
		c.mu.Unlock()
		return errors.New("jobmgr Function controller: construction abort after activation")
	}
	c.terminated = true
	groups := c.groups
	fixed := c.fixedGenerations
	c.groups = nil
	c.routes = nil
	c.fixedGenerations = nil
	c.mu.Unlock()
	return errors.Join(
		c.cleanupUnpublishedGroups(ctx, groups),
		c.cleanupInitialRoutes(ctx, fixed),
	)
}

func (c *Controller) Activate() error {
	if c == nil {
		return errors.New("jobmgr Function controller: nil activation")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.mutations == nil || c.publication == nil ||
		c.activated || c.draining || c.terminated || c.dirty != nil {
		return errors.New("jobmgr Function controller: invalid activation")
	}
	records := c.publicationRecordsLocked(c.routes)
	changes := make([]PublicationChange, 0, len(records))
	for index := range records {
		record := records[index]
		changes = append(changes, PublicationChange{Name: record.Name, Record: &record})
	}
	if err := c.publication.ApplyInitialSnapshot(
		c.epoch,
		c.version,
		c.catalog.storage.published.Load(),
		changes,
	); err != nil {
		c.dirty = errors.Join(c.dirty, err)
		return err
	}
	c.activated = true
	return nil
}

func (c *Controller) PublishJob(
	ctx context.Context,
	identity lifecycle.ResourceIdentity,
	job collectorapi.RuntimeJob,
) error {
	if ctx == nil || !identity.Valid() || job == nil ||
		identity.ID != job.FullName() || job.ModuleName() == "" || job.Name() == "" {
		return errors.New("jobmgr Function controller: invalid job publication")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.usableLocked(); err != nil {
		return err
	}
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
	var methods []funcapi.FunctionConfig
	if creator.InstanceFunctions != nil {
		methods = slices.Clone(creator.InstanceFunctions(job))
	}
	moduleJobs[job.Name()] = controllerJob{
		identity: identity, job: job, methods: methods,
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

func (c *Controller) CloseAndDrainJob(
	ctx context.Context,
	identity lifecycle.ResourceIdentity,
	job collectorapi.RuntimeJob,
) error {
	if ctx == nil || !identity.Valid() || job == nil {
		return errors.New("jobmgr Function controller: invalid job close")
	}
	c.mu.Lock()
	if c.draining {
		moduleJobs := c.jobs[job.ModuleName()]
		current, exists := moduleJobs[job.Name()]
		if !exists {
			c.mu.Unlock()
			return nil
		}
		if current.identity != identity || current.job != job {
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
	if current.identity != identity || current.job != job {
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

func (c *Controller) ReconcileModule(
	ctx context.Context,
	module string,
) error {
	if c == nil || ctx == nil || module == "" {
		return errors.New("jobmgr Function controller: invalid module reconcile")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.usableLocked(); err != nil {
		return err
	}
	creator, ok := c.modules.Lookup(module)
	if !ok {
		return errors.New("jobmgr Function controller: module is not registered")
	}
	if !c.moduleAvailabilityChangedLocked(module) {
		return nil
	}
	_, err := c.reconcileModuleLocked(ctx, module, creator)
	return err
}

func (c *Controller) Stop(epoch uint64) error {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if epoch != c.epoch || !c.draining || c.terminated {
		return errors.New("jobmgr Function controller: invalid stop")
	}
	c.terminated = true
	return c.dirty
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
		c.dirty = errors.Join(
			c.dirty,
			errors.New("jobmgr Function controller: shutdown before binding"),
		)
		return c.dirty
	}
	c.dirty = errors.Join(
		c.dirty,
		c.publication.Stop(epoch),
	)
	return c.dirty
}

func (c *Controller) usableLocked() error {
	if c.dirty != nil {
		return c.dirty
	}
	if !c.activated || c.draining || c.terminated ||
		c.mutations == nil || c.publication == nil {
		return errors.New("jobmgr Function controller: not active")
	}
	return nil
}

func (c *Controller) reconcileModuleLocked(
	ctx context.Context,
	module string,
	creator collectorapi.Creator,
) ([]*methodGeneration, error) {
	desired, unpublished, err := c.buildModuleGroups(module, creator)
	if err != nil {
		return nil, err
	}
	cleanupUnpublished := true
	defer func() {
		if cleanupUnpublished {
			_ = c.cleanupUnpublishedGroups(
				context.WithoutCancel(ctx),
				unpublished,
			)
		}
	}()

	nextGroups := make(map[string]*controllerGroup, len(c.groups)+len(desired))
	for key, group := range c.groups {
		if group.module != module {
			nextGroups[key] = group
		}
	}
	for key, group := range desired {
		if current := c.groups[key]; current != nil &&
			current.signature == group.signature {
			nextGroups[key] = current
			delete(unpublished, key)
		} else {
			nextGroups[key] = group
		}
	}
	nextRoutes, err := indexControllerRoutes(nextGroups)
	if err != nil {
		return nil, err
	}
	for name := range c.fixed {
		if _, exists := nextRoutes[name]; exists {
			return nil, errors.New(
				"jobmgr Function controller: collector Function collides with initial route",
			)
		}
	}
	routeChanges := controllerRouteChanges(c.routes, nextRoutes)
	if len(routeChanges) == 0 {
		c.groups = nextGroups
		cleanupUnpublished = false
		_ = c.cleanupUnpublishedGroups(
			context.WithoutCancel(ctx),
			unpublished,
		)
		c.refreshModuleAvailabilityLocked(module)
		return nil, nil
	}
	mutation, err := c.catalog.NewMutation(
		c.version,
		routeChanges,
	)
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
			version, mutationErr := c.mutations.CommitFunctions(
				transitionCtx,
				mutation,
			)
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
			return c.mutations.AbortFunctions(
				transitionCtx,
				mutation,
			)
		},
	)
	err = errors.Join(err, mutation.Discard())
	if err != nil {
		c.dirty = errors.Join(c.dirty, err)
		return nil, err
	}
	c.refreshModuleAvailabilityLocked(module)
	return retired, nil
}

func (c *Controller) moduleAvailabilityChangedLocked(module string) bool {
	state, ok := c.availability[module]
	if !ok {
		return true
	}
	for index := range state.probes {
		probe := &state.probes[index]
		var available bool
		if probe.job != nil {
			available = jobBackedFunctionAvailable(probe.job, probe.methodID)
		} else {
			available = probe.agent == nil || probe.agent()
		}
		if available != probe.observed {
			return true
		}
	}
	return false
}

func (c *Controller) refreshModuleAvailabilityLocked(module string) {
	plan := c.plans[module]
	jobs := c.jobs[module]
	capacity := len(plan.agent) + len(plan.shared)*len(jobs)
	for _, job := range jobs {
		capacity += len(job.methods)
	}
	state := controllerModuleAvailability{
		probes: make([]controllerAvailabilityProbe, 0, capacity),
	}
	currentAgent := c.groups[module+"/agent"]
	for _, method := range plan.agent {
		if method.Available == nil {
			continue
		}
		if currentAgent != nil && currentAgent.generation != nil {
			if _, published := currentAgent.generation.methods[method.ID]; published {
				continue
			}
		}
		state.probes = append(state.probes, controllerAvailabilityProbe{
			methodID: method.ID,
			agent:    method.Available,
			observed: method.Available(),
		})
	}
	for _, job := range jobs {
		for _, method := range plan.shared {
			state.probes = append(state.probes, controllerAvailabilityProbe{
				methodID: method.ID,
				job:      job.job,
				observed: jobBackedFunctionAvailable(job.job, method.ID),
			})
		}
		for _, method := range job.methods {
			state.probes = append(state.probes, controllerAvailabilityProbe{
				methodID: method.ID,
				job:      job.job,
				observed: jobBackedFunctionAvailable(job.job, method.ID),
			})
		}
	}
	c.availability[module] = state
}

func (c *Controller) buildModuleGroups(
	module string,
	creator collectorapi.Creator,
) (
	map[string]*controllerGroup,
	map[string]*controllerGroup,
	error,
) {
	desired := make(map[string]*controllerGroup)
	unpublished := make(map[string]*controllerGroup)
	add := func(group *controllerGroup, err error) error {
		if err != nil {
			return err
		}
		if group != nil {
			desired[group.key] = group
			unpublished[group.key] = group
		}
		return nil
	}
	if err := add(c.buildAgentGroup(module, creator)); err != nil {
		return nil, unpublished, err
	}
	if err := add(c.buildSharedGroup(module, creator)); err != nil {
		return nil, unpublished, err
	}
	jobs := c.jobs[module]
	names := slices.Sorted(maps.Keys(jobs))
	for _, name := range names {
		if err := add(c.buildInstanceGroup(module, creator, jobs[name])); err != nil {
			return nil, unpublished, err
		}
	}
	return desired, unpublished, nil
}

func (c *Controller) buildAgentGroup(
	module string,
	creator collectorapi.Creator,
) (*controllerGroup, error) {
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
		nil,
		true,
	)
}

func (c *Controller) buildSharedGroup(
	module string,
	creator collectorapi.Creator,
) (*controllerGroup, error) {
	if creator.SharedFunctions == nil || len(c.jobs[module]) == 0 {
		return nil, nil
	}
	jobs := make(map[string]collectorapi.RuntimeJob, len(c.jobs[module]))
	for name, job := range c.jobs[module] {
		jobs[name] = job.job
	}
	return c.buildGroup(
		module+"/shared",
		module,
		methodGenerationShared,
		creator,
		c.plans[module].shared,
		jobs,
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
		module+"/instance/"+job.job.Name(),
		module,
		methodGenerationInstance,
		creator,
		job.methods,
		map[string]collectorapi.RuntimeJob{job.job.Name(): job.job},
		false,
	)
}

func (c *Controller) buildGroup(
	key string,
	module string,
	kind methodGenerationKind,
	creator collectorapi.Creator,
	methods []funcapi.FunctionConfig,
	jobs map[string]collectorapi.RuntimeJob,
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
	signature, err := controllerGroupSignature(kind, methods, jobs)
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
	generation, err := newMethodGeneration(id, module, kind, creator, methods, jobs)
	if err != nil {
		return nil, err
	}
	declaration := generation.declaration()
	group := &controllerGroup{
		key: key, module: module, signature: signature,
		generation: generation, routes: make(map[string]controllerRoute),
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
					ID: method.ID, Generation: declaration,
					PublicName: name, RawPayload: method.RawRequest,
					CooperativeCancel: true, CooperativeDeadline: true,
				},
				publication: methodPublicationRecord(
					name,
					module,
					method,
					c.nextID,
				),
			}
		}
	}
	return group, nil
}

func (c *Controller) availableAgentMethods(
	module string,
) []funcapi.FunctionConfig {
	current := c.groups[module+"/agent"]
	methods := make([]funcapi.FunctionConfig, 0, len(c.plans[module].agent))
	for _, method := range c.plans[module].agent {
		published := false
		if current != nil && current.generation != nil {
			_, published = current.generation.methods[method.ID]
		}
		if published || method.Available == nil || method.Available() {
			methods = append(methods, method)
		}
	}
	return methods
}

func validateConfiguredMethods(
	module string,
	methods []funcapi.FunctionConfig,
) ([]funcapi.FunctionConfig, error) {
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
	jobs map[string]collectorapi.RuntimeJob,
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
			return "", fmt.Errorf(
				"jobmgr Function controller: method %q presentation: %w",
				method.ID,
				err,
			)
		}
		writeDigestString(digest, string(presentation))
	}
	names := slices.Sorted(maps.Keys(jobs))
	for _, name := range names {
		job := jobs[name]
		writeDigestString(digest, name)
		writeDigestString(digest, job.FullName())
		for _, method := range methods {
			writeDigestBool(digest, jobBackedFunctionAvailable(job, method.ID))
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
		Name: name, Generation: generation, Timeout: 60,
		Help: help, Tags: tags, Access: access, Priority: 100, Version: 3,
	}
}

func indexControllerRoutes(
	groups map[string]*controllerGroup,
) (map[string]controllerRoute, error) {
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

func controllerRouteChanges(
	current map[string]controllerRoute,
	next map[string]controllerRoute,
) []RouteChange {
	return controllerChanges(current, next,
		func(name string) RouteChange {
			return RouteChange{PublicName: name}
		},
		func(name string, route controllerRoute) RouteChange {
			declaration := route.declaration
			return RouteChange{PublicName: name, Declaration: &declaration}
		},
	)
}

func controllerPublicationChanges(
	current map[string]controllerRoute,
	next map[string]controllerRoute,
) []PublicationChange {
	return controllerChanges(current, next,
		func(name string) PublicationChange {
			return PublicationChange{Name: name}
		},
		func(name string, route controllerRoute) PublicationChange {
			record := route.publication
			return PublicationChange{Name: name, Record: &record}
		},
	)
}

func controllerChangedRouteNames(
	current map[string]controllerRoute,
	next map[string]controllerRoute,
) []string {
	changed := make(map[string]struct{})
	for name, currentRoute := range current {
		nextRoute, exists := next[name]
		if !exists ||
			currentRoute.publication != nextRoute.publication ||
			currentRoute.declaration.Generation != nextRoute.declaration.Generation {
			changed[name] = struct{}{}
		}
	}
	for name, nextRoute := range next {
		currentRoute, exists := current[name]
		if !exists ||
			currentRoute.publication != nextRoute.publication ||
			currentRoute.declaration.Generation != nextRoute.declaration.Generation {
			changed[name] = struct{}{}
		}
	}
	return slices.Sorted(maps.Keys(changed))
}

func sortedControllerRouteNames(routes map[string]controllerRoute) []string {
	return slices.Sorted(maps.Keys(routes))
}

func (c *Controller) publicationRecordsLocked(
	routes map[string]controllerRoute,
) []PublicationRecord {
	names := sortedControllerRouteNames(routes)
	fixedNames := slices.Sorted(maps.Keys(c.fixed))
	records := make(
		[]PublicationRecord,
		0,
		len(fixedNames)+len(names),
	)
	for _, name := range fixedNames {
		records = append(records, c.fixed[name])
	}
	for _, name := range names {
		records = append(records, routes[name].publication)
	}
	return records
}

func validateInitialRoute(route InitialRoute) error {
	if err := validateDeclaration(route.Declaration); err != nil {
		return err
	}
	record := route.Publication
	if record.Name != route.Declaration.PublicName ||
		record.Generation == 0 ||
		record.Timeout < 0 ||
		record.Priority < 0 ||
		record.Version < 0 ||
		record.Access == "" {
		return errors.New(
			"jobmgr Function controller: invalid initial publication",
		)
	}
	return nil
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
						fmt.Errorf(
							"jobmgr Function controller: initial route cleanup panic: %v",
							recovered,
						),
					)
				}
			}()
			err = errors.Join(err, generation.Cleanup(ctx))
		}()
	}
	return err
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

func generationReferencesJob(
	generation *methodGeneration,
	job collectorapi.RuntimeJob,
) bool {
	if generation == nil || job == nil {
		return false
	}
	for _, owned := range generation.jobs {
		if owned == job {
			return true
		}
	}
	return false
}

func (c *Controller) cleanupUnpublishedGroups(
	ctx context.Context,
	groups map[string]*controllerGroup,
) (err error) {
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
