// SPDX-License-Identifier: GPL-3.0-or-later

package functions

import (
	"errors"
	"math"
	"sync/atomic"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
)

const (
	MaximumMutationChanges = jobmgr.MaximumFunctionMutationChanges
	MaximumMutationQuantum = jobmgr.MaximumFunctionMutationQuantum
	maximumMutationBytes   = 8 * 1024 * 1024
)

// RouteChange replaces, adds, or removes one exact catalog route. A nil
// Declaration removes the route. Non-nil declarations in one mutation may
// share a HandlerGenerationDeclaration to publish multiple routes owned by one
// generation.
type RouteChange struct {
	PublicName  string
	Prefix      string
	Declaration *Declaration
}

type preparedGeneration struct {
	declaration HandlerGenerationDeclaration
	generation  *handlerGeneration
	references  int
	initialized bool
}

type preparedRouteChange struct {
	publicName  string
	prefix      string
	generation  int
	declaration *Declaration

	placeholder *route
	resolved    *route

	namePath     []*catalogNode
	nameBranches []uint8
	nameCopies   [2][]*catalogNode

	prefixPath     []*prefixNode
	prefixBranches []uint8
	prefixCopies   [2][]*prefixNode
}

// Mutation is an off-loop-owned, fully allocated mutation request. NewMutation
// performs payload-relative validation and allocates every path-copy record so
// KernelLoop preparation can be node-bounded and allocation-free.
type Mutation struct {
	expectedVersion     uint64
	changes             []preparedRouteChange
	generations         []preparedGeneration
	totalNodes          int
	storage             *catalogStorage
	storageBytes        int64
	pathStorageBytes    int64
	cleanupStorageBytes int64
	storageState        atomic.Uint32
	builder             MutationBuilder
	transitions         []routeTransition
	removals            []generationRemoval
	removalIndex        map[*handlerGeneration]int
}

func (*Mutation) FunctionCatalogMutation() {}

// NewMutation validates and owns one bounded route-change batch. The returned
// value may be admitted to exactly this Catalog.
func (catalog *Catalog) NewMutation(
	expectedVersion uint64,
	changes []RouteChange,
) (*Mutation, error) {
	if catalog == nil {
		return nil, errors.New("jobmgr Function catalog: nil mutation catalog")
	}
	if expectedVersion == 0 || len(changes) == 0 || len(changes) > MaximumMutationChanges {
		return nil, errors.New("jobmgr Function catalog: invalid mutation")
	}
	mutation := &Mutation{
		expectedVersion: expectedVersion,
		changes:         make([]preparedRouteChange, len(changes)),
		transitions:     make([]routeTransition, len(changes)),
		removals:        make([]generationRemoval, len(changes)),
		removalIndex:    make(map[*handlerGeneration]int, len(changes)),
	}
	seenRoutes := make(map[routeChangeKey]struct{}, len(changes))
	generationIndex := make(map[*HandlerGenerationDeclaration]int)
	generationIDs := make(map[string]*HandlerGenerationDeclaration)
	totalBytes := 0
	for index, change := range changes {
		if change.PublicName == "" ||
			len(change.PublicName) > maximumDeclarationMetadataBytes ||
			len(change.Prefix) > maximumDeclarationMetadataBytes {
			return nil, errors.New("jobmgr Function catalog: invalid mutation route")
		}
		key := routeChangeKey{name: change.PublicName, prefix: change.Prefix}
		if _, ok := seenRoutes[key]; ok {
			return nil, errors.New("jobmgr Function catalog: duplicate route in mutation")
		}
		seenRoutes[key] = struct{}{}
		totalBytes += len(change.PublicName) + len(change.Prefix)
		if totalBytes > maximumMutationBytes {
			return nil, errors.New("jobmgr Function catalog: mutation bytes exceed bound")
		}

		prepared := &mutation.changes[index]
		prepared.publicName = change.PublicName
		prepared.prefix = change.Prefix
		if change.Declaration != nil {
			declaration := cloneDeclaration(*change.Declaration)
			if err := validateDeclaration(declaration); err != nil {
				return nil, err
			}
			if declaration.PublicName != change.PublicName || declaration.Prefix != change.Prefix {
				return nil, errors.New("jobmgr Function catalog: mutation key/declaration mismatch")
			}
			prepared.declaration = &declaration
			prepared.placeholder = &route{}
			prepared.resolved = &route{}
			generation := declaration.Generation
			if owner := generationIDs[generation.ID]; owner != nil && owner != generation {
				return nil, errors.New("jobmgr Function catalog: duplicate handler generation identity")
			}
			generationIDs[generation.ID] = generation
			generationSlot, ok := generationIndex[generation]
			if !ok {
				generationSlot = len(mutation.generations)
				generationIndex[generation] = generationSlot
				mutation.generations = append(mutation.generations, preparedGeneration{
					declaration: *generation,
					generation:  &handlerGeneration{},
				})
			}
			prepared.generation = generationSlot + 1
			mutation.generations[generationSlot].references++
			prepared.declaration.Generation = nil
		}

		nameBits := len(change.PublicName) * 8
		prefixBits := len(change.Prefix) * 8
		perPhase := 2*nameBits + 4
		if change.Prefix != "" {
			perPhase += 2*prefixBits + 2
		}
		mutation.totalNodes += 2 * perPhase
	}
	mutation.totalNodes += len(mutation.generations)
	storageBytes, err := mutationPathStorageBound(changes)
	if err != nil {
		return nil, err
	}
	cleanupStorageBytes := int64(0)
	for index := range mutation.generations {
		if mutation.generations[index].declaration.Cleanup == nil {
			continue
		}
		if err := addStorageProduct(
			&cleanupStorageBytes,
			1,
			lifecycle.TaskChildExecutionBytes,
		); err != nil {
			return nil, err
		}
	}
	if storageBytes > MaximumCatalogStorageBytes-cleanupStorageBytes {
		return nil, errors.New(
			"jobmgr Function catalog: mutation storage exceeds process bound",
		)
	}
	totalStorageBytes := storageBytes + cleanupStorageBytes
	if err := catalog.storage.reservePreparation(totalStorageBytes); err != nil {
		return nil, err
	}
	mutation.storage = &catalog.storage
	mutation.storageBytes = totalStorageBytes
	mutation.pathStorageBytes = storageBytes
	mutation.cleanupStorageBytes = cleanupStorageBytes
	mutation.storageState.Store(mutationStorageReserved)
	for index := range mutation.changes {
		prepared := &mutation.changes[index]
		nameBits := len(prepared.publicName) * 8
		prepared.namePath = make([]*catalogNode, nameBits+1)
		prepared.nameBranches = make([]uint8, nameBits)
		for phase := range prepared.nameCopies {
			prepared.nameCopies[phase] = make(
				[]*catalogNode,
				len(prepared.namePath),
			)
			for node := range prepared.nameCopies[phase] {
				prepared.nameCopies[phase][node] = &catalogNode{}
			}
		}
		if prepared.prefix != "" {
			prefixBits := len(prepared.prefix) * 8
			prepared.prefixPath = make([]*prefixNode, prefixBits+1)
			prepared.prefixBranches = make([]uint8, prefixBits)
		}
		for phase := range prepared.prefixCopies {
			if len(prepared.prefixPath) == 0 {
				continue
			}
			prepared.prefixCopies[phase] = make(
				[]*prefixNode,
				len(prepared.prefixPath),
			)
			for node := range prepared.prefixCopies[phase] {
				prepared.prefixCopies[phase][node] = &prefixNode{}
			}
		}
	}
	return mutation, nil
}

const (
	mutationStorageReserved uint32 = iota + 1
	mutationStorageClaimed
	mutationStorageReleased
)

// Discard releases a prepared mutation that was not accepted by the catalog.
func (mutation *Mutation) Discard() error {
	if mutation == nil {
		return nil
	}
	if !mutation.storageState.CompareAndSwap(
		mutationStorageReserved,
		mutationStorageReleased,
	) {
		return nil
	}
	return mutation.storage.discardPreparation(mutation.storageBytes)
}

func (mutation *Mutation) claim(storage *catalogStorage) error {
	if mutation == nil || mutation.storage != storage ||
		!mutation.storageState.CompareAndSwap(
			mutationStorageReserved,
			mutationStorageClaimed,
		) {
		return errors.New("jobmgr Function catalog: stale mutation storage")
	}
	return nil
}

func (mutation *Mutation) abortStorage(retainedCleanupBytes int64) error {
	if mutation == nil ||
		retainedCleanupBytes < 0 ||
		retainedCleanupBytes > mutation.cleanupStorageBytes ||
		!mutation.storageState.CompareAndSwap(
			mutationStorageClaimed,
			mutationStorageReleased,
		) {
		return errors.New("jobmgr Function catalog: stale mutation abort")
	}
	return mutation.storage.abortPreparation(
		mutation.storageBytes,
		retainedCleanupBytes,
	)
}

func (mutation *Mutation) publishStorage(published int64) error {
	if mutation == nil ||
		!mutation.storageState.CompareAndSwap(
			mutationStorageClaimed,
			mutationStorageReleased,
		) {
		return errors.New("jobmgr Function catalog: stale mutation publication")
	}
	return mutation.storage.publishPreparation(
		mutation.pathStorageBytes,
		mutation.cleanupStorageBytes,
		published,
	)
}

type routeChangeKey struct {
	name   string
	prefix string
}

type routeTransition struct {
	oldRoute           *route
	newRoute           *route
	oldAdmissionClosed bool
	tombstone          bool
}

type generationRemoval struct {
	generation *handlerGeneration
	references int
}

type mutationPhase uint8

const (
	mutationTopology mutationPhase = iota
	mutationGenerations
	mutationMaterialize
	mutationReady
)

type routeStepPhase uint8

const (
	routeStepNameDescend routeStepPhase = iota
	routeStepRoute
	routeStepPrefixDescend
	routeStepPrefixTerminal
	routeStepPrefixUnwind
	routeStepNameLeaf
	routeStepNameUnwind
	routeStepDone
)

type routeMutationStep struct {
	change        *preparedRouteChange
	phaseIndex    int
	state         routeStepPhase
	nameDepth     int
	prefixDepth   int
	nameNode      *catalogNode
	prefixNode    *prefixNode
	set           routeSet
	oldRoute      *route
	hadRoute      bool
	replacement   *route
	updatedPrefix *prefixNode
	updatedName   *catalogNode
	pathByteDelta int64
}

type MutationProgress struct {
	CompletedNodes int
	TotalNodes     int
	LastStepNodes  int
}

// MutationBuilder is catalog-owned after BeginMutation. PrepareStep is the
// only preparation transition and consumes at most the requested node quantum.
type MutationBuilder struct {
	catalog      *Catalog
	mutation     *Mutation
	phase        mutationPhase
	root         *catalogNode
	change       int
	generation   int
	routeOrdinal uint64
	step         routeMutationStep
	transitions  []routeTransition
	removals     []generationRemoval
	removalIndex map[*handlerGeneration]int
	removalCount int
	completed    int
	lastStep     int
	postimage    *MutationPostimage
	pathBytes    int64
	quiesced     bool
	failed       bool
	finished     bool
}

type MutationPostimage struct {
	builder   *MutationBuilder
	root      *catalogNode
	pathBytes int64
	finished  bool
}

// BeginMutation transfers one prepared mutation to the loop-owned catalog.
func (catalog *Catalog) startMutation(mutation *Mutation) (*MutationBuilder, error) {
	if catalog == nil || catalog.closed || mutation == nil ||
		catalog.mutation != nil || mutation.expectedVersion != catalog.version {
		if mutation != nil {
			_ = mutation.Discard()
		}
		return nil, errors.New("jobmgr Function catalog: invalid mutation admission")
	}
	additions := 0
	for index := range mutation.changes {
		if mutation.changes[index].declaration != nil {
			additions++
		}
	}
	if uint64(additions) > math.MaxUint64-catalog.nextRouteID ||
		uint64(len(mutation.generations)) > uint64(math.MaxUint32-catalog.nextGenerationID) {
		_ = mutation.Discard()
		return nil, errors.New("jobmgr Function catalog: mutation identity exhausted")
	}
	if err := mutation.claim(&catalog.storage); err != nil {
		return nil, err
	}
	builder := &mutation.builder
	*builder = MutationBuilder{
		catalog: catalog, mutation: mutation, phase: mutationTopology,
		root: catalog.routes, transitions: mutation.transitions,
		removals: mutation.removals, removalIndex: mutation.removalIndex,
		pathBytes: catalog.storage.published.Load(),
	}
	catalog.mutation = builder
	return builder, nil
}

// PrepareStep advances private postimage construction. The visible catalog is
// unchanged until CommitMutation swaps the completed root.
func (builder *MutationBuilder) PrepareStep(quantum int) (*MutationPostimage, bool, error) {
	if builder == nil || builder.catalog == nil || builder.mutation == nil ||
		builder.finished || builder.failed || builder.postimage != nil ||
		quantum <= 0 || quantum > MaximumMutationQuantum {
		return nil, false, errors.New("jobmgr Function catalog: invalid mutation preparation step")
	}
	builder.lastStep = 0
	for builder.lastStep < quantum && builder.phase != mutationReady {
		progressed, err := builder.advanceOne()
		if err != nil {
			builder.failed = true
			return nil, false, err
		}
		if progressed {
			builder.lastStep++
			builder.completed++
		}
	}
	if builder.phase == mutationReady {
		if builder.completed != builder.mutation.totalNodes {
			builder.failed = true
			return nil, false, errors.New("jobmgr Function catalog: mutation work accounting differs")
		}
		builder.postimage = &MutationPostimage{
			builder:   builder,
			root:      builder.root,
			pathBytes: builder.pathBytes,
		}
		return builder.postimage, true, nil
	}
	return nil, false, nil
}

// PrepareQuiesceStep validates route topology and closes predecessor admission
// without making the private postimage visible.
func (builder *MutationBuilder) PrepareQuiesceStep(quantum int) (bool, error) {
	if builder == nil || builder.catalog == nil || builder.mutation == nil ||
		builder.finished || builder.failed || builder.postimage != nil ||
		builder.phase != mutationTopology ||
		quantum <= 0 || quantum > MaximumMutationQuantum {
		return false, errors.New("jobmgr Function catalog: invalid mutation quiesce step")
	}
	builder.lastStep = 0
	for builder.lastStep < quantum && builder.phase == mutationTopology {
		progressed, err := builder.advanceOne()
		if err != nil {
			builder.failed = true
			return false, err
		}
		if progressed {
			builder.lastStep++
			builder.completed++
		}
	}
	return builder.phase == mutationGenerations, nil
}

func (builder *MutationBuilder) advanceOne() (bool, error) {
	for {
		switch builder.phase {
		case mutationTopology, mutationMaterialize:
			if builder.change == len(builder.mutation.changes) {
				if builder.phase == mutationTopology {
					builder.finishTopology()
					return false, nil
				}
				builder.phase = mutationReady
				return false, nil
			}
			if builder.step.change == nil {
				builder.startRouteStep()
			}
			done, err := builder.step.advance()
			if err != nil {
				return false, err
			}
			if done {
				index := builder.change
				if builder.phase == mutationTopology {
					builder.transitions[index].oldRoute = builder.step.oldRoute
					if builder.step.oldRoute != nil {
						generation := builder.step.oldRoute.handler
						removal, ok := builder.removalIndex[generation]
						if !ok {
							removal = builder.removalCount
							builder.removalIndex[generation] = removal
							builder.removals[removal].generation = generation
							builder.removalCount++
						}
						builder.removals[removal].references++
					}
				} else if builder.step.oldRoute != builder.transitions[index].oldRoute {
					return false, errors.New("jobmgr Function catalog: validated mutation topology changed")
				}
				builder.root = builder.step.updatedName
				if builder.phase == mutationMaterialize {
					builder.pathBytes += builder.step.pathByteDelta
					if builder.pathBytes < 0 ||
						builder.pathBytes > MaximumCatalogStorageBytes {
						return false, errors.New(
							"jobmgr Function catalog: mutation postimage storage exceeds process bound",
						)
					}
				}
				builder.step = routeMutationStep{}
				builder.change++
			}
			return true, nil
		case mutationGenerations:
			if builder.generation == len(builder.mutation.generations) {
				builder.phase = mutationMaterialize
				builder.change = 0
				builder.root = builder.catalog.routes
				continue
			}
			prepared := &builder.mutation.generations[builder.generation]
			refSlot := builder.catalog.nextGenerationID + uint32(builder.generation) + 1
			*prepared.generation = handlerGeneration{
				cleanupRef: jobmgr.FunctionCleanupRef{Slot: refSlot, Generation: 1},
				id:         prepared.declaration.ID, handler: prepared.declaration.Handler,
				cleanup:          prepared.declaration.Cleanup,
				executionCharged: prepared.declaration.Cleanup != nil,
			}
			prepared.initialized = true
			builder.generation++
			return true, nil
		default:
			return false, errors.New("jobmgr Function catalog: invalid mutation phase")
		}
	}
}

func (builder *MutationBuilder) finishTopology() {
	for index := range builder.transitions {
		transition := &builder.transitions[index]
		if transition.oldRoute == nil {
			continue
		}
		transition.oldAdmissionClosed = transition.oldRoute.admissionClosed
		transition.oldRoute.admissionClosed = true
	}
	builder.phase = mutationGenerations
	builder.change = 0
	builder.root = builder.catalog.routes
	builder.quiesced = true
}

func (builder *MutationBuilder) startRouteStep() {
	change := &builder.mutation.changes[builder.change]
	phaseIndex := 0
	replacement := change.placeholder
	if builder.phase == mutationMaterialize {
		phaseIndex = 1
		replacement = nil
		transition := &builder.transitions[builder.change]
		if change.declaration != nil {
			preparedGeneration := &builder.mutation.generations[change.generation-1]
			builder.routeOrdinal++
			*change.resolved = route{
				id:         builder.catalog.nextRouteID + builder.routeOrdinal,
				publicName: change.publicName, prefix: change.prefix,
				method: change.declaration.ID, handler: preparedGeneration.generation,
				resource:            change.declaration.Resource,
				cooperativeCancel:   change.declaration.CooperativeCancel,
				cooperativeDeadline: change.declaration.CooperativeDeadline,
				rawPayload:          change.declaration.RawPayload,
				transaction: cloneResourceTransactionDeclaration(
					change.declaration.Transaction,
				),
			}
			replacement = change.resolved
			transition.newRoute = replacement
		} else if transition.oldRoute != nil &&
			transition.oldRoute.invocationLeases != 0 {
			replacement = transition.oldRoute
			transition.tombstone = true
			replacement.retiringNamePath = change.namePath
			replacement.retiringPrefixPath = change.prefixPath
		}
	}
	builder.step = routeMutationStep{
		change: change, phaseIndex: phaseIndex, state: routeStepNameDescend,
		nameNode: builder.root, replacement: replacement,
	}
}

func (step *routeMutationStep) advance() (bool, error) {
	switch step.state {
	case routeStepNameDescend:
		return step.advanceNameDescend()
	case routeStepRoute:
		return step.advanceDirectRoute()
	case routeStepPrefixDescend:
		return step.advancePrefixDescend()
	case routeStepPrefixTerminal:
		return step.advancePrefixTerminal()
	case routeStepPrefixUnwind:
		return step.advancePrefixUnwind()
	case routeStepNameLeaf:
		return step.advanceNameLeaf()
	case routeStepNameUnwind:
		return step.advanceNameUnwind()
	case routeStepDone:
		return true, nil
	default:
		return false, errors.New("jobmgr Function catalog: invalid route mutation step")
	}
}

func (step *routeMutationStep) advanceNameDescend() (bool, error) {
	bits := len(step.change.publicName) * 8
	step.change.namePath[step.nameDepth] = step.nameNode
	if step.nameDepth == bits {
		if step.nameNode != nil && step.nameNode.present {
			step.set = step.nameNode.routes
		}
		if step.change.prefix == "" {
			step.state = routeStepRoute
		} else {
			step.state = routeStepPrefixDescend
			step.prefixNode = step.set.prefixes
		}
		return false, nil
	}
	branch := keyBit(step.change.publicName, step.nameDepth)
	step.change.nameBranches[step.nameDepth] = branch
	if step.nameNode != nil {
		step.nameNode = step.nameNode.child[branch]
	}
	step.nameDepth++
	return false, nil
}

func (step *routeMutationStep) advanceDirectRoute() (bool, error) {
	step.oldRoute = step.set.direct
	step.hadRoute = step.oldRoute != nil
	if step.oldRoute != nil && step.oldRoute.retiring {
		step.oldRoute = nil
	}
	if step.replacement == nil && step.oldRoute == nil {
		return false, errors.New("jobmgr Function catalog: mutation removes missing direct route")
	}
	step.set.direct = step.replacement
	step.state = routeStepNameLeaf
	return false, nil
}

func (step *routeMutationStep) advancePrefixDescend() (bool, error) {
	bits := len(step.change.prefix) * 8
	step.change.prefixPath[step.prefixDepth] = step.prefixNode
	if step.prefixDepth == bits {
		step.state = routeStepPrefixTerminal
		return false, nil
	}
	if step.prefixNode != nil && step.prefixNode.resolved != nil {
		return false, errors.New("jobmgr Function catalog: prefix overlaps a shorter prefix")
	}
	branch := keyBit(step.change.prefix, step.prefixDepth)
	step.change.prefixBranches[step.prefixDepth] = branch
	if step.prefixNode != nil {
		step.prefixNode = step.prefixNode.child[branch]
	}
	step.prefixDepth++
	return false, nil
}

func (step *routeMutationStep) advancePrefixTerminal() (bool, error) {
	original := step.change.prefixPath[step.prefixDepth]
	if original != nil {
		step.oldRoute = original.resolved
	}
	step.hadRoute = step.oldRoute != nil
	if step.oldRoute != nil && step.oldRoute.retiring {
		step.oldRoute = nil
	}
	if step.replacement == nil && step.oldRoute == nil {
		return false, errors.New("jobmgr Function catalog: mutation removes missing prefix route")
	}
	if step.replacement != nil && original != nil &&
		(original.child[0] != nil || original.child[1] != nil) {
		return false, errors.New("jobmgr Function catalog: prefix overlaps a longer prefix")
	}
	copyNode := step.change.prefixCopies[step.phaseIndex][step.prefixDepth]
	*copyNode = prefixNode{}
	if original != nil {
		*copyNode = *original
	}
	copyNode.resolved = step.replacement
	if copyNode.resolved == nil && copyNode.child[0] == nil && copyNode.child[1] == nil {
		step.updatedPrefix = nil
	} else {
		step.updatedPrefix = copyNode
	}
	step.updatePathStorage(
		original != nil,
		step.updatedPrefix != nil,
		prefixNodeStorageBytes,
	)
	if !step.hadRoute && step.replacement != nil {
		step.set.prefixCount++
	}
	if step.hadRoute && step.replacement == nil {
		step.set.prefixCount--
	}
	step.state = routeStepPrefixUnwind
	return false, nil
}

func (step *routeMutationStep) advancePrefixUnwind() (bool, error) {
	if step.prefixDepth == 0 {
		step.set.prefixes = step.updatedPrefix
		step.state = routeStepNameLeaf
		return false, nil
	}
	step.prefixDepth--
	original := step.change.prefixPath[step.prefixDepth]
	copyNode := step.change.prefixCopies[step.phaseIndex][step.prefixDepth]
	*copyNode = prefixNode{}
	if original != nil {
		*copyNode = *original
	}
	copyNode.child[step.change.prefixBranches[step.prefixDepth]] = step.updatedPrefix
	if copyNode.resolved == nil && copyNode.child[0] == nil && copyNode.child[1] == nil {
		step.updatedPrefix = nil
	} else {
		step.updatedPrefix = copyNode
	}
	step.updatePathStorage(
		original != nil,
		step.updatedPrefix != nil,
		prefixNodeStorageBytes,
	)
	return false, nil
}

func (step *routeMutationStep) advanceNameLeaf() (bool, error) {
	original := step.change.namePath[step.nameDepth]
	copyNode := step.change.nameCopies[step.phaseIndex][step.nameDepth]
	*copyNode = catalogNode{}
	if original != nil {
		*copyNode = *original
	}
	copyNode.routes = step.set
	copyNode.present = !step.set.empty()
	if !copyNode.present && copyNode.child[0] == nil && copyNode.child[1] == nil {
		step.updatedName = nil
	} else {
		step.updatedName = copyNode
	}
	step.updatePathStorage(
		original != nil,
		step.updatedName != nil,
		catalogNodeStorageBytes,
	)
	step.state = routeStepNameUnwind
	return false, nil
}

func (step *routeMutationStep) advanceNameUnwind() (bool, error) {
	if step.nameDepth == 0 {
		step.state = routeStepDone
		return true, nil
	}
	step.nameDepth--
	original := step.change.namePath[step.nameDepth]
	copyNode := step.change.nameCopies[step.phaseIndex][step.nameDepth]
	*copyNode = catalogNode{}
	if original != nil {
		*copyNode = *original
	}
	copyNode.child[step.change.nameBranches[step.nameDepth]] = step.updatedName
	if !copyNode.present && copyNode.child[0] == nil && copyNode.child[1] == nil {
		step.updatedName = nil
	} else {
		step.updatedName = copyNode
	}
	step.updatePathStorage(
		original != nil,
		step.updatedName != nil,
		catalogNodeStorageBytes,
	)
	return false, nil
}

func (step *routeMutationStep) updatePathStorage(
	hadOriginal bool,
	hasReplacement bool,
	bytes int64,
) {
	if step.phaseIndex != 1 {
		return
	}
	if hadOriginal {
		step.pathByteDelta -= bytes
	}
	if hasReplacement {
		step.pathByteDelta += bytes
	}
}

func keyBit(key string, depth int) uint8 {
	value := key[depth/8]
	return (value >> uint(7-depth%8)) & 1
}

func (builder *MutationBuilder) Progress() MutationProgress {
	if builder == nil || builder.mutation == nil {
		return MutationProgress{}
	}
	return MutationProgress{
		CompletedNodes: builder.completed,
		TotalNodes:     builder.mutation.totalNodes,
		LastStepNodes:  builder.lastStep,
	}
}

// CommitMutation performs the one visible root swap and returns cleanup work
// for old generations that became drained in the same turn.
func (catalog *Catalog) commitMutation(postimage *MutationPostimage, cleanups *[jobmgr.MaximumFunctionCleanupBatch]jobmgr.FunctionCleanupPlan) (int, error) {
	if catalog == nil || catalog.closed || postimage == nil || postimage.finished ||
		postimage.builder == nil || postimage.builder.postimage != postimage ||
		catalog.mutation != postimage.builder || postimage.builder.failed ||
		postimage.builder.mutation.expectedVersion != catalog.version || cleanups == nil {
		return 0, errors.New("jobmgr Function catalog: invalid mutation commit")
	}
	builder := postimage.builder
	for index := 0; index < builder.removalCount; index++ {
		removal := builder.removals[index]
		if removal.generation == nil || removal.generation.admissionClosed ||
			removal.generation.routeReferences < removal.references {
			return 0, errors.New("jobmgr Function catalog: invalid retired route references")
		}
	}
	for index := range builder.mutation.generations {
		prepared := &builder.mutation.generations[index]
		if !prepared.initialized || prepared.references <= 0 ||
			prepared.generation.cleanupRef.Slot != catalog.nextGenerationID+uint32(index)+1 {
			return 0, errors.New("jobmgr Function catalog: invalid prepared handler generation")
		}
	}
	if err := builder.mutation.publishStorage(postimage.pathBytes); err != nil {
		return 0, err
	}

	for index := range builder.mutation.generations {
		prepared := &builder.mutation.generations[index]
		prepared.generation.routeReferences = prepared.references
		catalog.generations[prepared.generation.cleanupRef] = prepared.generation
	}
	for _, transition := range builder.transitions {
		if transition.oldRoute != nil {
			catalog.unlinkCloseRoute(transition.oldRoute)
			transition.oldRoute.handler.routeReferences--
			catalog.routeCount--
		}
		if transition.newRoute != nil {
			catalog.appendCloseRoute(transition.newRoute)
			catalog.routeCount++
		}
		if transition.tombstone {
			retired := transition.oldRoute
			retired.retiring = true
			catalog.retireDrainedRoute(retired)
		}
	}
	catalog.routes = postimage.root
	deferred := catalog.deferredPrune
	catalog.deferredPrune = nil
	catalog.pruneRetiringRoutes(deferred)
	cleanupCount := 0
	for index := 0; index < builder.removalCount; index++ {
		generation := builder.removals[index].generation
		if generation.routeReferences != 0 {
			continue
		}
		generation.admissionClosed = true
		if generation.invocationLeases != 0 {
			continue
		}
		cleanup := catalog.cleanupDrainedGeneration(generation)
		if cleanup.Ref.Valid() {
			cleanups[cleanupCount] = cleanup
			cleanupCount++
		}
	}
	catalog.nextRouteID += postimage.builder.routeOrdinal
	catalog.nextGenerationID += uint32(len(builder.mutation.generations))
	catalog.version++
	catalog.mutation = nil
	builder.finished = true
	postimage.finished = true
	return cleanupCount, nil
}

// Abort releases ownership of every private generation initialized after
// topology validation. Cleanup work remains off-loop.
func (builder *MutationBuilder) Abort(cleanups *[jobmgr.MaximumFunctionCleanupBatch]jobmgr.FunctionCleanupPlan) (int, error) {
	if builder == nil || builder.catalog == nil || builder.finished || cleanups == nil {
		return 0, errors.New("jobmgr Function catalog: invalid mutation abort")
	}
	count := 0
	retainedCleanupBytes := int64(0)
	for index := range builder.mutation.generations {
		prepared := &builder.mutation.generations[index]
		if !prepared.initialized {
			continue
		}
		generation := prepared.generation
		if generation.executionCharged {
			retainedCleanupBytes += lifecycle.TaskChildExecutionBytes
		}
		if generation.cleanupRef.Valid() {
			builder.catalog.generations[generation.cleanupRef] = generation
		}
		generation.admissionClosed = true
		cleanup := builder.catalog.cleanupDrainedGeneration(generation)
		if cleanup.Ref.Valid() {
			cleanups[count] = cleanup
			count++
		}
	}
	if builder.catalog.mutation == builder {
		builder.catalog.mutation = nil
	}
	if builder.quiesced {
		for index := range builder.transitions {
			transition := &builder.transitions[index]
			if transition.oldRoute != nil {
				transition.oldRoute.admissionClosed =
					transition.oldAdmissionClosed
			}
		}
	}
	deferred := builder.catalog.deferredPrune
	builder.catalog.deferredPrune = nil
	builder.catalog.pruneRetiringRoutes(deferred)
	builder.finished = true
	if builder.postimage != nil {
		builder.postimage.finished = true
	}
	return count, builder.mutation.abortStorage(retainedCleanupBytes)
}

func (catalog *Catalog) BeginMutation(mutation jobmgr.FunctionCatalogMutation) error {
	prepared, ok := mutation.(*Mutation)
	if !ok {
		return errors.New("jobmgr Function catalog: foreign mutation")
	}
	_, err := catalog.startMutation(prepared)
	return err
}

func (catalog *Catalog) AdvanceMutationQuiesce(quantum int) (jobmgr.FunctionCatalogMutationProgress, error) {
	if catalog == nil || catalog.mutation == nil {
		return jobmgr.FunctionCatalogMutationProgress{}, errors.New("jobmgr Function catalog: no active mutation")
	}
	builder := catalog.mutation
	quiesced, err := builder.PrepareQuiesceStep(quantum)
	progress := builder.Progress()
	result := jobmgr.FunctionCatalogMutationProgress{
		CompletedNodes: progress.CompletedNodes,
		TotalNodes:     progress.TotalNodes,
		Version:        catalog.version,
		Quiesced:       quiesced,
	}
	return result, err
}

func (catalog *Catalog) ResumeMutation(mutation jobmgr.FunctionCatalogMutation) error {
	prepared, ok := mutation.(*Mutation)
	if catalog == nil || !ok || catalog.mutation == nil ||
		catalog.mutation != &prepared.builder ||
		catalog.mutation.phase != mutationGenerations ||
		catalog.mutation.failed || catalog.mutation.finished {
		return errors.New("jobmgr Function catalog: invalid mutation resume")
	}
	return nil
}

func (catalog *Catalog) AdvanceMutation(quantum int, cleanups *[jobmgr.MaximumFunctionCleanupBatch]jobmgr.FunctionCleanupPlan) (jobmgr.FunctionCatalogMutationProgress, int, error) {
	if catalog == nil || catalog.mutation == nil {
		return jobmgr.FunctionCatalogMutationProgress{}, 0, errors.New("jobmgr Function catalog: no active mutation")
	}
	builder := catalog.mutation
	postimage, done, err := builder.PrepareStep(quantum)
	progress := builder.Progress()
	result := jobmgr.FunctionCatalogMutationProgress{
		CompletedNodes: progress.CompletedNodes,
		TotalNodes:     progress.TotalNodes,
		Version:        catalog.version,
	}
	if err != nil {
		return result, 0, err
	}
	if !done {
		return result, 0, nil
	}
	count, err := catalog.commitMutation(postimage, cleanups)
	if err != nil {
		return result, 0, err
	}
	result.Version = catalog.version
	result.Done = true
	return result, count, nil
}

func (catalog *Catalog) AbortMutation(cleanups *[jobmgr.MaximumFunctionCleanupBatch]jobmgr.FunctionCleanupPlan) (int, error) {
	if catalog == nil || catalog.mutation == nil {
		return 0, errors.New("jobmgr Function catalog: no active mutation")
	}
	return catalog.mutation.Abort(cleanups)
}
