// SPDX-License-Identifier: GPL-3.0-or-later

package composition

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/netdata/netdata/go/plugins/plugin/framework/vnodes"
	"github.com/stretchr/testify/require"
)

type acquisitionAttempt struct {
	config   vnodes.SNMPConfig
	reply    chan acquisitionReply
	canceled chan struct{}
}

func TestCredentialFormTransitionsDiscardInactiveFields(t *testing.T) {
	jobs := testRunJobServices(t)
	acquirer := controlledAcquirer{attempts: make(chan acquisitionAttempt, 8)}
	jobs.SNMPVnodeAcquirer = acquirer
	output := newProcessSynchronizedBuffer()
	frames, err := lifecycle.NewFrameOwner(output)
	require.NoError(t, err)
	generation, err := newTestRunGeneration(t, runGenerationConfig{Generation: 1, ShutdownTimeout: time.Second, UIDs: lifecycle.NewUIDLedger(), Frames: frames, Modules: collectorapi.Registry{}, Jobs: jobs, Discovery: testRunDiscoveryServices(t)})
	require.NoError(t, err)
	require.NoError(t, generation.start(context.Background()))
	t.Cleanup(func() {
		generation.Stop()
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		require.NoError(t, generation.Wait(ctx))
	})
	var current acquisitionAttempt
	for i, tc := range []struct{ auth, absent string }{
		{`"version":"2c","credentials":{"community":"fixture"},"credentials3":{"username":"hidden"}`, "credentials3"},
		{`"version":"3","credentials":{"community":"hidden"},"credentials3":{"username":"fixture","security_level":"authPriv","auth_password":"test-auth","priv_password":"test-priv"}`, "community"},
		{`"version":"3","credentials3":{"username":"fixture","security_level":"authNoPriv","auth_password":"test-auth","priv_password":"hidden-priv","priv_protocol":"aes"}`, "priv_password"},
		{`"version":"3","credentials3":{"username":"fixture","security_level":"noAuthNoPriv","auth_password":"hidden-auth","auth_protocol":"sha","priv_password":"hidden-priv"}`, "auth_password"},
	} {
		uid := fmt.Sprintf("transition-%d", i)
		args := []string{"go.d:vnode:router", "update"}
		if i == 0 {
			args = []string{"go.d:vnode", "add", "router"}
		}
		payload := `{"mode":"snmp","mode_snmp":{"address":"device",` + tc.auth + `}}`
		require.NoError(t, generation.kernel.Submit(context.Background(), jobmgr.Request{UID: uid, Route: "config", Source: lifecycle.SourceFunction, Args: args, HasPayload: true, Payload: []byte(payload), ContentType: "application/json", CallerSource: "user=test"}))
		output.waitContains(t, "FUNCTION_RESULT_BEGIN "+uid+" ")
		require.Contains(t, output.String(), "FUNCTION_RESULT_BEGIN "+uid+" 202 application/json")
		if i > 0 {
			select {
			case <-current.canceled:
			case <-time.After(time.Second):
				t.Fatal("active credential edit did not cancel previous acquisition")
			}
		}
		current = nextAcquisition(t, acquirer)
		raw, err := json.Marshal(current.config)
		require.NoError(t, err)
		require.NotContains(t, string(raw), tc.absent)
		require.NoError(t, generation.kernel.Submit(context.Background(), jobmgr.Request{UID: uid + "-get", Route: "config", Source: lifecycle.SourceFunction, Args: []string{"go.d:vnode:router", "get"}}))
		output.waitContains(t, "FUNCTION_RESULT_BEGIN "+uid+"-get 200 application/json")
		_, body, ok := strings.Cut(output.String(), "FUNCTION_RESULT_BEGIN "+uid+"-get 200 application/json")
		require.True(t, ok)
		require.NotContains(t, body, tc.absent, "get must omit inactive credentials from stored configuration")
	}
}

type acquisitionReply struct {
	metadata *vnodes.Metadata
	err      error
}
type controlledAcquirer struct{ attempts chan acquisitionAttempt }

func (a controlledAcquirer) Acquire(ctx context.Context, c vnodes.SNMPConfig) (*vnodes.Metadata, error) {
	attempt := acquisitionAttempt{config: c, reply: make(chan acquisitionReply, 1), canceled: make(chan struct{})}
	select {
	case a.attempts <- attempt:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	select {
	case r := <-attempt.reply:
		return r.metadata, r.err
	case <-ctx.Done():
		close(attempt.canceled)
		return nil, ctx.Err()
	}
}
func nextAcquisition(t *testing.T, a controlledAcquirer) acquisitionAttempt {
	t.Helper()
	select {
	case attempt := <-a.attempts:
		return attempt
	case <-time.After(12 * time.Second):
		t.Fatal("acquisition did not start")
		return acquisitionAttempt{}
	}
}
func TestRunOwnsIndependentAcquisitionAndModeAwareUpdates(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var initial vnodes.Config
		require.NoError(t, json.Unmarshal([]byte(`{"name":"router","mode":"snmp","mode_snmp":{"address":"device","credentials":{"community":"first"}},"labels":{"site":"before"}}`), &initial))
		initial.SourceType = "dyncfg"
		initial.Source = "user=test"
		jobs := testRunJobServices(t)
		jobs.InitialVnodes = map[string]*vnodes.Config{"router": &initial}
		acquirer := controlledAcquirer{attempts: make(chan acquisitionAttempt, 8)}
		jobs.SNMPVnodeAcquirer = acquirer
		output := newProcessSynchronizedBuffer()
		frames, err := lifecycle.NewFrameOwner(output)
		require.NoError(t, err)
		generation, err := newTestRunGeneration(t, runGenerationConfig{Generation: 1, ShutdownTimeout: time.Second, UIDs: lifecycle.NewUIDLedger(), Frames: frames, Modules: collectorapi.Registry{}, Jobs: jobs, Discovery: testRunDiscoveryServices(t)})
		require.NoError(t, err)
		require.NoError(t, generation.start(context.Background()))
		t.Cleanup(func() {
			generation.Stop()
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			require.NoError(t, generation.Wait(ctx))
			require.NoError(t, generation.run.FinishShutdown())
		})
		first := nextAcquisition(t, acquirer)
		pending, ok := generation.vnodeConfig.Lookup("router")
		require.True(t, ok)
		require.Nil(t, pending.Vnode)
		require.NotContains(t, output.String(), "HOST_DEFINE")
		update := func(uid, payload string) {
			require.NoError(t, generation.kernel.Submit(context.Background(), jobmgr.Request{UID: uid, Route: "config", Source: lifecycle.SourceFunction, Args: []string{"go.d:vnode:router", "update"}, HasPayload: true, Payload: []byte(payload), ContentType: "application/json", CallerSource: "user=test"}))
			output.waitContains(t, "FUNCTION_RESULT_BEGIN "+uid+" 202 application/json")
		}
		require.NoError(t, generation.kernel.Submit(context.Background(), jobmgr.Request{UID: "validate-only", Route: "config", Source: lifecycle.SourceFunction, Args: []string{"go.d:vnode", "test", "probe"}, HasPayload: true, Payload: []byte(`{"mode":"snmp","mode_snmp":{"address":"other-device","credentials":{"community":"test-only"}}}`), ContentType: "application/json", CallerSource: "user=test"}))
		output.waitContains(t, "FUNCTION_RESULT_BEGIN validate-only 202 application/json")
		require.Empty(t, acquirer.attempts, "test must not acquire metadata")
		update("labels", `{"mode":"snmp","mode_snmp":{"address":"device","credentials":{"community":"first"}},"labels":{"site":"after"}}`)
		select {
		case <-acquirer.attempts:
			t.Fatal("label-only edit restarted acquisition")
		default:
		}
		first.reply <- acquisitionReply{metadata: &vnodes.Metadata{Hostname: "device-name", Labels: map[string]string{"serial": "good"}}, err: errors.New("enrichment incomplete")}
		require.Eventually(t, func() bool { s, _ := generation.vnodeConfig.Lookup("router"); return s.Vnode != nil }, time.Second, time.Millisecond)
		ready, _ := generation.vnodeConfig.Lookup("router")
		require.Equal(t, "device-name", ready.Vnode.Hostname)
		output.waitContains(t, "CONFIG go.d:vnode:router create failed job")
		require.Equal(t, "after", ready.Vnode.Labels["site"])
		require.NoError(t, generation.kernel.Submit(context.Background(), jobmgr.Request{UID: "authored-get", Route: "config", Source: lifecycle.SourceFunction, Args: []string{"go.d:vnode:router", "get"}}))
		output.waitContains(t, "FUNCTION_RESULT_BEGIN authored-get 200 application/json")
		require.NotContains(t, output.String(), "device-name", "get must return authored configuration rather than acquired metadata")
		require.NoError(t, generation.kernel.Submit(context.Background(), jobmgr.Request{UID: "change-target", Route: "config", Source: lifecycle.SourceFunction, Args: []string{"go.d:vnode:router", "update"}, HasPayload: true, Payload: []byte(`{"mode":"snmp","mode_snmp":{"address":"replacement","credentials":{"community":"first"}}}`), ContentType: "application/json", CallerSource: "user=test"}))
		output.waitContains(t, "FUNCTION_RESULT_BEGIN change-target 400 application/json")
		retry := nextAcquisition(t, acquirer)
		require.Equal(t, "first", retry.config.Credentials.Community)
		update("rotate", `{"mode":"snmp","mode_snmp":{"address":"device","credentials":{"community":"rotated"}},"labels":{"site":"after"}}`)
		select {
		case <-retry.canceled:
		case <-time.After(time.Second):
			t.Fatal("credential rotation did not cancel old acquisition")
		}
		rotated := nextAcquisition(t, acquirer)
		require.Equal(t, dyncfg.StatusAccepted, generation.vnodes.configStatus("router"), "new credentials are pending even while last-good identity remains usable")
		require.Equal(t, "rotated", rotated.config.Credentials.Community)
		kept, _ := generation.vnodeConfig.Lookup("router")
		require.Equal(t, "good", kept.Vnode.Labels["serial"])
		require.NoError(t, generation.kernel.Submit(context.Background(), jobmgr.Request{UID: "remove", Route: "config", Source: lifecycle.SourceFunction, Args: []string{"go.d:vnode:router", "remove"}}))
		output.waitContains(t, "FUNCTION_RESULT_BEGIN remove 200 application/json")
		select {
		case <-rotated.canceled:
		case <-time.After(time.Second):
			t.Fatal("removal did not cancel acquisition")
		}
		require.NotContains(t, output.String(), "HOST_DEFINE")

	})
}

func TestAcquiredHostnameCollisionIsDiagnosedAfterCommit(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		jobs := testRunJobServices(t)
		var router, other vnodes.Config
		require.NoError(t, json.Unmarshal([]byte(`{"name":"router","mode":"snmp","mode_snmp":{"address":"device","credentials":{"community":"fixture-secret"}}}`), &router))
		require.NoError(t, json.Unmarshal([]byte(`{"name":"other","hostname":"taken","guid":"6e17cd2c-0518-4e94-965b-d25673decb21"}`), &other))
		router.SourceType, other.SourceType = "dyncfg", "dyncfg"
		jobs.InitialVnodes = map[string]*vnodes.Config{"router": &router, "other": &other}
		acquirer := controlledAcquirer{attempts: make(chan acquisitionAttempt, 8)}
		jobs.SNMPVnodeAcquirer = acquirer
		diagnostics := &recordingCompositionDiagnosticObserver{}
		output := newProcessSynchronizedBuffer()
		frames, err := lifecycle.NewFrameOwner(output)
		require.NoError(t, err)
		generation, err := newTestRunGeneration(t, runGenerationConfig{Generation: 1, ShutdownTimeout: time.Second, UIDs: lifecycle.NewUIDLedger(), Frames: frames, Modules: collectorapi.Registry{}, Jobs: jobs, Discovery: testRunDiscoveryServices(t), Diagnostics: diagnostics})
		require.NoError(t, err)
		require.NoError(t, generation.start(context.Background()))
		t.Cleanup(func() {
			generation.Stop()
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			require.NoError(t, generation.Wait(ctx))
			require.NoError(t, generation.run.FinishShutdown())
		})
		first := nextAcquisition(t, acquirer)
		first.reply <- acquisitionReply{metadata: &vnodes.Metadata{Hostname: "original", Labels: map[string]string{"serial": "good"}}}
		synctest.Wait()
		ready, _ := generation.vnodeConfig.Lookup("router")
		require.NotNil(t, ready.Vnode)
		time.Sleep(vnodeRefreshInterval)
		refresh := nextAcquisition(t, acquirer)
		refresh.reply <- acquisitionReply{metadata: &vnodes.Metadata{Hostname: "taken", Labels: map[string]string{"serial": "replacement"}}}
		synctest.Wait()
		retained, _ := generation.vnodeConfig.Authored("router")
		require.True(t, retained.Failed)
		require.Equal(t, ready.Vnode, retained.Snapshot.Vnode)
		var collisions []jobmgr.DiagnosticEvent
		for _, event := range diagnostics.snapshot() {
			if event.Name == "vnode acquired metadata rejected" {
				collisions = append(collisions, event)
			}
		}
		require.Len(t, collisions, 1)
		require.Equal(t, "vnode:router", collisions[0].Resource)
		require.Equal(t, jobmgr.DiagnosticWarning, collisions[0].Level)
		require.ErrorContains(t, collisions[0].Err, "duplicate configured vnode hostname")
		require.NotContains(t, fmt.Sprint(collisions), "fixture-secret")
		retry := nextAcquisition(t, acquirer)
		retry.reply <- acquisitionReply{metadata: &vnodes.Metadata{Hostname: "recovered"}}
		synctest.Wait()
		recovered, _ := generation.vnodeConfig.Authored("router")
		require.False(t, recovered.Failed)
		require.Equal(t, "recovered", recovered.Snapshot.Vnode.Hostname)
	})
}
