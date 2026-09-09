// SPDX-License-Identifier: GPL-3.0-or-later

package dyncfg

import (
	"io"
	"strconv"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
)

// Result is one terminal DynCfg Function result.
type Result struct {
	UID         string
	Code        int
	ContentType string
	Payload     string
}

// Output receives typed DynCfg results and configuration notifications.
type Output interface {
	FunctionResult(Result)
	ConfigCreate(netdataapi.ConfigOpts)
	ConfigStatus(string, Status)
	ConfigDelete(string)
}

type protocolOutput struct {
	api *netdataapi.API
}

// NewProtocolOutput writes typed DynCfg output using the plugins.d protocol.
func NewProtocolOutput(writer io.Writer) Output {
	return &protocolOutput{api: netdataapi.New(writer)}
}

func (output *protocolOutput) FunctionResult(result Result) {
	output.api.FUNCRESULT(netdataapi.FunctionResult{
		UID:             result.UID,
		ContentType:     result.ContentType,
		Payload:         result.Payload,
		Code:            strconv.Itoa(result.Code),
		ExpireTimestamp: strconv.FormatInt(time.Now().Unix(), 10),
	})
}

func (output *protocolOutput) ConfigCreate(opts netdataapi.ConfigOpts) {
	output.api.CONFIGCREATE(opts)
}

func (output *protocolOutput) ConfigStatus(id string, status Status) {
	output.api.CONFIGSTATUS(id, status.String())
}

func (output *protocolOutput) ConfigDelete(id string) {
	output.api.CONFIGDELETE(id)
}

var _ Output = (*protocolOutput)(nil)
