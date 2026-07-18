// SPDX-License-Identifier: GPL-3.0-or-later

package functions

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
)

const (
	maximumDeclarationMetadataBytes = 15_487
	MaximumCloseQuantum             = jobmgr.MaximumFunctionCloseQuantum
	invalidDynCfgLaneScope          = "\x00dyncfg-invalid"
)

var dynCfgJobNameReplacer = strings.NewReplacer(" ", "_", ":", "_")

type HandlerInput struct {
	UID          string
	Method       string
	Args         []string
	Payload      []byte
	ContentType  string
	Permissions  string
	CallerSource string
	Timeout      time.Duration
	HasPayload   bool
}

type Handler func(context.Context, HandlerInput) (lifecycle.SealedResult, error)

type ResourceTransactionHandler func(
	context.Context,
	HandlerInput,
	lifecycle.ReadyResource,
	lifecycle.ResourceTransactionScope,
	lifecycle.LongLivedPermit,
) (lifecycle.PreparedResourceTransaction, error)

type ResourceTransactionCommand struct {
	Name              string
	AllocateSuccessor bool
}

type ResourceTransactionDeclaration struct {
	Prepare         ResourceTransactionHandler
	Permit          lifecycle.LongLivedPlan
	CommandArgument uint16
	GlobalClaim     string
	Commands        []ResourceTransactionCommand
}

// HandlerGenerationDeclaration describes one job- or module-owned handler
// generation. Multiple routes may reference the same declaration; the catalog
// copies it and owns exactly one cleanup for the resulting generation.
type HandlerGenerationDeclaration struct {
	ID      string
	Handler Handler
	Cleanup func(context.Context) error
}

type LanePolicyKind uint8

const (
	LaneByRoute LanePolicyKind = iota + 1
	LaneByArgument
	LaneByDynCfgJob
)

type LanePolicy struct {
	Kind        LanePolicyKind
	Argument    uint16
	Prefix      string
	ScopePrefix string
}

func RouteLane() LanePolicy {
	return LanePolicy{Kind: LaneByRoute}
}

func ArgumentLane(index uint16) LanePolicy {
	return LanePolicy{Kind: LaneByArgument, Argument: index}
}

func DynCfgJobLane(index uint16, prefix string) LanePolicy {
	return LanePolicy{
		Kind: LaneByDynCfgJob, Argument: index, Prefix: prefix,
	}
}

func ScopedDynCfgJobLane(index uint16, prefix, scopePrefix string) LanePolicy {
	return LanePolicy{
		Kind: LaneByDynCfgJob, Argument: index, Prefix: prefix,
		ScopePrefix: scopePrefix,
	}
}

func (policy LanePolicy) validate() error {
	switch policy.Kind {
	case LaneByRoute:
		if policy.Argument != 0 || policy.Prefix != "" ||
			policy.ScopePrefix != "" {
			return errors.New("jobmgr Function catalog: route lane has an argument")
		}
	case LaneByArgument:
		if policy.Argument >= 1_024 || policy.Prefix != "" ||
			policy.ScopePrefix != "" {
			return errors.New("jobmgr Function catalog: lane argument exceeds request bound")
		}
	case LaneByDynCfgJob:
		if policy.Argument >= 1_024 ||
			policy.Prefix == "" ||
			len(policy.Prefix)+len(policy.ScopePrefix) >
				maximumDeclarationMetadataBytes ||
			strings.TrimSpace(policy.ScopePrefix) != policy.ScopePrefix {
			return errors.New(
				"jobmgr Function catalog: invalid DynCfg job lane policy",
			)
		}
	default:
		return errors.New("jobmgr Function catalog: unknown lane policy")
	}
	return nil
}

func (policy LanePolicy) resolve(routeID uint64, arguments []string) jobmgr.FunctionLane {
	lane := jobmgr.FunctionLane{Route: routeID}
	if policy.Kind == LaneByArgument &&
		int(policy.Argument) < len(arguments) {
		lane.Scope = arguments[policy.Argument]
	} else if policy.Kind == LaneByDynCfgJob {
		lane.Scope = resolveDynCfgJobLane(policy, arguments)
		if lane.Scope != "" {
			lane.Scope = policy.ScopePrefix + lane.Scope
		}
		lane.Resource = true
	}
	return lane
}

func resolveDynCfgJobLane(
	policy LanePolicy,
	arguments []string,
) string {
	if int(policy.Argument) >= len(arguments) {
		return invalidDynCfgLaneScope
	}
	target := strings.TrimPrefix(
		arguments[policy.Argument],
		policy.Prefix,
	)
	target = strings.TrimPrefix(target, ":")
	if target == "" {
		if len(arguments) > 2 &&
			len(arguments) > 1 &&
			strings.EqualFold(arguments[1], "add") {
			name := dynCfgJobNameReplacer.Replace(arguments[2])
			if name != "" {
				return name
			}
		}
		return invalidDynCfgLaneScope
	}
	module, name, hasName := strings.Cut(target, ":")
	if module == "" {
		return invalidDynCfgLaneScope
	}
	if len(arguments) > 2 &&
		len(arguments) > 1 &&
		strings.EqualFold(arguments[1], "add") {
		name = dynCfgJobNameReplacer.Replace(arguments[2])
		hasName = name != ""
	}
	if !hasName || name == module {
		return module
	}
	return module + "_" + name
}

type Declaration struct {
	ID                  string
	Generation          *HandlerGenerationDeclaration
	Transaction         *ResourceTransactionDeclaration
	PublicName          string
	Prefix              string
	Lane                LanePolicy
	CooperativeCancel   bool
	CooperativeDeadline bool
	RawPayload          bool
}

type HandlerGenerationCensus struct {
	ID               string
	RouteReferences  int
	InvocationLeases int
	AdmissionClosed  bool
	CleanupPending   bool
	Cleaned          bool
	CleanupFailed    bool
}

type CatalogCensus = jobmgr.FunctionCatalogCensus

type handlerGeneration struct {
	cleanupRef jobmgr.FunctionCleanupRef
	id         string
	handler    Handler
	cleanup    func(context.Context) error

	routeReferences  int
	invocationLeases int
	admissionClosed  bool
	cleanupPending   bool
	cleaned          bool
	cleanupFailed    bool
}

func (generation *handlerGeneration) RunTask(ctx context.Context) (lifecycle.TaskOutcome, error) {
	if generation == nil || !generation.cleanupPending || generation.cleanup == nil {
		return lifecycle.NoValueOutcome(), errors.New("jobmgr Function handler: invalid cleanup work")
	}
	return lifecycle.NoValueOutcome(), generation.cleanup(ctx)
}

func (generation *handlerGeneration) census() HandlerGenerationCensus {
	if generation == nil {
		return HandlerGenerationCensus{}
	}
	return HandlerGenerationCensus{
		ID: generation.id, RouteReferences: generation.routeReferences,
		InvocationLeases: generation.invocationLeases, AdmissionClosed: generation.admissionClosed,
		CleanupPending: generation.cleanupPending, Cleaned: generation.cleaned,
		CleanupFailed: generation.cleanupFailed,
	}
}

type route struct {
	id                  uint64
	publicName          string
	prefix              string
	method              string
	handler             *handlerGeneration
	lane                LanePolicy
	cooperativeCancel   bool
	cooperativeDeadline bool
	rawPayload          bool
	transaction         *ResourceTransactionDeclaration
	admissionClosed     bool
	invocationLeases    int
	retiring            bool
	retiringDrained     bool
	retiringNamePath    []*catalogNode
	retiringPrefixPath  []*prefixNode
	retiringNext        *route
	closePrevious       *route
	closeNext           *route
}

type prefixNode struct {
	resolved *route
	child    [2]*prefixNode
}

type routeSet struct {
	direct      *route
	prefixes    *prefixNode
	prefixCount int
}

func (set routeSet) empty() bool {
	return set.direct == nil && set.prefixCount == 0
}

func (set routeSet) resolve(arguments []string) *route {
	if set.prefixCount == 0 {
		return set.direct
	}
	if len(arguments) == 0 {
		return nil
	}
	node := set.prefixes
	argument := arguments[0]
	for byteIndex := 0; byteIndex < len(argument) && node != nil; byteIndex++ {
		value := argument[byteIndex]
		for bitIndex := 7; bitIndex >= 0 && node != nil; bitIndex-- {
			node = node.child[(value>>uint(bitIndex))&1]
		}
		if node != nil && node.resolved != nil {
			return node.resolved
		}
	}
	return nil
}

func insertInitialPrefix(root *prefixNode, prefix string, resolved *route) (*prefixNode, error) {
	if root == nil {
		root = &prefixNode{}
	}
	node := root
	for byteIndex := 0; byteIndex < len(prefix); byteIndex++ {
		if node.resolved != nil {
			return nil, errors.New("jobmgr Function catalog: prefix overlaps a shorter prefix")
		}
		value := prefix[byteIndex]
		for bitIndex := 7; bitIndex >= 0; bitIndex-- {
			branch := (value >> uint(bitIndex)) & 1
			if node.child[branch] == nil {
				node.child[branch] = &prefixNode{}
			}
			node = node.child[branch]
		}
	}
	if node.resolved != nil {
		return nil, errors.New("jobmgr Function catalog: duplicate prefix")
	}
	if node.child[0] != nil || node.child[1] != nil {
		return nil, errors.New("jobmgr Function catalog: prefix overlaps a longer prefix")
	}
	node.resolved = resolved
	return root, nil
}

type catalogNode struct {
	routes  routeSet
	present bool
	child   [2]*catalogNode
}

func catalogRouteSet(root *catalogNode, name string) (routeSet, bool) {
	node := root
	for byteIndex := 0; byteIndex < len(name) && node != nil; byteIndex++ {
		value := name[byteIndex]
		for bitIndex := 7; bitIndex >= 0 && node != nil; bitIndex-- {
			node = node.child[(value>>uint(bitIndex))&1]
		}
	}
	if node == nil || !node.present {
		return routeSet{}, false
	}
	return node.routes, true
}

func setInitialCatalogRouteSet(root *catalogNode, name string, set routeSet) *catalogNode {
	if root == nil {
		root = &catalogNode{}
	}
	node := root
	for byteIndex := 0; byteIndex < len(name); byteIndex++ {
		value := name[byteIndex]
		for bitIndex := 7; bitIndex >= 0; bitIndex-- {
			branch := (value >> uint(bitIndex)) & 1
			if node.child[branch] == nil {
				node.child[branch] = &catalogNode{}
			}
			node = node.child[branch]
		}
	}
	node.routes = set
	node.present = !set.empty()
	return root
}

type invocationSlot struct {
	generation      uint64
	freeNext        uint32
	resolved        *route
	input           HandlerInput
	claims          [1]string
	transactionPlan jobmgr.ResourceTransactionPlan
}

func (slot *invocationSlot) RunTask(ctx context.Context) (lifecycle.TaskOutcome, error) {
	if slot == nil || slot.resolved == nil || slot.resolved.handler == nil {
		return lifecycle.NoValueOutcome(), errors.New("jobmgr Function catalog: invalid invocation work")
	}
	if slot.input.HasPayload && len(slot.input.Payload) != 0 && !slot.resolved.rawPayload &&
		!json.Valid(slot.input.Payload) {
		result, err := lifecycle.NewControlResult(lifecycle.ControlBadRequest)
		if err != nil {
			return lifecycle.NoValueOutcome(), err
		}
		return lifecycle.NewFrameOutcome(result)
	}
	result, err := slot.resolved.handler.handler(ctx, slot.input)
	if err != nil {
		return lifecycle.NoValueOutcome(), err
	}
	return lifecycle.NewFrameOutcome(result)
}

func (slot *invocationSlot) prepareResourceTransaction(
	ctx context.Context,
	current lifecycle.ReadyResource,
	scope lifecycle.ResourceTransactionScope,
	permit lifecycle.LongLivedPermit,
) (lifecycle.PreparedResourceTransaction, error) {
	if slot == nil ||
		slot.resolved == nil ||
		slot.resolved.transaction == nil ||
		slot.resolved.transaction.Prepare == nil {
		return nil, errors.New(
			"jobmgr Function catalog: invalid resource transaction invocation",
		)
	}
	return slot.resolved.transaction.Prepare(
		ctx,
		slot.input,
		current,
		scope,
		permit,
	)
}

type Catalog struct {
	routes      *catalogNode
	generations map[jobmgr.FunctionCleanupRef]*handlerGeneration
	mutation    *MutationBuilder
	storage     catalogStorage

	invocations []*invocationSlot
	freeLease   uint32

	closeHead *route
	closeTail *route

	deferredPrune *route

	nextRouteID      uint64
	nextGenerationID uint32
	version          uint64
	routeCount       int
	invocationCount  int
	pendingCleanups  int
	completedCleanup int
	failedCleanup    int
	closed           bool
}

func NewCatalog(declarations []Declaration) (*Catalog, error) {
	type initialRoute struct {
		declaration Declaration
		generation  *handlerGeneration
	}
	for _, declaration := range declarations {
		if err := validateDeclaration(declaration); err != nil {
			return nil, err
		}
	}
	if _, err := initialPathStorageBound(declarations); err != nil {
		return nil, err
	}
	checked := make([]initialRoute, 0, len(declarations))
	checkedSets := make(map[string]routeSet, len(declarations))
	generationByDeclaration := make(map[*HandlerGenerationDeclaration]*handlerGeneration)
	generationIDOwner := make(map[string]*HandlerGenerationDeclaration)
	for _, declaration := range declarations {
		declaration = cloneDeclaration(declaration)
		if owner := generationIDOwner[declaration.Generation.ID]; owner != nil && owner != declaration.Generation {
			return nil, errors.New("jobmgr Function catalog: duplicate handler generation identity")
		}
		generationIDOwner[declaration.Generation.ID] = declaration.Generation
		generation := generationByDeclaration[declaration.Generation]
		if generation == nil {
			generation = &handlerGeneration{
				id: declaration.Generation.ID, handler: declaration.Generation.Handler,
				cleanup: declaration.Generation.Cleanup,
			}
			generationByDeclaration[declaration.Generation] = generation
		}
		set := checkedSets[declaration.PublicName]
		if declaration.Prefix == "" {
			if set.direct != nil {
				return nil, errors.New("jobmgr Function catalog: duplicate direct route")
			}
			set.direct = &route{}
		} else {
			root, err := insertInitialPrefix(set.prefixes, declaration.Prefix, &route{})
			if err != nil {
				return nil, err
			}
			set.prefixes = root
			set.prefixCount++
		}
		checkedSets[declaration.PublicName] = set
		checked = append(checked, initialRoute{declaration: declaration, generation: generation})
	}
	catalog := &Catalog{
		generations: make(map[jobmgr.FunctionCleanupRef]*handlerGeneration, len(declarations)),
		invocations: make([]*invocationSlot, 1),
		nextRouteID: 1,
		version:     1,
	}

	for _, checkedRoute := range checked {
		if err := catalog.addInitial(checkedRoute.declaration, checkedRoute.generation); err != nil {
			return nil, err
		}
	}
	if err := catalog.storage.initialize(
		catalogPathStorage(catalog.routes),
	); err != nil {
		return nil, err
	}
	return catalog, nil
}

func (catalog *Catalog) addInitial(declaration Declaration, generation *handlerGeneration) error {
	set, _ := catalogRouteSet(catalog.routes, declaration.PublicName)
	if !generation.cleanupRef.Valid() {
		catalog.nextGenerationID++
		if catalog.nextGenerationID == 0 {
			return errors.New("jobmgr Function catalog: handler generation wrapped")
		}
		generation.cleanupRef = jobmgr.FunctionCleanupRef{Slot: catalog.nextGenerationID, Generation: 1}
		catalog.generations[generation.cleanupRef] = generation
	}
	catalog.nextRouteID++
	if catalog.nextRouteID == 0 {
		return errors.New("jobmgr Function catalog: route identity wrapped")
	}
	resolved := &route{
		id: catalog.nextRouteID, publicName: declaration.PublicName,
		prefix: declaration.Prefix, method: declaration.ID,
		handler: generation, lane: declaration.Lane,
		cooperativeCancel:   declaration.CooperativeCancel,
		cooperativeDeadline: declaration.CooperativeDeadline,
		rawPayload:          declaration.RawPayload,
		transaction:         cloneResourceTransactionDeclaration(declaration.Transaction),
	}
	if declaration.Prefix == "" {
		set.direct = resolved
	} else {
		root, err := insertInitialPrefix(set.prefixes, declaration.Prefix, resolved)
		if err != nil {
			return err
		}
		set.prefixes = root
		set.prefixCount++
	}
	generation.routeReferences++
	catalog.routes = setInitialCatalogRouteSet(catalog.routes, declaration.PublicName, set)
	catalog.appendCloseRoute(resolved)
	catalog.routeCount++
	return nil
}

func validateDeclaration(declaration Declaration) error {
	if declaration.ID == "" || declaration.Generation == nil ||
		declaration.Generation.ID == "" || declaration.Generation.Handler == nil ||
		declaration.PublicName == "" ||
		len(declaration.ID) > maximumDeclarationMetadataBytes ||
		len(declaration.Generation.ID) > maximumDeclarationMetadataBytes ||
		len(declaration.PublicName) > maximumDeclarationMetadataBytes ||
		len(declaration.Prefix) > maximumDeclarationMetadataBytes {
		return errors.New("jobmgr Function catalog: invalid declaration")
	}
	if declaration.Prefix == "" &&
		(declaration.Lane.Kind == LaneByArgument ||
			declaration.Lane.Kind == LaneByDynCfgJob) {
		return errors.New("jobmgr Function catalog: direct route has argument lane policy")
	}
	if err := validateResourceTransactionDeclaration(
		declaration.Transaction,
	); err != nil {
		return err
	}
	return declaration.Lane.validate()
}

func validateResourceTransactionDeclaration(
	declaration *ResourceTransactionDeclaration,
) error {
	if declaration == nil {
		return nil
	}
	if declaration.Prepare == nil ||
		declaration.GlobalClaim == "" ||
		len(declaration.GlobalClaim) > maximumDeclarationMetadataBytes ||
		declaration.CommandArgument >= 1_024 ||
		len(declaration.Commands) == 0 ||
		len(declaration.Commands) > 16 {
		return errors.New(
			"jobmgr Function catalog: invalid resource transaction declaration",
		)
	}
	hasSuccessor := false
	for index, command := range declaration.Commands {
		if command.Name == "" ||
			len(command.Name) > maximumDeclarationMetadataBytes {
			return errors.New(
				"jobmgr Function catalog: invalid resource transaction command",
			)
		}
		hasSuccessor = hasSuccessor || command.AllocateSuccessor
		for previous := 0; previous < index; previous++ {
			if strings.EqualFold(
				declaration.Commands[previous].Name,
				command.Name,
			) {
				return errors.New(
					"jobmgr Function catalog: duplicate resource transaction command",
				)
			}
		}
	}
	if hasSuccessor {
		if err := declaration.Permit.Validate(); err != nil {
			return err
		}
	} else if declaration.Permit.Class() != 0 ||
		declaration.Permit.Bytes() != 0 {
		return errors.New(
			"jobmgr Function catalog: transaction without successor has a permit",
		)
	}
	return nil
}

func cloneDeclaration(declaration Declaration) Declaration {
	declaration.Transaction = cloneResourceTransactionDeclaration(
		declaration.Transaction,
	)
	return declaration
}

func cloneResourceTransactionDeclaration(
	declaration *ResourceTransactionDeclaration,
) *ResourceTransactionDeclaration {
	if declaration == nil {
		return nil
	}
	cloned := *declaration
	cloned.Commands = append(
		[]ResourceTransactionCommand(nil),
		declaration.Commands...,
	)
	return &cloned
}

func resourceTransactionCommand(
	declaration *ResourceTransactionDeclaration,
	arguments []string,
) (ResourceTransactionCommand, bool) {
	if declaration == nil ||
		int(declaration.CommandArgument) >= len(arguments) {
		return ResourceTransactionCommand{}, false
	}
	name := arguments[declaration.CommandArgument]
	for _, command := range declaration.Commands {
		if strings.EqualFold(command.Name, name) {
			return command, true
		}
	}
	return ResourceTransactionCommand{}, false
}

func (catalog *Catalog) ResolveAndAcquire(lookup jobmgr.FunctionLookup) (jobmgr.FunctionCatalogDecision, error) {
	if catalog == nil || catalog.closed {
		return jobmgr.FunctionCatalogDecision{
			Lane:     jobmgr.FunctionLane{Route: 1, Scope: lookup.Route},
			Rejected: lifecycle.ControlUnavailable,
		}, nil
	}
	set, ok := catalogRouteSet(catalog.routes, lookup.Route)
	if !ok {
		return jobmgr.FunctionCatalogDecision{
			Lane:     jobmgr.FunctionLane{Route: 1, Scope: lookup.Route},
			Rejected: lifecycle.ControlNotFound,
		}, nil
	}
	resolved := set.resolve(lookup.Args)
	if resolved == nil || resolved.retiringDrained {
		return jobmgr.FunctionCatalogDecision{
			Lane:     jobmgr.FunctionLane{Route: 1, Scope: lookup.Route},
			Rejected: lifecycle.ControlNotFound,
		}, nil
	}
	lane := resolved.lane.resolve(resolved.id, lookup.Args)
	generation := resolved.handler
	if resolved.admissionClosed || generation.admissionClosed {
		return jobmgr.FunctionCatalogDecision{Lane: lane, Rejected: lifecycle.ControlUnavailable}, nil
	}

	slotIndex := catalog.freeLease
	var slot *invocationSlot
	if slotIndex == 0 {
		if uint64(len(catalog.invocations)) > uint64(^uint32(0)) {
			return jobmgr.FunctionCatalogDecision{},
				errors.New("jobmgr Function catalog: invocation reference exhausted")
		}
		slotIndex = uint32(len(catalog.invocations))
		slot = &invocationSlot{}
		catalog.invocations = append(catalog.invocations, slot)
	} else {
		slot = catalog.invocations[slotIndex]
		if slot == nil {
			return jobmgr.FunctionCatalogDecision{},
				errors.New("jobmgr Function catalog: invalid free invocation slot")
		}
		catalog.freeLease = slot.freeNext
	}
	nextGeneration := slot.generation + 1
	if nextGeneration == 0 {
		slot.freeNext = catalog.freeLease
		catalog.freeLease = slotIndex
		return jobmgr.FunctionCatalogDecision{}, errors.New("jobmgr Function catalog: invocation generation wrapped")
	}
	*slot = invocationSlot{
		generation: nextGeneration, resolved: resolved,
		input: HandlerInput{
			UID: lookup.UID, Method: resolved.method, Args: lookup.Args,
			Payload: lookup.Payload, ContentType: lookup.ContentType,
			Permissions: lookup.Permissions, CallerSource: lookup.CallerSource,
			Timeout: lookup.Timeout, HasPayload: lookup.HasPayload,
		},
	}
	resolved.invocationLeases++
	generation.invocationLeases++
	catalog.invocationCount++
	plan := jobmgr.WorkPlan{
		Runner:              slot,
		CooperativeCancel:   resolved.cooperativeCancel,
		CooperativeDeadline: resolved.cooperativeDeadline,
	}
	if command, ok := resourceTransactionCommand(
		resolved.transaction,
		lookup.Args,
	); ok {
		slot.claims[0] = resolved.transaction.GlobalClaim
		slot.transactionPlan = jobmgr.ResourceTransactionPlan{
			ID:                lane.Scope,
			AllocateSuccessor: command.AllocateSuccessor,
			Prepare:           slot.prepareResourceTransaction,
		}
		if command.AllocateSuccessor {
			slot.transactionPlan.Permit = resolved.transaction.Permit
		}
		plan = jobmgr.WorkPlan{
			Claims:      slot.claims[:],
			Transaction: &slot.transactionPlan,
		}
	}
	return jobmgr.FunctionCatalogDecision{
		Lane:  lane,
		Plan:  plan,
		Lease: jobmgr.FunctionInvocationRef{Slot: slotIndex, Generation: nextGeneration},
	}, nil
}

func (catalog *Catalog) ReleaseInvocation(ref jobmgr.FunctionInvocationRef) (jobmgr.FunctionCleanupPlan, error) {
	if catalog == nil || !ref.Valid() || uint64(ref.Slot) >= uint64(len(catalog.invocations)) {
		return jobmgr.FunctionCleanupPlan{}, errors.New("jobmgr Function catalog: invalid invocation release")
	}
	slot := catalog.invocations[ref.Slot]
	if slot == nil {
		return jobmgr.FunctionCleanupPlan{}, errors.New("jobmgr Function catalog: invalid invocation release")
	}
	if slot.generation != ref.Generation || slot.resolved == nil || slot.resolved.handler == nil {
		return jobmgr.FunctionCleanupPlan{}, errors.New("jobmgr Function catalog: stale invocation release")
	}
	generation := slot.resolved.handler
	if slot.resolved.invocationLeases <= 0 ||
		generation.invocationLeases <= 0 ||
		catalog.invocationCount <= 0 {
		return jobmgr.FunctionCleanupPlan{}, errors.New("jobmgr Function catalog: invocation lease underflow")
	}
	resolved := slot.resolved
	resolved.invocationLeases--
	generation.invocationLeases--
	catalog.invocationCount--
	slotGeneration := slot.generation
	*slot = invocationSlot{generation: slotGeneration, freeNext: catalog.freeLease}
	catalog.freeLease = ref.Slot
	catalog.retireDrainedRoute(resolved)
	return catalog.maybeCleanup(generation)
}

func (catalog *Catalog) maybeCleanup(generation *handlerGeneration) (jobmgr.FunctionCleanupPlan, error) {
	if generation == nil || generation.routeReferences < 0 || generation.invocationLeases < 0 {
		return jobmgr.FunctionCleanupPlan{}, errors.New("jobmgr Function catalog: invalid handler generation")
	}
	if !generation.admissionClosed || generation.routeReferences != 0 ||
		generation.invocationLeases != 0 || generation.cleanupPending || generation.cleaned {
		return jobmgr.FunctionCleanupPlan{}, nil
	}
	return catalog.cleanupDrainedGeneration(generation), nil
}

func (catalog *Catalog) retireDrainedRoute(retired *route) {
	if catalog == nil || retired == nil || !retired.retiring ||
		retired.retiringDrained || retired.invocationLeases != 0 {
		return
	}
	retired.retiringDrained = true
	if catalog.mutation != nil {
		retired.retiringNext = catalog.deferredPrune
		catalog.deferredPrune = retired
		return
	}
	catalog.pruneRetiringRoutes(retired)
}

func (catalog *Catalog) pruneRetiringRoutes(routes *route) {
	var released int64
	for route := routes; route != nil; {
		next := route.retiringNext
		route.retiringNext = nil
		route.retiringDrained = true
		released += pruneRetiringRoute(&catalog.routes, route)
		route = next
	}
	catalog.storage.releasePublishedPaths(released)
}

func pruneRetiringRoute(
	root **catalogNode,
	retired *route,
) int64 {
	if root == nil || retired == nil || *root == nil {
		return 0
	}
	defer func() {
		retired.retiringNamePath = nil
		retired.retiringPrefixPath = nil
	}()

	nameBits := len(retired.publicName) * 8
	if len(retired.retiringNamePath) != nameBits+1 {
		return 0
	}
	node := *root
	for depth := 0; depth <= nameBits; depth++ {
		if node == nil {
			return 0
		}
		retired.retiringNamePath[depth] = node
		if depth != nameBits {
			node = node.child[keyBit(retired.publicName, depth)]
		}
	}
	leaf := retired.retiringNamePath[nameBits]
	var released int64
	if retired.prefix == "" {
		if leaf.routes.direct != retired {
			return 0
		}
		leaf.routes.direct = nil
	} else {
		prefixBits := len(retired.prefix) * 8
		if len(retired.retiringPrefixPath) != prefixBits+1 ||
			leaf.routes.prefixCount <= 0 {
			return 0
		}
		prefix := leaf.routes.prefixes
		for depth := 0; depth <= prefixBits; depth++ {
			if prefix == nil {
				return 0
			}
			retired.retiringPrefixPath[depth] = prefix
			if depth != prefixBits {
				prefix = prefix.child[keyBit(retired.prefix, depth)]
			}
		}
		if retired.retiringPrefixPath[prefixBits].resolved != retired {
			return 0
		}
		retired.retiringPrefixPath[prefixBits].resolved = nil
		leaf.routes.prefixCount--
		for depth := prefixBits; depth >= 0; depth-- {
			current := retired.retiringPrefixPath[depth]
			if current.resolved != nil ||
				current.child[0] != nil ||
				current.child[1] != nil {
				break
			}
			if depth == 0 {
				leaf.routes.prefixes = nil
			} else {
				parent := retired.retiringPrefixPath[depth-1]
				parent.child[keyBit(retired.prefix, depth-1)] = nil
			}
			released += prefixNodeStorageBytes
		}
	}

	leaf.present = !leaf.routes.empty()
	for depth := nameBits; depth >= 0; depth-- {
		current := retired.retiringNamePath[depth]
		if current.present ||
			current.child[0] != nil ||
			current.child[1] != nil {
			break
		}
		if depth == 0 {
			*root = nil
		} else {
			parent := retired.retiringNamePath[depth-1]
			parent.child[keyBit(retired.publicName, depth-1)] = nil
		}
		released += catalogNodeStorageBytes
	}
	return released
}

func (catalog *Catalog) cleanupDrainedGeneration(generation *handlerGeneration) jobmgr.FunctionCleanupPlan {
	if generation.cleanup == nil {
		generation.cleaned = true
		catalog.completedCleanup++
		delete(catalog.generations, generation.cleanupRef)
		return jobmgr.FunctionCleanupPlan{}
	}
	generation.cleanupPending = true
	catalog.pendingCleanups++
	return jobmgr.FunctionCleanupPlan{Ref: generation.cleanupRef, Runner: generation}
}

func (catalog *Catalog) CompleteCleanup(ref jobmgr.FunctionCleanupRef, cleanupErr error) error {
	if catalog == nil || !ref.Valid() {
		return errors.New("jobmgr Function catalog: invalid cleanup completion")
	}
	generation := catalog.generations[ref]
	if generation == nil || generation.cleanupRef != ref || !generation.cleanupPending ||
		generation.cleaned || generation.routeReferences != 0 || generation.invocationLeases != 0 {
		return errors.New("jobmgr Function catalog: stale cleanup completion")
	}
	generation.cleanupPending = false
	generation.cleaned = true
	generation.cleanupFailed = cleanupErr != nil
	catalog.pendingCleanups--
	catalog.completedCleanup++
	if cleanupErr != nil {
		catalog.failedCleanup++
	}
	delete(catalog.generations, ref)
	return nil
}

func (catalog *Catalog) BeginClose() error {
	if catalog == nil || catalog.closed || catalog.mutation != nil {
		return errors.New("jobmgr Function catalog: duplicate close")
	}
	catalog.closed = true
	return nil
}

func (catalog *Catalog) CloseStep(quantum int, cleanups *[jobmgr.MaximumFunctionCleanupBatch]jobmgr.FunctionCleanupPlan) (int, bool, error) {
	if catalog == nil || !catalog.closed || quantum <= 0 || quantum > MaximumCloseQuantum || cleanups == nil {
		return 0, false, errors.New("jobmgr Function catalog: invalid close step")
	}
	count := 0
	for quantum > 0 && catalog.closeHead != nil {
		resolved := catalog.closeHead
		catalog.unlinkCloseRoute(resolved)
		generation := resolved.handler
		if generation == nil || generation.routeReferences <= 0 {
			return count, true, errors.New("jobmgr Function catalog: route reference underflow")
		}
		generation.routeReferences--
		if generation.routeReferences == 0 {
			generation.admissionClosed = true
			cleanup, err := catalog.maybeCleanup(generation)
			if err != nil {
				return count, true, err
			}
			if cleanup.Ref.Valid() {
				cleanups[count] = cleanup
				count++
			}
		}
		catalog.routeCount--
		quantum--
	}
	more := catalog.closeHead != nil
	if !more {
		catalog.routes = nil
		if err := catalog.storage.releasePublished(); err != nil {
			return count, false, err
		}
	}
	return count, more, nil
}

func (catalog *Catalog) appendCloseRoute(resolved *route) {
	resolved.closePrevious = catalog.closeTail
	if catalog.closeTail != nil {
		catalog.closeTail.closeNext = resolved
	} else {
		catalog.closeHead = resolved
	}
	catalog.closeTail = resolved
}

func (catalog *Catalog) unlinkCloseRoute(resolved *route) {
	if resolved.closePrevious != nil {
		resolved.closePrevious.closeNext = resolved.closeNext
	} else {
		catalog.closeHead = resolved.closeNext
	}
	if resolved.closeNext != nil {
		resolved.closeNext.closePrevious = resolved.closePrevious
	} else {
		catalog.closeTail = resolved.closePrevious
	}
	resolved.closePrevious = nil
	resolved.closeNext = nil
}

func (catalog *Catalog) Census() CatalogCensus {
	if catalog == nil {
		return CatalogCensus{}
	}
	return CatalogCensus{
		Version: catalog.version, Routes: catalog.routeCount,
		InvocationLeases: catalog.invocationCount, PendingCleanups: catalog.pendingCleanups,
		CompletedCleanups: catalog.completedCleanup, FailedCleanups: catalog.failedCleanup,
		Closed: catalog.closed, CloseRoutesPending: catalog.routeCount,
		MutationActive: catalog.mutation != nil,
	}
}

func (catalog *Catalog) LifecycleCensus() jobmgr.FunctionCatalogCensus {
	return catalog.Census()
}

func (catalog *Catalog) HandlerCensus(ref jobmgr.FunctionCleanupRef) HandlerGenerationCensus {
	if catalog == nil {
		return HandlerGenerationCensus{}
	}
	return catalog.generations[ref].census()
}
