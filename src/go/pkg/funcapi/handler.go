// SPDX-License-Identifier: GPL-3.0-or-later

package funcapi

import (
	"context"
	"fmt"
	"time"
)

// MethodHandler defines the interface for handling Function method requests.
// Functions are declared by collector creators; this interface handles requests.
//
// Example implementation:
//
//	type funcTopQueries struct {
//	    db *sql.DB
//	}
//
//	func (f *funcTopQueries) MethodParams(ctx context.Context, method string) ([]ParamConfig, error) {
//	    return nil, nil // or return dynamic params from database
//	}
//
//	func (f *funcTopQueries) Handle(ctx context.Context, method string, params ResolvedParams) *FunctionResponse {
//	    // query database and build response
//	}
type MethodHandler interface {
	// MethodParams returns dynamic params for a method.
	// Return nil to use static params from FunctionConfig.RequiredParams.
	// The context should be used for timeout/cancellation of database queries.
	MethodParams(ctx context.Context, method string) ([]ParamConfig, error)

	// Handle processes a method request and returns the response.
	// The context should be used for timeout/cancellation of database queries.
	Handle(ctx context.Context, method string, params ResolvedParams) *FunctionResponse

	// Cleanup releases any resources held by the handler.
	// Called when the collector is being stopped.
	Cleanup(ctx context.Context)
}

// RawMethodRequest is the complete Function request for methods that cannot be
// represented as predeclared selector parameters.
type RawMethodRequest struct {
	Method      string
	Info        bool
	Args        []string
	Payload     []byte
	ContentType string
	Timeout     time.Duration
	Permissions string
	Source      string
}

// RawMethodHandler handles methods whose request/response contract is owned by
// the collector or an embedded domain API instead of funcapi's table wrapper.
type RawMethodHandler interface {
	MethodHandler
	HandleRaw(ctx context.Context, req RawMethodRequest) *FunctionResponse
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

// RawResponse returns a complete Function response envelope.
func RawResponse(resp map[string]any) *FunctionResponse {
	return &FunctionResponse{
		RawResponse: resp,
	}
}
