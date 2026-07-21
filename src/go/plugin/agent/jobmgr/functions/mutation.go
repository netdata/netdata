// SPDX-License-Identifier: GPL-3.0-or-later

package functions

import (
	"errors"
	"math"
	"sync/atomic"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
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
	declaration HandlerGenerationDeclaration // the handler-generation declaration
	generation  *handlerGeneration           // the built handler generation
	references  int                          // route references to assign at commit
	initialized bool                         // generation has been constructed
}

type preparedRouteChange struct {
	publicName  string       // route name key
	prefix      string       // optional prefix (empty = direct route)
	generation  int          // 1-based index into the mutation's generations (0 = removal)
	declaration *Declaration // desired route declaration (nil = removal)

	placeholder *route // topology-phase stand-in route
	resolved    *route // materialize-phase final route

	namePath     []*catalogNode    // captured name-trie node path
	nameBranches []uint8           // per-depth child branch taken on the name descent
	nameCopies   [2][]*catalogNode // pre-allocated copy-on-write node sets (quiesce/materialize)

	prefixPath     []*prefixNode    // captured prefix-trie node path
	prefixBranches []uint8          // per-depth child branch on the prefix descent
	prefixCopies   [2][]*prefixNode // pre-allocated prefix copy-on-write node sets
}

// Mutation is an off-loop-owned, fully allocated mutation request. NewMutation
// performs payload-relative validation and allocates every path-copy record so
// KernelLoop preparation can be node-bounded and allocation-free.
type Mutation struct {
	expectedVersion     uint64                     // catalog version this mutation was built against
	changes             []preparedRouteChange      // per-route changes to apply
	generations         []preparedGeneration       // new handler generations introduced
	totalNodes          int                        // total trie nodes touched (accounting)
	storage             *catalogStorage            // the catalog storage budget
	storageBytes        int64                      // total storage bytes reserved
	pathStorageBytes    int64                      // trie-path bytes reserved
	cleanupStorageBytes int64                      // cleanup-retention bytes reserved
	storageState        atomic.Uint32              // atomic publish/abort state of the reservation
	builder             MutationBuilder            // the mutation builder driving apply
	transitions         []routeTransition          // route transitions to commit
	removals            []generationRemoval        // generation removals to commit
	removalIndex        map[*handlerGeneration]int // generation -> removal index
}

func (*Mutation) FunctionCatalogMutation() {}

// NewMutation validates and owns one bounded route-change batch. The returned
// value may be admitted to exactly this Catalog.
func (c *Catalog) NewMutation(
	expectedVersion uint64,
	changes []RouteChange,
) (*Mutation, error) {
	if c == nil {
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
			catalogGenerationRetentionBytes,
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
	if err := c.storage.reservePreparation(totalStorageBytes); err != nil {
		return nil, err
	}
	mutation.storage = &c.storage
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
func (m *Mutation) Discard() error {
	if m == nil {
		return nil
	}
	if !m.storageState.CompareAndSwap(
		mutationStorageReserved,
		mutationStorageReleased,
	) {
		return nil
	}
	return m.storage.discardPreparation(m.storageBytes)
}

func (m *Mutation) claim(storage *catalogStorage) error {
	if m == nil || m.storage != storage ||
		!m.storageState.CompareAndSwap(
			mutationStorageReserved,
			mutationStorageClaimed,
		) {
		return errors.New("jobmgr Function catalog: stale mutation storage")
	}
	return nil
}

func (m *Mutation) abortStorage(retainedCleanupBytes int64) error {
	if m == nil ||
		retainedCleanupBytes < 0 ||
		retainedCleanupBytes > m.cleanupStorageBytes ||
		!m.storageState.CompareAndSwap(
			mutationStorageClaimed,
			mutationStorageReleased,
		) {
		return errors.New("jobmgr Function catalog: stale mutation abort")
	}
	return m.storage.abortPreparation(
		m.storageBytes,
		retainedCleanupBytes,
	)
}

func (m *Mutation) publishStorage(published int64) error {
	if m == nil ||
		!m.storageState.CompareAndSwap(
			mutationStorageClaimed,
			mutationStorageReleased,
		) {
		return errors.New("jobmgr Function catalog: stale mutation publication")
	}
	return m.storage.publishPreparation(
		m.pathStorageBytes,
		m.cleanupStorageBytes,
		published,
	)
}

type routeChangeKey struct {
	name   string
	prefix string
}

type routeTransition struct {
	oldRoute           *route // route being replaced (nil for a pure add)
	newRoute           *route // successor route (nil for a removal)
	oldAdmissionClosed bool   // old route's admission-closed state (for rollback)
	tombstone          bool   // transition is a removal tombstone
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
	oldRoute      *route               // route being replaced at this step
	updatedName   *catalogNode         // rewritten name-trie leaf
	updatedPrefix *prefixNode          // rewritten prefix-trie leaf
	change        *preparedRouteChange // the prepared change being applied
	replacement   *route               // the successor route
	nameNode      *catalogNode         // current name-trie node in the descent
	prefixNode    *prefixNode          // current prefix-trie node in the descent
	set           routeSet             // which route set (name/prefix) is being walked
	nameDepth     int                  // current depth in the name descent
	prefixDepth   int                  // current depth in the prefix descent
	phaseIndex    int                  // index within the current phase
	pathByteDelta int64                // storage byte delta from path copies
	hadRoute      bool                 // a route already existed at this key
	state         routeStepPhase       // step phase (topology/quiesce/materialize)
}

type MutationProgress struct {
	CompletedNodes int
	TotalNodes     int
}

// MutationBuilder is catalog-owned after BeginMutation. PrepareStep is the
// only preparation transition and consumes at most the requested node quantum.
type MutationBuilder struct {
	catalog      *Catalog                   // the catalog being mutated
	mutation     *Mutation                  // the mutation being applied
	phase        mutationPhase              // topology -> generations -> materialize state machine
	root         *catalogNode               // working postimage trie root during construction
	change       int                        // per-phase change cursor index
	generation   int                        // per-phase generation cursor index
	step         routeMutationStep          // scratch state for the current route step
	transitions  []routeTransition          // accumulated route transitions
	removals     []generationRemoval        // accumulated generation removals
	removalIndex map[*handlerGeneration]int // generation -> removal index
	removalCount int                        // valid prefix length of removals
	completed    int                        // count of completed steps
	lastStep     int                        // index of the last processed step
	postimage    *MutationPostimage         // the committed postimage once finished
	pathBytes    int64                      // trie-path bytes accounted so far
	quiesced     bool                       // predecessors quiesced (admission closed)
	failed       bool                       // mutation failed and must be aborted
	finished     bool                       // mutation finished
}

type MutationPostimage struct {
	builder   *MutationBuilder // owning builder
	root      *catalogNode     // the postimage trie root
	pathBytes int64            // trie-path bytes in the postimage
	finished  bool             // postimage committed
}

// BeginMutation transfers one prepared mutation to the loop-owned catalog.
func (c *Catalog) startMutation(mutation *Mutation) (*MutationBuilder, error) {
	if c == nil || c.closed || mutation == nil ||
		c.mutation != nil || mutation.expectedVersion != c.version {
		if mutation != nil {
			_ = mutation.Discard()
		}
		return nil, errors.New("jobmgr Function catalog: invalid mutation admission")
	}
	if uint64(len(mutation.generations)) > uint64(math.MaxUint32-c.nextGenerationID) {
		_ = mutation.Discard()
		return nil, errors.New("jobmgr Function catalog: mutation identity exhausted")
	}
	if err := mutation.claim(&c.storage); err != nil {
		return nil, err
	}
	builder := &mutation.builder
	*builder = MutationBuilder{
		catalog: c, mutation: mutation, phase: mutationTopology,
		root: c.routes, transitions: mutation.transitions,
		removals: mutation.removals, removalIndex: mutation.removalIndex,
		pathBytes: c.storage.published.Load(),
	}
	c.mutation = builder
	return builder, nil
}

// PrepareStep advances private postimage construction. The visible catalog is
// unchanged until CommitMutation swaps the completed root.
func (mb *MutationBuilder) PrepareStep(quantum int) (*MutationPostimage, bool, error) {
	if mb == nil || mb.catalog == nil || mb.mutation == nil ||
		mb.finished || mb.failed || mb.postimage != nil ||
		quantum <= 0 || quantum > MaximumMutationQuantum {
		return nil, false, errors.New("jobmgr Function catalog: invalid mutation preparation step")
	}
	mb.lastStep = 0
	for mb.lastStep < quantum && mb.phase != mutationReady {
		progressed, err := mb.advanceOne()
		if err != nil {
			mb.failed = true
			return nil, false, err
		}
		if progressed {
			mb.lastStep++
			mb.completed++
		}
	}
	if mb.phase == mutationReady {
		if mb.completed != mb.mutation.totalNodes {
			mb.failed = true
			return nil, false, errors.New("jobmgr Function catalog: mutation work accounting differs")
		}
		mb.postimage = &MutationPostimage{
			builder:   mb,
			root:      mb.root,
			pathBytes: mb.pathBytes,
		}
		return mb.postimage, true, nil
	}
	return nil, false, nil
}

// PrepareQuiesceStep validates route topology and closes predecessor admission
// without making the private postimage visible.
func (mb *MutationBuilder) PrepareQuiesceStep(quantum int) (bool, error) {
	if mb == nil || mb.catalog == nil || mb.mutation == nil ||
		mb.finished || mb.failed || mb.postimage != nil ||
		mb.phase != mutationTopology ||
		quantum <= 0 || quantum > MaximumMutationQuantum {
		return false, errors.New("jobmgr Function catalog: invalid mutation quiesce step")
	}
	mb.lastStep = 0
	for mb.lastStep < quantum && mb.phase == mutationTopology {
		progressed, err := mb.advanceOne()
		if err != nil {
			mb.failed = true
			return false, err
		}
		if progressed {
			mb.lastStep++
			mb.completed++
		}
	}
	return mb.phase == mutationGenerations, nil
}

func (mb *MutationBuilder) advanceOne() (bool, error) {
	for {
		switch mb.phase {
		case mutationTopology, mutationMaterialize:
			if mb.change == len(mb.mutation.changes) {
				if mb.phase == mutationTopology {
					mb.finishTopology()
					return false, nil
				}
				mb.phase = mutationReady
				return false, nil
			}
			if mb.step.change == nil {
				mb.startRouteStep()
			}
			done, err := mb.step.advance()
			if err != nil {
				return false, err
			}
			if done {
				index := mb.change
				if mb.phase == mutationTopology {
					mb.transitions[index].oldRoute = mb.step.oldRoute
					if mb.step.oldRoute != nil {
						generation := mb.step.oldRoute.handler
						removal, ok := mb.removalIndex[generation]
						if !ok {
							removal = mb.removalCount
							mb.removalIndex[generation] = removal
							mb.removals[removal].generation = generation
							mb.removalCount++
						}
						mb.removals[removal].references++
					}
				} else if mb.step.oldRoute != mb.transitions[index].oldRoute {
					return false, errors.New("jobmgr Function catalog: validated mutation topology changed")
				}
				mb.root = mb.step.updatedName
				if mb.phase == mutationMaterialize {
					mb.pathBytes += mb.step.pathByteDelta
					if mb.pathBytes < 0 ||
						mb.pathBytes > MaximumCatalogStorageBytes {
						return false, errors.New(
							"jobmgr Function catalog: mutation postimage storage exceeds process bound",
						)
					}
				}
				mb.step = routeMutationStep{}
				mb.change++
			}
			return true, nil
		case mutationGenerations:
			if mb.generation == len(mb.mutation.generations) {
				mb.phase = mutationMaterialize
				mb.change = 0
				mb.root = mb.catalog.routes
				continue
			}
			prepared := &mb.mutation.generations[mb.generation]
			refSlot := mb.catalog.nextGenerationID + uint32(mb.generation) + 1
			*prepared.generation = handlerGeneration{
				cleanupRef: jobmgr.FunctionCleanupRef{Slot: refSlot, Generation: 1},
				id:         prepared.declaration.ID, handler: prepared.declaration.Handler,
				cleanup:          prepared.declaration.Cleanup,
				retentionCharged: prepared.declaration.Cleanup != nil,
			}
			prepared.initialized = true
			mb.generation++
			return true, nil
		default:
			return false, errors.New("jobmgr Function catalog: invalid mutation phase")
		}
	}
}

func (mb *MutationBuilder) finishTopology() {
	for index := range mb.transitions {
		transition := &mb.transitions[index]
		if transition.oldRoute == nil {
			continue
		}
		transition.oldAdmissionClosed = transition.oldRoute.admissionClosed
		transition.oldRoute.admissionClosed = true
	}
	mb.phase = mutationGenerations
	mb.change = 0
	mb.root = mb.catalog.routes
	mb.quiesced = true
}

func (mb *MutationBuilder) startRouteStep() {
	change := &mb.mutation.changes[mb.change]
	phaseIndex := 0
	replacement := change.placeholder
	if mb.phase == mutationMaterialize {
		phaseIndex = 1
		replacement = nil
		transition := &mb.transitions[mb.change]
		if change.declaration != nil {
			preparedGeneration := &mb.mutation.generations[change.generation-1]
			*change.resolved = route{
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
	mb.step = routeMutationStep{
		change: change, phaseIndex: phaseIndex, state: routeStepNameDescend,
		nameNode: mb.root, replacement: replacement,
	}
}

func (rms *routeMutationStep) advance() (bool, error) {
	switch rms.state {
	case routeStepNameDescend:
		return rms.advanceNameDescend()
	case routeStepRoute:
		return rms.advanceDirectRoute()
	case routeStepPrefixDescend:
		return rms.advancePrefixDescend()
	case routeStepPrefixTerminal:
		return rms.advancePrefixTerminal()
	case routeStepPrefixUnwind:
		return rms.advancePrefixUnwind()
	case routeStepNameLeaf:
		return rms.advanceNameLeaf()
	case routeStepNameUnwind:
		return rms.advanceNameUnwind()
	case routeStepDone:
		return true, nil
	default:
		return false, errors.New("jobmgr Function catalog: invalid route mutation step")
	}
}

func (rms *routeMutationStep) advanceNameDescend() (bool, error) {
	bits := len(rms.change.publicName) * 8
	rms.change.namePath[rms.nameDepth] = rms.nameNode
	if rms.nameDepth == bits {
		if rms.nameNode != nil && rms.nameNode.present {
			rms.set = rms.nameNode.routes
		}
		if rms.change.prefix == "" {
			rms.state = routeStepRoute
		} else {
			rms.state = routeStepPrefixDescend
			rms.prefixNode = rms.set.prefixes
		}
		return false, nil
	}
	branch := keyBit(rms.change.publicName, rms.nameDepth)
	rms.change.nameBranches[rms.nameDepth] = branch
	if rms.nameNode != nil {
		rms.nameNode = rms.nameNode.child[branch]
	}
	rms.nameDepth++
	return false, nil
}

func (rms *routeMutationStep) advanceDirectRoute() (bool, error) {
	rms.oldRoute = rms.set.direct
	rms.hadRoute = rms.oldRoute != nil
	if rms.oldRoute != nil && rms.oldRoute.retiring {
		rms.oldRoute = nil
	}
	if rms.replacement == nil && rms.oldRoute == nil {
		return false, errors.New("jobmgr Function catalog: mutation removes missing direct route")
	}
	rms.set.direct = rms.replacement
	rms.state = routeStepNameLeaf
	return false, nil
}

func (rms *routeMutationStep) advancePrefixDescend() (bool, error) {
	bits := len(rms.change.prefix) * 8
	rms.change.prefixPath[rms.prefixDepth] = rms.prefixNode
	if rms.prefixDepth == bits {
		rms.state = routeStepPrefixTerminal
		return false, nil
	}
	if rms.prefixNode != nil && rms.prefixNode.resolved != nil {
		return false, errors.New("jobmgr Function catalog: prefix overlaps a shorter prefix")
	}
	branch := keyBit(rms.change.prefix, rms.prefixDepth)
	rms.change.prefixBranches[rms.prefixDepth] = branch
	if rms.prefixNode != nil {
		rms.prefixNode = rms.prefixNode.child[branch]
	}
	rms.prefixDepth++
	return false, nil
}

func (rms *routeMutationStep) advancePrefixTerminal() (bool, error) {
	original := rms.change.prefixPath[rms.prefixDepth]
	if original != nil {
		rms.oldRoute = original.resolved
	}
	rms.hadRoute = rms.oldRoute != nil
	if rms.oldRoute != nil && rms.oldRoute.retiring {
		rms.oldRoute = nil
	}
	if rms.replacement == nil && rms.oldRoute == nil {
		return false, errors.New("jobmgr Function catalog: mutation removes missing prefix route")
	}
	if rms.replacement != nil && original != nil &&
		(original.child[0] != nil || original.child[1] != nil) {
		return false, errors.New("jobmgr Function catalog: prefix overlaps a longer prefix")
	}
	copyNode := rms.change.prefixCopies[rms.phaseIndex][rms.prefixDepth]
	*copyNode = prefixNode{}
	if original != nil {
		*copyNode = *original
	}
	copyNode.resolved = rms.replacement
	if copyNode.resolved == nil && copyNode.child[0] == nil && copyNode.child[1] == nil {
		rms.updatedPrefix = nil
	} else {
		rms.updatedPrefix = copyNode
	}
	rms.updatePathStorage(
		original != nil,
		rms.updatedPrefix != nil,
		prefixNodeStorageBytes,
	)
	if !rms.hadRoute && rms.replacement != nil {
		rms.set.prefixCount++
	}
	if rms.hadRoute && rms.replacement == nil {
		rms.set.prefixCount--
	}
	rms.state = routeStepPrefixUnwind
	return false, nil
}

func (rms *routeMutationStep) advancePrefixUnwind() (bool, error) {
	if rms.prefixDepth == 0 {
		rms.set.prefixes = rms.updatedPrefix
		rms.state = routeStepNameLeaf
		return false, nil
	}
	rms.prefixDepth--
	original := rms.change.prefixPath[rms.prefixDepth]
	copyNode := rms.change.prefixCopies[rms.phaseIndex][rms.prefixDepth]
	*copyNode = prefixNode{}
	if original != nil {
		*copyNode = *original
	}
	copyNode.child[rms.change.prefixBranches[rms.prefixDepth]] = rms.updatedPrefix
	if copyNode.resolved == nil && copyNode.child[0] == nil && copyNode.child[1] == nil {
		rms.updatedPrefix = nil
	} else {
		rms.updatedPrefix = copyNode
	}
	rms.updatePathStorage(
		original != nil,
		rms.updatedPrefix != nil,
		prefixNodeStorageBytes,
	)
	return false, nil
}

func (rms *routeMutationStep) advanceNameLeaf() (bool, error) {
	original := rms.change.namePath[rms.nameDepth]
	copyNode := rms.change.nameCopies[rms.phaseIndex][rms.nameDepth]
	*copyNode = catalogNode{}
	if original != nil {
		*copyNode = *original
	}
	copyNode.routes = rms.set
	copyNode.present = !rms.set.empty()
	if !copyNode.present && copyNode.child[0] == nil && copyNode.child[1] == nil {
		rms.updatedName = nil
	} else {
		rms.updatedName = copyNode
	}
	rms.updatePathStorage(
		original != nil,
		rms.updatedName != nil,
		catalogNodeStorageBytes,
	)
	rms.state = routeStepNameUnwind
	return false, nil
}

func (rms *routeMutationStep) advanceNameUnwind() (bool, error) {
	if rms.nameDepth == 0 {
		rms.state = routeStepDone
		return true, nil
	}
	rms.nameDepth--
	original := rms.change.namePath[rms.nameDepth]
	copyNode := rms.change.nameCopies[rms.phaseIndex][rms.nameDepth]
	*copyNode = catalogNode{}
	if original != nil {
		*copyNode = *original
	}
	copyNode.child[rms.change.nameBranches[rms.nameDepth]] = rms.updatedName
	if !copyNode.present && copyNode.child[0] == nil && copyNode.child[1] == nil {
		rms.updatedName = nil
	} else {
		rms.updatedName = copyNode
	}
	rms.updatePathStorage(
		original != nil,
		rms.updatedName != nil,
		catalogNodeStorageBytes,
	)
	return false, nil
}

func (rms *routeMutationStep) updatePathStorage(
	hadOriginal bool,
	hasReplacement bool,
	bytes int64,
) {
	if rms.phaseIndex != 1 {
		return
	}
	if hadOriginal {
		rms.pathByteDelta -= bytes
	}
	if hasReplacement {
		rms.pathByteDelta += bytes
	}
}

func keyBit(key string, depth int) uint8 {
	value := key[depth/8]
	return (value >> uint(7-depth%8)) & 1
}

func (mb *MutationBuilder) Progress() MutationProgress {
	if mb == nil || mb.mutation == nil {
		return MutationProgress{}
	}
	return MutationProgress{
		CompletedNodes: mb.completed,
		TotalNodes:     mb.mutation.totalNodes,
	}
}

// CommitMutation performs the one visible root swap and returns cleanup work
// for old generations that became drained in the same turn.
func (c *Catalog) commitMutation(postimage *MutationPostimage, cleanups *[jobmgr.MaximumFunctionCleanupBatch]jobmgr.FunctionCleanupPlan) (int, error) {
	if c == nil || c.closed || postimage == nil || postimage.finished ||
		postimage.builder == nil || postimage.builder.postimage != postimage ||
		c.mutation != postimage.builder || postimage.builder.failed ||
		postimage.builder.mutation.expectedVersion != c.version || cleanups == nil {
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
			prepared.generation.cleanupRef.Slot != c.nextGenerationID+uint32(index)+1 {
			return 0, errors.New("jobmgr Function catalog: invalid prepared handler generation")
		}
	}
	if err := builder.mutation.publishStorage(postimage.pathBytes); err != nil {
		return 0, err
	}

	for index := range builder.mutation.generations {
		prepared := &builder.mutation.generations[index]
		prepared.generation.routeReferences = prepared.references
		c.generations[prepared.generation.cleanupRef] = prepared.generation
	}
	for _, transition := range builder.transitions {
		if transition.oldRoute != nil {
			c.unlinkCloseRoute(transition.oldRoute)
			transition.oldRoute.handler.routeReferences--
			c.routeCount--
		}
		if transition.newRoute != nil {
			c.appendCloseRoute(transition.newRoute)
			c.routeCount++
		}
		if transition.tombstone {
			retired := transition.oldRoute
			retired.retiring = true
			c.retireDrainedRoute(retired)
		}
	}
	c.routes = postimage.root
	deferred := c.deferredPrune
	c.deferredPrune = nil
	c.pruneRetiringRoutes(deferred)
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
		cleanup := c.cleanupDrainedGeneration(generation)
		if cleanup.Ref.Valid() {
			cleanups[cleanupCount] = cleanup
			cleanupCount++
		}
	}
	c.nextGenerationID += uint32(len(builder.mutation.generations))
	c.version++
	c.mutation = nil
	builder.finished = true
	postimage.finished = true
	return cleanupCount, nil
}

// Abort releases ownership of every private generation initialized after
// topology validation. Cleanup work remains off-loop.
func (mb *MutationBuilder) Abort(cleanups *[jobmgr.MaximumFunctionCleanupBatch]jobmgr.FunctionCleanupPlan) (int, error) {
	if mb == nil || mb.catalog == nil || mb.finished || cleanups == nil {
		return 0, errors.New("jobmgr Function catalog: invalid mutation abort")
	}
	count := 0
	retainedCleanupBytes := int64(0)
	for index := range mb.mutation.generations {
		prepared := &mb.mutation.generations[index]
		if !prepared.initialized {
			continue
		}
		generation := prepared.generation
		if generation.retentionCharged {
			retainedCleanupBytes += catalogGenerationRetentionBytes
		}
		if generation.cleanupRef.Valid() {
			mb.catalog.generations[generation.cleanupRef] = generation
		}
		generation.admissionClosed = true
		cleanup := mb.catalog.cleanupDrainedGeneration(generation)
		if cleanup.Ref.Valid() {
			cleanups[count] = cleanup
			count++
		}
	}
	if mb.catalog.mutation == mb {
		mb.catalog.mutation = nil
	}
	if mb.quiesced {
		for index := range mb.transitions {
			transition := &mb.transitions[index]
			if transition.oldRoute != nil {
				transition.oldRoute.admissionClosed =
					transition.oldAdmissionClosed
			}
		}
	}
	deferred := mb.catalog.deferredPrune
	mb.catalog.deferredPrune = nil
	mb.catalog.pruneRetiringRoutes(deferred)
	mb.finished = true
	if mb.postimage != nil {
		mb.postimage.finished = true
	}
	return count, mb.mutation.abortStorage(retainedCleanupBytes)
}

func (c *Catalog) BeginMutation(mutation jobmgr.FunctionCatalogMutation) error {
	prepared, ok := mutation.(*Mutation)
	if !ok {
		return errors.New("jobmgr Function catalog: foreign mutation")
	}
	_, err := c.startMutation(prepared)
	return err
}

func (c *Catalog) AdvanceMutationQuiesce(quantum int) (jobmgr.FunctionCatalogMutationProgress, error) {
	if c == nil || c.mutation == nil {
		return jobmgr.FunctionCatalogMutationProgress{}, errors.New("jobmgr Function catalog: no active mutation")
	}
	builder := c.mutation
	quiesced, err := builder.PrepareQuiesceStep(quantum)
	progress := builder.Progress()
	result := jobmgr.FunctionCatalogMutationProgress{
		CompletedNodes: progress.CompletedNodes,
		TotalNodes:     progress.TotalNodes,
		Version:        c.version,
		Quiesced:       quiesced,
	}
	return result, err
}

func (c *Catalog) ResumeMutation(mutation jobmgr.FunctionCatalogMutation) error {
	prepared, ok := mutation.(*Mutation)
	if c == nil || !ok || c.mutation == nil ||
		c.mutation != &prepared.builder ||
		c.mutation.phase != mutationGenerations ||
		c.mutation.failed || c.mutation.finished {
		return errors.New("jobmgr Function catalog: invalid mutation resume")
	}
	return nil
}

func (c *Catalog) AdvanceMutation(quantum int, cleanups *[jobmgr.MaximumFunctionCleanupBatch]jobmgr.FunctionCleanupPlan) (jobmgr.FunctionCatalogMutationProgress, int, error) {
	if c == nil || c.mutation == nil {
		return jobmgr.FunctionCatalogMutationProgress{}, 0, errors.New("jobmgr Function catalog: no active mutation")
	}
	builder := c.mutation
	postimage, done, err := builder.PrepareStep(quantum)
	progress := builder.Progress()
	result := jobmgr.FunctionCatalogMutationProgress{
		CompletedNodes: progress.CompletedNodes,
		TotalNodes:     progress.TotalNodes,
		Version:        c.version,
	}
	if err != nil {
		return result, 0, err
	}
	if !done {
		return result, 0, nil
	}
	count, err := c.commitMutation(postimage, cleanups)
	if err != nil {
		return result, 0, err
	}
	result.Version = c.version
	result.Done = true
	return result, count, nil
}

func (c *Catalog) AbortMutation(cleanups *[jobmgr.MaximumFunctionCleanupBatch]jobmgr.FunctionCleanupPlan) (int, error) {
	if c == nil || c.mutation == nil {
		return 0, errors.New("jobmgr Function catalog: no active mutation")
	}
	return c.mutation.Abort(cleanups)
}
