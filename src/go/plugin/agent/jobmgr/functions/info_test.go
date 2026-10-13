// SPDX-License-Identifier: GPL-3.0-or-later

package functions

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type functionInfoHarness struct {
	controller *Controller
	catalog    *Catalog
	handles    map[string]*JobHandle
	uid        int
	route      string
}

func newFunctionInfoHarness(
	t *testing.T,
	method funcapi.FunctionConfig,
	mode string,
	newHandler func(collectorapi.RuntimeJob) funcapi.MethodHandler,
) *functionInfoHarness {
	t.Helper()
	creator := collectorapi.Creator{
		MethodHandler: newHandler,
	}
	names := []string{"alpha", "beta rack,1"}
	methods := func() []funcapi.FunctionConfig { return []funcapi.FunctionConfig{method} }
	switch mode {
	case "agent":
		creator.AgentFunctions, names = methods, nil
	case "single":
		creator.SharedFunctions, creator.InstancePolicy, names = methods, collectorapi.InstancePolicySingle, []string{
			"module",
		}
	case "instance":
		creator.InstanceFunctions, names = func(collectorapi.RuntimeJob) []funcapi.FunctionConfig { return methods() }, []string{
			"alpha",
		}
	default:
		creator.SharedFunctions = methods
	}
	c, catalog, err := newContainedControllerTest(t, 1, collectorapi.Registry{
		"module": creator,
	})
	require.NoError(t, err)
	publication, err := NewPublication(1, newRecordingPublicationPort())
	require.NoError(t, err)
	require.NoError(t, c.Bind(&controllerTestMutationPort{
		catalog: catalog,
	}, publication))
	require.NoError(t, c.Activate())
	h := &functionInfoHarness{
		controller: c,
		catalog:    catalog,
		handles:    make(map[string]*JobHandle),
		route:      "module:" + method.ID,
	}
	for _, name := range names {
		job := &controllerTestJob{
			fullName: "module_" + name,
			module:   "module",
			name:     name,
			running:  true,
		}
		handle, err := prepareControllerTestJob(
			t,
			c,
			lifecycle.ResourceIdentity{
				ID:         job.FullName(),
				Generation: 1,
			},
			job,
		)
		require.NoError(t, err)
		require.NoError(t, handle.Publish())
		h.handles[name] = handle
	}
	return h
}

func (h *functionInfoHarness) call(t *testing.T, args []string, payload []byte, status int) []byte {
	t.Helper()
	h.uid++
	decision, err := h.catalog.ResolveAndAcquire(jobmgr.FunctionLookup{
		UID:         fmt.Sprintf("metadata-%d", h.uid),
		Route:       h.route,
		Args:        args,
		Payload:     payload,
		HasPayload:  len(payload) != 0,
		ContentType: "application/json",
		Timeout:     10 * time.Second,
	})
	require.NoError(t, err)
	require.Zero(t, decision.Rejected)
	var output bytes.Buffer
	owner, err := lifecycle.NewFrameOwner(&output)
	require.NoError(t, err)
	supervisor, err := lifecycle.NewTaskSupervisor(owner)
	require.NoError(t, err)
	_, err = supervisor.Enqueue(
		lifecycle.TaskClassGenericFunction,
		lifecycle.TaskPlan{
			Source: lifecycle.SourceFunction,
			Work:   decision.Plan.Work,
		},
	)
	require.NoError(t, err)
	var started [lifecycle.TaskStartServiceQuantum]lifecycle.TaskStart
	count, _, err := supervisor.Dispatch(t.Context(), 1, &started)
	require.NoError(t, err)
	require.Equal(t, 1, count)
	completion := <-supervisor.CompletionCh()
	require.NoError(t, completion.Err)
	require.NoError(t, supervisor.SendAction(lifecycle.TaskAction{
		Ref:      completion.Ref,
		Sequence: 2,
		Kind:     lifecycle.TaskActionEncodeWrite,
		UID:      "function-test",
		Expiry:   1,
	}))
	ack := <-supervisor.AcknowledgementCh()
	require.NoError(t, ack.Err)
	require.NoError(t, supervisor.SendAction(lifecycle.TaskAction{
		Ref:      completion.Ref,
		Sequence: 3,
		Kind:     lifecycle.TaskActionTerminate,
	}))
	ack = <-supervisor.AcknowledgementCh()
	require.NoError(t, ack.Err)
	require.NoError(t, supervisor.Release(completion.Ref))
	cleanup, err := h.catalog.ReleaseInvocation(decision.Lease)
	require.NoError(t, err)
	require.False(t, cleanup.Valid())
	return functionFramePayload(t, output.String(), status)
}

func assertFunctionInfoSchema(t *testing.T, payload []byte, definition string) {
	t.Helper()
	data, err := os.ReadFile("../../../../../plugins.d/FUNCTION_UI_SCHEMA.json")
	require.NoError(t, err)
	var document, response any
	require.NoError(t, json.Unmarshal(data, &document))
	require.NoError(t, json.Unmarshal(payload, &response))
	compiler := jsonschema.NewCompiler()
	require.NoError(t, compiler.AddResource("function.json", document))
	schema, err := compiler.Compile("function.json#/definitions/" + definition)
	require.NoError(t, err)
	assert.NoError(t, schema.Validate(response))
}
