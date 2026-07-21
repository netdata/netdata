// SPDX-License-Identifier: GPL-3.0-or-later

package composition

import (
	"bytes"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServiceDiscoveryBindingCapturesTypedInvocationOutput(t *testing.T) {
	tests := map[string]struct {
		emit              func(*serviceDiscoveryBinding)
		wantResult        string
		wantNotifications string
		wantError         string
	}{
		"result": {
			emit: func(binding *serviceDiscoveryBinding) {
				binding.FunctionResult(dyncfg.Result{
					UID: "uid", Code: 200, ContentType: "application/json", Payload: `{"ok":true}`,
				})
			},
			wantResult: "FUNCTION_RESULT_BEGIN result 200 application/json 1\n" +
				"{\"ok\":true}\nFUNCTION_RESULT_END\n\n",
		},
		"result and notification": {
			emit: func(binding *serviceDiscoveryBinding) {
				binding.FunctionResult(dyncfg.Result{
					UID: "uid", Code: 204, ContentType: "application/json",
				})
				binding.ConfigStatus("go.d:sd:type:job", dyncfg.StatusRunning)
			},
			wantResult: "FUNCTION_RESULT_BEGIN result 204 application/json 1\n" +
				"FUNCTION_RESULT_END\n\n",
			wantNotifications: "CONFIG go.d:sd:type:job status running\n\n",
		},
		"missing result": {
			emit:      func(*serviceDiscoveryBinding) {},
			wantError: "produced no terminal result",
		},
		"multiple results": {
			emit: func(binding *serviceDiscoveryBinding) {
				result := dyncfg.Result{UID: "uid", Code: 200, ContentType: "application/json"}
				binding.FunctionResult(result)
				binding.FunctionResult(result)
			},
			wantError: "produced multiple results",
		},
		"different result UID": {
			emit: func(binding *serviceDiscoveryBinding) {
				binding.FunctionResult(dyncfg.Result{
					UID: "other", Code: 200, ContentType: "application/json",
				})
			},
			wantError: "result UID differs from invocation",
		},
		"handler panic": {
			emit: func(*serviceDiscoveryBinding) {
				panic("failed")
			},
			wantError: "service discovery Function handler: failed",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			var notifications bytes.Buffer
			frames, err := lifecycle.NewFrameOwner(&notifications)
			require.NoError(t, err)
			binding, err := newServiceDiscoveryBinding(1, "go.d", frames)
			require.NoError(t, err)

			result, cleanup, err := binding.invoke("uid", func() {
				test.emit(binding)
			})
			if test.wantError != "" {
				require.ErrorContains(t, err, test.wantError)
				assert.Nil(t, cleanup)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, cleanup)
			require.NoError(t, cleanup())
			assert.Equal(t, test.wantNotifications, notifications.String())

			var encoded bytes.Buffer
			resultFrames, err := lifecycle.NewFrameOwner(&encoded)
			require.NoError(t, err)
			frame, err := lifecycle.PrepareFrame("result", result, 1)
			require.NoError(t, err)
			require.NoError(t, resultFrames.Commit(frame))
			assert.Equal(t, test.wantResult, encoded.String())
		})
	}
}

func TestServiceDiscoveryBindingRoutesNotificationsOutsideInvocations(t *testing.T) {
	var output bytes.Buffer
	frames, err := lifecycle.NewFrameOwner(&output)
	require.NoError(t, err)
	binding, err := newServiceDiscoveryBinding(1, "go.d", frames)
	require.NoError(t, err)

	binding.ConfigDelete("go.d:sd:type:gone")

	assert.Equal(t, "CONFIG go.d:sd:type:gone delete\n\n", output.String())
}

func TestServiceDiscoveryBindingRejectsResultOutsideInvocation(t *testing.T) {
	frames, err := lifecycle.NewFrameOwner(&bytes.Buffer{})
	require.NoError(t, err)
	binding, err := newServiceDiscoveryBinding(1, "go.d", frames)
	require.NoError(t, err)

	binding.FunctionResult(dyncfg.Result{
		UID: "late", Code: 200, ContentType: "application/json",
	})
	_, _, err = binding.invoke("next", func() {})

	require.ErrorContains(t, err, "result outside invocation")
}

func TestServiceDiscoveryHandlerPanicIsClassifiedAsTaskPanic(t *testing.T) {
	err := callServiceDiscoveryHandler(func() {
		panic("handler failed")
	})
	require.ErrorIs(t, err, lifecycle.ErrTaskPanic)
}
