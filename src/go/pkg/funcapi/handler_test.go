// SPDX-License-Identifier: GPL-3.0-or-later

package funcapi

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

// mockHandler implements FunctionHandler for testing.
type mockHandler struct {
	methods      []MethodConfig
	methodParams []ParamConfig
	response     *FunctionResponse
}

func (m *mockHandler) Methods() []MethodConfig {
	return m.methods
}

func (m *mockHandler) MethodParams(method string) ([]ParamConfig, error) {
	return m.methodParams, nil
}

func (m *mockHandler) Handle(ctx context.Context, method string, params ResolvedParams) *FunctionResponse {
	return m.response
}

func TestFunctionHandler_Interface(t *testing.T) {
	// Verify mockHandler implements FunctionHandler
	var _ FunctionHandler = &mockHandler{}

	h := &mockHandler{
		methods:  []MethodConfig{{ID: "test", Name: "Test"}},
		response: &FunctionResponse{Status: 200},
	}

	assert.Len(t, h.Methods(), 1)
	assert.Equal(t, "test", h.Methods()[0].ID)

	params, err := h.MethodParams("test")
	assert.NoError(t, err)
	assert.Nil(t, params)

	resp := h.Handle(context.Background(), "test", nil)
	assert.Equal(t, 200, resp.Status)
}

func TestErrorResponse(t *testing.T) {
	resp := ErrorResponse(500, "error: %s", "test")

	assert.Equal(t, 500, resp.Status)
	assert.Equal(t, "error: test", resp.Message)
}

func TestErrorResponse_NoArgs(t *testing.T) {
	resp := ErrorResponse(400, "bad request")

	assert.Equal(t, 400, resp.Status)
	assert.Equal(t, "bad request", resp.Message)
}

func TestNotFoundResponse(t *testing.T) {
	resp := NotFoundResponse("my-method")

	assert.Equal(t, 404, resp.Status)
	assert.Contains(t, resp.Message, "my-method")
}

func TestUnavailableResponse(t *testing.T) {
	resp := UnavailableResponse("data not ready")

	assert.Equal(t, 503, resp.Status)
	assert.Equal(t, "data not ready", resp.Message)
}

func TestInternalErrorResponse(t *testing.T) {
	resp := InternalErrorResponse("failed: %v", "connection refused")

	assert.Equal(t, 500, resp.Status)
	assert.Equal(t, "failed: connection refused", resp.Message)
}
