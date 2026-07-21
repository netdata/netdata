// SPDX-License-Identifier: GPL-3.0-or-later

package functions

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
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

type SuccessorPermitPolicy uint8

const (
	SuccessorPermitSecretStorePayload SuccessorPermitPolicy = iota + 1
)

type ResourceTransactionCommand struct {
	Name              string
	AllocateSuccessor bool
	Claims            []string
}

type ResourceTransactionDeclaration struct {
	Prepare          ResourceTransactionHandler          // prepares a plain resource transaction
	PrepareComposite CompositeResourceTransactionHandler // prepares a composite resource transaction (child commands)
	Permit           lifecycle.LongLivedPlan             // long-lived plan for the successor
	PermitPolicy     SuccessorPermitPolicy               // when to issue the successor permit
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
	if rp.Argument >= 1_024 ||
		rp.Prefix == "" ||
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
	cleanupRef jobmgr.FunctionCleanupRef   // kernel cleanup ref for this generation
	id         string                      // generation identity
	handler    Handler                     // the Function handler
	cleanup    func(context.Context) error // collector cleanup callback (nil = no cleanup)

	routeReferences  int  // routes currently pointing at this generation
	invocationLeases int  // outstanding in-flight invocations
	admissionClosed  bool // no new invocations admitted (retiring)
	cleanupPending   bool // cleanup task dispatched, awaiting acknowledgement
	cleaned          bool // cleanup acknowledged; generation released
	cleanupFailed    bool // cleanup failed
	retentionCharged bool // the 4-KiB cleanup retention allowance is charged
}

func (hg *handlerGeneration) RunTask(ctx context.Context) (lifecycle.TaskOutcome, error) {
	if hg == nil || !hg.cleanupPending || hg.cleanup == nil {
		return lifecycle.NoValueOutcome(), errors.New("jobmgr Function handler: invalid cleanup work")
	}
	return lifecycle.NoValueOutcome(), hg.cleanup(ctx)
}

func (hg *handlerGeneration) census() HandlerGenerationCensus {
	if hg == nil {
		return HandlerGenerationCensus{}
	}
	return HandlerGenerationCensus{
		ID: hg.id, RouteReferences: hg.routeReferences,
		InvocationLeases: hg.invocationLeases, AdmissionClosed: hg.admissionClosed,
		CleanupPending: hg.cleanupPending, Cleaned: hg.cleaned,
		CleanupFailed: hg.cleanupFailed,
	}
}

type route struct {
	id                  uint64                          // stable catalog-assigned route identity
	publicName          string                          // exact Function name key into the name trie
	prefix              string                          // optional arg[0] prefix; empty = an exact route
	method              string                          // handler method ID passed to the generation on invoke
	handler             *handlerGeneration              // owning handler generation (shared across routes)
	resource            ResourcePolicy                  // policy deriving the resource ID from call args
	cooperativeCancel   bool                            // request honors cooperative cancellation
	cooperativeDeadline bool                            // request honors the caller deadline
	rawPayload          bool                            // skip JSON validation; pass the payload verbatim
	transaction         *ResourceTransactionDeclaration // optional resource-transaction declaration
	admissionClosed     bool                            // no new invocations admitted (retiring/closing)
	invocationLeases    int                             // outstanding in-flight invocations on this route
	retiring            bool                            // marked for removal by a committed mutation
	retiringDrained     bool                            // retiring AND all leases released (prune-eligible)
	retiringNamePath    []*catalogNode                  // pre-allocated name-trie node path used at prune time
	retiringPrefixPath  []*prefixNode                   // pre-allocated prefix-trie node path used at prune time
	retiringNext        *route                          // singly-linked deferred-prune list link
	closePrevious       *route                          // doubly-linked close-order list back link
	closeNext           *route                          // doubly-linked close-order list forward link
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
	for depth := 0; depth < len(argument)*8 && node != nil; depth++ {
		node = node.child[keyBit(argument, depth)]
		if (depth+1)%8 == 0 && node != nil && node.resolved != nil {
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
	for byteIndex := range len(prefix) {
		if node.resolved != nil {
			return nil, errors.New("jobmgr Function catalog: prefix overlaps a shorter prefix")
		}
		for depth := byteIndex * 8; depth < (byteIndex+1)*8; depth++ {
			branch := keyBit(prefix, depth)
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
	for depth := 0; depth < len(name)*8 && node != nil; depth++ {
		node = node.child[keyBit(name, depth)]
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
	for depth := range len(name) * 8 {
		branch := keyBit(name, depth)
		if node.child[branch] == nil {
			node.child[branch] = &catalogNode{}
		}
		node = node.child[branch]
	}
	node.routes = set
	node.present = !set.empty()
	return root
}

type invocationSlot struct {
	generation      uint64                         // bumped per reuse; validates the lease ref
	freeNext        uint32                         // next free slot index on the free-list
	resolved        *route                         // route this invocation targets
	input           HandlerInput                   // materialized handler input for the call
	claims          [17]string                     // fixed claim buffer: [0]=global claim, [1:]=command claims
	transactionPlan jobmgr.ResourceTransactionPlan // resource-transaction plan when the route is transactional
}

// maxCommandClaims is the number of per-command claim slots in an
// invocationSlot's fixed claim buffer; index 0 is reserved for the global claim.
const maxCommandClaims = len(invocationSlot{}.claims) - 1

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

type Catalog struct {
	routes      *catalogNode                                     // root of the public-name route trie
	generations map[jobmgr.FunctionCleanupRef]*handlerGeneration // handler generations by cleanup ref
	mutation    *MutationBuilder                                 // the in-flight mutation builder, if any
	storage     catalogStorage                                   // two-bucket storage budget (published + cleanup retention)

	invocations []*invocationSlot // invocation slot pool
	freeSlot    uint32            // head of the invocation-slot free-list

	closeHead *route // head of the close-order route list
	closeTail *route // tail of the close-order route list

	deferredPrune *route // routes whose prune is deferred while a mutation is active

	nextRouteID      uint64 // next route ID to assign
	nextGenerationID uint32 // next handler-generation ID to assign
	version          uint64 // catalog version, bumped per committed mutation
	routeCount       int    // count of live routes
	invocationCount  int    // count of active invocations
	pendingCleanups  int    // count of generations with cleanup pending
	completedCleanup int    // count of completed cleanups
	failedCleanup    int    // count of failed cleanups
	closed           bool   // catalog is closed (shutdown)
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
				cleanup:          declaration.Generation.Cleanup,
				retentionCharged: declaration.Generation.Cleanup != nil,
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
	var cleanupBytes int64
	for _, generation := range generationByDeclaration {
		if generation.retentionCharged {
			if err := addStorageProduct(
				&cleanupBytes,
				1,
				catalogGenerationRetentionBytes,
			); err != nil {
				return nil, err
			}
		}
	}
	if err := catalog.storage.initialize(
		catalogPathStorage(catalog.routes),
		cleanupBytes,
	); err != nil {
		return nil, err
	}
	return catalog, nil
}

func (c *Catalog) addInitial(declaration Declaration, generation *handlerGeneration) error {
	set, _ := catalogRouteSet(c.routes, declaration.PublicName)
	if !generation.cleanupRef.Valid() {
		c.nextGenerationID++
		if c.nextGenerationID == 0 {
			return errors.New("jobmgr Function catalog: handler generation wrapped")
		}
		generation.cleanupRef = jobmgr.FunctionCleanupRef{Slot: c.nextGenerationID, Generation: 1}
		c.generations[generation.cleanupRef] = generation
	}
	c.nextRouteID++
	if c.nextRouteID == 0 {
		return errors.New("jobmgr Function catalog: route identity wrapped")
	}
	resolved := &route{
		id: c.nextRouteID, publicName: declaration.PublicName,
		prefix: declaration.Prefix, method: declaration.ID,
		handler: generation, resource: declaration.Resource,
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
	c.routes = setInitialCatalogRouteSet(c.routes, declaration.PublicName, set)
	c.appendCloseRoute(resolved)
	c.routeCount++
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
			len(command.Name) > maximumDeclarationMetadataBytes ||
			len(command.Claims) > maxCommandClaims {
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
		hasStaticPermit := declaration.Permit.Class() != 0 ||
			declaration.Permit.Bytes() != 0
		hasPermitPolicy := declaration.PermitPolicy != 0
		if hasStaticPermit == hasPermitPolicy {
			return errors.New(
				"jobmgr Function catalog: successor transaction must declare exactly one permit source",
			)
		}
		if hasStaticPermit {
			if err := declaration.Permit.Validate(); err != nil {
				return err
			}
		} else if declaration.PermitPolicy !=
			SuccessorPermitSecretStorePayload {
			return errors.New(
				"jobmgr Function catalog: invalid successor permit policy",
			)
		}
	} else if declaration.Permit.Class() != 0 ||
		declaration.Permit.Bytes() != 0 ||
		declaration.PermitPolicy != 0 {
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
	set, ok := catalogRouteSet(c.routes, lookup.Route)
	if !ok {
		return jobmgr.FunctionCatalogDecision{
			Rejected: lifecycle.ControlNotFound,
		}, nil
	}
	resolved := set.resolve(lookup.Args)
	if resolved == nil || resolved.retiringDrained {
		return jobmgr.FunctionCatalogDecision{
			Rejected: lifecycle.ControlNotFound,
		}, nil
	}
	resourceID := resolved.resource.resolve(lookup.Args)
	generation := resolved.handler
	if resolved.admissionClosed || generation.admissionClosed {
		return jobmgr.FunctionCatalogDecision{Rejected: lifecycle.ControlUnavailable}, nil
	}
	command, transactionCommand := resourceTransactionCommand(
		resolved.transaction,
		lookup.Args,
	)
	var transactionPermit lifecycle.LongLivedPlan
	if transactionCommand && command.AllocateSuccessor {
		transactionPermit = resolved.transaction.Permit
		switch resolved.transaction.PermitPolicy {
		case 0:
		case SuccessorPermitSecretStorePayload:
			retained := int64(len(lookup.Payload))
			if retained == 0 {
				retained = 1
			}
			var err error
			transactionPermit, err =
				lifecycle.NewSecretStoreLongLivedPlan(retained)
			if err != nil {
				return jobmgr.FunctionCatalogDecision{}, err
			}
		default:
			return jobmgr.FunctionCatalogDecision{},
				errors.New(
					"jobmgr Function catalog: unknown successor permit policy",
				)
		}
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
		Runner:              slot,
		CooperativeCancel:   resolved.cooperativeCancel,
		CooperativeDeadline: resolved.cooperativeDeadline,
	}
	if transactionCommand {
		slot.claims[0] = resolved.transaction.GlobalClaim
		claimCount := 1 + copy(
			slot.claims[1:],
			command.Claims,
		)
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
			Claims:      slot.claims[:claimCount],
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
	slotGeneration := slot.generation
	*slot = invocationSlot{generation: slotGeneration, freeNext: c.freeSlot}
	c.freeSlot = ref.Slot
	c.retireDrainedRoute(resolved)
	return c.maybeCleanup(generation)
}

func (c *Catalog) maybeCleanup(generation *handlerGeneration) (jobmgr.FunctionCleanupPlan, error) {
	if generation == nil || generation.routeReferences < 0 || generation.invocationLeases < 0 {
		return jobmgr.FunctionCleanupPlan{}, errors.New("jobmgr Function catalog: invalid handler generation")
	}
	if !generation.admissionClosed || generation.routeReferences != 0 ||
		generation.invocationLeases != 0 || generation.cleanupPending || generation.cleaned {
		return jobmgr.FunctionCleanupPlan{}, nil
	}
	return c.cleanupDrainedGeneration(generation), nil
}

func (c *Catalog) retireDrainedRoute(retired *route) {
	if c == nil || retired == nil || !retired.retiring ||
		retired.retiringDrained || retired.invocationLeases != 0 {
		return
	}
	retired.retiringDrained = true
	if c.mutation != nil {
		retired.retiringNext = c.deferredPrune
		c.deferredPrune = retired
		return
	}
	c.pruneRetiringRoutes(retired)
}

func (c *Catalog) pruneRetiringRoutes(head *route) {
	var released int64
	for route := head; route != nil; {
		next := route.retiringNext
		route.retiringNext = nil
		released += pruneRetiringRoute(&c.routes, route)
		route = next
	}
	c.storage.releasePublishedPaths(released)
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

func (c *Catalog) cleanupDrainedGeneration(generation *handlerGeneration) jobmgr.FunctionCleanupPlan {
	if generation.cleanup == nil {
		generation.cleaned = true
		c.completedCleanup++
		delete(c.generations, generation.cleanupRef)
		return jobmgr.FunctionCleanupPlan{}
	}
	generation.cleanupPending = true
	c.pendingCleanups++
	return jobmgr.FunctionCleanupPlan{Ref: generation.cleanupRef, Runner: generation}
}

func (c *Catalog) CompleteCleanup(ref jobmgr.FunctionCleanupRef, cleanupErr error) error {
	if c == nil || !ref.Valid() {
		return errors.New("jobmgr Function catalog: invalid cleanup completion")
	}
	generation := c.generations[ref]
	if generation == nil || generation.cleanupRef != ref || !generation.cleanupPending ||
		generation.cleaned || generation.routeReferences != 0 || generation.invocationLeases != 0 {
		return errors.New("jobmgr Function catalog: stale cleanup completion")
	}
	if !generation.retentionCharged {
		return errors.New(
			"jobmgr Function catalog: cleanup has no generation retention",
		)
	}
	if err := c.storage.releaseCleanup(
		catalogGenerationRetentionBytes,
	); err != nil {
		return err
	}
	generation.retentionCharged = false
	generation.cleanupPending = false
	generation.cleaned = true
	generation.cleanupFailed = cleanupErr != nil
	c.pendingCleanups--
	c.completedCleanup++
	if cleanupErr != nil {
		c.failedCleanup++
	}
	delete(c.generations, ref)
	return nil
}

func (c *Catalog) BeginClose() error {
	if c == nil || c.closed || c.mutation != nil {
		return errors.New("jobmgr Function catalog: duplicate close")
	}
	c.closed = true
	return nil
}

func (c *Catalog) CloseStep(quantum int, cleanups *[jobmgr.MaximumFunctionCleanupBatch]jobmgr.FunctionCleanupPlan) (int, bool, error) {
	if c == nil || !c.closed || quantum <= 0 || quantum > MaximumCloseQuantum || cleanups == nil {
		return 0, false, errors.New("jobmgr Function catalog: invalid close step")
	}
	count := 0
	for quantum > 0 && c.closeHead != nil {
		resolved := c.closeHead
		c.unlinkCloseRoute(resolved)
		generation := resolved.handler
		if generation == nil || generation.routeReferences <= 0 {
			return count, true, errors.New("jobmgr Function catalog: route reference underflow")
		}
		generation.routeReferences--
		if generation.routeReferences == 0 {
			generation.admissionClosed = true
			cleanup, err := c.maybeCleanup(generation)
			if err != nil {
				return count, true, err
			}
			if cleanup.Ref.Valid() {
				cleanups[count] = cleanup
				count++
			}
		}
		c.routeCount--
		quantum--
	}
	more := c.closeHead != nil
	if !more {
		c.routes = nil
		if err := c.storage.releasePublished(); err != nil {
			return count, false, err
		}
	}
	return count, more, nil
}

func (c *Catalog) appendCloseRoute(resolved *route) {
	resolved.closePrevious = c.closeTail
	if c.closeTail != nil {
		c.closeTail.closeNext = resolved
	} else {
		c.closeHead = resolved
	}
	c.closeTail = resolved
}

func (c *Catalog) unlinkCloseRoute(resolved *route) {
	if resolved.closePrevious != nil {
		resolved.closePrevious.closeNext = resolved.closeNext
	} else {
		c.closeHead = resolved.closeNext
	}
	if resolved.closeNext != nil {
		resolved.closeNext.closePrevious = resolved.closePrevious
	} else {
		c.closeTail = resolved.closePrevious
	}
	resolved.closePrevious = nil
	resolved.closeNext = nil
}

func (c *Catalog) Census() CatalogCensus {
	if c == nil {
		return CatalogCensus{}
	}
	return CatalogCensus{
		Version: c.version, Routes: c.routeCount,
		InvocationLeases: c.invocationCount, PendingCleanups: c.pendingCleanups,
		CompletedCleanups: c.completedCleanup, FailedCleanups: c.failedCleanup,
		Closed: c.closed, CloseRoutesPending: c.routeCount,
		MutationActive: c.mutation != nil,
	}
}

func (c *Catalog) LifecycleCensus() jobmgr.FunctionCatalogCensus {
	return c.Census()
}

func (c *Catalog) HandlerCensus(ref jobmgr.FunctionCleanupRef) HandlerGenerationCensus {
	if c == nil {
		return HandlerGenerationCensus{}
	}
	return c.generations[ref].census()
}
