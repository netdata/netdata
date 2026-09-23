// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"context"
	"errors"
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
	"github.com/stretchr/testify/assert"
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

func TestSupersedeWithinRequestBoundsTheWait(t *testing.T) {
	identity := jobAttemptIdentity(jobmgr.ProcessAttemptJobRuntime, "module_job")
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	tests := map[string]struct {
		ctx          context.Context
		candidateCtx context.Context
		wantErr      error
		wantDeadline bool
	}{
		"request deadline turns a slow release into busy": {
			ctx:          lifecycle.WithApplyDeadline(context.Background(), time.Now().Add(10*time.Millisecond)),
			candidateCtx: context.Background(),
			wantErr:      jobmgr.ErrProcessAttemptBusy,
			wantDeadline: true,
		},
		"a cut candidate keeps its cause": {
			ctx:          lifecycle.WithApplyDeadline(context.Background(), time.Now().Add(time.Hour)),
			candidateCtx: cancelled,
			wantErr:      context.Canceled,
			wantDeadline: true,
		},
		"work without a request waits for the supersession grace": {
			ctx:          context.Background(),
			candidateCtx: cancelled,
			wantErr:      context.Canceled,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			authority := &blockingSupersessionAuthority{}
			err := supersedeWithinRequest(test.ctx, test.candidateCtx, authority, identity)
			require.ErrorIs(t, err, test.wantErr)
			if errors.Is(test.wantErr, jobmgr.ErrProcessAttemptBusy) {
				require.NotErrorIs(t, err, context.DeadlineExceeded)
			}
			assert.Equal(t, test.wantDeadline, authority.hadDeadline)
		})
	}
}

// blockingSupersessionAuthority never sees the previous runtime release.
type blockingSupersessionAuthority struct {
	jobmgr.ProcessAttemptAuthority
	hadDeadline bool
}

func (a *blockingSupersessionAuthority) SupersedeProcessAttempt(
	ctx context.Context,
	_ jobmgr.ProcessAttemptIdentity,
) error {
	_, a.hadDeadline = ctx.Deadline()
	<-ctx.Done()
	return ctx.Err()
}

func TestAdoptedUpdateWithBusyRuntimeStartsOnceTheRuntimeReleases(t *testing.T) {
	controller, graph, _, _, state := newDynCfgJobTestHarness(t)
	useV2CheckFailureCollector(controller, state, nil)
	commands := &autoDetectionRetryTestCommands{}
	require.NoError(t, controller.BindBackgroundWorkers(commands, 9, func(err error) {
		t.Errorf("background worker failed: %v", err)
	}))
	t.Cleanup(func() {
		controller.scheduler.StopBackgroundWorkers()
		require.NoError(t, controller.scheduler.WaitBackgroundWorkers(context.Background()))
	})
	controller.factory.config.Attempts = attemptFailureTestAuthority{
		delegate:  controller.factory.config.Attempts,
		namespace: jobmgr.ProcessAttemptJobRuntime,
		err:       jobmgr.ErrProcessAttemptBusy,
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
	require.Equal(t, dyncfg.StatusFailed.String(), record.Status)

	// The previous runtime is already gone, so the pending start is submitted
	// at once; it reconciles the adopted config on the job's lane.
	commands.waitForSubmissions(t, 1)
	submitted, _, _ := commands.snapshot()
	require.Equal(t, "internal/jobs/pending", submitted[0].Route)
	require.Equal(t, config.FullName(), submitted[0].LaneKey)
}
