// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/stretchr/testify/require"
)

// TestDynCfgJobResultsHaveOneOwner keeps every result of the collector-job
// DynCfg Function in dyncfg_reply.go, where codes follow from the outcome.
func TestDynCfgJobResultsHaveOneOwner(t *testing.T) {
	files, err := filepath.Glob("*.go")
	require.NoError(t, err)
	builders := make(map[string][]string)
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		source, err := os.ReadFile(path)
		require.NoError(t, err)
		file, err := parser.ParseFile(token.NewFileSet(), path, source, 0)
		require.NoError(t, err)
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			switch fun := call.Fun.(type) {
			case *ast.SelectorExpr:
				if pkg, ok := fun.X.(*ast.Ident); ok && pkg.Name == "lifecycle" &&
					(fun.Sel.Name == "NewSealedResult" || fun.Sel.Name == "NewControlResult") {
					builders[path] = append(builders[path], "lifecycle."+fun.Sel.Name)
				}
			case *ast.Ident:
				if fun.Name == "messageResult" {
					builders[path] = append(builders[path], fun.Name)
				}
			}
			return true
		})
	}
	for path := range builders {
		require.Equal(t, "dyncfg_reply.go", path, "result built outside the reply owner: %v", builders[path])
	}
}

func TestAdoptedUpdateWithBusyRuntimeStartsOnceTheRuntimeReleases(t *testing.T) {
	controller, graph, _, _, state := newDynCfgJobTestHarness(t)
	useV2CheckFailureCollector(controller, state, nil)
	commands := &activationTestCommands{queue: make(chan activationTestSubmission, 8), stop: make(chan struct{})}
	require.NoError(t, controller.BindBackgroundWorkers(commands, 9, func(err error) {
		t.Errorf("background worker failed: %v", err)
	}))
	t.Cleanup(func() {
		close(commands.stop)
		controller.scheduler.StopBackgroundWorkers()
		require.NoError(t, controller.scheduler.WaitBackgroundWorkers(context.Background()))
	})
	release := make(chan struct{})
	controller.factory.config.Attempts = releaseBusyRuntimeAuthority{
		ProcessAttemptAuthority: controller.factory.config.Attempts,
		release:                 release,
	}

	config := factoryTestConfig(false)
	config.SetSourceType(confgroup.TypeDyncfg)
	config.SetSource("user=test")
	config.SetProvider(confgroup.TypeDyncfg)
	seedDynCfgJobGraphRecord(t, graph, config, dyncfg.StatusRunning)
	record, exists := graph.Lookup(config.FullName())
	require.True(t, exists)
	var events []string
	current := &transactionTestReadyResource{
		identity: lifecycle.ResourceIdentity{ID: config.FullName(), Generation: 1},
		prefix:   "current",
		events:   &events,
	}
	permit, _ := issueTestJobPermit(t, config.FullName(), 2)
	scope := lifecycle.ResourceTransactionScope{
		ID:        config.FullName(),
		Current:   current.identity,
		Successor: lifecycle.ResourceIdentity{ID: config.FullName(), Generation: 2},
	}
	transaction, err := controller.prepareUpdate(
		context.Background(),
		DynCfgJobRequest{
			Payload:      []byte(`{"option_str":"replacement"}`),
			ContentType:  "application/json",
			CallerSource: "user=test",
			HasPayload:   true,
		},
		dynCfgTarget{
			command:    dyncfg.CommandUpdate,
			module:     config.Module(),
			name:       config.Name(),
			resourceID: config.FullName(),
			creator:    controller.modules["module"],
		},
		record,
		true,
		current,
		scope,
		permit,
	)
	require.NoError(t, err)
	applied, err := transaction.Apply(context.Background())
	require.NoError(t, err)
	require.Equal(t, 202, applied.ResultStatus())
	record, exists = graph.Lookup(config.FullName())
	require.True(t, exists)
	require.Equal(t, dyncfg.StatusAccepted.String(), record.Status)

	// The release signal admits one staged attempt under the accepted config.
	close(release)
	call := commands.next(t, "internal/jobs/accepted-activation")
	require.Equal(t, config.FullName(), call.request.LaneKey)
	running := applyActivationTestSubmission(t, call, nil, 3)
	t.Cleanup(func() { stopRuntimeTestResource(t, running) })
	require.NotNil(t, running)
	running = applyActivationTestSubmission(t, commands.next(t, "internal/jobs/runtime-ready"), running, 0)
	record, exists = graph.Lookup(config.FullName())
	require.True(t, exists)
	require.Equal(t, dyncfg.StatusRunning.String(), record.Status)
	require.Contains(t, record.Payload(), "replacement")
}

func TestRolledBackTransactionAnswersUnavailable(t *testing.T) {
	tests := map[string]struct {
		reply jobReply
		want  int
	}{
		"command": {
			reply: adoptedReply(dyncfg.CommandUpdate, dyncfg.StatusRunning, jobFailure{}),
			want:  503,
		},
		"response-free work": {
			reply: internalReply(),
			want:  204,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			identity := lifecycle.ResourceIdentity{ID: "job", Generation: 1}
			graph, err := dyncfg.NewGraph(nil)
			require.NoError(t, err)
			postimage := dyncfg.GraphConfig{ID: identity.ID, Module: "module", Name: "job", Status: dyncfg.StatusRunning.String()}
			mutation, err := graph.PrepareMutation([]dyncfg.GraphChange{{ID: identity.ID, Config: &postimage}})
			require.NoError(t, err)
			var events []string
			transaction, err := PrepareResourceTransaction(ResourceTransactionSpec{
				Scope:       lifecycle.ResourceTransactionScope{ID: identity.ID, Successor: identity},
				Disposition: lifecycle.ResourceTransactionInstalled,
				Successor: &transactionTestPreparedResource{
					identity:  identity,
					events:    &events,
					acceptErr: jobmgr.ErrProcessAttemptRetired,
				},
				Graph:            graph,
				Mutation:         mutation,
				MutationPrepared: true,
				Cleanup:          func() error { return nil },
				reply:            &test.reply,
			})
			require.NoError(t, err)

			applied, err := transaction.Apply(context.Background())
			require.NoError(t, err)
			// Run retirement rolled the change back, so the reply cannot claim it.
			require.Equal(t, test.want, applied.ResultStatus())
			_, exists := graph.Lookup(identity.ID)
			require.False(t, exists)
		})
	}
}

func TestAdoptedUpdateAnswersWithinTheRequestDeadline(t *testing.T) {
	controller, graph, supervisor, output, state := newDynCfgJobTestHarness(t)
	useV2CheckFailureCollector(controller, state, nil)
	controller.factory.config.Attempts = blockingRuntimeTestAuthority{delegate: controller.factory.config.Attempts}

	config := factoryTestConfig(false)
	config.SetSourceType(confgroup.TypeDyncfg)
	config.SetSource("user=test")
	config.SetProvider(confgroup.TypeDyncfg)
	seedDynCfgJobGraphRecord(t, graph, config, dyncfg.StatusRunning)
	var events []string
	scope := lifecycle.ResourceTransactionScope{
		ID:        config.FullName(),
		Current:   lifecycle.ResourceIdentity{ID: config.FullName(), Generation: 1},
		Successor: lifecycle.ResourceIdentity{ID: config.FullName(), Generation: 2},
	}
	current := &transactionTestReadyResource{identity: scope.Current, prefix: "current", events: &events}
	request := DynCfgJobRequest{
		Args:         []string{"go.d:collector:module:job", string(dyncfg.CommandUpdate)},
		Payload:      []byte(`{"option_str":"replacement"}`),
		ContentType:  "application/json",
		CallerSource: "user=test",
		HasPayload:   true,
	}
	deadline := time.Now().Add(200 * time.Millisecond)
	plan, err := lifecycle.NewResourceTransactionPermitTaskPlan(
		lifecycle.SourceFunction,
		deadline,
		lifecycle.TransactionTaskPhases,
		current,
		scope,
		lifecycle.NewJobLongLivedPlan(),
		func(
			ctx context.Context,
			current lifecycle.ReadyResource,
			taskScope lifecycle.ResourceTransactionScope,
			permit lifecycle.LongLivedPermit,
		) (lifecycle.PreparedResourceTransaction, error) {
			return controller.Prepare(ctx, request, current, taskScope, permit)
		},
	)
	require.NoError(t, err)

	// A busy previous runtime must not keep acceptance waiting for its deadline.
	applyAndEncodeDynCfgJobTestTask(t, supervisor, plan, scope, "update-within-deadline")
	require.True(t, time.Now().Before(deadline), "acceptance must finish before the request deadline")
	require.Contains(t, output.String(), "FUNCTION_RESULT_BEGIN update-within-deadline 202 application/json")
	record, exists := graph.Lookup(config.FullName())
	require.True(t, exists)
	require.Equal(t, dyncfg.StatusAccepted.String(), record.Status)
}

// blockingRuntimeTestAuthority keeps the job runtime identity busy and never
// sees the previous runtime release.
type blockingRuntimeTestAuthority struct {
	delegate jobmgr.ProcessAttemptAuthority
}

func (a blockingRuntimeTestAuthority) StartProcessAttempt(
	ctx context.Context,
	plan jobmgr.ProcessAttemptPlan,
) (jobmgr.ProcessAttempt, error) {
	if plan.Identity.Namespace == jobmgr.ProcessAttemptJobRuntime {
		return nil, jobmgr.ErrProcessAttemptBusy
	}
	return a.delegate.StartProcessAttempt(ctx, plan)
}

func (a blockingRuntimeTestAuthority) SupersedeProcessAttempt(
	ctx context.Context,
	identity jobmgr.ProcessAttemptIdentity,
) error {
	if identity.Namespace == jobmgr.ProcessAttemptJobRuntime {
		<-ctx.Done()
		return ctx.Err()
	}
	return a.delegate.SupersedeProcessAttempt(ctx, identity)
}

func (a blockingRuntimeTestAuthority) CutProcessAttempt(identity jobmgr.ProcessAttemptIdentity, cause error) bool {
	return a.delegate.CutProcessAttempt(identity, cause)
}

func (a blockingRuntimeTestAuthority) ProcessAttemptReleased(
	identity jobmgr.ProcessAttemptIdentity,
) (<-chan struct{}, bool) {
	return a.delegate.ProcessAttemptReleased(identity)
}

// The same authority owns the busy interval and its physical-release signal.
type releaseBusyRuntimeAuthority struct {
	jobmgr.ProcessAttemptAuthority
	release <-chan struct{}
}

func (a releaseBusyRuntimeAuthority) StartProcessAttempt(ctx context.Context, plan jobmgr.ProcessAttemptPlan) (jobmgr.ProcessAttempt, error) {
	if plan.Identity.Namespace == jobmgr.ProcessAttemptJobRuntime {
		select {
		case <-a.release:
		default:
			return nil, jobmgr.ErrProcessAttemptBusy
		}
	}
	return a.ProcessAttemptAuthority.StartProcessAttempt(ctx, plan)
}
func (a releaseBusyRuntimeAuthority) ProcessAttemptReleased(identity jobmgr.ProcessAttemptIdentity) (<-chan struct{}, bool) {
	if identity.Namespace == jobmgr.ProcessAttemptJobRuntime {
		select {
		case <-a.release:
		default:
			return a.release, true
		}
	}
	return a.ProcessAttemptAuthority.ProcessAttemptReleased(identity)
}
