// SPDX-License-Identifier: GPL-3.0-or-later

package functions

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"sync"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

type ModuleCatalog interface {
	Lookup(string) (collectorapi.Creator, bool)
}

type JobIdentity struct {
	ID         string
	Generation uint64
}

func (identity JobIdentity) valid() bool {
	return identity.ID != "" && identity.Generation != 0
}

type RuntimeJob interface {
	collectorapi.RuntimeJob
}

type Controller struct {
	mu sync.Mutex

	epoch       uint64
	modules     collectorapi.Registry
	catalog     *Catalog
	mutations   jobmgr.FunctionMutationPort
	publication *Publication

	plans        map[string]controllerModulePlan
	jobs         map[string]map[string]controllerJob
	groups       map[string]*controllerGroup
	routes       map[string]controllerRoute
	fixed        map[string]PublicationRecord
	fixedRoute   []*HandlerGenerationDeclaration
	availability map[string]controllerModuleAvailability
	version      uint64
	nextID       uint64
	activated    bool
	draining     bool
	terminated   bool
	dirty        error
}

type controllerJob struct {
	identity JobIdentity
	job      RuntimeJob
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
	methodID string
	job      RuntimeJob
	agent    func() bool
	observed bool
}

type controllerGroup struct {
	key        string
	module     string
	signature  string
	generation *methodGeneration
	routes     map[string]controllerRoute
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
	mu sync.Mutex

	controller *Controller
	identity   JobIdentity
	job        RuntimeJob
	published  bool
	closed     bool
	cleaned    bool
}

func (controller *Controller) PrepareJob(
	identity JobIdentity,
	job RuntimeJob,
) (*JobHandle, error) {
	if controller == nil || !identity.valid() || job == nil ||
		identity.ID != job.FullName() || job.ModuleName() == "" || job.Name() == "" {
		return nil, errors.New("jobmgr Function controller: invalid job preparation")
	}
	if _, ok := controller.modules.Lookup(job.ModuleName()); !ok {
		return nil, errors.New("jobmgr Function controller: job module is not registered")
	}
	return &JobHandle{controller: controller, identity: identity, job: job}, nil
}

func (handle *JobHandle) Publish() error {
	if handle == nil {
		return errors.New("jobmgr Function controller: nil job handle")
	}
	handle.mu.Lock()
	defer handle.mu.Unlock()
	if handle.published || handle.closed || handle.cleaned {
		return errors.New("jobmgr Function controller: invalid job-handle publication")
	}
	if err := handle.controller.PublishJob(
		context.Background(),
		handle.identity,
		handle.job,
	); err != nil {
		return err
	}
	handle.published = true
	return nil
}

func (handle *JobHandle) CloseAndDrain(ctx context.Context) error {
	if handle == nil || ctx == nil {
		return errors.New("jobmgr Function controller: invalid job-handle close")
	}
	handle.mu.Lock()
	defer handle.mu.Unlock()
	if handle.closed {
		return nil
	}
	if handle.cleaned {
		return errors.New("jobmgr Function controller: close after cleanup")
	}
	if handle.published {
		if err := handle.controller.CloseAndDrainJob(ctx, handle.identity, handle.job); err != nil {
			return err
		}
	}
	handle.closed = true
	return nil
}

func (handle *JobHandle) Cleanup(ctx context.Context) error {
	if handle == nil || ctx == nil {
		return errors.New("jobmgr Function controller: invalid job-handle cleanup")
	}
	handle.mu.Lock()
	defer handle.mu.Unlock()
	if handle.cleaned {
		return nil
	}
	if !handle.closed {
		return errors.New("jobmgr Function controller: cleanup before close")
	}
	// Catalog-owned cleanup plans execute the exact MethodHandler cleanup before
	// CloseAndDrain returns. This acknowledgement makes the per-job lifecycle
	// explicit without re-resolving the job through a second owner.
	handle.cleaned = true
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
	sort.Strings(names)
	for _, module := range names {
		creator := controller.modules[module]
		var plan controllerModulePlan
		if creator.AgentFunctions != nil {
			plan.agent = append([]funcapi.FunctionConfig(nil), creator.AgentFunctions()...)
		}
		if creator.SharedFunctions != nil {
			plan.shared = append([]funcapi.FunctionConfig(nil), creator.SharedFunctions()...)
		}
		controller.plans[module] = plan
		group, err := controller.buildAgentGroup(module, controller.modules[module])
		if err != nil {
			controller.cleanupUnpublishedGroups(context.Background(), controller.groups)
			return nil, nil, err
		}
		if group != nil {
			controller.groups[group.key] = group
		}
		controller.refreshModuleAvailabilityLocked(module)
	}
	routes, err := indexControllerRoutes(controller.groups)
	if err != nil {
		controller.cleanupUnpublishedGroups(context.Background(), controller.groups)
		return nil, nil, err
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
			controller.cleanupUnpublishedGroups(
				context.Background(),
				controller.groups,
			)
			controller.cleanupInitialRoutes(
				context.Background(),
				controller.fixedRoute,
			)
			return nil, nil, err
		}
		if _, exists := routes[route.Declaration.PublicName]; exists {
			controller.cleanupUnpublishedGroups(
				context.Background(),
				controller.groups,
			)
			controller.cleanupInitialRoutes(
				context.Background(),
				controller.fixedRoute,
			)
			return nil, nil, errors.New(
				"jobmgr Function controller: initial route collides with collector Function",
			)
		}
		if current, exists := controller.fixed[route.Publication.Name]; exists &&
			current != route.Publication {
			controller.cleanupUnpublishedGroups(
				context.Background(),
				controller.groups,
			)
			controller.cleanupInitialRoutes(
				context.Background(),
				controller.fixedRoute,
			)
			return nil, nil, errors.New(
				"jobmgr Function controller: initial routes disagree on publication",
			)
		}
		controller.fixed[route.Publication.Name] = route.Publication
		if _, exists := fixedGenerations[route.Declaration.Generation]; !exists {
			fixedGenerations[route.Declaration.Generation] = struct{}{}
			controller.fixedRoute = append(
				controller.fixedRoute,
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
		controller.cleanupUnpublishedGroups(context.Background(), controller.groups)
		_ = controller.cleanupInitialRoutes(
			context.Background(),
			controller.fixedRoute,
		)
		return nil, nil, err
	}
	controller.routes = routes
	controller.catalog = catalog
	return controller, catalog, nil
}

func (controller *Controller) Bind(
	mutations jobmgr.FunctionMutationPort,
	publication *Publication,
) error {
	if controller == nil || mutations == nil || publication == nil {
		return errors.New("jobmgr Function controller: invalid binding")
	}
	controller.mu.Lock()
	defer controller.mu.Unlock()
	if controller.mutations != nil || controller.publication != nil ||
		controller.activated || controller.terminated {
		return errors.New("jobmgr Function controller: duplicate binding")
	}
	controller.mutations = mutations
	controller.publication = publication
	return nil
}

// AbortConstruction cleans handler generations that never became externally
// visible. A private kernel may already hold the catalog and mutation port, but
// its loop must not have started.
func (controller *Controller) AbortConstruction(ctx context.Context) error {
	if controller == nil {
		return nil
	}
	if ctx == nil {
		return errors.New("jobmgr Function controller: nil construction abort context")
	}
	controller.mu.Lock()
	if controller.activated ||
		controller.draining ||
		controller.terminated {
		controller.mu.Unlock()
		return errors.New("jobmgr Function controller: construction abort after activation")
	}
	controller.terminated = true
	groups := controller.groups
	fixed := controller.fixedRoute
	controller.groups = nil
	controller.routes = nil
	controller.fixedRoute = nil
	controller.mu.Unlock()
	controller.cleanupUnpublishedGroups(ctx, groups)
	return controller.cleanupInitialRoutes(ctx, fixed)
}

func (controller *Controller) Activate() error {
	if controller == nil {
		return errors.New("jobmgr Function controller: nil activation")
	}
	controller.mu.Lock()
	defer controller.mu.Unlock()
	if controller.mutations == nil || controller.publication == nil ||
		controller.activated || controller.terminated {
		return errors.New("jobmgr Function controller: invalid activation")
	}
	records := controller.publicationRecordsLocked(controller.routes)
	digest, err := DigestSortedPublications(records)
	if err != nil {
		return err
	}
	changes := make([]PublicationChange, 0, len(records))
	for index := range records {
		record := records[index]
		changes = append(changes, PublicationChange{Name: record.Name, Record: &record})
	}
	if err := controller.publication.ApplyInitialSnapshot(
		controller.epoch,
		controller.version,
		digest,
		controller.catalog.storage.published.Load(),
		changes,
	); err != nil {
		controller.dirty = errors.Join(controller.dirty, err)
		return err
	}
	controller.activated = true
	return nil
}

func (controller *Controller) PublishJob(
	ctx context.Context,
	identity JobIdentity,
	job RuntimeJob,
) error {
	if ctx == nil || !identity.valid() || job == nil ||
		identity.ID != job.FullName() || job.ModuleName() == "" || job.Name() == "" {
		return errors.New("jobmgr Function controller: invalid job publication")
	}
	controller.mu.Lock()
	defer controller.mu.Unlock()
	if err := controller.usableLocked(); err != nil {
		return err
	}
	creator, ok := controller.modules.Lookup(job.ModuleName())
	if !ok {
		return errors.New("jobmgr Function controller: job module is not registered")
	}
	moduleJobs := controller.jobs[job.ModuleName()]
	if moduleJobs == nil {
		moduleJobs = make(map[string]controllerJob)
		controller.jobs[job.ModuleName()] = moduleJobs
	}
	if _, exists := moduleJobs[job.Name()]; exists {
		return errors.New("jobmgr Function controller: job is already published")
	}
	var methods []funcapi.FunctionConfig
	if creator.InstanceFunctions != nil {
		methods = append([]funcapi.FunctionConfig(nil), creator.InstanceFunctions(job)...)
	}
	moduleJobs[job.Name()] = controllerJob{
		identity: identity, job: job, methods: methods,
	}
	if _, err := controller.reconcileModuleLocked(ctx, job.ModuleName(), creator); err != nil {
		if controller.dirty == nil {
			delete(moduleJobs, job.Name())
			if len(moduleJobs) == 0 {
				delete(controller.jobs, job.ModuleName())
			}
		}
		return err
	}
	return nil
}

func (controller *Controller) CloseAndDrainJob(
	ctx context.Context,
	identity JobIdentity,
	job RuntimeJob,
) error {
	if ctx == nil || !identity.valid() || job == nil {
		return errors.New("jobmgr Function controller: invalid job close")
	}
	controller.mu.Lock()
	if controller.draining {
		moduleJobs := controller.jobs[job.ModuleName()]
		current, exists := moduleJobs[job.Name()]
		if !exists {
			controller.mu.Unlock()
			return nil
		}
		if current.identity != identity || current.job != job {
			controller.mu.Unlock()
			return errors.New("jobmgr Function controller: stale draining job close")
		}
		delete(moduleJobs, job.Name())
		if len(moduleJobs) == 0 {
			delete(controller.jobs, job.ModuleName())
		}
		retired := make([]*methodGeneration, 0, len(controller.groups))
		for _, group := range controller.groups {
			if generationReferencesJob(group.generation, job) {
				retired = append(retired, group.generation)
			}
		}
		controller.mu.Unlock()
		for _, generation := range retired {
			if err := generation.wait(ctx); err != nil {
				return err
			}
		}
		return nil
	}
	if err := controller.usableLocked(); err != nil {
		controller.mu.Unlock()
		return err
	}
	moduleJobs := controller.jobs[job.ModuleName()]
	current, exists := moduleJobs[job.Name()]
	if !exists {
		controller.mu.Unlock()
		return nil
	}
	if current.identity != identity || current.job != job {
		controller.mu.Unlock()
		return errors.New("jobmgr Function controller: stale job close")
	}
	creator := controller.modules[job.ModuleName()]
	delete(moduleJobs, job.Name())
	if len(moduleJobs) == 0 {
		delete(controller.jobs, job.ModuleName())
	}
	retired, err := controller.reconcileModuleLocked(ctx, job.ModuleName(), creator)
	if err != nil {
		if controller.dirty == nil {
			moduleJobs = controller.jobs[job.ModuleName()]
			if moduleJobs == nil {
				moduleJobs = make(map[string]controllerJob)
				controller.jobs[job.ModuleName()] = moduleJobs
			}
			moduleJobs[job.Name()] = current
		}
		controller.mu.Unlock()
		return err
	}
	controller.mu.Unlock()
	for _, generation := range retired {
		if generationReferencesJob(generation, job) {
			if err := generation.wait(ctx); err != nil {
				return err
			}
		}
	}
	return nil
}

func (controller *Controller) ReconcileJob(
	ctx context.Context,
	identity JobIdentity,
	job RuntimeJob,
) error {
	if ctx == nil || !identity.valid() || job == nil {
		return errors.New("jobmgr Function controller: invalid job reconcile")
	}
	controller.mu.Lock()
	defer controller.mu.Unlock()
	if err := controller.usableLocked(); err != nil {
		return err
	}
	current, ok := controller.jobs[job.ModuleName()][job.Name()]
	if !ok || current.identity != identity || current.job != job {
		return errors.New("jobmgr Function controller: stale job reconcile")
	}
	_, err := controller.reconcileModuleLocked(
		ctx,
		job.ModuleName(),
		controller.modules[job.ModuleName()],
	)
	return err
}

func (controller *Controller) ReconcileModule(
	ctx context.Context,
	module string,
) error {
	if controller == nil || ctx == nil || module == "" {
		return errors.New("jobmgr Function controller: invalid module reconcile")
	}
	controller.mu.Lock()
	defer controller.mu.Unlock()
	if err := controller.usableLocked(); err != nil {
		return err
	}
	creator, ok := controller.modules.Lookup(module)
	if !ok {
		return errors.New("jobmgr Function controller: module is not registered")
	}
	if !controller.moduleAvailabilityChangedLocked(module) {
		return nil
	}
	_, err := controller.reconcileModuleLocked(ctx, module, creator)
	return err
}

func (controller *Controller) Stop(epoch uint64) error {
	if controller == nil {
		return nil
	}
	beginErr := controller.BeginShutdown(epoch)
	controller.mu.Lock()
	defer controller.mu.Unlock()
	if controller.terminated {
		return errors.Join(beginErr, controller.dirty)
	}
	controller.terminated = true
	return errors.Join(beginErr, controller.dirty)
}

// BeginShutdown withdraws external routes before CommandKernel closes the
// catalog. Job handles may still close afterward; in that mode they wait for
// catalog-owned generation cleanup without submitting another mutation.
func (controller *Controller) BeginShutdown(epoch uint64) error {
	if controller == nil {
		return nil
	}
	controller.mu.Lock()
	defer controller.mu.Unlock()
	if controller.terminated {
		return controller.dirty
	}
	if controller.draining {
		return controller.dirty
	}
	controller.draining = true
	if controller.publication == nil {
		controller.dirty = errors.Join(
			controller.dirty,
			errors.New("jobmgr Function controller: shutdown before binding"),
		)
		return controller.dirty
	}
	controller.dirty = errors.Join(
		controller.dirty,
		controller.publication.Stop(epoch),
	)
	return controller.dirty
}

func (controller *Controller) usableLocked() error {
	if controller.dirty != nil {
		return controller.dirty
	}
	if !controller.activated || controller.draining || controller.terminated ||
		controller.mutations == nil || controller.publication == nil {
		return errors.New("jobmgr Function controller: not active")
	}
	return nil
}

func (controller *Controller) reconcileModuleLocked(
	ctx context.Context,
	module string,
	creator collectorapi.Creator,
) ([]*methodGeneration, error) {
	desired, unpublished, err := controller.buildModuleGroups(module, creator)
	if err != nil {
		return nil, err
	}
	cleanupUnpublished := true
	defer func() {
		if cleanupUnpublished {
			controller.cleanupUnpublishedGroups(context.WithoutCancel(ctx), unpublished)
		}
	}()

	nextGroups := make(map[string]*controllerGroup, len(controller.groups)+len(desired))
	for key, group := range controller.groups {
		if group.module != module {
			nextGroups[key] = group
		}
	}
	for key, group := range desired {
		if current := controller.groups[key]; current != nil &&
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
	for name := range controller.fixed {
		if _, exists := nextRoutes[name]; exists {
			return nil, errors.New(
				"jobmgr Function controller: collector Function collides with initial route",
			)
		}
	}
	routeChanges := controllerRouteChanges(controller.routes, nextRoutes)
	if len(routeChanges) == 0 {
		controller.groups = nextGroups
		cleanupUnpublished = false
		controller.cleanupUnpublishedGroups(context.WithoutCancel(ctx), unpublished)
		controller.refreshModuleAvailabilityLocked(module)
		return nil, nil
	}
	mutation, err := controller.catalog.NewMutation(
		controller.version,
		routeChanges,
	)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = mutation.Discard()
	}()
	retired := retiredMethodGenerations(controller.groups, nextGroups)
	publicationChanges := controllerPublicationChanges(controller.routes, nextRoutes)
	records := controller.publicationRecordsLocked(nextRoutes)
	digest, err := DigestSortedPublications(records)
	if err != nil {
		return nil, errors.Join(err, mutation.Discard())
	}
	expectedVersion := controller.version + 1
	err = controller.publication.ApplyTransition(
		controller.epoch,
		expectedVersion,
		digest,
		publicationChanges,
		func() error {
			version, mutationErr := controller.mutations.MutateFunctions(ctx, mutation)
			if mutationErr != nil {
				return mutationErr
			}
			// The catalog commit transfers ownership of every new generation
			// and retires the predecessors. No later publication failure may
			// roll that transition back or clean catalog-owned handlers.
			controller.version = version
			controller.groups = nextGroups
			controller.routes = nextRoutes
			cleanupUnpublished = false
			if version != expectedVersion {
				return errors.New("jobmgr Function controller: mutation version mismatch")
			}
			return nil
		},
	)
	err = errors.Join(err, mutation.Discard())
	if err != nil {
		controller.dirty = errors.Join(controller.dirty, err)
		return nil, err
	}
	controller.refreshModuleAvailabilityLocked(module)
	return retired, nil
}

func (controller *Controller) moduleAvailabilityChangedLocked(module string) bool {
	state, ok := controller.availability[module]
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

func (controller *Controller) refreshModuleAvailabilityLocked(module string) {
	plan := controller.plans[module]
	jobs := controller.jobs[module]
	capacity := len(plan.agent) + len(plan.shared)*len(jobs)
	for _, job := range jobs {
		capacity += len(job.methods)
	}
	state := controllerModuleAvailability{
		probes: make([]controllerAvailabilityProbe, 0, capacity),
	}
	currentAgent := controller.groups[module+"/agent"]
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
	controller.availability[module] = state
}

func (controller *Controller) buildModuleGroups(
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
	if err := add(controller.buildAgentGroup(module, creator)); err != nil {
		return nil, unpublished, err
	}
	if err := add(controller.buildSharedGroup(module, creator)); err != nil {
		return nil, unpublished, err
	}
	jobs := controller.jobs[module]
	names := make([]string, 0, len(jobs))
	for name := range jobs {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if err := add(controller.buildInstanceGroup(module, creator, jobs[name])); err != nil {
			return nil, unpublished, err
		}
	}
	return desired, unpublished, nil
}

func (controller *Controller) buildAgentGroup(
	module string,
	creator collectorapi.Creator,
) (*controllerGroup, error) {
	if creator.AgentFunctions == nil {
		return nil, nil
	}
	methods := controller.availableAgentMethods(module)
	return controller.buildGroup(
		module+"/agent",
		module,
		methodGenerationAgent,
		creator,
		methods,
		nil,
		true,
	)
}

func (controller *Controller) buildSharedGroup(
	module string,
	creator collectorapi.Creator,
) (*controllerGroup, error) {
	if creator.SharedFunctions == nil || len(controller.jobs[module]) == 0 {
		return nil, nil
	}
	jobs := make(map[string]collectorapi.RuntimeJob, len(controller.jobs[module]))
	for name, job := range controller.jobs[module] {
		jobs[name] = job.job
	}
	return controller.buildGroup(
		module+"/shared",
		module,
		methodGenerationShared,
		creator,
		controller.plans[module].shared,
		jobs,
		true,
	)
}

func (controller *Controller) buildInstanceGroup(
	module string,
	creator collectorapi.Creator,
	job controllerJob,
) (*controllerGroup, error) {
	if creator.InstanceFunctions == nil {
		return nil, nil
	}
	return controller.buildGroup(
		module+"/instance/"+job.job.Name(),
		module,
		methodGenerationInstance,
		creator,
		job.methods,
		map[string]collectorapi.RuntimeJob{job.job.Name(): job.job},
		false,
	)
}

func (controller *Controller) buildGroup(
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
	if current := controller.groups[key]; current != nil && current.signature == signature {
		return current, nil
	}
	controller.nextID++
	if controller.nextID == 0 {
		return nil, errors.New("jobmgr Function controller: generation wrapped")
	}
	id := module + "/" + strconv.FormatUint(controller.nextID, 10)
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
					PublicName: name, Lane: RouteLane(), RawPayload: method.RawRequest,
					CooperativeCancel: true, CooperativeDeadline: true,
				},
				publication: methodPublicationRecord(
					name,
					module,
					method,
					controller.nextID,
				),
			}
		}
	}
	return group, nil
}

func (controller *Controller) availableAgentMethods(
	module string,
) []funcapi.FunctionConfig {
	current := controller.groups[module+"/agent"]
	methods := make([]funcapi.FunctionConfig, 0, len(controller.plans[module].agent))
	for _, method := range controller.plans[module].agent {
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
	sort.Slice(valid, func(left, right int) bool {
		return valid[left].ID < valid[right].ID
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
		writeControllerDigestString(digest, method.ID)
		writeControllerDigestString(digest, method.FunctionName)
		writeControllerDigestString(digest, method.Name)
		writeControllerDigestUint64(digest, uint64(method.UpdateEvery))
		writeControllerDigestString(digest, method.Help)
		writeControllerDigestBool(digest, method.RequireCloud)
		writeControllerDigestString(digest, method.Tags)
		writeControllerDigestString(digest, method.ResponseType)
		writeControllerDigestBool(digest, method.RawRequest)
		for _, alias := range method.Aliases {
			writeControllerDigestString(digest, alias)
		}
		for _, parameter := range method.RequiredParams {
			writeControllerDigestString(digest, parameter.ID)
			writeControllerDigestString(digest, parameter.Name)
			writeControllerDigestString(digest, parameter.Help)
			writeControllerDigestUint64(digest, uint64(parameter.Selection))
			writeControllerDigestBool(digest, parameter.UniqueView)
			for _, option := range parameter.Options {
				writeControllerDigestString(digest, option.ID)
				writeControllerDigestString(digest, option.Name)
				writeControllerDigestBool(digest, option.Default)
				writeControllerDigestBool(digest, option.Disabled)
				writeControllerDigestString(digest, option.Column)
				if option.Sort != nil {
					writeControllerDigestString(digest, option.Sort.String())
				} else {
					writeControllerDigestString(digest, "")
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
		writeControllerDigestString(digest, string(presentation))
	}
	names := make([]string, 0, len(jobs))
	for name := range jobs {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		job := jobs[name]
		writeControllerDigestString(digest, name)
		writeControllerDigestString(digest, job.FullName())
		for _, method := range methods {
			writeControllerDigestBool(digest, jobBackedFunctionAvailable(job, method.ID))
		}
	}
	return fmt.Sprintf("%x", digest.Sum(nil)), nil
}

type controllerDigestWriter interface {
	Write([]byte) (int, error)
}

func writeControllerDigestString(writer controllerDigestWriter, value string) {
	var size [8]byte
	binary.BigEndian.PutUint64(size[:], uint64(len(value)))
	_, _ = writer.Write(size[:])
	_, _ = writer.Write([]byte(value))
}

func writeControllerDigestUint64(writer controllerDigestWriter, value uint64) {
	var encoded [8]byte
	binary.BigEndian.PutUint64(encoded[:], value)
	_, _ = writer.Write(encoded[:])
}

func writeControllerDigestBool(writer controllerDigestWriter, value bool) {
	if value {
		_, _ = writer.Write([]byte{1})
		return
	}
	_, _ = writer.Write([]byte{0})
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
	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)
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

func controllerRouteChanges(
	current map[string]controllerRoute,
	next map[string]controllerRoute,
) []RouteChange {
	names := controllerChangedRouteNames(current, next)
	changes := make([]RouteChange, 0, len(names))
	for _, name := range names {
		route, exists := next[name]
		if !exists {
			changes = append(changes, RouteChange{PublicName: name})
			continue
		}
		declaration := route.declaration
		changes = append(changes, RouteChange{
			PublicName: name, Declaration: &declaration,
		})
	}
	return changes
}

func controllerPublicationChanges(
	current map[string]controllerRoute,
	next map[string]controllerRoute,
) []PublicationChange {
	names := controllerChangedRouteNames(current, next)
	changes := make([]PublicationChange, 0, len(names))
	for _, name := range names {
		route, exists := next[name]
		if !exists {
			changes = append(changes, PublicationChange{Name: name})
			continue
		}
		record := route.publication
		changes = append(changes, PublicationChange{Name: name, Record: &record})
	}
	return changes
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
	names := make([]string, 0, len(changed))
	for name := range changed {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func sortedControllerRouteNames(routes map[string]controllerRoute) []string {
	names := make([]string, 0, len(routes))
	for name := range routes {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (controller *Controller) publicationRecordsLocked(
	routes map[string]controllerRoute,
) []PublicationRecord {
	names := sortedControllerRouteNames(routes)
	fixedNames := make([]string, 0, len(controller.fixed))
	for name := range controller.fixed {
		fixedNames = append(fixedNames, name)
	}
	sort.Strings(fixedNames)
	records := make(
		[]PublicationRecord,
		0,
		len(fixedNames)+len(names),
	)
	for _, name := range fixedNames {
		records = append(records, controller.fixed[name])
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

func (controller *Controller) cleanupInitialRoutes(
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
	job RuntimeJob,
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

func (controller *Controller) cleanupUnpublishedGroups(
	ctx context.Context,
	groups map[string]*controllerGroup,
) {
	seen := make(map[*methodGeneration]struct{})
	for _, group := range groups {
		if group == nil || group.generation == nil {
			continue
		}
		if _, exists := seen[group.generation]; exists {
			continue
		}
		seen[group.generation] = struct{}{}
		_ = group.generation.cleanup(ctx)
	}
}

func (identity JobIdentity) String() string {
	return identity.ID + "@" + strconv.FormatUint(identity.Generation, 10)
}
