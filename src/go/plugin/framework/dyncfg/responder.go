// SPDX-License-Identifier: GPL-3.0-or-later

package dyncfg

import (
	"fmt"

	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	fnpkg "github.com/netdata/netdata/go/plugins/plugin/framework/functions"
)

// Responder handles standardized responses for dyncfg operations
type Responder struct {
	output Output
}

// NewResponder creates a new responder
func NewResponder(output Output) *Responder {
	return &Responder{output: output}
}

// SendCodef sends a response with a specific code and message
func (r *Responder) SendCodef(fn Function, code int, message string, args ...any) {
	if fn.UID() == "" {
		return
	}

	msg := message
	if len(args) > 0 {
		msg = fmt.Sprintf(message, args...)
	}

	payload := fnpkg.BuildJSONPayload(code, msg)

	r.output.FunctionResult(Result{
		UID:         fn.UID(),
		Code:        code,
		ContentType: "application/json",
		Payload:     string(payload),
	})
}

// SendJSON sends a JSON payload response with HTTP 200 status
func (r *Responder) SendJSON(fn Function, payload string) {
	r.sendPayload(fn, payload, "application/json")
}

// SendYAML sends a YAML payload response
func (r *Responder) SendYAML(fn Function, payload string) {
	r.sendPayload(fn, payload, "application/yaml")
}

// sendPayload sends a response with a specific payload and content type
func (r *Responder) sendPayload(fn Function, payload, contentType string) {
	if fn.UID() == "" {
		return
	}

	r.output.FunctionResult(Result{
		UID:         fn.UID(),
		Code:        200,
		ContentType: contentType,
		Payload:     payload,
	})
}

func (r *Responder) ConfigCreate(opts netdataapi.ConfigOpts) {
	r.output.ConfigCreate(opts)
}

// ConfigStatus sends a CONFIG STATUS command
func (r *Responder) ConfigStatus(id string, status Status) {
	r.output.ConfigStatus(id, status)
}

// ConfigDelete sends a CONFIG DELETE command
func (r *Responder) ConfigDelete(id string) {
	r.output.ConfigDelete(id)
}
