// SPDX-License-Identifier: GPL-3.0-or-later

package dyncfg

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/functions"
)

// Responder handles standardized responses for dyncfg operations
type Responder struct {
	api *netdataapi.API
}

// NewResponder creates a new responder
func NewResponder(api *netdataapi.API) *Responder {
	return &Responder{api: api}
}

// SendCodef sends a response with a specific code and message
func (r *Responder) SendCodef(fn functions.Function, code int, message string, args ...any) {
	if fn.UID == "" {
		return
	}

	msg := message
	if len(args) > 0 {
		msg = fmt.Sprintf(message, args...)
	}

	response := struct {
		Status  int    `json:"status"`
		Message string `json:"message"`
	}{
		Status:  code,
		Message: msg,
	}

	payload, _ := json.Marshal(response)

	r.api.FUNCRESULT(netdataapi.FunctionResult{
		UID:             fn.UID,
		ContentType:     "application/json",
		Payload:         string(payload),
		Code:            strconv.Itoa(code),
		ExpireTimestamp: strconv.FormatInt(time.Now().Unix(), 10),
	})
}

// SendJSON sends a JSON payload response
func (r *Responder) SendJSON(fn functions.Function, payload string) {
	r.sendPayload(fn, payload, "application/json")
}

// SendYAML sends a YAML payload response
func (r *Responder) SendYAML(fn functions.Function, payload string) {
	r.sendPayload(fn, payload, "application/yaml")
}

// sendPayload sends a response with a specific payload and content type
func (r *Responder) sendPayload(fn functions.Function, payload, contentType string) {
	if fn.UID == "" {
		return
	}

	r.api.FUNCRESULT(netdataapi.FunctionResult{
		UID:             fn.UID,
		ContentType:     contentType,
		Payload:         payload,
		Code:            "200",
		ExpireTimestamp: strconv.FormatInt(time.Now().Unix(), 10),
	})
}

func (r *Responder) ConfigCreate(opts netdataapi.ConfigOpts) {
	r.api.CONFIGCREATE(opts)
}

// ConfigStatus sends a CONFIG STATUS command
func (r *Responder) ConfigStatus(id string, status Status) {
	r.api.CONFIGSTATUS(id, status.String())
}

// ConfigDelete sends a CONFIG DELETE command
func (r *Responder) ConfigDelete(id string) {
	r.api.CONFIGDELETE(id)
}
