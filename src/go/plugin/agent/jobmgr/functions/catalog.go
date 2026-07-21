// SPDX-License-Identifier: GPL-3.0-or-later

package functions

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
)

const (
	maximumDeclarationMetadataBytes = jobmgr.MaximumPluginSDLineBytes
	MaximumCloseQuantum             = jobmgr.MaximumFunctionCloseQuantum
	invalidDynCfgResourceID         = "\x00dyncfg-invalid"
)

var dynCfgJobNameReplacer = strings.NewReplacer(" ", "_", ":", "_")

type HandlerInput struct {
	UID          string        // call UID
	Method       string        // resolved method ID passed to the handler
	Args         []string      // call arguments
	Payload      []byte        // request payload
	ContentType  string        // payload content type
	Permissions  string        // caller permissions
	CallerSource string        // caller source string
	Timeout      time.Duration // caller-requested timeout
	HasPayload   bool          // whether a payload is present
}

type Handler func(context.Context, HandlerInput) (lifecycle.SealedResult, error)

type ResourceTransactionHandler func(
	context.Context,
	HandlerInput,
	lifecycle.ReadyResource,
	lifecycle.ResourceTransactionScope,
	lifecycle.LongLivedPermit,
) (lifecycle.PreparedResourceTransaction, error)

type CompositeResourceTransactionHandler func(
	context.Context,
	HandlerInput,
	lifecycle.ReadyResource,
	lifecycle.ResourceTransactionScope,
	lifecycle.LongLivedPermit,
) (jobmgr.PreparedCompositeResourceTransaction, error)

type ResourceTransactionCommand struct {
	Name              string
	AllocateSuccessor bool
	Claims            []string
}

type ResourceTransactionDeclaration struct {
	Prepare          ResourceTransactionHandler          // prepares a plain resource transaction
	PrepareComposite CompositeResourceTransactionHandler // prepares a composite resource transaction (child commands)
	Permit           lifecycle.LongLivedPlan             // long-lived plan for the successor
	CommandArgument  uint16                              // argument index carrying the dyncfg command
	GlobalClaim      string                              // claim key held for the whole transaction domain
	Commands         []ResourceTransactionCommand        // accepted transaction commands
}

// HandlerGenerationDeclaration describes one job- or module-owned handler
// generation. Multiple routes may reference the same declaration; the catalog
// copies it and owns exactly one cleanup for the resulting generation.
type HandlerGenerationDeclaration struct {
	ID      string
	Handler Handler
	Cleanup func(context.Context) error
}

// ResourcePolicy derives the shared Job Manager resource identity for a
// resource-scoped Function. Its zero value schedules each invocation
// independently.
type ResourcePolicy struct {
	Argument    uint16
	Prefix      string
	ScopePrefix string
}

// DynCfgJobResource derives a collector job resource from DynCfg arguments.
func DynCfgJobResource(index uint16, prefix string) ResourcePolicy {
	return ResourcePolicy{
		Argument: index, Prefix: prefix,
	}
}

// ScopedDynCfgJobResource additionally namespaces the derived resource.
func ScopedDynCfgJobResource(index uint16, prefix, scopePrefix string) ResourcePolicy {
	return ResourcePolicy{
		Argument: index, Prefix: prefix,
		ScopePrefix: scopePrefix,
	}
}

func (rp ResourcePolicy) validate() error {
	if rp == (ResourcePolicy{}) {
		return nil
	}
	if rp.Prefix == "" ||
		len(rp.Prefix)+len(rp.ScopePrefix) >
			maximumDeclarationMetadataBytes ||
		strings.TrimSpace(rp.ScopePrefix) != rp.ScopePrefix {
		return errors.New(
			"jobmgr Function catalog: invalid resource policy",
		)
	}
	return nil
}

func (rp ResourcePolicy) resolve(arguments []string) string {
	if rp == (ResourcePolicy{}) {
		return ""
	}
	resourceID := resolveDynCfgJobResource(rp, arguments)
	if resourceID != "" {
		resourceID = rp.ScopePrefix + resourceID
	}
	return resourceID
}

func resolveDynCfgJobResource(
	policy ResourcePolicy,
	arguments []string,
) string {
	if int(policy.Argument) >= len(arguments) {
		return invalidDynCfgResourceID
	}
	target := strings.TrimPrefix(
		arguments[policy.Argument],
		policy.Prefix,
	)
	target = strings.TrimPrefix(target, ":")
	if target == "" {
		if name, isAdd := addCommandJobName(arguments); isAdd && name != "" {
			return name
		}
		return invalidDynCfgResourceID
	}
	module, name, hasName := strings.Cut(target, ":")
	if module == "" {
		return invalidDynCfgResourceID
	}
	if replaced, isAdd := addCommandJobName(arguments); isAdd {
		name = replaced
		hasName = name != ""
	}
	if !hasName || name == module {
		return module
	}
	return module + "_" + name
}

// addCommandJobName extracts the job name from a DynCfg "add" command's
// arguments. It reports whether this is an add command (arguments[1] == "add");
// the returned name is the replacer-normalized arguments[2] and may be empty.
func addCommandJobName(arguments []string) (string, bool) {
	if len(arguments) > 2 && strings.EqualFold(arguments[1], "add") {
		return dynCfgJobNameReplacer.Replace(arguments[2]), true
	}
	return "", false
}

type Declaration struct {
	ID                  string                          // stable route/declaration identity
	Generation          *HandlerGenerationDeclaration   // handler generation for this route (nil for pure transactions)
	Transaction         *ResourceTransactionDeclaration // optional resource-transaction declaration
	PublicName          string                          // exact Function name advertised to Netdata
	Prefix              string                          // optional arg[0] prefix; empty = exact route
	Resource            ResourcePolicy                  // policy deriving the resource ID from call args
	CooperativeCancel   bool                            // request honors cooperative cancellation
	CooperativeDeadline bool                            // request honors the caller deadline
	RawPayload          bool                            // skip JSON validation; pass the payload verbatim
}

type CatalogCensus = jobmgr.FunctionCatalogCensus

type handlerGeneration struct {
	cleanupRef jobmgr.FunctionCleanupRef   // kernel cleanup ref for this generation
	handler    Handler                     // the Function handler
	cleanup    func(context.Context) error // collector cleanup callback (nil = no cleanup)

	routeReferences  int  // routes currently pointing at this generation
	invocationLeases int  // outstanding in-flight invocations
	cleanupPending   bool // cleanup task dispatched, awaiting acknowledgement
	cleaned          bool // cleanup acknowledged; generation released
}

func (hg *handlerGeneration) RunTask(ctx context.Context) (lifecycle.TaskOutcome, error) {
	if hg == nil || !hg.cleanupPending || hg.cleanup == nil {
		return lifecycle.NoValueOutcome(), errors.New("jobmgr Function handler: invalid cleanup work")
	}
	return lifecycle.NoValueOutcome(), hg.cleanup(ctx)
}

type route struct {
	publicName          string                          // external Function name
	prefix              string                          // optional arg[0] prefix; empty = an exact route
	method              string                          // handler method ID passed to the generation on invoke
	handler             *handlerGeneration              // owning handler generation (shared across routes)
	resource            ResourcePolicy                  // policy deriving the resource ID from call args
	cooperativeCancel   bool                            // request honors cooperative cancellation
	cooperativeDeadline bool                            // request honors the caller deadline
	rawPayload          bool                            // skip JSON validation; pass the payload verbatim
	transaction         *ResourceTransactionDeclaration // optional resource-transaction declaration
	invocationLeases    int                             // outstanding in-flight invocations on this route
}

type routeSet struct {
	direct   *route
	prefixes []*route
}

func (set routeSet) empty() bool {
	return set.direct == nil && len(set.prefixes) == 0
}

func (set routeSet) resolve(arguments []string) *route {
	if len(set.prefixes) == 0 {
		return set.direct
	}
	if len(arguments) == 0 {
		return nil
	}
	argument := arguments[0]
	for _, resolved := range set.prefixes {
		if strings.HasPrefix(argument, resolved.prefix) {
			return resolved
		}
	}
	return nil
}

func appendInitialPrefix(set routeSet, resolved *route) (routeSet, error) {
	for _, existing := range set.prefixes {
		if strings.HasPrefix(existing.prefix, resolved.prefix) ||
			strings.HasPrefix(resolved.prefix, existing.prefix) {
			return routeSet{}, errors.New("jobmgr Function catalog: overlapping prefix route")
		}
	}
	set.prefixes = append(set.prefixes, resolved)
	return set, nil
}

type invocationSlot struct {
	generation      uint64                         // bumped per reuse; validates the lease ref
	freeNext        uint32                         // next free slot index on the free-list
	resolved        *route                         // route this invocation targets
	input           HandlerInput                   // materialized handler input for the call
	claims          []string                       // global claim followed by command claims
	transactionPlan jobmgr.ResourceTransactionPlan // resource-transaction plan when the route is transactional
}

func (is *invocationSlot) RunTask(ctx context.Context) (lifecycle.TaskOutcome, error) {
	if is == nil || is.resolved == nil || is.resolved.handler == nil {
		return lifecycle.NoValueOutcome(), errors.New("jobmgr Function catalog: invalid invocation work")
	}
	if is.input.HasPayload && len(is.input.Payload) != 0 && !is.resolved.rawPayload &&
		!json.Valid(is.input.Payload) {
		result, err := lifecycle.NewControlResult(lifecycle.ControlBadRequest)
		if err != nil {
			return lifecycle.NoValueOutcome(), err
		}
		return lifecycle.NewFrameOutcome(result)
	}
	result, err := is.resolved.handler.handler(ctx, is.input)
	if err != nil {
		return lifecycle.NoValueOutcome(), err
	}
	return lifecycle.NewFrameOutcome(result)
}

func (is *invocationSlot) prepareResourceTransaction(
	ctx context.Context,
	current lifecycle.ReadyResource,
	scope lifecycle.ResourceTransactionScope,
	permit lifecycle.LongLivedPermit,
) (lifecycle.PreparedResourceTransaction, error) {
	if is == nil ||
		is.resolved == nil ||
		is.resolved.transaction == nil ||
		is.resolved.transaction.Prepare == nil {
		return nil, errors.New(
			"jobmgr Function catalog: invalid resource transaction invocation",
		)
	}
	return is.resolved.transaction.Prepare(
		ctx,
		is.input,
		current,
		scope,
		permit,
	)
}

func (is *invocationSlot) prepareCompositeResourceTransaction(
	ctx context.Context,
	current lifecycle.ReadyResource,
	scope lifecycle.ResourceTransactionScope,
	permit lifecycle.LongLivedPermit,
) (jobmgr.PreparedCompositeResourceTransaction, error) {
	if is == nil ||
		is.resolved == nil ||
		is.resolved.transaction == nil ||
		is.resolved.transaction.PrepareComposite == nil {
		return nil, errors.New(
			"jobmgr Function catalog: invalid composite transaction invocation",
		)
	}
	return is.resolved.transaction.PrepareComposite(
		ctx,
		is.input,
		current,
		scope,
		permit,
	)
}

type catalogSnapshot struct {
	version uint64
	routes  map[string]routeSet
	active  []*route
}

type Catalog struct {
	snapshot        atomic.Pointer[catalogSnapshot]
	mutation        *MutationBuilder
	retiring        map[string]map[*route]struct{}
	cleanupByRef    map[jobmgr.FunctionCleanupRef]*handlerGeneration
	nextCleanupSlot atomic.Uint32

	invocations []*invocationSlot // invocation slot pool
	freeSlot    uint32            // head of the invocation-slot free-list

	closeRoutes []*route
	closeIndex  int

	version         uint64 // catalog version, bumped per committed mutation
	routeCount      int    // count of live routes
	invocationCount int    // count of active invocations
	pendingCleanups int    // count of generations with cleanup pending
	closed          bool   // catalog is closed (shutdown)
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
				handler: declaration.Generation.Handler,
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
			var err error
			set, err = appendInitialPrefix(set, &route{prefix: declaration.Prefix})
			if err != nil {
				return nil, err
			}
		}
		checkedSets[declaration.PublicName] = set
		checked = append(checked, initialRoute{declaration: declaration, generation: generation})
	}
	catalog := &Catalog{
		cleanupByRef: make(map[jobmgr.FunctionCleanupRef]*handlerGeneration),
		retiring:     make(map[string]map[*route]struct{}),
		invocations:  make([]*invocationSlot, 1),
		version:      1,
	}
	snapshot := &catalogSnapshot{version: 1, routes: make(map[string]routeSet)}
	for _, checkedRoute := range checked {
		if err := catalog.addInitial(snapshot, checkedRoute.declaration, checkedRoute.generation); err != nil {
			return nil, err
		}
	}
	catalog.snapshot.Store(snapshot)
	return catalog, nil
}

func (c *Catalog) addInitial(snapshot *catalogSnapshot, declaration Declaration, generation *handlerGeneration) error {
	set := snapshot.routes[declaration.PublicName]
	if generation.cleanup != nil && !generation.cleanupRef.Valid() {
		ref, err := c.allocateCleanupRef()
		if err != nil {
			return err
		}
		generation.cleanupRef = ref
	}
	resolved := &route{
		publicName: declaration.PublicName,
		prefix:     declaration.Prefix, method: declaration.ID,
		handler: generation, resource: declaration.Resource,
		cooperativeCancel:   declaration.CooperativeCancel,
		cooperativeDeadline: declaration.CooperativeDeadline,
		rawPayload:          declaration.RawPayload,
		transaction:         cloneResourceTransactionDeclaration(declaration.Transaction),
	}
	if declaration.Prefix == "" {
		set.direct = resolved
	} else {
		var err error
		set, err = appendInitialPrefix(set, resolved)
		if err != nil {
			return err
		}
	}
	generation.routeReferences++
	snapshot.routes[declaration.PublicName] = set
	snapshot.active = append(snapshot.active, resolved)
	c.routeCount++
	return nil
}

func (c *Catalog) allocateCleanupRef() (jobmgr.FunctionCleanupRef, error) {
	slot := c.nextCleanupSlot.Add(1)
	if slot == 0 {
		return jobmgr.FunctionCleanupRef{}, errors.New("jobmgr Function catalog: cleanup identity exhausted")
	}
	return jobmgr.FunctionCleanupRef{Slot: slot, Generation: 1}, nil
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
	if err := validateResourceTransactionDeclaration(
		declaration.Transaction,
	); err != nil {
		return err
	}
	if declaration.Transaction != nil &&
		declaration.Resource == (ResourcePolicy{}) {
		return errors.New(
			"jobmgr Function catalog: transaction has no resource policy",
		)
	}
	return declaration.Resource.validate()
}

func validateResourceTransactionDeclaration(
	declaration *ResourceTransactionDeclaration,
) error {
	if declaration == nil {
		return nil
	}
	if (declaration.Prepare == nil) ==
		(declaration.PrepareComposite == nil) ||
		declaration.GlobalClaim == "" ||
		len(declaration.GlobalClaim) > maximumDeclarationMetadataBytes ||
		len(declaration.Commands) == 0 {
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
		for claimIndex, claim := range command.Claims {
			if claim == "" ||
				len(claim) > maximumDeclarationMetadataBytes ||
				claim == declaration.GlobalClaim {
				return errors.New(
					"jobmgr Function catalog: invalid command claim",
				)
			}
			for previous := range claimIndex {
				if command.Claims[previous] == claim {
					return errors.New(
						"jobmgr Function catalog: duplicate command claim",
					)
				}
			}
		}
		hasSuccessor = hasSuccessor || command.AllocateSuccessor
		for previous := range index {
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
	} else if declaration.Permit.Class() != 0 {
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
	cloned.Commands = slices.Clone(declaration.Commands)
	for index := range cloned.Commands {
		cloned.Commands[index].Claims = slices.Clone(
			declaration.Commands[index].Claims,
		)
	}
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

func (c *Catalog) ResolveAndAcquire(lookup jobmgr.FunctionLookup) (jobmgr.FunctionCatalogDecision, error) {
	if c == nil || c.closed {
		return jobmgr.FunctionCatalogDecision{
			Rejected: lifecycle.ControlUnavailable,
		}, nil
	}
	snapshot := c.snapshot.Load()
	if snapshot == nil {
		return jobmgr.FunctionCatalogDecision{Rejected: lifecycle.ControlUnavailable}, nil
	}
	set, ok := snapshot.routes[lookup.Route]
	if !ok {
		if len(c.retiring[lookup.Route]) != 0 {
			return jobmgr.FunctionCatalogDecision{
				Rejected: lifecycle.ControlUnavailable,
			}, nil
		}
		return jobmgr.FunctionCatalogDecision{
			Rejected: lifecycle.ControlNotFound,
		}, nil
	}
	resolved := set.resolve(lookup.Args)
	if resolved == nil {
		return jobmgr.FunctionCatalogDecision{
			Rejected: lifecycle.ControlNotFound,
		}, nil
	}
	resourceID := resolved.resource.resolve(lookup.Args)
	generation := resolved.handler
	if c.mutation != nil && c.mutation.quiesces(resolved) {
		return jobmgr.FunctionCatalogDecision{Rejected: lifecycle.ControlUnavailable}, nil
	}
	command, transactionCommand := resourceTransactionCommand(
		resolved.transaction,
		lookup.Args,
	)
	var transactionPermit lifecycle.LongLivedPlan
	if transactionCommand && command.AllocateSuccessor {
		transactionPermit = resolved.transaction.Permit
	}

	slotIndex := c.freeSlot
	var slot *invocationSlot
	if slotIndex == 0 {
		if uint64(len(c.invocations)) > uint64(^uint32(0)) {
			return jobmgr.FunctionCatalogDecision{},
				errors.New("jobmgr Function catalog: invocation reference exhausted")
		}
		slotIndex = uint32(len(c.invocations))
		slot = &invocationSlot{}
		c.invocations = append(c.invocations, slot)
	} else {
		slot = c.invocations[slotIndex]
		if slot == nil {
			return jobmgr.FunctionCatalogDecision{},
				errors.New("jobmgr Function catalog: invalid free invocation slot")
		}
		c.freeSlot = slot.freeNext
	}
	nextGeneration := slot.generation + 1
	if nextGeneration == 0 {
		slot.freeNext = c.freeSlot
		c.freeSlot = slotIndex
		return jobmgr.FunctionCatalogDecision{}, errors.New("jobmgr Function catalog: invocation generation wrapped")
	}
	*slot = invocationSlot{
		generation: nextGeneration, resolved: resolved,
		input: handlerInput(lookup, resolved.method),
	}
	resolved.invocationLeases++
	generation.invocationLeases++
	c.invocationCount++
	plan := jobmgr.WorkPlan{
		Work:                slot.RunTask,
		CooperativeCancel:   resolved.cooperativeCancel,
		CooperativeDeadline: resolved.cooperativeDeadline,
	}
	if transactionCommand {
		slot.claims = append(slot.claims, resolved.transaction.GlobalClaim)
		slot.claims = append(slot.claims, command.Claims...)
		slot.transactionPlan = jobmgr.ResourceTransactionPlan{
			ID:                resourceID,
			AllocateSuccessor: command.AllocateSuccessor,
		}
		if resolved.transaction.PrepareComposite != nil {
			slot.transactionPlan.PrepareComposite =
				slot.prepareCompositeResourceTransaction
		} else {
			slot.transactionPlan.Prepare =
				slot.prepareResourceTransaction
		}
		if command.AllocateSuccessor {
			slot.transactionPlan.Permit = transactionPermit
		}
		plan = jobmgr.WorkPlan{
			Claims:      slot.claims,
			Transaction: &slot.transactionPlan,
		}
	}
	return jobmgr.FunctionCatalogDecision{
		ResourceID: resourceID,
		Plan:       plan,
		Lease:      jobmgr.FunctionInvocationRef{Slot: slotIndex, Generation: nextGeneration},
	}, nil
}

func handlerInput(
	lookup jobmgr.FunctionLookup,
	method string,
) HandlerInput {
	return HandlerInput{
		UID: lookup.UID, Method: method, Args: lookup.Args,
		Payload: lookup.Payload, ContentType: lookup.ContentType,
		Permissions: lookup.Permissions, CallerSource: lookup.CallerSource,
		Timeout: lookup.Timeout, HasPayload: lookup.HasPayload,
	}
}

func (c *Catalog) ReleaseInvocation(ref jobmgr.FunctionInvocationRef) (jobmgr.FunctionCleanupPlan, error) {
	if c == nil || !ref.Valid() || uint64(ref.Slot) >= uint64(len(c.invocations)) {
		return jobmgr.FunctionCleanupPlan{}, errors.New("jobmgr Function catalog: invalid invocation release")
	}
	slot := c.invocations[ref.Slot]
	if slot == nil {
		return jobmgr.FunctionCleanupPlan{}, errors.New("jobmgr Function catalog: invalid invocation release")
	}
	if slot.generation != ref.Generation || slot.resolved == nil || slot.resolved.handler == nil {
		return jobmgr.FunctionCleanupPlan{}, errors.New("jobmgr Function catalog: stale invocation release")
	}
	generation := slot.resolved.handler
	if slot.resolved.invocationLeases <= 0 ||
		generation.invocationLeases <= 0 ||
		c.invocationCount <= 0 {
		return jobmgr.FunctionCleanupPlan{}, errors.New("jobmgr Function catalog: invocation lease underflow")
	}
	resolved := slot.resolved
	resolved.invocationLeases--
	generation.invocationLeases--
	c.invocationCount--
	if resolved.invocationLeases == 0 {
		retired := c.retiring[resolved.publicName]
		delete(retired, resolved)
		if len(retired) == 0 {
			delete(c.retiring, resolved.publicName)
		}
	}
	slotGeneration := slot.generation
	*slot = invocationSlot{generation: slotGeneration, freeNext: c.freeSlot}
	c.freeSlot = ref.Slot
	return c.maybeCleanup(generation)
}

func (c *Catalog) maybeCleanup(generation *handlerGeneration) (jobmgr.FunctionCleanupPlan, error) {
	if generation == nil || generation.routeReferences < 0 || generation.invocationLeases < 0 {
		return jobmgr.FunctionCleanupPlan{}, errors.New("jobmgr Function catalog: invalid handler generation")
	}
	if generation.routeReferences != 0 ||
		generation.invocationLeases != 0 || generation.cleanupPending || generation.cleaned {
		return jobmgr.FunctionCleanupPlan{}, nil
	}
	return c.cleanupDrainedGeneration(generation)
}

func (c *Catalog) cleanupDrainedGeneration(generation *handlerGeneration) (jobmgr.FunctionCleanupPlan, error) {
	if generation.cleanup == nil {
		generation.cleaned = true
		return jobmgr.FunctionCleanupPlan{}, nil
	}
	if !generation.cleanupRef.Valid() || c.cleanupByRef[generation.cleanupRef] != nil {
		return jobmgr.FunctionCleanupPlan{}, errors.New("jobmgr Function catalog: invalid cleanup dispatch")
	}
	generation.cleanupPending = true
	c.cleanupByRef[generation.cleanupRef] = generation
	c.pendingCleanups++
	return jobmgr.FunctionCleanupPlan{Ref: generation.cleanupRef, Work: generation.RunTask}, nil
}

func (c *Catalog) CompleteCleanup(ref jobmgr.FunctionCleanupRef) error {
	if c == nil || !ref.Valid() {
		return errors.New("jobmgr Function catalog: invalid cleanup completion")
	}
	generation := c.cleanupByRef[ref]
	if generation == nil || generation.cleanupRef != ref || !generation.cleanupPending ||
		generation.cleaned || generation.routeReferences != 0 || generation.invocationLeases != 0 {
		return errors.New("jobmgr Function catalog: stale cleanup completion")
	}
	generation.cleanupPending = false
	generation.cleaned = true
	c.pendingCleanups--
	delete(c.cleanupByRef, ref)
	return nil
}

func (c *Catalog) BeginClose() error {
	if c == nil || c.closed || c.mutation != nil {
		return errors.New("jobmgr Function catalog: duplicate close")
	}
	c.closed = true
	snapshot := c.snapshot.Load()
	if snapshot != nil {
		c.closeRoutes = snapshot.active
	}
	return nil
}

func (c *Catalog) CloseStep(quantum int) ([]jobmgr.FunctionCleanupPlan, bool, error) {
	if c == nil || !c.closed || quantum <= 0 || quantum > MaximumCloseQuantum {
		return nil, false, errors.New("jobmgr Function catalog: invalid close step")
	}
	cleanups := make([]jobmgr.FunctionCleanupPlan, 0, quantum)
	for quantum > 0 && c.closeIndex < len(c.closeRoutes) {
		resolved := c.closeRoutes[c.closeIndex]
		c.closeIndex++
		generation := resolved.handler
		if generation == nil || generation.routeReferences <= 0 {
			return cleanups, true, errors.New("jobmgr Function catalog: route reference underflow")
		}
		generation.routeReferences--
		if generation.routeReferences == 0 {
			cleanup, err := c.maybeCleanup(generation)
			if err != nil {
				return cleanups, true, err
			}
			if cleanup.Ref.Valid() {
				cleanups = append(cleanups, cleanup)
			}
		}
		c.routeCount--
		quantum--
	}
	more := c.closeIndex < len(c.closeRoutes)
	if !more {
		c.closeRoutes = nil
		c.snapshot.Store(nil)
	}
	return cleanups, more, nil
}

func (c *Catalog) Census() CatalogCensus {
	if c == nil {
		return CatalogCensus{}
	}
	return CatalogCensus{
		Version: c.version, Routes: c.routeCount,
		InvocationLeases: c.invocationCount, PendingCleanups: c.pendingCleanups,
		Closed:         c.closed,
		MutationActive: c.mutation != nil,
	}
}

func (c *Catalog) LifecycleCensus() jobmgr.FunctionCatalogCensus {
	return c.Census()
}
