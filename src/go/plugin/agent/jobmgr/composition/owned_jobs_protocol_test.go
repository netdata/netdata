// SPDX-License-Identifier: GPL-3.0-or-later

package composition

import (
	"context"
	"fmt"
	"io"
	"sync/atomic"
	"testing"
	"time"

	agentdiscovery "github.com/netdata/netdata/go/plugins/plugin/agent/discovery"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/ownedjobs"
	"github.com/stretchr/testify/require"
)

func TestOwnedJobsProductionProtocolAndDiscoveryCannotTakeAuthority(t *testing.T) {
	reader, writer := io.Pipe()
	output := newProcessSynchronizedBuffer()
	service := ownerTestService{registries: make(chan ownedjobs.Registry, 1)}
	batches := make(chan []*confgroup.Group, 2)
	var ownedCreates atomic.Int64
	creator := func(create func() collectorapi.CollectorV1) collectorapi.Creator {
		return collectorapi.Creator{
			Create:          create,
			Config:          func() any { return &collectorapi.MockConfiguration{} },
			JobConfigSchema: collectorapi.MockConfigSchema,
		}
	}
	config := testProductionProcessConfig(reader, output)
	config.Modules = collectorapi.Registry{
		"module":   creator(func() collectorapi.CollectorV1 { ownedCreates.Add(1); return ownedTestCollector() }),
		"ordinary": creator(ownedTestCollector),
	}
	config.Defaults = confgroup.Registry{"module": {UpdateEvery: 1}, "ordinary": {UpdateEvery: 1}}
	config.OwnerOnlyModules = []string{"module"}
	config.OwnedJobServices = []ownedjobs.Service{service}
	config.AutoEnable = true
	config.DiscoveryProviders = []agentdiscovery.ProviderFactory{
		agentdiscovery.NewProviderFactory("test", func(agentdiscovery.BuildContext) (agentdiscovery.Discoverer, bool, error) {
			return continuationDiscoverer{batches: batches}, true, nil
		}),
	}
	process, err := NewProcess(config)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- process.Run(ctx) }()
	t.Cleanup(func() {
		shutdown, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		defer cancel()
		defer reader.Close()
		defer writer.Close()
		require.NoError(t, process.Terminate(shutdown))
		select {
		case err := <-done:
			require.NoError(t, err)
		case <-shutdown.Done():
			t.Error("owned-job protocol process did not join")
		}
		if t.Failed() {
			t.Log(output.String())
		}
	})
	var registry ownedjobs.Registry
	select {
	case registry = <-service.registries:
	case <-time.After(3 * time.Second):
		t.Fatal("owner service did not bind")
	}
	owner, err := registry.Bind(t.Context(), "site")
	require.NoError(t, err)
	output.waitContains(t, "CONFIG go.d:collector:ordinary create accepted template")
	require.NotContains(t, output.String(), "CONFIG go.d:collector:module create")
	command := func(uid, route, payload, source string, code int) {
		t.Helper()
		var err error
		if payload == "" {
			_, err = fmt.Fprintf(writer, "FUNCTION %s 5 %q 0xFFFF %q\n", uid, route, source)
		} else {
			_, err = fmt.Fprintf(writer, "FUNCTION_PAYLOAD %s 5 %q 0xFFFF %q application/json\n%s\nFUNCTION_PAYLOAD_END\n", uid, route, source, payload)
		}
		require.NoError(t, err)
		output.waitContains(t, fmt.Sprintf("FUNCTION_RESULT_BEGIN %s %d ", uid, code))
	}
	assertProtected := func(phase string) {
		t.Helper()
		for _, verb := range []string{"add", "update", "enable", "disable", "remove", "restart"} {
			route := "config go.d:collector:module:job-000 " + verb
			payload := ""
			if verb == "add" {
				route = "config go.d:collector:module add job-000"
			}
			if verb == "add" || verb == "update" {
				payload = `{"option_str":"independent override","option_int":2}`
			}
			command(phase+"-"+verb, route, payload, "user=test", 403)
		}
		for _, verb := range []string{"enable", "disable"} {
			command(phase+"-echo-"+verb, "config go.d:collector:module:job-000 "+verb, "", "", 403)
		}
	}
	assertProtected("before")
	children := ownedTestChildren(1)
	ordinaryConfig, err := children[0].Config.Clone()
	require.NoError(t, err)
	ordinaryConfig.Set("module", "ordinary")
	_, err = owner.Prepare(t.Context(), 1, []ownedjobs.Child{{Key: "independent", Config: ordinaryConfig, Enabled: true}})
	require.ErrorContains(t, err, "does not declare parent-owned jobs")
	require.Empty(t, owner.Status())
	observeCollision := func(marker string) {
		t.Helper()
		collision, err := children[0].Config.Clone()
		require.NoError(t, err)
		collision.SetSourceType(confgroup.TypeUser).SetSource("file=independent").SetProvider("test")
		collision.Set("option_str", "source override")
		ordinary, err := collision.Clone()
		require.NoError(t, err)
		ordinary.Set("module", "ordinary")
		ordinary.Set("name", marker)
		batches <- []*confgroup.Group{{Source: "file=independent", Configs: []confgroup.Config{collision, ordinary}}}
		output.waitContains(t, "CONFIG go.d:collector:ordinary:"+marker+" status running")
	}
	observeCollision("before")
	// Successful preparation also proves ignored discovery did not reserve the ID.
	applyOwned(t, owner, 1, children)
	before := waitOwnedRunning(t, owner, 1)
	require.EqualValues(t, 1, ownedCreates.Load())
	assertProtected("running")
	observeCollision("after")
	command("after-barrier", "config go.d:collector:ordinary:after disable", "", "user=test", 200)
	after := waitOwnedRunning(t, owner, 1)
	require.Equal(t, before[0].Desired, after[0].Desired)
	require.Equal(t, before[0].Generation, after[0].Generation)
	require.EqualValues(t, 1, ownedCreates.Load())
	children[0].Enabled = false
	applyOwned(t, owner, 2, children)
	require.Eventually(t, func() bool {
		states := owner.Status()
		return len(states) == 1 && states[0].ConfigStatus == "disabled" && states[0].Desired == states[0].Applied
	}, 3*time.Second, time.Millisecond)
	assertProtected("disabled")
	require.NoError(t, owner.Retire(t.Context()))
	assertProtected("removed")
	// Ordinary collector configuration keeps its independent mutation surface.
	command("ordinary-add", "config go.d:collector:ordinary add independent", `{"option_str":"ordinary","option_int":1}`, "user=test", 202)
	command("ordinary-enable", "config go.d:collector:ordinary:independent enable", "", "user=test", 202)
	output.waitContains(t, "CONFIG go.d:collector:ordinary:independent status running")
	command("ordinary-disable", "config go.d:collector:ordinary:independent disable", "", "user=test", 200)
	command("ordinary-remove", "config go.d:collector:ordinary:independent remove", "", "user=test", 200)
}
