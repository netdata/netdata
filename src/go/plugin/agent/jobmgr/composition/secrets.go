// SPDX-License-Identifier: GPL-3.0-or-later

package composition

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	functionadapter "github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/functions"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/joboutput"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/secretsctl"
	secretresolver "github.com/netdata/netdata/go/plugins/plugin/agent/secrets/resolver"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	frameworkfunctions "github.com/netdata/netdata/go/plugins/plugin/framework/functions"
	"gopkg.in/yaml.v2"
)

const dynCfgSecretGraphClaim = "dyncfg:secretstores"

type runSecretServices struct {
	Initial []secretstore.Config
}

type secretBinding struct {
	mu sync.Mutex

	epoch      uint64
	pluginName string
	capture    *legacyProtocolCapture
	controller *secretsctl.Controller
	jobs       *joboutput.DynCfgJobController
	graph      *dyncfg.Graph
	commands   jobmgr.PreparedCommandPort
	nextUID    uint64
}

func newSecretBinding(
	epoch uint64,
	pluginName string,
	frames *lifecycle.FrameOwner,
) (*secretBinding, error) {
	if epoch == 0 || pluginName == "" || frames == nil {
		return nil, errors.New("jobmgr composition: invalid secret binding")
	}
	capture, err := newLegacyProtocolCapture(frames)
	if err != nil {
		return nil, err
	}
	return &secretBinding{
		epoch: epoch, pluginName: pluginName, capture: capture,
	}, nil
}

func (binding *secretBinding) prefix() string {
	return fmt.Sprintf("%s:secretstore:", binding.pluginName)
}

func (binding *secretBinding) bind(
	service secretstore.Service,
	initial []secretstore.Config,
	jobs *joboutput.DynCfgJobController,
	graph *dyncfg.Graph,
) error {
	if binding == nil || service == nil || jobs == nil || graph == nil {
		return errors.New("jobmgr composition: invalid secret controller binding")
	}
	binding.mu.Lock()
	defer binding.mu.Unlock()
	if binding.controller != nil {
		return errors.New("jobmgr composition: duplicate secret controller binding")
	}
	responder := dyncfg.NewResponder(netdataapi.New(binding.capture))
	binding.jobs = jobs
	binding.graph = graph
	binding.controller = secretsctl.New(secretsctl.Options{
		Logger:  logger.New(),
		API:     responder,
		Service: service,
		Plugin:  binding.pluginName,
		Initial: append([]secretstore.Config(nil), initial...),
		AffectedJobs: func(key string) []secretstore.JobRef {
			return binding.affectedJobs(key, false)
		},
		RestartableAffectedJobs: func(key string) []secretstore.JobRef {
			return binding.affectedJobs(key, true)
		},
		StageDependentRestarts: binding.stageDependentRestarts,
	})
	return nil
}

func (binding *secretBinding) bindCommands(
	commands jobmgr.PreparedCommandPort,
) error {
	if binding == nil || commands == nil {
		return errors.New("jobmgr composition: invalid secret command binding")
	}
	binding.mu.Lock()
	defer binding.mu.Unlock()
	if binding.controller == nil || binding.commands != nil {
		return errors.New("jobmgr composition: late or duplicate secret command binding")
	}
	binding.commands = commands
	return nil
}

func (binding *secretBinding) prepare(
	_ context.Context,
	input functionadapter.HandlerInput,
	current lifecycle.ReadyResource,
	scope lifecycle.ResourceTransactionScope,
	permit lifecycle.LongLivedPermit,
) (lifecycle.PreparedResourceTransaction, error) {
	if binding == nil || current != nil || scope.Current.Valid() ||
		scope.Successor.Valid() || permit.Valid() {
		return nil, errors.New("jobmgr composition: invalid secret transaction scope")
	}
	binding.mu.Lock()
	controller := binding.controller
	binding.mu.Unlock()
	if controller == nil {
		return nil, errors.New("jobmgr composition: unbound secret controller")
	}
	fn := dyncfg.NewFunction(frameworkfunctions.Function{
		UID: input.UID, Timeout: input.Timeout,
		Name: input.Method, Args: append([]string(nil), input.Args...),
		Payload:     append([]byte(nil), input.Payload...),
		Permissions: input.Permissions, Source: input.CallerSource,
		ContentType: input.ContentType,
	})
	result, cleanup, err := binding.capture.invoke(
		input.UID,
		func() { controller.SeqExec(fn) },
	)
	if err != nil {
		return nil, err
	}
	return joboutput.PrepareNoopResourceTransaction(
		scope,
		nil,
		lifecycle.LongLivedPermit{},
		result,
		cleanup,
	)
}

func newSecretInitialRoute(
	epoch uint64,
	binding *secretBinding,
) (functionadapter.InitialRoute, error) {
	if epoch == 0 || binding == nil {
		return functionadapter.InitialRoute{},
			errors.New("jobmgr composition: invalid secret route")
	}
	commands := []functionadapter.ResourceTransactionCommand{
		{Name: string(dyncfg.CommandAdd)},
		{Name: string(dyncfg.CommandSchema)},
		{Name: string(dyncfg.CommandGet)},
		{Name: string(dyncfg.CommandUpdate)},
		{Name: string(dyncfg.CommandTest)},
		{Name: string(dyncfg.CommandUserconfig)},
		{Name: string(dyncfg.CommandRemove)},
	}
	return functionadapter.InitialRoute{
		Declaration: functionadapter.Declaration{
			ID: "dyncfg/secretstores",
			Generation: &functionadapter.HandlerGenerationDeclaration{
				ID: fmt.Sprintf("dyncfg/secretstores/%d", epoch),
				Handler: func(
					context.Context,
					functionadapter.HandlerInput,
				) (lifecycle.SealedResult, error) {
					return lifecycle.SealedResult{},
						errors.New("jobmgr composition: secret route requires transaction")
				},
			},
			Transaction: &functionadapter.ResourceTransactionDeclaration{
				Prepare:         binding.prepare,
				CommandArgument: 1,
				GlobalClaim:     dynCfgSecretGraphClaim,
				Commands:        commands,
			},
			PublicName: joboutput.DynCfgFunctionName,
			Prefix:     binding.prefix(),
			Lane: functionadapter.ScopedDynCfgJobLane(
				0,
				binding.prefix(),
				"secretstore:",
			),
			CooperativeCancel:   true,
			CooperativeDeadline: true,
			RawPayload:          true,
		},
		Publication: dynCfgPublication(epoch),
	}, nil
}

func dynCfgPublication(epoch uint64) functionadapter.PublicationRecord {
	return functionadapter.PublicationRecord{
		Name:       joboutput.DynCfgFunctionName,
		Generation: epoch,
		Timeout:    120,
		Help:       "dynamic configuration",
		Tags:       "top",
		Access:     "0x0013",
		Priority:   100,
		Version:    3,
	}
}

func (binding *secretBinding) publishInitial(
	ctx context.Context,
	commands jobmgr.PreparedCommandPort,
) error {
	if binding == nil || ctx == nil || commands == nil {
		return errors.New("jobmgr composition: invalid secret initial publication")
	}
	binding.mu.Lock()
	controller := binding.controller
	binding.mu.Unlock()
	if controller == nil {
		return errors.New("jobmgr composition: missing secret controller")
	}
	result, err := lifecycle.NewSealedResult(204, "application/json", nil)
	if err != nil {
		return err
	}
	plan := jobmgr.WorkPlan{
		Claims:     []string{dynCfgSecretGraphClaim},
		NoResponse: true,
		Transaction: &jobmgr.ResourceTransactionPlan{
			ID: "__jobmgr_secret_boot__",
			Prepare: func(
				_ context.Context,
				current lifecycle.ReadyResource,
				scope lifecycle.ResourceTransactionScope,
				permit lifecycle.LongLivedPermit,
			) (lifecycle.PreparedResourceTransaction, error) {
				if current != nil ||
					scope.ID != "__jobmgr_secret_boot__" ||
					scope.Current.Valid() ||
					scope.Successor.Valid() ||
					permit.Valid() {
					return nil, errors.New(
						"jobmgr composition: invalid secret publication scope",
					)
				}
				cleanup, prepareErr := binding.capture.preparePublication(func() {
					controller.CreateTemplates()
					controller.PublishExisting()
				})
				if prepareErr != nil {
					return nil, prepareErr
				}
				return joboutput.PrepareNoopResourceTransaction(
					scope,
					nil,
					lifecycle.LongLivedPermit{},
					result,
					cleanup,
				)
			},
		},
	}
	return commands.SubmitPreparedAndWait(
		ctx,
		jobmgr.Request{
			UID:     fmt.Sprintf("jobmgr-secrets-%d", binding.epoch),
			LaneKey: "__jobmgr_secret_boot__",
			Source:  lifecycle.SourceJobManager,
			Route:   "internal/secrets/publish",
		},
		plan,
	)
}

func (binding *secretBinding) affectedJobs(
	storeKey string,
	runningOnly bool,
) []secretstore.JobRef {
	binding.mu.Lock()
	graph := binding.graph
	binding.mu.Unlock()
	if graph == nil {
		return nil
	}
	var refs []secretstore.JobRef
	for _, id := range graph.IDs() {
		record, ok := graph.Lookup(id)
		if !ok || runningOnly &&
			record.Status != dyncfg.StatusRunning.String() {
			continue
		}
		var config confgroup.Config
		if yaml.Unmarshal([]byte(record.Payload()), &config) != nil ||
			config == nil {
			continue
		}
		keys, err := secretresolver.StoreReferences(config)
		if err != nil || !containsString(keys, storeKey) {
			continue
		}
		refs = append(refs, secretstore.JobRef{
			ID: id, Display: config.Module() + ":" + config.Name(),
		})
	}
	sort.Slice(refs, func(i, j int) bool {
		if refs[i].ID == refs[j].ID {
			return refs[i].Display < refs[j].Display
		}
		return refs[i].ID < refs[j].ID
	})
	return refs
}

func containsString(values []string, want string) bool {
	index := sort.SearchStrings(values, strings.TrimSpace(want))
	return index < len(values) && values[index] == strings.TrimSpace(want)
}

func (binding *secretBinding) stageDependentRestarts(
	storeKey string,
) *secretsctl.StagedRestarts {
	refs := binding.affectedJobs(storeKey, true)
	return &secretsctl.StagedRestarts{
		Run: func(ctx context.Context) (string, error) {
			var failures []string
			for _, ref := range refs {
				if err := binding.restartJob(ctx, ref.ID); err != nil {
					failures = append(
						failures,
						fmt.Sprintf("%s (%v)", ref.Display, err),
					)
				}
			}
			if len(failures) == 0 {
				return "", nil
			}
			return "Secretstore change applied, but dependent collector restarts failed: " +
				strings.Join(failures, "; ") + ".", nil
		},
		Flush: func() {},
	}
}

func (binding *secretBinding) restartJob(
	ctx context.Context,
	id string,
) error {
	binding.mu.Lock()
	graph, jobs, commands := binding.graph, binding.jobs, binding.commands
	binding.nextUID++
	nextUID := binding.nextUID
	binding.mu.Unlock()
	if graph == nil || jobs == nil || commands == nil || nextUID == 0 {
		return errors.New("jobmgr composition: secret restart bridge is unavailable")
	}
	record, ok := graph.Lookup(id)
	if !ok || record.Status != dyncfg.StatusRunning.String() {
		return nil
	}
	var config confgroup.Config
	if err := yaml.Unmarshal([]byte(record.Payload()), &config); err != nil {
		return err
	}
	plan, err := jobs.PlanDiscovered(joboutput.DiscoveredJobChange{
		Config: config, Status: dyncfg.StatusRunning, Restart: true,
	})
	if err != nil {
		return err
	}
	return commands.SubmitPreparedAndWait(
		ctx,
		jobmgr.Request{
			UID: fmt.Sprintf(
				"jobmgr-secret-restart-%d-%d",
				binding.epoch,
				nextUID,
			),
			LaneKey: id,
			Source:  lifecycle.SourceJobManager,
			Route:   "internal/secrets/restart",
		},
		plan,
	)
}
