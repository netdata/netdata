// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

// adoptionInjection selects where a command fails.
type adoptionInjection uint8

const (
	injectNone adoptionInjection = iota
	injectInvalidPayload
	injectInitUnclassified
	injectCheckUnclassified
	injectCheckPermanent
	injectCheckTemporary
	injectMissingVnode
	injectJobBusy
	injectJobDeadline
	injectJobQuarantined
	injectV1CheckUnclassified
	injectInitPermanent
	injectInitTemporary
	injectJobSuperseded
)

// TestDynCfgJobMutationAdoptionAndRestartPreflight drives collector-job DynCfg commands
// through the production Prepare, Apply and encode path and checks the reply
// contract on every row: the daemon saves add, update, enable, disable and
// remove on 2xx only, so a 2xx means the graph holds the requested state and a
// non-2xx means the record, the incumbent, the retries and the registrations
// are unchanged. RESTART rows cover preflight rejection only; composition tests
// exercise its external reply after the separate activation receipt settles.
func TestDynCfgJobMutationAdoptionAndRestartPreflight(t *testing.T) {
	tests := map[string]struct {
		command  dyncfg.Command
		status   dyncfg.Status // preimage record status; empty = no record
		source   string        // preimage source type
		name     string        // job name in the request; empty = "job"
		inject   adoptionInjection
		retry    int // autodetection_retry of the stored and the requested config
		wantCode int
	}{
		"update running succeeds":                      {command: dyncfg.CommandUpdate, status: dyncfg.StatusRunning, wantCode: 202},
		"update running with an invalid payload":       {command: dyncfg.CommandUpdate, status: dyncfg.StatusRunning, inject: injectInvalidPayload, wantCode: 400},
		"update running with a permanent error":        {command: dyncfg.CommandUpdate, status: dyncfg.StatusRunning, inject: injectCheckPermanent, retry: 1, wantCode: 422},
		"update running with a check failure":          {command: dyncfg.CommandUpdate, status: dyncfg.StatusRunning, inject: injectCheckUnclassified, wantCode: 422},
		"update running with a retried check failure":  {command: dyncfg.CommandUpdate, status: dyncfg.StatusRunning, inject: injectCheckUnclassified, retry: 1, wantCode: 422},
		"update running with a temporary error":        {command: dyncfg.CommandUpdate, status: dyncfg.StatusRunning, inject: injectCheckTemporary, wantCode: 503},
		"update running with a retried temporary":      {command: dyncfg.CommandUpdate, status: dyncfg.StatusRunning, inject: injectCheckTemporary, retry: 1, wantCode: 503},
		"update running with an init failure":          {command: dyncfg.CommandUpdate, status: dyncfg.StatusRunning, inject: injectInitUnclassified, retry: 1, wantCode: 422},
		"update running with a permanent init error":   {command: dyncfg.CommandUpdate, status: dyncfg.StatusRunning, inject: injectInitPermanent, retry: 1, wantCode: 422},
		"update running with a temporary init error":   {command: dyncfg.CommandUpdate, status: dyncfg.StatusRunning, inject: injectInitTemporary, wantCode: 503},
		"update running with a retried temporary init": {command: dyncfg.CommandUpdate, status: dyncfg.StatusRunning, inject: injectInitTemporary, retry: 1, wantCode: 503},
		"update running with a busy job":               {command: dyncfg.CommandUpdate, status: dyncfg.StatusRunning, inject: injectJobBusy, wantCode: 503},
		"update running with a superseded job":         {command: dyncfg.CommandUpdate, status: dyncfg.StatusRunning, inject: injectJobSuperseded, wantCode: 503},
		"update running past its deadline":             {command: dyncfg.CommandUpdate, status: dyncfg.StatusRunning, inject: injectJobDeadline, wantCode: 503},
		"update running with a quarantined job":        {command: dyncfg.CommandUpdate, status: dyncfg.StatusRunning, inject: injectJobQuarantined, wantCode: 503},
		"update running with a missing vnode":          {command: dyncfg.CommandUpdate, status: dyncfg.StatusRunning, inject: injectMissingVnode, wantCode: 503},
		"update running with a retried missing vnode":  {command: dyncfg.CommandUpdate, status: dyncfg.StatusRunning, inject: injectMissingVnode, retry: 1, wantCode: 503},
		"update running stock succeeds":                {command: dyncfg.CommandUpdate, status: dyncfg.StatusRunning, source: confgroup.TypeStock, wantCode: 202},
		"update running stock with a permanent error":  {command: dyncfg.CommandUpdate, status: dyncfg.StatusRunning, source: confgroup.TypeStock, inject: injectCheckPermanent, wantCode: 422},
		"update running user succeeds":                 {command: dyncfg.CommandUpdate, status: dyncfg.StatusRunning, source: confgroup.TypeUser, wantCode: 202},
		"update running user with a retried check":     {command: dyncfg.CommandUpdate, status: dyncfg.StatusRunning, source: confgroup.TypeUser, inject: injectCheckUnclassified, retry: 1, wantCode: 422},
		"update running user with a permanent error":   {command: dyncfg.CommandUpdate, status: dyncfg.StatusRunning, source: confgroup.TypeUser, inject: injectCheckPermanent, wantCode: 422},
		"update failed with a permanent error":         {command: dyncfg.CommandUpdate, status: dyncfg.StatusFailed, inject: injectCheckPermanent, wantCode: 422},
		"update failed succeeds":                       {command: dyncfg.CommandUpdate, status: dyncfg.StatusFailed, wantCode: 202},
		"update failed with a check failure":           {command: dyncfg.CommandUpdate, status: dyncfg.StatusFailed, inject: injectCheckUnclassified, wantCode: 422},
		"enable failed with a check failure":           {command: dyncfg.CommandEnable, status: dyncfg.StatusFailed, inject: injectCheckUnclassified, wantCode: 202},
		"update disabled succeeds":                     {command: dyncfg.CommandUpdate, status: dyncfg.StatusDisabled, wantCode: 200},
		"update disabled with an invalid payload":      {command: dyncfg.CommandUpdate, status: dyncfg.StatusDisabled, inject: injectInvalidPayload, wantCode: 400},
		"update disabled with a missing vnode":         {command: dyncfg.CommandUpdate, status: dyncfg.StatusDisabled, inject: injectMissingVnode, wantCode: 200},
		"update disabled with a busy job":              {command: dyncfg.CommandUpdate, status: dyncfg.StatusDisabled, inject: injectJobBusy, wantCode: 503},
		"update accepted":                              {command: dyncfg.CommandUpdate, status: dyncfg.StatusAccepted, wantCode: 403},
		"update unknown job":                           {command: dyncfg.CommandUpdate, wantCode: 404},
		"enable disabled succeeds":                     {command: dyncfg.CommandEnable, status: dyncfg.StatusDisabled, wantCode: 202},
		"enable disabled with a check failure":         {command: dyncfg.CommandEnable, status: dyncfg.StatusDisabled, inject: injectCheckUnclassified, wantCode: 202},
		"enable disabled with a retried check":         {command: dyncfg.CommandEnable, status: dyncfg.StatusDisabled, inject: injectCheckUnclassified, retry: 1, wantCode: 202},
		"enable disabled with a permanent error":       {command: dyncfg.CommandEnable, status: dyncfg.StatusDisabled, inject: injectCheckPermanent, retry: 1, wantCode: 202},
		"enable disabled with a stored invalid config": {command: dyncfg.CommandEnable, status: dyncfg.StatusDisabled, inject: injectInvalidPayload, wantCode: 202},
		"enable disabled v1 with a check failure":      {command: dyncfg.CommandEnable, status: dyncfg.StatusDisabled, inject: injectV1CheckUnclassified, wantCode: 202},
		"enable disabled v1 with a retried check":      {command: dyncfg.CommandEnable, status: dyncfg.StatusDisabled, inject: injectV1CheckUnclassified, retry: 1, wantCode: 202},
		"enable disabled stock with a retried check":   {command: dyncfg.CommandEnable, status: dyncfg.StatusDisabled, source: confgroup.TypeStock, inject: injectCheckUnclassified, retry: 1, wantCode: 202},
		"enable failed with a quarantined job":         {command: dyncfg.CommandEnable, status: dyncfg.StatusFailed, inject: injectJobQuarantined, wantCode: 202},
		"enable running":                               {command: dyncfg.CommandEnable, status: dyncfg.StatusRunning, wantCode: 200},
		"enable accepted":                              {command: dyncfg.CommandEnable, status: dyncfg.StatusAccepted, wantCode: 202},
		"restart running with a retried check":         {command: dyncfg.CommandRestart, status: dyncfg.StatusRunning, inject: injectCheckUnclassified, retry: 1, wantCode: 422},
		"restart running with a retried temporary":     {command: dyncfg.CommandRestart, status: dyncfg.StatusRunning, inject: injectCheckTemporary, retry: 1, wantCode: 503},
		"restart failed with a retried check":          {command: dyncfg.CommandRestart, status: dyncfg.StatusFailed, inject: injectCheckUnclassified, retry: 1, wantCode: 422},
		"restart disabled":                             {command: dyncfg.CommandRestart, status: dyncfg.StatusDisabled, wantCode: 405},
		"disable running":                              {command: dyncfg.CommandDisable, status: dyncfg.StatusRunning, wantCode: 200},
		"disable disabled":                             {command: dyncfg.CommandDisable, status: dyncfg.StatusDisabled, wantCode: 200},
		"disable accepted":                             {command: dyncfg.CommandDisable, status: dyncfg.StatusAccepted, wantCode: 200},
		"remove accepted":                              {command: dyncfg.CommandRemove, status: dyncfg.StatusAccepted, wantCode: 200},
		"add over an accepted job":                     {command: dyncfg.CommandAdd, status: dyncfg.StatusAccepted, wantCode: 202},
		"remove running":                               {command: dyncfg.CommandRemove, status: dyncfg.StatusRunning, wantCode: 200},
		"remove stock":                                 {command: dyncfg.CommandRemove, status: dyncfg.StatusRunning, source: confgroup.TypeStock, wantCode: 405},
		"add succeeds":                                 {command: dyncfg.CommandAdd, wantCode: 202},
		"add with an invalid payload":                  {command: dyncfg.CommandAdd, inject: injectInvalidPayload, wantCode: 400},
		"add with a missing vnode":                     {command: dyncfg.CommandAdd, inject: injectMissingVnode, wantCode: 202},
		"add with a retried missing vnode":             {command: dyncfg.CommandAdd, inject: injectMissingVnode, retry: 1, wantCode: 202},
		"add with a busy job":                          {command: dyncfg.CommandAdd, inject: injectJobBusy, wantCode: 503},
		"add over a running job":                       {command: dyncfg.CommandAdd, status: dyncfg.StatusRunning, wantCode: 202},
		"add a name the daemon would save as sent":     {command: dyncfg.CommandAdd, name: "a:b", wantCode: 400},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			controller, graph, supervisor, output, state := newDynCfgJobTestHarness(t)
			configureAdoptionTestCollector(controller, state, test.inject)

			stored := factoryTestConfig(false).Set("option_str", "stored")
			source := test.source
			if source == "" {
				source = confgroup.TypeDyncfg
			}
			stored.SetSourceType(source)
			stored.SetSource("src")
			stored.SetProvider(source)
			if test.retry > 0 {
				stored.Set("autodetection_retry", test.retry)
			}
			if test.inject == injectInvalidPayload && test.command == dyncfg.CommandEnable {
				stored.Set("option_str", map[string]any{"nested": 1})
			}
			if test.inject == injectMissingVnode && test.command != dyncfg.CommandUpdate && test.command != dyncfg.CommandAdd {
				stored.Set("vnode", "missing")
			}
			if test.status != "" {
				seedDynCfgJobGraphRecord(t, graph, stored, test.status)
			}
			preimage, preimageExists := graph.Lookup(stored.FullName())
			if test.status == dyncfg.StatusFailed {
				// A failed job may already wait for its own retry; a rejected
				// command must leave that retry in place.
				controller.scheduler.retries.schedule(stored, 60)
			}
			retryBefore := runtimeTestHasRetry(controller, stored.FullName())

			var events []string
			var current lifecycle.ReadyResource
			scope := lifecycle.ResourceTransactionScope{ID: stored.FullName()}
			next := uint64(1)
			if test.status == dyncfg.StatusRunning {
				scope.Current = lifecycle.ResourceIdentity{ID: stored.FullName(), Generation: 1}
				current = &transactionTestReadyResource{identity: scope.Current, prefix: "current", events: &events}
				next++
			}
			// Like the production route, only commands that may start a job
			// allocate a successor.
			allocates := test.command == dyncfg.CommandUpdate ||
				test.command == dyncfg.CommandEnable ||
				test.command == dyncfg.CommandRestart
			if allocates {
				scope.Successor = lifecycle.ResourceIdentity{ID: stored.FullName(), Generation: next}
			}
			// Inject attempt failures only once the preimage is in place.
			injectAdoptionAttemptFailure(controller, test.inject)

			request := adoptionTestRequest(test.command, test.name, test.inject, test.retry)
			prepare := func(
				ctx context.Context,
				current lifecycle.ReadyResource,
				taskScope lifecycle.ResourceTransactionScope,
				permit lifecycle.LongLivedPermit,
			) (lifecycle.PreparedResourceTransaction, error) {
				return controller.Prepare(ctx, request, current, taskScope, permit)
			}
			var plan lifecycle.TaskPlan
			var err error
			if allocates {
				plan, err = lifecycle.NewResourceTransactionPermitTaskPlan(
					lifecycle.SourceFunction, time.Time{}, lifecycle.TransactionTaskPhases,
					current, scope, lifecycle.NewJobLongLivedPlan(), prepare,
				)
			} else {
				plan, err = lifecycle.NewResourceTransactionTaskPlan(
					lifecycle.SourceFunction, time.Time{}, lifecycle.TransactionTaskPhases,
					current, scope, prepare,
				)
			}
			require.NoError(t, err)
			uid := strings.ReplaceAll(name, " ", "-")
			_, active := applyAndEncodeDynCfgJobTestTask(t, supervisor, plan, scope, uid)
			t.Cleanup(func() { stopRuntimeTestResource(t, active) })

			code := adoptionTestReplyCode(t, output.String(), uid)
			require.Equal(t, test.wantCode, code, "wire: %s", output.String())

			record, exists := graph.Lookup(stored.FullName())
			wire := output.String()
			persisted := test.command != dyncfg.CommandRestart
			adopted := code >= 200 && code < 300
			// restart is not saved, but a restart of a running job that fails
			// before stopping it must still keep everything.
			restartKeeps := test.command == dyncfg.CommandRestart && !adopted &&
				preimage.Status == dyncfg.StatusRunning.String()
			switch {
			case persisted && adopted:
				requireAdoptionHoldsRequest(t, controller, test.command, request, preimage, preimageExists, record, exists)
				if test.command == dyncfg.CommandUpdate && source != confgroup.TypeDyncfg {
					require.Regexp(t,
						regexp.MustCompile(`CONFIG go.d:collector:module:job create `+record.Status+` job \S+ dyncfg `),
						wire, "an adopted update takes the job into dyncfg ownership")
				}
				if exists && record.Status == dyncfg.StatusFailed.String() {
					require.Equal(t, 202, code, "an adopted failure answers 202")
					require.True(t, runtimeTestHasRetry(controller, stored.FullName()),
						"an adopted preparation failure is retried")
				}
			case persisted, restartKeeps:
				require.Equal(t, preimageExists, exists, "a rejection keeps the record")
				require.Equal(t, preimage.Status, record.Status, "a rejection keeps the status")
				require.Equal(t, preimage.Payload(), record.Payload(), "a rejection keeps the payload")
				require.Empty(t, events, "a rejection keeps the incumbent")
				require.Equal(t, current != nil, active != nil, "a rejection keeps the running job")
				require.Equal(t, retryBefore, runtimeTestHasRetry(controller, stored.FullName()),
					"a rejection keeps the retry state")
				require.NotRegexp(t, regexp.MustCompile(`CONFIG \S+ (create|delete)`), wire,
					"a rejection registers and deletes nothing")
				if preimageExists && test.command != dyncfg.CommandRemove && test.command != dyncfg.CommandDisable {
					require.Contains(t, wire, "CONFIG go.d:collector:module:job status "+preimage.Status,
						"a rejection restates the kept status")
				}
			case adopted:
				require.True(t, exists)
				require.Equal(t, dyncfg.StatusRunning.String(), record.Status, "restart answers 2xx only for a running job")
			}
		})
	}
}

func requireAdoptionHoldsRequest(
	t *testing.T,
	controller *DynCfgJobController,
	command dyncfg.Command,
	request DynCfgJobRequest,
	preimage dyncfg.GraphRecord,
	preimageExists bool,
	record dyncfg.GraphRecord,
	exists bool,
) {
	t.Helper()
	switch command {
	case dyncfg.CommandAdd, dyncfg.CommandUpdate:
		require.True(t, exists, "the adopted configuration is stored")
		requested, failure := controller.parseConfig(request, record.Module, record.Name)
		require.False(t, failure.valid)
		payload, err := yaml.Marshal(requested)
		require.NoError(t, err)
		require.Equal(t, string(payload), record.Payload(), "the stored payload is the requested one")
		want := map[bool][]string{
			true:  {dyncfg.StatusDisabled.String()},
			false: {dyncfg.StatusAccepted.String(), dyncfg.StatusRunning.String(), dyncfg.StatusFailed.String()},
		}[preimageExists && preimage.Status == dyncfg.StatusDisabled.String()]
		if command == dyncfg.CommandAdd {
			want = []string{dyncfg.StatusAccepted.String()}
		}
		require.Contains(t, want, record.Status, "the requested enabledness holds")
	case dyncfg.CommandEnable:
		if exists {
			require.NotEqual(t, dyncfg.StatusDisabled.String(), record.Status, "the job is enabled")
			return
		}
		stored, err := graphRecordConfig(preimage)
		require.NoError(t, err)
		require.Equal(t, confgroup.TypeStock, stored.SourceType(), "only a plain stock job is removed when enabled")
	case dyncfg.CommandDisable:
		require.True(t, exists)
		require.Equal(t, dyncfg.StatusDisabled.String(), record.Status)
	case dyncfg.CommandRemove:
		require.False(t, exists)
	}
}

func adoptionTestRequest(command dyncfg.Command, name string, inject adoptionInjection, retry int) DynCfgJobRequest {
	args := []string{"go.d:collector:module:job", string(command)}
	if command == dyncfg.CommandAdd {
		if name == "" {
			name = "job"
		}
		args = []string{"go.d:collector:module", string(command), name}
	}
	request := DynCfgJobRequest{Args: args, ContentType: "application/json", CallerSource: "user=test"}
	if command != dyncfg.CommandAdd && command != dyncfg.CommandUpdate {
		return request
	}
	fields := []string{`"option_str":"requested"`}
	switch inject {
	case injectInvalidPayload:
		fields = []string{`"option_str":{"nested":1}`}
	case injectMissingVnode:
		fields = append(fields, `"vnode":"missing"`)
	}
	if retry > 0 {
		fields = append(fields, fmt.Sprintf(`"autodetection_retry":%d`, retry))
	}
	request.Payload = []byte("{" + strings.Join(fields, ",") + "}")
	request.HasPayload = true
	return request
}

func adoptionTestReplyCode(t *testing.T, wire, uid string) int {
	t.Helper()
	match := regexp.MustCompile(`FUNCTION_RESULT_BEGIN ` + regexp.QuoteMeta(uid) + ` (\d+) `).FindStringSubmatch(wire)
	require.NotNil(t, match, "no reply for %s in %q", uid, wire)
	code, err := strconv.Atoi(match[1])
	require.NoError(t, err)
	return code
}

func configureAdoptionTestCollector(controller *DynCfgJobController, state *factoryTestState, inject adoptionInjection) {
	if inject == injectV1CheckUnclassified {
		creator := controller.modules["module"]
		creator.CreateV2 = nil
		creator.Create = func() collectorapi.CollectorV1 {
			return state.module(func(context.Context) error { return errors.New("endpoint unreachable") }, false)
		}
		controller.modules["module"] = creator
		return
	}
	var checkErr error
	switch inject {
	case injectCheckUnclassified:
		checkErr = errors.New("endpoint unreachable")
	case injectCheckPermanent:
		checkErr = collectorapi.PermanentError(errors.New("unknown profile"))
	case injectCheckTemporary:
		checkErr = collectorapi.TemporaryError(errors.New("dependency not ready"))
	}
	useV2CheckFailureCollector(controller, state, checkErr)
	var initErr error
	switch inject {
	case injectInitUnclassified:
		initErr = errors.New("invalid option")
	case injectInitPermanent:
		initErr = collectorapi.PermanentError(errors.New("unknown profile"))
	case injectInitTemporary:
		initErr = collectorapi.TemporaryError(errors.New("dependency not ready"))
	}
	if initErr != nil {
		creator := controller.modules["module"]
		create := creator.CreateV2
		creator.CreateV2 = func() collectorapi.CollectorV2 {
			collector := create().(*factoryTestV2)
			collector.init = func(context.Context) error { return initErr }
			return collector
		}
		controller.modules["module"] = creator
	}
}

func injectAdoptionAttemptFailure(controller *DynCfgJobController, inject adoptionInjection) {
	var namespace jobmgr.ProcessAttemptNamespace
	var err error
	switch inject {
	case injectJobBusy:
		namespace, err = jobmgr.ProcessAttemptJob, jobmgr.ErrProcessAttemptBusy
	case injectJobDeadline:
		namespace, err = jobmgr.ProcessAttemptJob, jobmgr.ErrProcessAttemptDeadline
	case injectJobQuarantined:
		namespace, err = jobmgr.ProcessAttemptJob, jobmgr.ErrProcessAttemptQuarantined
	case injectJobSuperseded:
		namespace, err = jobmgr.ProcessAttemptJob, jobmgr.ErrProcessAttemptSuperseded
	default:
		return
	}
	controller.factory.config.Attempts = attemptFailureTestAuthority{
		delegate:  controller.factory.config.Attempts,
		namespace: namespace,
		err:       err,
	}
}

// attemptFailureTestAuthority fails every attempt of one namespace.
type attemptFailureTestAuthority struct {
	delegate  jobmgr.ProcessAttemptAuthority
	namespace jobmgr.ProcessAttemptNamespace
	err       error
}

func (a attemptFailureTestAuthority) StartProcessAttempt(
	ctx context.Context,
	plan jobmgr.ProcessAttemptPlan,
) (jobmgr.ProcessAttempt, error) {
	if plan.Identity.Namespace == a.namespace {
		return nil, a.err
	}
	return a.delegate.StartProcessAttempt(ctx, plan)
}

func (a attemptFailureTestAuthority) SupersedeProcessAttempt(
	ctx context.Context,
	identity jobmgr.ProcessAttemptIdentity,
) error {
	if identity.Namespace == a.namespace && a.namespace == jobmgr.ProcessAttemptJobRuntime {
		return a.err
	}
	return a.delegate.SupersedeProcessAttempt(ctx, identity)
}

func (a attemptFailureTestAuthority) CutProcessAttempt(identity jobmgr.ProcessAttemptIdentity, cause error) bool {
	return a.delegate.CutProcessAttempt(identity, cause)
}

func (a attemptFailureTestAuthority) ProcessAttemptReleased(
	identity jobmgr.ProcessAttemptIdentity,
) (<-chan struct{}, bool) {
	return a.delegate.ProcessAttemptReleased(identity)
}
