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
	invalidDynCfgResourceID         = "\x00dyncfg-invalid"
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
	Prepare          ResourceTransactionHandler
	PrepareComposite CompositeResourceTransactionHandler
	Permit           lifecycle.LongLivedPlan
	PermitPolicy     SuccessorPermitPolicy
	CommandArgument  uint16
	GlobalClaim      string
	Commands         []ResourceTransactionCommand
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
		if len(arguments) > 2 &&
			len(arguments) > 1 &&
			strings.EqualFold(arguments[1], "add") {
			name := dynCfgJobNameReplacer.Replace(arguments[2])
			if name != "" {
				return name
			}
		}
		return invalidDynCfgResourceID
	}
	module, name, hasName := strings.Cut(target, ":")
	if module == "" {
		return invalidDynCfgResourceID
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
	Resource            ResourcePolicy
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
	retentionCharged bool
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
	id                  uint64
	publicName          string
	prefix              string
	method              string
	handler             *handlerGeneration
	resource            ResourcePolicy
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
	claims          [17]string
	transactionPlan jobmgr.ResourceTransactionPlan
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
			len(command.Claims) > len(
				(invocationSlot{}).claims,
			)-1 {
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
			for previous := 0; previous < claimIndex; previous++ {
				if command.Claims[previous] == claim {
					return errors.New(
						"jobmgr Function catalog: duplicate command claim",
					)
				}
			}
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
	cloned.Commands = append(
		[]ResourceTransactionCommand(nil),
		declaration.Commands...,
	)
	for index := range cloned.Commands {
		cloned.Commands[index].Claims = append(
			[]string(nil),
			declaration.Commands[index].Claims...,
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

	slotIndex := c.freeLease
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
		c.freeLease = slot.freeNext
	}
	nextGeneration := slot.generation + 1
	if nextGeneration == 0 {
		slot.freeNext = c.freeLease
		c.freeLease = slotIndex
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
	*slot = invocationSlot{generation: slotGeneration, freeNext: c.freeLease}
	c.freeLease = ref.Slot
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

func (c *Catalog) pruneRetiringRoutes(routes *route) {
	var released int64
	for route := routes; route != nil; {
		next := route.retiringNext
		route.retiringNext = nil
		route.retiringDrained = true
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
