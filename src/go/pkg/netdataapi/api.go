// SPDX-License-Identifier: GPL-3.0-or-later

package netdataapi

import (
	"bytes"
	"fmt"
	"io"
	"strconv"
)

// API implements Netdata external plugins API.
// See: https://learn.netdata.cloud/docs/agent/plugins.d#the-output-of-the-plugin
type API struct {
	io.Writer
}

const quotes = "' '"

var (
	end          = []byte("END\n\n")
	clabelCommit = []byte("CLABEL_COMMIT\n")
	newLine      = []byte("\n")
)

// New creates a new API instance for interacting with Netdata.
// Panics if the provided writer is nil.
func New(w io.Writer) *API {
	if w == nil {
		panic("writer cannot be nil")
	}
	return &API{w}
}

// CHART creates or updates a chart.
func (a *API) CHART(opts ChartOpts) {
	_, _ = a.Write([]byte("CHART " + "'" +
		opts.TypeID + "." + opts.ID + quotes +
		opts.Name + quotes +
		opts.Title + quotes +
		opts.Units + quotes +
		opts.Family + quotes +
		opts.Context + quotes +
		opts.ChartType + quotes +
		strconv.Itoa(opts.Priority) + quotes +
		strconv.Itoa(opts.UpdateEvery) + quotes +
		opts.Options + quotes +
		opts.Plugin + quotes +
		opts.Module + "'\n"))
}

// DIMENSION adds or updates a dimension to the most recently created chart.
func (a *API) DIMENSION(opts DimensionOpts) {
	_, _ = a.Write([]byte("DIMENSION '" +
		opts.ID + quotes +
		opts.Name + quotes +
		opts.Algorithm + quotes +
		strconv.Itoa(opts.Multiplier) + quotes +
		strconv.Itoa(opts.Divisor) + quotes +
		opts.Options + "'\n"))
}

// CLABEL adds or updates a label to the most recently created chart.
func (a *API) CLABEL(key, value string, source int) {
	_, _ = a.Write([]byte("CLABEL '" +
		key + quotes +
		value + quotes +
		strconv.Itoa(source) + "'\n"))
}

// CLABELCOMMIT adds labels to the chart. Should be called after one or more CLABEL.
func (a *API) CLABELCOMMIT() {
	_, _ = a.Write(clabelCommit)
}

// BEGIN initializes data collection for a chart.
func (a *API) BEGIN(typeID string, id string, msSince int) {
	if msSince > 0 {
		_, _ = a.Write([]byte("BEGIN " + "'" + typeID + "." + id + "' " + strconv.Itoa(msSince) + "\n"))
	} else {
		_, _ = a.Write([]byte("BEGIN " + "'" + typeID + "." + id + "'\n"))
	}
}

// SET sets the value of a dimension for the initialized chart.
func (a *API) SET(id string, value int64) {
	_, _ = a.Write([]byte("SET '" + id + "' = " + strconv.FormatInt(value, 10) + "\n"))
}

// SETFLOAT sets the value of a dimension for the initialized chart.
func (a *API) SETFLOAT(id string, value float64) {
	v := strconv.FormatFloat(value, 'f', -1, 64)
	_, _ = a.Write([]byte("SET '" + id + "' = " + v + "\n"))
}

// SETEMPTY sets an empty value for a dimension in the initialized chart.
func (a *API) SETEMPTY(id string) {
	_, _ = a.Write([]byte("SET '" + id + "' = \n"))
}

// VARIABLE sets the value of a CHART scope variable for the initialized chart.
func (a *API) VARIABLE(ID string, value float64) {
	v := strconv.FormatFloat(value, 'f', -1, 64)
	_, _ = a.Write([]byte("VARIABLE CHART '" + ID + "' = " + v + "\n"))
}

// END completes data collection for the initialized chart.
// Should be called after all SET operations are complete.
func (a *API) END() {
	_, _ = a.Write(end)
}

// DISABLE disables this plugin.
// This will prevent Netdata from restarting the plugin.
func (a *API) DISABLE() {
	_, _ = a.Write([]byte("DISABLE\n"))
}

// EMPTYLINE writes an empty line to the output.
func (a *API) EMPTYLINE() error {
	_, err := a.Write(newLine)
	return err
}

// HOSTINFO defines a host with its labels.
func (a *API) HOSTINFO(info HostInfo) {
	var buf bytes.Buffer

	buf.WriteString(fmt.Sprintf("HOST_DEFINE '%s' '%s'\n", info.GUID, info.Hostname))
	for k, v := range info.Labels {
		buf.WriteString(fmt.Sprintf("HOST_LABEL '%s' '%s'\n", k, v))
	}
	buf.WriteString("HOST_DEFINE_END\n\n")

	_, _ = buf.WriteTo(a)
}

// HOST switches the current context to a specific host.
func (a *API) HOST(guid string) {
	_, _ = a.Write([]byte("HOST " + "'" + guid + "'\n\n"))
}

// FUNCRESULT writes a function result to Netdata.
func (a *API) FUNCRESULT(result FunctionResult) {
	var buf bytes.Buffer

	buf.WriteString("FUNCTION_RESULT_BEGIN " +
		result.UID + " " +
		result.Code + " " +
		result.ContentType + " " +
		result.ExpireTimestamp + "\n",
	)

	if result.Payload != "" {
		buf.WriteString(result.Payload + "\n")
	}

	buf.WriteString("FUNCTION_RESULT_END\n\n")

	_, _ = buf.WriteTo(a)
}

// CONFIGCREATE creates a new configuration
func (a *API) CONFIGCREATE(opts ConfigOpts) {
	// https://learn.netdata.cloud/docs/contributing/external-plugins/#config

	_, _ = a.Write([]byte("CONFIG " +
		opts.ID + " " +
		"create" + " " +
		opts.Status + " " +
		opts.ConfigType + " " +
		opts.Path + " " +
		opts.SourceType + " '" +
		opts.Source + "' '" +
		opts.SupportedCommands + "' 0x0000 0x0000\n\n",
	))
}

// CONFIGDELETE deletes a configuration
func (a *API) CONFIGDELETE(id string) {
	_, _ = a.Write([]byte("CONFIG " + id + " delete\n\n"))
}

// CONFIGSTATUS updates a configuration status
func (a *API) CONFIGSTATUS(id, status string) {
	_, _ = a.Write([]byte("CONFIG " + id + " status " + status + "\n\n"))
}
