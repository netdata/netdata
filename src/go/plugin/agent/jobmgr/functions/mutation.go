// SPDX-License-Identifier: GPL-3.0-or-later

package functions

import (
	"errors"
	"maps"
	"sync/atomic"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
)

const MaximumMutationQuantum = jobmgr.MaximumFunctionMutationQuantum

// RouteChange replaces, adds, or removes one exact catalog route. A nil
// Declaration removes the route. Initial prefix routes are immutable because
// the controller only changes exact collector routes after construction.
type RouteChange struct {
	PublicName  string
	Declaration *Declaration
}

type routeTransition struct {
	oldRoute *route
	newRoute *route
}

type Mutation struct {
	owner           *Catalog
	expectedVersion uint64
	preimage        *catalogSnapshot
	postimage       *catalogSnapshot
	transitions     []routeTransition
	retirements     []generationRetirement
	quiesced        map[*route]struct{}
	state           atomic.Uint32
	builder         MutationBuilder
}

type generationRetirement struct {
	generation *handlerGeneration
	references int
}

const (
	mutationPrepared uint32 = iota + 1
	mutationClaimed
	mutationReleased
)

func (*Mutation) FunctionCatalogMutation() {}

// NewMutation builds an immutable postimage outside the kernel loop. BeginMutation
// verifies that the catalog still exposes the same preimage before claiming it.
func (c *Catalog) NewMutation(expectedVersion uint64, changes []RouteChange) (*Mutation, error) {
	if c == nil || expectedVersion == 0 || expectedVersion == ^uint64(0) || len(changes) == 0 {
		return nil, errors.New("jobmgr Function catalog: invalid mutation")
	}
	preimage := c.snapshot.Load()
	if preimage == nil || preimage.version != expectedVersion {
		return nil, errors.New("jobmgr Function catalog: stale mutation version")
	}

	mutation := &Mutation{
		owner:           c,
		expectedVersion: expectedVersion,
		preimage:        preimage,
		transitions:     make([]routeTransition, len(changes)),
		quiesced:        make(map[*route]struct{}, len(changes)),
	}
	mutation.state.Store(mutationPrepared)

	routes := maps.Clone(preimage.routes)
	seenRoutes := make(map[string]struct{}, len(changes))
	retirementIndices := make(map[*handlerGeneration]int)
	generationByDeclaration := make(map[*HandlerGenerationDeclaration]*handlerGeneration)
	generationIDOwner := make(map[string]*HandlerGenerationDeclaration)
	for index, change := range changes {
		if change.PublicName == "" || len(change.PublicName) > maximumDeclarationMetadataBytes {
			return nil, errors.New("jobmgr Function catalog: invalid mutation route")
		}
		if _, exists := seenRoutes[change.PublicName]; exists {
			return nil, errors.New("jobmgr Function catalog: duplicate route in mutation")
		}
		seenRoutes[change.PublicName] = struct{}{}

		set := routes[change.PublicName]
		oldRoute := set.direct
		var newRoute *route
		if change.Declaration == nil {
			if oldRoute == nil {
				return nil, errors.New("jobmgr Function catalog: mutation removes missing direct route")
			}
			set.direct = nil
		} else {
			declaration := cloneDeclaration(*change.Declaration)
			if err := validateDeclaration(declaration); err != nil {
				return nil, err
			}
			if declaration.PublicName != change.PublicName || declaration.Prefix != "" {
				return nil, errors.New("jobmgr Function catalog: mutation key/declaration mismatch")
			}

			generationDeclaration := declaration.Generation
			if owner := generationIDOwner[generationDeclaration.ID]; owner != nil && owner != generationDeclaration {
				return nil, errors.New("jobmgr Function catalog: duplicate handler generation identity")
			}
			generationIDOwner[generationDeclaration.ID] = generationDeclaration
			generation := generationByDeclaration[generationDeclaration]
			if generation == nil {
				generation = &handlerGeneration{
					handler: generationDeclaration.Handler,
					cleanup: generationDeclaration.Cleanup,
				}
				if err := c.prepareGenerationCleanup(generation); err != nil {
					return nil, err
				}
				generationByDeclaration[generationDeclaration] = generation
			}

			newRoute = &route{
				publicName:          declaration.PublicName,
				method:              declaration.ID,
				handler:             generation,
				resource:            declaration.Resource,
				cooperativeCancel:   declaration.CooperativeCancel,
				cooperativeDeadline: declaration.CooperativeDeadline,
				rawPayload:          declaration.RawPayload,
				transaction:         cloneResourceTransactionDeclaration(declaration.Transaction),
			}
			generation.routeReferences++
			set.direct = newRoute
		}

		if set.empty() {
			delete(routes, change.PublicName)
		} else {
			routes[change.PublicName] = set
		}
		mutation.transitions[index] = routeTransition{oldRoute: oldRoute, newRoute: newRoute}
		if oldRoute != nil {
			mutation.quiesced[oldRoute] = struct{}{}
			generation := oldRoute.handler
			retirementIndex, exists := retirementIndices[generation]
			if !exists {
				retirementIndices[generation] = len(mutation.retirements)
				mutation.retirements = append(mutation.retirements, generationRetirement{generation: generation})
				retirementIndex = len(mutation.retirements) - 1
			}
			mutation.retirements[retirementIndex].references++
		}
	}

	replacements := make(map[*route]*route, len(mutation.transitions))
	for _, transition := range mutation.transitions {
		if transition.oldRoute != nil {
			replacements[transition.oldRoute] = transition.newRoute
		}
	}
	active := make([]*route, 0, len(preimage.active)+len(changes))
	for _, current := range preimage.active {
		replacement, changed := replacements[current]
		if !changed {
			active = append(active, current)
		} else if replacement != nil {
			active = append(active, replacement)
		}
	}
	for _, transition := range mutation.transitions {
		if transition.oldRoute == nil && transition.newRoute != nil {
			active = append(active, transition.newRoute)
		}
	}
	mutation.postimage = &catalogSnapshot{version: expectedVersion + 1, routes: routes, active: active}
	return mutation, nil
}

// Discard releases a mutation that was never accepted by the catalog.
func (m *Mutation) Discard() error {
	if m == nil {
		return nil
	}
	m.state.CompareAndSwap(mutationPrepared, mutationReleased)
	return nil
}

func (m *Mutation) claim(c *Catalog) error {
	if m == nil || m.owner != c || !m.state.CompareAndSwap(mutationPrepared, mutationClaimed) {
		return errors.New("jobmgr Function catalog: stale mutation")
	}
	return nil
}

func (m *Mutation) release() {
	if m != nil {
		m.state.CompareAndSwap(mutationClaimed, mutationReleased)
	}
}

type MutationBuilder struct {
	mutation                 *Mutation
	index                    int
	preflightTransitionIndex int
	preflightRetirementIndex int
	admissionClosed          bool
	preflightComplete        bool
	resumed                  bool
	finished                 bool
}

func (mb *MutationBuilder) quiesces(resolved *route) bool {
	if mb == nil || mb.mutation == nil || !mb.admissionClosed {
		return false
	}
	_, ok := mb.mutation.quiesced[resolved]
	return ok
}

func (c *Catalog) BeginMutation(mutation jobmgr.FunctionCatalogMutation) error {
	prepared, ok := mutation.(*Mutation)
	if c == nil || c.closed || !ok || prepared == nil || c.mutation != nil {
		if ok && prepared != nil {
			_ = prepared.Discard()
		}
		return errors.New("jobmgr Function catalog: invalid mutation admission")
	}
	if current := c.snapshot.Load(); current != prepared.preimage ||
		current == nil || current.version != prepared.expectedVersion ||
		c.version != prepared.expectedVersion {
		_ = prepared.Discard()
		return errors.New("jobmgr Function catalog: stale mutation admission")
	}
	if err := prepared.claim(c); err != nil {
		return err
	}
	builder := &prepared.builder
	*builder = MutationBuilder{mutation: prepared}
	c.mutation = builder
	return nil
}

func (c *Catalog) AdvanceMutationQuiesce(quantum int) (jobmgr.FunctionCatalogMutationProgress, error) {
	if c == nil || c.mutation == nil || c.mutation.finished || c.mutation.resumed ||
		c.mutation.preflightComplete ||
		quantum <= 0 || quantum > MaximumMutationQuantum {
		return jobmgr.FunctionCatalogMutationProgress{}, errors.New("jobmgr Function catalog: invalid mutation quiesce")
	}
	builder := c.mutation
	builder.admissionClosed = true
	for quantum > 0 && builder.preflightTransitionIndex < len(builder.mutation.transitions) {
		transition := builder.mutation.transitions[builder.preflightTransitionIndex]
		if transition.oldRoute != nil &&
			(transition.oldRoute.handler == nil || transition.oldRoute.invocationLeases < 0) {
			return jobmgr.FunctionCatalogMutationProgress{},
				errors.New("jobmgr Function catalog: invalid mutation route")
		}
		builder.preflightTransitionIndex++
		quantum--
	}
	if builder.preflightTransitionIndex < len(builder.mutation.transitions) {
		return jobmgr.FunctionCatalogMutationProgress{Version: c.version}, nil
	}
	for quantum > 0 && builder.preflightRetirementIndex < len(builder.mutation.retirements) {
		if err := c.validateRetirement(builder.mutation.retirements[builder.preflightRetirementIndex]); err != nil {
			return jobmgr.FunctionCatalogMutationProgress{}, err
		}
		builder.preflightRetirementIndex++
		quantum--
	}
	if builder.preflightRetirementIndex < len(builder.mutation.retirements) {
		return jobmgr.FunctionCatalogMutationProgress{Version: c.version}, nil
	}
	builder.preflightComplete = true
	return jobmgr.FunctionCatalogMutationProgress{Version: c.version, Quiesced: true}, nil
}

func (c *Catalog) validateRetirement(retirement generationRetirement) error {
	generation := retirement.generation
	if generation == nil || retirement.references <= 0 || generation.routeReferences < retirement.references {
		return errors.New("jobmgr Function catalog: route reference underflow")
	}
	if err := c.validateGenerationTransition(
		generation,
		generation.routeReferences-retirement.references,
		generation.invocationLeases,
	); err != nil {
		return err
	}
	if generation.routeReferences == retirement.references {
		return c.validateCleanupOwnership(generation)
	}
	return nil
}

func (c *Catalog) ResumeMutation(mutation jobmgr.FunctionCatalogMutation) error {
	prepared, ok := mutation.(*Mutation)
	if c == nil || !ok || prepared == nil || c.mutation != &prepared.builder ||
		c.mutation.finished || !c.mutation.admissionClosed ||
		!c.mutation.preflightComplete || c.mutation.resumed {
		return errors.New("jobmgr Function catalog: invalid mutation resume")
	}
	c.mutation.resumed = true
	return nil
}

func (c *Catalog) AdvanceMutation(quantum int) (jobmgr.FunctionCatalogMutationProgress, []jobmgr.FunctionCleanupPlan) {
	builder := c.mutation
	cleanups := make([]jobmgr.FunctionCleanupPlan, 0, min(quantum, len(builder.mutation.transitions)-builder.index))
	for quantum > 0 && builder.index < len(builder.mutation.transitions) {
		transition := builder.mutation.transitions[builder.index]
		builder.index++
		quantum--
		if transition.oldRoute == nil {
			continue
		}
		generation := transition.oldRoute.handler
		generation.routeReferences--
		if transition.oldRoute.invocationLeases != 0 {
			name := transition.oldRoute.publicName
			if c.retiring[name] == nil {
				c.retiring[name] = make(map[*route]struct{})
			}
			c.retiring[name][transition.oldRoute] = struct{}{}
		}
		cleanup := c.cleanupIfDrained(generation)
		if cleanup.Valid() {
			cleanups = append(cleanups, cleanup)
		}
	}
	if builder.index < len(builder.mutation.transitions) {
		return jobmgr.FunctionCatalogMutationProgress{Version: c.version}, cleanups
	}

	c.snapshot.Store(builder.mutation.postimage)
	c.version = builder.mutation.postimage.version
	c.routeCount = len(builder.mutation.postimage.active)
	c.mutation = nil
	builder.finished = true
	builder.mutation.release()
	return jobmgr.FunctionCatalogMutationProgress{Version: c.version, Done: true}, cleanups
}

func (c *Catalog) AbortMutation(mutation jobmgr.FunctionCatalogMutation) error {
	prepared, ok := mutation.(*Mutation)
	if c == nil || !ok || prepared == nil || c.mutation != &prepared.builder ||
		c.mutation.finished || c.mutation.resumed {
		return errors.New("jobmgr Function catalog: invalid mutation abort")
	}
	builder := c.mutation
	c.mutation = nil
	builder.finished = true
	builder.mutation.release()
	return nil
}
