// SPDX-License-Identifier: GPL-3.0-or-later

package funcapi

import (
	"context"
	"fmt"
)

// FunctionHandler defines the interface for function implementations.
// Each function (e.g., "top-queries", "interfaces") should have its own handler.
//
// Example implementation:
//
//	type funcTopQueries struct {
//	    db *sql.DB
//	}
//
//	func (f *funcTopQueries) Methods() []MethodConfig {
//	    return []MethodConfig{{ID: "top-queries", Name: "Top Queries", ...}}
//	}
//
//	func (f *funcTopQueries) MethodParams(method string) ([]ParamConfig, error) {
//	    return nil, nil // or return dynamic params
//	}
//
//	func (f *funcTopQueries) Handle(ctx context.Context, method string, params ResolvedParams) *FunctionResponse {
//	    // query database and build response
//	}
type FunctionHandler interface {
	// Methods returns the method configurations this handler provides.
	Methods() []MethodConfig

	// MethodParams returns dynamic params for a method.
	// Return nil to use static params from MethodConfig.RequiredParams.
	MethodParams(method string) ([]ParamConfig, error)

	// Handle processes a function request and returns the response.
	// The context should be used for timeout/cancellation of database queries.
	Handle(ctx context.Context, method string, params ResolvedParams) *FunctionResponse
}

// ErrorResponse creates an error FunctionResponse.
func ErrorResponse(status int, format string, args ...any) *FunctionResponse {
	msg := format
	if len(args) > 0 {
		msg = fmt.Sprintf(format, args...)
	}
	return &FunctionResponse{
		Status:  status,
		Message: msg,
	}
}

// NotFoundResponse returns a 404 response for unknown methods.
func NotFoundResponse(method string) *FunctionResponse {
	return &FunctionResponse{
		Status:  404,
		Message: "unknown method: " + method,
	}
}

// UnavailableResponse returns a 503 response when data is not yet available.
func UnavailableResponse(msg string) *FunctionResponse {
	return &FunctionResponse{
		Status:  503,
		Message: msg,
	}
}

// InternalErrorResponse returns a 500 response for internal errors.
func InternalErrorResponse(format string, args ...any) *FunctionResponse {
	return &FunctionResponse{
		Status:  500,
		Message: fmt.Sprintf(format, args...),
	}
}
