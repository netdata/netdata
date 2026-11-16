// SPDX-License-Identifier: GPL-3.0-or-later

package netdataapi

import (
	"bytes"
	"fmt"
	"io"
	"sort"
	"strconv"
)

// API implements Netdata external plugins API.
// See: https://learn.netdata.cloud/docs/agent/plugins.d#the-output-of-the-plugin
//
// Hot-path commands append into the writer's available buffer when it has one
// (bytes.Buffer, bufio.Writer), otherwise into a scratch buffer reused across
// commands, so formatting does not allocate per command. That state makes an API
// unsafe for concurrent use; give each goroutine its own.
type API struct {
	io.Writer
	scratch []byte
	// direct reports whether the command being formatted uses the writer's buffer.
	direct bool
}

// availableBufferWriter is implemented by writers that expose their spare capacity.
type availableBufferWriter interface {
	AvailableBuffer() []byte
}

// line returns an empty buffer to append one command into.
func (a *API) line() []byte {
	if w, ok := a.Writer.(availableBufferWriter); ok {
		a.direct = true
		return w.AvailableBuffer()
	}
	a.direct = false
	return a.scratch[:0]
}

// send writes one appended command and keeps a grown scratch buffer.
func (a *API) send(line []byte) {
	_, _ = a.Write(line)
	if !a.direct {
		a.scratch = line[:0]
	}
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
	return &API{
		Writer: w,
	}
}

// CHART creates or updates a chart.
func (a *API) CHART(opts ChartOpts) {
	b := append(a.line(), "CHART '"...)
	b = append(b, opts.TypeID...)
	b = append(b, '.')
	b = append(b, opts.ID...)
	b = append(b, quotes...)
	b = append(b, opts.Name...)
	b = append(b, quotes...)
	b = append(b, opts.Title...)
	b = append(b, quotes...)
	b = append(b, opts.Units...)
	b = append(b, quotes...)
	b = append(b, opts.Family...)
	b = append(b, quotes...)
	b = append(b, opts.Context...)
	b = append(b, quotes...)
	b = append(b, opts.ChartType...)
	b = append(b, quotes...)
	b = strconv.AppendInt(b, int64(opts.Priority), 10)
	b = append(b, quotes...)
	b = strconv.AppendInt(b, int64(opts.UpdateEvery), 10)
	b = append(b, quotes...)
	b = append(b, opts.Options...)
	b = append(b, quotes...)
	b = append(b, opts.Plugin...)
	b = append(b, quotes...)
	b = append(b, opts.Module...)
	b = append(b, "'\n"...)
	a.send(b)
}

// DIMENSION adds or updates a dimension to the most recently created chart.
func (a *API) DIMENSION(opts DimensionOpts) {
	b := append(a.line(), "DIMENSION '"...)
	b = append(b, opts.ID...)
	b = append(b, quotes...)
	b = append(b, opts.Name...)
	b = append(b, quotes...)
	b = append(b, opts.Algorithm...)
	b = append(b, quotes...)
	b = strconv.AppendInt(b, int64(opts.Multiplier), 10)
	b = append(b, quotes...)
	b = strconv.AppendInt(b, int64(opts.Divisor), 10)
	b = append(b, quotes...)
	b = append(b, opts.Options...)
	b = append(b, "'\n"...)
	a.send(b)
}

// CLABEL adds or updates a label to the most recently created chart.
func (a *API) CLABEL(key, value string, source int) {
	b := append(a.line(), "CLABEL '"...)
	b = append(b, key...)
	b = append(b, quotes...)
	b = append(b, value...)
	b = append(b, quotes...)
	b = strconv.AppendInt(b, int64(source), 10)
	b = append(b, "'\n"...)
	a.send(b)
}

// CLABELCOMMIT adds labels to the chart. Should be called after one or more CLABEL.
func (a *API) CLABELCOMMIT() {
	_, _ = a.Write(clabelCommit)
}

// BEGIN initializes data collection for a chart.
func (a *API) BEGIN(typeID string, id string, msSince int) {
	b := append(a.line(), "BEGIN '"...)
	b = append(b, typeID...)
	b = append(b, '.')
	b = append(b, id...)
	b = append(b, '\'')
	if msSince > 0 {
		b = append(b, ' ')
		b = strconv.AppendInt(b, int64(msSince), 10)
	}
	b = append(b, '\n')
	a.send(b)
}

// SET sets the value of a dimension for the initialized chart.
func (a *API) SET(id string, value int64) {
	b := a.setPrefix(id)
	b = strconv.AppendInt(b, value, 10)
	a.send(append(b, '\n'))
}

// SETFLOAT sets the value of a dimension for the initialized chart.
func (a *API) SETFLOAT(id string, value float64) {
	b := a.setPrefix(id)
	b = strconv.AppendFloat(b, value, 'f', -1, 64)
	a.send(append(b, '\n'))
}

// SETEMPTY sets an empty value for a dimension in the initialized chart.
func (a *API) SETEMPTY(id string) {
	a.send(append(a.setPrefix(id), '\n'))
}

func (a *API) setPrefix(id string) []byte {
	b := append(a.line(), "SET '"...)
	b = append(b, id...)
	return append(b, "' = "...)
}

// VARIABLE sets the value of a CHART scope variable for the initialized chart.
func (a *API) VARIABLE(ID string, value float64) {
	b := append(a.line(), "VARIABLE CHART '"...)
	b = append(b, ID...)
	b = append(b, "' = "...)
	b = strconv.AppendFloat(b, value, 'f', -1, 64)
	a.send(append(b, '\n'))
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

	_, _ = fmt.Fprintf(&buf, "HOST_DEFINE '%s' '%s'\n", info.GUID, info.Hostname)
	keys := make([]string, 0, len(info.Labels))
	for k := range info.Labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		v := info.Labels[k]
		_, _ = fmt.Fprintf(&buf, "HOST_LABEL '%s' '%s'\n", k, v)
	}
	buf.WriteString("HOST_DEFINE_END\n\n")

	_, _ = buf.WriteTo(a)
}

// HOST switches the current context to a specific host.
func (a *API) HOST(guid string) {
	b := append(a.line(), "HOST '"...)
	b = append(b, guid...)
	a.send(append(b, "'\n\n"...))
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

// CONFIGCREATE creates a new configuration. Invalid fields are ignored; use
// TryCONFIGCREATE when the caller must observe an encoding rejection.
func (a *API) CONFIGCREATE(opts ConfigOpts) {
	_ = a.TryCONFIGCREATE(opts)
}

// TryCONFIGCREATE creates a new configuration or returns an encoding error.
func (a *API) TryCONFIGCREATE(opts ConfigOpts) error {
	// https://learn.netdata.cloud/docs/contributing/external-plugins/#config

	if err := opts.Validate(); err != nil {
		return err
	}
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
	return nil
}

// CONFIGDELETE deletes a configuration. Invalid IDs are ignored; use
// TryCONFIGDELETE when the caller must observe an encoding rejection.
func (a *API) CONFIGDELETE(id string) {
	_ = a.TryCONFIGDELETE(id)
}

// TryCONFIGDELETE deletes a configuration or returns an encoding error.
func (a *API) TryCONFIGDELETE(id string) error {
	if !ValidBareProtocolField(id) {
		return fmt.Errorf("netdataapi: invalid CONFIG id")
	}
	_, _ = a.Write([]byte("CONFIG " + id + " delete\n\n"))
	return nil
}

// CONFIGSTATUS updates a configuration status. Invalid fields are ignored; use
// TryCONFIGSTATUS when the caller must observe an encoding rejection.
func (a *API) CONFIGSTATUS(id, status string) {
	_ = a.TryCONFIGSTATUS(id, status)
}

// TryCONFIGSTATUS updates a configuration status or returns an encoding error.
func (a *API) TryCONFIGSTATUS(id, status string) error {
	if !ValidBareProtocolField(id) {
		return fmt.Errorf("netdataapi: invalid CONFIG id")
	}
	if !ValidBareProtocolField(status) {
		return fmt.Errorf("netdataapi: invalid CONFIG status")
	}
	_, _ = a.Write([]byte("CONFIG " + id + " status " + status + "\n\n"))
	return nil
}

// FUNCTIONGLOBAL registers a global function with Netdata.
// Format: FUNCTION GLOBAL "<name>" <timeout> "<help>" "<tags>" <access> <priority> <version>
func (a *API) FUNCTIONGLOBAL(opts FunctionGlobalOpts) {
	_, _ = a.Write([]byte("FUNCTION GLOBAL \"" +
		opts.Name + "\" " +
		strconv.Itoa(opts.Timeout) + " \"" +
		opts.Help + "\" \"" +
		opts.Tags + "\" " +
		opts.Access + " " +
		strconv.Itoa(opts.Priority) + " " +
		strconv.Itoa(opts.Version) + "\n\n"))
}

// FUNCTIONREMOVE removes a function from Netdata.
func (a *API) FUNCTIONREMOVE(name string) {
	_, _ = a.Write([]byte("FUNCTION_DEL GLOBAL \"" + name + "\"\n\n"))
}
