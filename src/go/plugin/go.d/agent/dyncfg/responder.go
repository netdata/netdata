// SPDX-License-Identifier: GPL-3.0-or-later

package dyncfg

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
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
func (r *Responder) SendCodef(fn Function, code int, message string, args ...any) {
	if fn.UID() == "" {
		return
	}

	msg := message
	if len(args) > 0 {
		msg = fmt.Sprintf(message, args...)
	}

	var payload []byte
	if code >= 400 && code < 600 {
		payload, _ = json.Marshal(struct {
			Status       int    `json:"status"`
			ErrorMessage string `json:"errorMessage"`
		}{
			Status:       code,
			ErrorMessage: msg,
		})
	} else {
		payload, _ = json.Marshal(struct {
			Status  int    `json:"status"`
			Message string `json:"message"`
		}{
			Status:  code,
			Message: msg,
		})
	}

	r.api.FUNCRESULT(netdataapi.FunctionResult{
		UID:             fn.UID(),
		ContentType:     "application/json",
		Payload:         string(payload),
		Code:            strconv.Itoa(code),
		ExpireTimestamp: strconv.FormatInt(time.Now().Unix(), 10),
	})
}

// SendJSON sends a JSON payload response with HTTP 200 status
func (r *Responder) SendJSON(fn Function, payload string) {
	r.sendPayload(fn, payload, "application/json")
}

// SendJSONWithCode sends a JSON payload response with a specific HTTP status code
func (r *Responder) SendJSONWithCode(fn Function, payload string, code int) {
	if fn.UID() == "" {
		return
	}

	r.api.FUNCRESULT(netdataapi.FunctionResult{
		UID:             fn.UID(),
		ContentType:     "application/json",
		Payload:         payload,
		Code:            strconv.Itoa(code),
		ExpireTimestamp: strconv.FormatInt(time.Now().Unix(), 10),
	})
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

	r.api.FUNCRESULT(netdataapi.FunctionResult{
		UID:             fn.UID(),
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

// FunctionGlobal registers a global function with Netdata
func (r *Responder) FunctionGlobal(opts netdataapi.FunctionGlobalOpts) {
	r.api.FUNCTIONGLOBAL(opts)
}

// FunctionRemove removes a function from Netdata (no-op until Netdata core supports it)
func (r *Responder) FunctionRemove(name string) {
	r.api.FUNCTIONREMOVE(name)
}
