// SPDX-License-Identifier: GPL-3.0-or-later

package composition

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	agentdiscovery "github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/discovery"
	functionadapter "github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/functions"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/joboutput"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	frameworkfunctions "github.com/netdata/netdata/go/plugins/plugin/framework/functions"
	"github.com/netdata/netdata/go/plugins/plugin/framework/jobruntime"
	"github.com/netdata/netdata/go/plugins/plugin/framework/vnodes"
	"gopkg.in/yaml.v2"
)

// The leading NUL reserves an internal-only identity that cannot collide with
// DynCfg-valid vnode resource IDs.
const vnodeBootResourceID = "\x00jobmgr-vnode-boot"

type vnodeBinding struct {
	epoch       uint64                             // run generation
	pluginName  string                             // owning plugin name
	frames      *lifecycle.FrameOwner              // protocol frame sink
	config      *agentdiscovery.VNodeConfiguration // configured-vnode authority it mutates
	graph       *dyncfg.Graph                      // dyncfg graph for vnode config entries
	diagnostics jobmgr.DiagnosticObserver          // operational log sink
}

func newVNodeBinding(
	epoch uint64,
	pluginName string,
	frames *lifecycle.FrameOwner,
	config *agentdiscovery.VNodeConfiguration,
	graph *dyncfg.Graph,
	diagnostics jobmgr.DiagnosticObserver,
) (*vnodeBinding, error) {
	if epoch == 0 || pluginName == "" || frames == nil || config == nil || graph == nil {
		return nil, errors.New("jobmgr composition: invalid vnode binding")
	}
	binding := &vnodeBinding{
		epoch:       epoch,
		pluginName:  pluginName,
		frames:      frames,
		config:      config,
		graph:       graph,
		diagnostics: diagnostics,
	}
	if err := binding.validateInitial(); err != nil {
		return nil, err
	}
	return binding, nil
}

func (vb *vnodeBinding) prefix() string {
	return vb.pluginName + ":vnode"
}

func (vb *vnodeBinding) path() string {
	return fmt.Sprintf("/collectors/%s/Vnodes", vb.pluginName)
}

func (vb *vnodeBinding) handle(
	_ context.Context,
	input functionadapter.HandlerInput,
) (lifecycle.SealedResult, error) {
	command := vnodeCommand(input)
	switch command {
	case dyncfg.CommandSchema:
		return lifecycle.NewSealedResult(200, "application/json", []byte(vnodes.ConfigSchema))
	case dyncfg.CommandUserconfig:
		return vb.userConfig(input)
	case dyncfg.CommandGet:
		return vb.get(input)
	case dyncfg.CommandTest:
		return vb.test(input)
	default:
		return dynCfgMessage(
			501,
			fmt.Sprintf("Function '%s' command '%s' is not implemented.", input.Method, command),
		)
	}
}

func (vb *vnodeBinding) prepare(
	_ context.Context,
	input functionadapter.HandlerInput,
	current lifecycle.ReadyResource,
	scope lifecycle.ResourceTransactionScope,
	permit lifecycle.LongLivedPermit,
) (lifecycle.PreparedResourceTransaction, error) {
	if vb == nil || current != nil || scope.Current.Valid() ||
		scope.Successor.Valid() || permit.Valid() ||
		!scope.Valid() ||
		!strings.HasPrefix(scope.ID, "vnode:") {
		return nil, errors.New("jobmgr composition: invalid vnode transaction scope")
	}
	command := vnodeCommand(input)
	var transaction lifecycle.PreparedResourceTransaction
	var err error
	switch command {
	case dyncfg.CommandAdd:
		transaction, err = vb.prepareAdd(input, scope)
	case dyncfg.CommandUpdate:
		transaction, err = vb.prepareUpdate(input, scope)
	case dyncfg.CommandRemove:
		transaction, err = vb.prepareRemove(input, scope)
	default:
		transaction, err = vb.noop(scope, mustDynCfgMessage(501, "Vnode command is not implemented."), nil)
	}
	return vb.observeTransaction(command, scope.ID, transaction, err)
}

func newVNodeInitialRoute(epoch uint64, binding *vnodeBinding) (functionadapter.InitialRoute, error) {
	if epoch == 0 || binding == nil {
		return functionadapter.InitialRoute{}, errors.New("jobmgr composition: invalid vnode route")
	}
	return functionadapter.InitialRoute{
		Declaration: functionadapter.Declaration{
			ID: "dyncfg/vnodes",
			Generation: &functionadapter.HandlerGenerationDeclaration{
				ID:      fmt.Sprintf("dyncfg/vnodes/%d", epoch),
				Handler: binding.handle,
			},
			Transaction: &functionadapter.ResourceTransactionDeclaration{
				Prepare:         binding.prepare,
				CommandArgument: 1,
				GlobalClaim:     joboutput.DynCfgJobGraphClaim,
				Commands: []functionadapter.ResourceTransactionCommand{
					{Name: string(dyncfg.CommandAdd)},
					{Name: string(dyncfg.CommandUpdate)},
					{Name: string(dyncfg.CommandRemove)},
				},
			},
			PublicName:          joboutput.DynCfgFunctionName,
			Prefix:              binding.prefix(),
			Resource:            functionadapter.ScopedDynCfgJobResource(0, binding.prefix(), "vnode:"),
			CooperativeCancel:   true,
			CooperativeDeadline: true,
			RawPayload:          true,
		},
	}, nil
}

func (vb *vnodeBinding) prepareAdd(
	input functionadapter.HandlerInput,
	scope lifecycle.ResourceTransactionScope,
) (lifecycle.PreparedResourceTransaction, error) {
	if len(input.Args) < 3 {
		return vb.noop(
			scope,
			mustDynCfgMessage(400, fmt.Sprintf("missing required arguments: need 3, got %d", len(input.Args))),
			nil,
		)
	}
	name := vnodeJobName(input.Args[2])
	if name == "" {
		return vb.noop(scope, mustDynCfgMessage(400, "Missing vnode name."), nil)
	}
	if err := dyncfg.JobNameRuleAllowDots(name); err != nil {
		return vb.noop(
			scope,
			mustDynCfgMessage(400, fmt.Sprintf("Unacceptable vnode name '%s': %v.", name, err)),
			nil,
		)
	}
	next, failure := vb.parseVNode(input, name)
	if failure != nil {
		return vb.noop(scope, *failure, nil)
	}
	current, exists := vb.config.Lookup(name)
	expected := uint64(0)
	if exists {
		expected = current.Revision
	}
	prepared, err := vb.config.PrepareUpsert(name, expected, next)
	if errors.Is(err, agentdiscovery.ErrVNodeNoChange) {
		return vb.noop(scope, mustDynCfgMessage(202, ""), vb.configCreateCleanup(next))
	}
	if err != nil {
		return nil, err
	}
	return newPreparedVNodeTransaction(scope, prepared, mustDynCfgMessage(202, ""), vb.configCreateCleanup(next))
}

// resolveConfiguredVNode extracts the vnode name from input and looks up its
// current snapshot. On a malformed config ID or an unknown vnode it returns
// done=true with a ready noop transaction for the caller to return directly.
func (vb *vnodeBinding) resolveConfiguredVNode(
	input functionadapter.HandlerInput,
	scope lifecycle.ResourceTransactionScope,
) (name string, current jobruntime.VnodeSnapshot, done bool, result lifecycle.PreparedResourceTransaction, err error) {
	name, ok := vb.configName(input)
	if !ok {
		result, err = vb.noop(scope, mustDynCfgMessage(400, "invalid config ID format."), nil)
		return name, current, true, result, err
	}
	current, exists := vb.config.Lookup(name)
	if !exists {
		result, err = vb.noop(
			scope,
			mustDynCfgMessage(404, fmt.Sprintf("The specified vnode '%s' is not registered.", name)),
			nil,
		)
		return name, current, true, result, err
	}
	return name, current, false, nil, nil
}

func (vb *vnodeBinding) prepareUpdate(
	input functionadapter.HandlerInput,
	scope lifecycle.ResourceTransactionScope,
) (lifecycle.PreparedResourceTransaction, error) {
	name, current, done, result, err := vb.resolveConfiguredVNode(input, scope)
	if done {
		return result, err
	}
	next, failure := vb.parseVNode(input, name)
	if failure != nil {
		return vb.noop(scope, *failure, nil)
	}
	prepared, err := vb.config.PrepareUpsert(name, current.Revision, next)
	if errors.Is(err, agentdiscovery.ErrVNodeNoChange) {
		return vb.noop(scope, mustDynCfgMessage(202, ""), nil)
	}
	if err != nil {
		return nil, err
	}
	return newPreparedVNodeTransaction(scope, prepared, mustDynCfgMessage(202, ""), vb.configCreateCleanup(next))
}

func (vb *vnodeBinding) prepareRemove(
	input functionadapter.HandlerInput,
	scope lifecycle.ResourceTransactionScope,
) (lifecycle.PreparedResourceTransaction, error) {
	name, current, done, result, err := vb.resolveConfiguredVNode(input, scope)
	if done {
		return result, err
	}
	if current.Vnode.SourceType != confgroup.TypeDyncfg {
		return vb.noop(
			scope,
			mustDynCfgMessage(
				405,
				fmt.Sprintf(
					"Removing vnode of type '%s' is not supported. Only 'dyncfg' vnodes can be removed.",
					current.Vnode.SourceType,
				),
			),
			nil,
		)
	}
	if affected := vb.affectedJobs(name); len(affected) != 0 {
		return vb.noop(
			scope,
			mustDynCfgMessage(
				409,
				fmt.Sprintf(
					"The specified vnode '%s' is referenced by configs (%s).",
					name,
					strings.Join(affected, ", "),
				),
			),
			nil,
		)
	}
	prepared, err := vb.config.PrepareRemove(name, current.Revision)
	if err != nil {
		return nil, err
	}
	return newPreparedVNodeTransaction(scope, prepared, mustDynCfgMessage(200, ""), vb.configDeleteCleanup(name))
}

func (vb *vnodeBinding) get(input functionadapter.HandlerInput) (lifecycle.SealedResult, error) {
	name, ok := vb.configName(input)
	if !ok {
		return dynCfgMessage(400, "invalid config ID format.")
	}
	snapshot, exists := vb.config.Lookup(name)
	if !exists {
		return dynCfgMessage(404, fmt.Sprintf("The specified vnode '%s' is not registered.", name))
	}
	payload, err := json.Marshal(snapshot.Vnode)
	if err != nil {
		return lifecycle.SealedResult{}, err
	}
	return lifecycle.NewSealedResult(200, "application/json", payload)
}

func (vb *vnodeBinding) userConfig(input functionadapter.HandlerInput) (lifecycle.SealedResult, error) {
	var config vnodes.VirtualNode
	if err := unmarshalVNodePayload(input, &config); err != nil {
		return dynCfgMessage(
			400,
			fmt.Sprintf("Invalid configuration format. Failed to create configuration from payload: %v.", err),
		)
	}
	name := "test"
	if len(input.Args) >= 3 {
		if requested := vnodeJobName(input.Args[2]); requested != "" {
			name = requested
		}
	}
	normalizeVNode(&config, name, input.CallerSource)
	payload, err := yaml.Marshal([]any{config})
	if err != nil {
		return lifecycle.SealedResult{}, err
	}
	return lifecycle.NewSealedResult(200, "application/yaml", payload)
}

func (vb *vnodeBinding) test(input functionadapter.HandlerInput) (lifecycle.SealedResult, error) {
	if len(input.Args) < 3 {
		return dynCfgMessage(400, fmt.Sprintf("missing required arguments: need 3, got %d", len(input.Args)))
	}
	name := vnodeJobName(input.Args[2])
	next, failure := vb.parseVNode(input, name)
	if failure != nil {
		return *failure, nil
	}
	affected := vb.affectedJobs(next.Name)
	if len(affected) != 0 {
		return dynCfgMessage(202, "Updated configuration will affect configs: "+strings.Join(affected, ", ")+".")
	}
	return dynCfgMessage(202, "No configs will be affected by this change.")
}

func (vb *vnodeBinding) parseVNode(
	input functionadapter.HandlerInput,
	name string,
) (*vnodes.VirtualNode, *lifecycle.SealedResult) {
	if !input.HasPayload || len(input.Payload) == 0 {
		result := mustDynCfgMessage(400, "Missing configuration payload.")
		return nil, &result
	}
	var config vnodes.VirtualNode
	if err := unmarshalVNodePayload(input, &config); err != nil {
		result := mustDynCfgMessage(
			400,
			fmt.Sprintf("Failed to create configuration from payload. Invalid configuration format: %v.", err),
		)
		return nil, &result
	}
	normalizeVNode(&config, name, input.CallerSource)
	if err := vnodes.ValidateConfigured(&config); err != nil {
		result := mustDynCfgMessage(
			400,
			fmt.Sprintf("Failed to create configuration from payload. Invalid vnode configuration: %v.", err),
		)
		return nil, &result
	}
	if err := vb.validateUnique(&config); err != nil {
		result := mustDynCfgMessage(400, fmt.Sprintf("Failed to create configuration from payload: %v.", err))
		return nil, &result
	}
	return &config, nil
}

func (vb *vnodeBinding) configName(input functionadapter.HandlerInput) (string, bool) {
	if len(input.Args) == 0 {
		return "", false
	}
	name, ok := strings.CutPrefix(input.Args[0], vb.prefix()+":")
	return name, ok && name != ""
}

func (vb *vnodeBinding) validateUnique(next *vnodes.VirtualNode) error {
	nextGUID, err := vnodes.ConfiguredGUIDKey(next.GUID)
	if err != nil {
		return err
	}
	for _, entry := range vb.config.Entries() {
		current := entry.Snapshot.Vnode
		if entry.ID == next.Name {
			continue
		}
		if current.Hostname == next.Hostname {
			return fmt.Errorf("duplicate virtual node hostname detected (job '%s')", entry.ID)
		}
		currentGUID, err := vnodes.ConfiguredGUIDKey(current.GUID)
		if err != nil {
			return fmt.Errorf("invalid existing virtual node guid (job '%s'): %w", entry.ID, err)
		}
		if currentGUID == nextGUID {
			return fmt.Errorf("duplicate virtual node guid detected (job '%s')", entry.ID)
		}
	}
	return nil
}

func (vb *vnodeBinding) validateInitial() error {
	initial := make(map[string]*vnodes.VirtualNode)
	for _, entry := range vb.config.Entries() {
		initial[entry.ID] = entry.Snapshot.Vnode
	}
	return validateInitialVNodeSet(initial)
}

func validateInitialVNodeSet(initial map[string]*vnodes.VirtualNode) error {
	if err := vnodes.ValidateConfiguredSet(initial); err != nil {
		return fmt.Errorf("jobmgr composition: invalid initial vnode configuration: %w", err)
	}
	return nil
}

func (vb *vnodeBinding) affectedJobs(name string) []string {
	var affected []string
	for _, id := range vb.graph.IDs() {
		record, ok := vb.graph.Lookup(id)
		if !ok {
			continue
		}
		var config confgroup.Config
		if yaml.Unmarshal([]byte(record.Payload()), &config) != nil || config == nil || config.Vnode() != name {
			continue
		}
		affected = append(affected, fmt.Sprintf("%s:%s", config.Module(), config.Name()))
	}
	return affected
}

func (vb *vnodeBinding) noop(
	scope lifecycle.ResourceTransactionScope,
	result lifecycle.SealedResult,
	cleanup lifecycle.TaskCleanup,
) (lifecycle.PreparedResourceTransaction, error) {
	if cleanup == nil {
		cleanup = func() error { return nil }
	}
	return joboutput.PrepareNoopResourceTransaction(scope, nil, lifecycle.LongLivedPermit{}, result, cleanup, nil)
}

// configCreateVNodeJob emits the CONFIG CREATE that declares one vnode as a
// dyncfg job entry, with the removable command added for dyncfg-sourced vnodes.
func (vb *vnodeBinding) configCreateVNodeJob(api *netdataapi.API, vnode *vnodes.VirtualNode) error {
	commands := dyncfg.JoinCommands(
		dyncfg.CommandUserconfig,
		dyncfg.CommandSchema,
		dyncfg.CommandGet,
		dyncfg.CommandUpdate,
		dyncfg.CommandTest,
	)
	if vnode.SourceType == confgroup.TypeDyncfg {
		commands += " " + string(dyncfg.CommandRemove)
	}
	return api.TryCONFIGCREATE(netdataapi.ConfigOpts{
		ID:                vb.prefix() + ":" + vnode.Name,
		Status:            dyncfg.StatusRunning.String(),
		ConfigType:        dyncfg.ConfigTypeJob.String(),
		Path:              vb.path(),
		SourceType:        vnode.SourceType,
		Source:            vnode.Source,
		SupportedCommands: commands,
	})
}

func (vb *vnodeBinding) configCreateCleanup(vnode *vnodes.VirtualNode) lifecycle.TaskCleanup {
	return vb.protocolCleanup(func(api *netdataapi.API) error {
		return vb.configCreateVNodeJob(api, vnode)
	})
}

func (vb *vnodeBinding) configDeleteCleanup(name string) lifecycle.TaskCleanup {
	return vb.protocolCleanup(func(api *netdataapi.API) error {
		return api.TryCONFIGDELETE(vb.prefix() + ":" + name)
	})
}

func (vb *vnodeBinding) initialCleanup() lifecycle.TaskCleanup {
	return vb.protocolCleanup(func(api *netdataapi.API) error {
		if err := api.TryCONFIGCREATE(netdataapi.ConfigOpts{
			ID:         vb.prefix(),
			Status:     dyncfg.StatusAccepted.String(),
			ConfigType: dyncfg.ConfigTypeTemplate.String(),
			Path:       vb.path(),
			SourceType: "internal",
			Source:     "internal",
			SupportedCommands: dyncfg.JoinCommands(
				dyncfg.CommandAdd,
				dyncfg.CommandSchema,
				dyncfg.CommandUserconfig,
				dyncfg.CommandTest,
			),
		}); err != nil {
			return err
		}
		for _, entry := range vb.config.Entries() {
			if err := vb.configCreateVNodeJob(api, entry.Snapshot.Vnode); err != nil {
				return err
			}
		}
		return nil
	})
}

func (vb *vnodeBinding) protocolCleanup(build func(*netdataapi.API) error) lifecycle.TaskCleanup {
	var payload bytes.Buffer
	if err := build(netdataapi.New(&payload)); err != nil {
		return func() error { return err }
	}
	prepared, err := lifecycle.PrepareProtocolFrame(payload.Bytes())
	if err != nil {
		return func() error { return err }
	}
	return func() error {
		return vb.frames.CommitPreparedProtocolFrame(prepared)
	}
}

func (vb *vnodeBinding) publishInitial(ctx context.Context, commands jobmgr.PreparedCommandPort) error {
	if vb == nil || ctx == nil || commands == nil {
		return errors.New("jobmgr composition: invalid vnode initial publication")
	}
	result, err := lifecycle.NewSealedResult(204, "application/json", nil)
	if err != nil {
		return err
	}
	plan := jobmgr.WorkPlan{
		Claims:     []string{joboutput.DynCfgJobGraphClaim},
		NoResponse: true,
		Transaction: &jobmgr.ResourceTransactionPlan{
			ID: vnodeBootResourceID,
			Prepare: func(
				_ context.Context,
				current lifecycle.ReadyResource,
				scope lifecycle.ResourceTransactionScope,
				permit lifecycle.LongLivedPermit,
			) (lifecycle.PreparedResourceTransaction, error) {
				if current != nil ||
					scope.ID != vnodeBootResourceID ||
					scope.Current.Valid() ||
					scope.Successor.Valid() ||
					permit.Valid() {
					return nil, errors.New("jobmgr composition: invalid vnode boot scope")
				}
				return joboutput.PrepareNoopResourceTransaction(
					scope,
					nil,
					lifecycle.LongLivedPermit{},
					result,
					vb.initialCleanup(),
					nil,
				)
			},
		},
	}
	return commands.SubmitPreparedAndWait(
		ctx,
		jobmgr.Request{
			UID:     fmt.Sprintf("jobmgr-vnodes-%d", vb.epoch),
			LaneKey: vnodeBootResourceID,
			Source:  lifecycle.SourceJobManager,
			Route:   "internal/vnodes/publish",
		},
		plan,
	)
}

type preparedVNodeTransaction struct {
	mu sync.Mutex // guards consumed (single-shot take)

	consumed bool                               // the transaction has been taken (apply/dispose)
	scope    lifecycle.ResourceTransactionScope // resource-transaction scope this vnode change belongs to
	prepared agentdiscovery.PreparedVNode       // the prepared vnode mutation to commit
	result   lifecycle.SealedResult             // sealed dyncfg response
	cleanup  lifecycle.TaskCleanup              // post-commit protocol emit
}

func newPreparedVNodeTransaction(
	scope lifecycle.ResourceTransactionScope,
	prepared agentdiscovery.PreparedVNode,
	result lifecycle.SealedResult,
	cleanup lifecycle.TaskCleanup,
) (*preparedVNodeTransaction, error) {
	if cleanup == nil {
		return nil, errors.New("jobmgr composition: vnode transaction has no cleanup")
	}
	if _, err := lifecycle.NewAppliedResourceTransaction(
		scope,
		lifecycle.ResourceTransactionUnchanged,
		nil,
		result,
		cleanup,
	); err != nil {
		return nil, err
	}
	return &preparedVNodeTransaction{
		scope:    scope,
		prepared: prepared,
		result:   result,
		cleanup:  cleanup,
	}, nil
}

func (pvt *preparedVNodeTransaction) Scope() lifecycle.ResourceTransactionScope {
	if pvt == nil {
		return lifecycle.ResourceTransactionScope{}
	}
	pvt.mu.Lock()
	defer pvt.mu.Unlock()
	if pvt.consumed {
		return lifecycle.ResourceTransactionScope{}
	}
	return pvt.scope
}

func (pvt *preparedVNodeTransaction) Apply(context.Context) (lifecycle.AppliedResourceTransaction, error) {
	scope, prepared, result, cleanup, err := pvt.take()
	if err != nil {
		return lifecycle.AppliedResourceTransaction{}, err
	}
	if _, err := prepared.Commit(); err != nil {
		return lifecycle.AppliedResourceTransaction{}, errors.Join(err, prepared.Abort())
	}
	return lifecycle.NewAppliedResourceTransaction(
		scope,
		lifecycle.ResourceTransactionUnchanged,
		nil,
		result,
		cleanup,
	)
}

func (pvt *preparedVNodeTransaction) Dispose(context.Context) (lifecycle.ReadyResource, error) {
	_, prepared, _, _, err := pvt.take()
	if err != nil {
		return nil, err
	}
	return nil, prepared.Abort()
}

func (pvt *preparedVNodeTransaction) take() (
	lifecycle.ResourceTransactionScope,
	agentdiscovery.PreparedVNode,
	lifecycle.SealedResult,
	lifecycle.TaskCleanup,
	error,
) {
	if pvt == nil {
		return lifecycle.ResourceTransactionScope{},
			agentdiscovery.PreparedVNode{},
			lifecycle.SealedResult{},
			nil,
			errors.New("jobmgr composition: nil vnode transaction")
	}
	pvt.mu.Lock()
	defer pvt.mu.Unlock()
	if pvt.consumed {
		return lifecycle.ResourceTransactionScope{},
			agentdiscovery.PreparedVNode{},
			lifecycle.SealedResult{},
			nil,
			errors.New("jobmgr composition: vnode transaction consumed")
	}
	pvt.consumed = true
	return pvt.scope, pvt.prepared, pvt.result, pvt.cleanup, nil
}

func vnodeCommand(input functionadapter.HandlerInput) dyncfg.Command {
	return dyncfg.CommandFromArgs(input.Args)
}

func vnodeJobName(value string) string {
	return dyncfg.NormalizeJobName(value)
}

func normalizeVNode(vnode *vnodes.VirtualNode, name string, source string) {
	vnode.Name = name
	if vnode.Hostname == "" {
		vnode.Hostname = name
	}
	vnode.Source = source
	vnode.SourceType = confgroup.TypeDyncfg
}

func unmarshalVNodePayload(input functionadapter.HandlerInput, target *vnodes.VirtualNode) error {
	if input.ContentType == "application/json" {
		return json.Unmarshal(input.Payload, target)
	}
	return yaml.Unmarshal(input.Payload, target)
}

func dynCfgMessage(code int, message string) (lifecycle.SealedResult, error) {
	return lifecycle.NewSealedResult(code, "application/json", frameworkfunctions.BuildJSONPayload(code, message))
}

func mustDynCfgMessage(code int, message string) lifecycle.SealedResult {
	result, err := dynCfgMessage(code, message)
	if err != nil {
		panic(err)
	}
	return result
}
