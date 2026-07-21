// SPDX-License-Identifier: GPL-3.0-or-later

package dyncfg

import (
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	fnpkg "github.com/netdata/netdata/go/plugins/plugin/framework/functions"
)

// Responder handles standardized responses for dyncfg operations
type Responder struct {
	api *netdataapi.API

	finalizeMux sync.RWMutex
	finalize    fnpkg.TerminalFinalizer
}

// NewResponder creates a new responder
func NewResponder(api *netdataapi.API) *Responder {
	return &Responder{
		api:      api,
		finalize: fnpkg.DirectTerminalFinalizer,
	}
}

// SetTerminalFinalizer overrides terminal response finalization behavior.
func (r *Responder) SetTerminalFinalizer(finalize fnpkg.TerminalFinalizer) {
	r.finalizeMux.Lock()
	defer r.finalizeMux.Unlock()

	if finalize == nil {
		r.finalize = fnpkg.DirectTerminalFinalizer
		return
	}
	r.finalize = finalize
}

func (r *Responder) finalizeTerminal(uid, source string, emit func()) bool {
	r.finalizeMux.RLock()
	finalize := r.finalize
	r.finalizeMux.RUnlock()
	return finalize(uid, source, emit)
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

	res := netdataapi.FunctionResult{
		UID:             fn.UID(),
		ContentType:     "application/json",
		Payload:         string(payload),
		Code:            strconv.Itoa(code),
		ExpireTimestamp: strconv.FormatInt(time.Now().Unix(), 10),
	}
	r.finalizeTerminal(fn.UID(), "dyncfg.responder.sendcodef", func() {
		r.api.FUNCRESULT(res)
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

	res := netdataapi.FunctionResult{
		UID:             fn.UID(),
		ContentType:     contentType,
		Payload:         payload,
		Code:            "200",
		ExpireTimestamp: strconv.FormatInt(time.Now().Unix(), 10),
	}
	r.finalizeTerminal(fn.UID(), "dyncfg.responder.sendpayload", func() {
		r.api.FUNCRESULT(res)
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
