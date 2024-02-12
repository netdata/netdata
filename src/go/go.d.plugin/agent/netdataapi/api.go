// SPDX-License-Identifier: GPL-3.0-or-later

package netdataapi

import (
	"bytes"
	"fmt"
	"io"
	"strconv"
)

type (
	// API implements Netdata external plugins API.
	// https://learn.netdata.cloud/docs/agent/collectors/plugins.d#the-output-of-the-plugin
	API struct {
		io.Writer
	}
)

const quotes = "' '"

var (
	end          = []byte("END\n\n")
	clabelCommit = []byte("CLABEL_COMMIT\n")
	newLine      = []byte("\n")
)

func New(w io.Writer) *API { return &API{w} }

// CHART  creates or update a chart.
func (a *API) CHART(
	typeID string,
	ID string,
	name string,
	title string,
	units string,
	family string,
	context string,
	chartType string,
	priority int,
	updateEvery int,
	options string,
	plugin string,
	module string) error {
	_, err := a.Write([]byte("CHART " + "'" +
		typeID + "." + ID + quotes +
		name + quotes +
		title + quotes +
		units + quotes +
		family + quotes +
		context + quotes +
		chartType + quotes +
		strconv.Itoa(priority) + quotes +
		strconv.Itoa(updateEvery) + quotes +
		options + quotes +
		plugin + quotes +
		module + "'\n"))
	return err
}

// DIMENSION adds or update a dimension to the chart just created.
func (a *API) DIMENSION(
	ID string,
	name string,
	algorithm string,
	multiplier int,
	divisor int,
	options string) error {
	_, err := a.Write([]byte("DIMENSION '" +
		ID + quotes +
		name + quotes +
		algorithm + quotes +
		strconv.Itoa(multiplier) + quotes +
		strconv.Itoa(divisor) + quotes +
		options + "'\n"))
	return err
}

// CLABEL adds or update a label to the chart.
func (a *API) CLABEL(key, value string, source int) error {
	_, err := a.Write([]byte("CLABEL '" +
		key + quotes +
		value + quotes +
		strconv.Itoa(source) + "'\n"))
	return err
}

// CLABELCOMMIT adds labels to the chart. Should be called after one or more CLABEL.
func (a *API) CLABELCOMMIT() error {
	_, err := a.Write(clabelCommit)
	return err
}

// BEGIN initializes data collection for a chart.
func (a *API) BEGIN(typeID string, ID string, msSince int) (err error) {
	if msSince > 0 {
		_, err = a.Write([]byte("BEGIN " + "'" + typeID + "." + ID + "' " + strconv.Itoa(msSince) + "\n"))
	} else {
		_, err = a.Write([]byte("BEGIN " + "'" + typeID + "." + ID + "'\n"))
	}
	return err
}

// SET sets the value of a dimension for the initialized chart.
func (a *API) SET(ID string, value int64) error {
	_, err := a.Write([]byte("SET '" + ID + "' = " + strconv.FormatInt(value, 10) + "\n"))
	return err
}

// SETEMPTY sets the empty value of a dimension for the initialized chart.
func (a *API) SETEMPTY(ID string) error {
	_, err := a.Write([]byte("SET '" + ID + "' = \n"))
	return err
}

// VARIABLE sets the value of a CHART scope variable for the initialized chart.
func (a *API) VARIABLE(ID string, value int64) error {
	_, err := a.Write([]byte("VARIABLE CHART '" + ID + "' = " + strconv.FormatInt(value, 10) + "\n"))
	return err
}

// END completes data collection for the initialized chart.
func (a *API) END() error {
	_, err := a.Write(end)
	return err
}

// DISABLE disables this plugin. This will prevent Netdata from restarting the plugin.
func (a *API) DISABLE() error {
	_, err := a.Write([]byte("DISABLE\n"))
	return err
}

// EMPTYLINE writes an empty line.
func (a *API) EMPTYLINE() error {
	_, err := a.Write(newLine)
	return err
}

func (a *API) HOSTINFO(guid, hostname string, labels map[string]string) error {
	if err := a.HOSTDEFINE(guid, hostname); err != nil {
		return err
	}
	for k, v := range labels {
		if err := a.HOSTLABEL(k, v); err != nil {
			return err
		}
	}
	return a.HOSTDEFINEEND()
}

func (a *API) HOSTDEFINE(guid, hostname string) error {
	_, err := fmt.Fprintf(a, "HOST_DEFINE '%s' '%s'\n", guid, hostname)
	return err
}

func (a *API) HOSTLABEL(name, value string) error {
	_, err := fmt.Fprintf(a, "HOST_LABEL '%s' '%s'\n", name, value)
	return err
}

func (a *API) HOSTDEFINEEND() error {
	_, err := fmt.Fprintf(a, "HOST_DEFINE_END\n\n")
	return err
}

func (a *API) HOST(guid string) error {
	_, err := a.Write([]byte("HOST " + "'" + guid + "'" + "\n\n"))
	return err
}

func (a *API) DynCfgEnable(pluginName string) error {
	_, err := a.Write([]byte("DYNCFG_ENABLE '" + pluginName + "'\n\n"))
	return err
}

func (a *API) DynCfgReset() error {
	_, err := a.Write([]byte("DYNCFG_RESET\n"))
	return err
}

func (a *API) DyncCfgRegisterModule(moduleName string) error {
	_, err := fmt.Fprintf(a, "DYNCFG_REGISTER_MODULE '%s' job_array\n\n", moduleName)
	return err
}

func (a *API) DynCfgRegisterJob(moduleName, jobName, jobType string) error {
	_, err := fmt.Fprintf(a, "DYNCFG_REGISTER_JOB '%s' '%s' '%s' 0\n\n", moduleName, jobName, jobType)
	return err
}

func (a *API) DynCfgReportJobStatus(moduleName, jobName, status, reason string) error {
	_, err := fmt.Fprintf(a, "REPORT_JOB_STATUS '%s' '%s' '%s' 0 '%s'\n\n", moduleName, jobName, status, reason)
	return err
}

func (a *API) FunctionResultSuccess(uid, contentType, payload string) error {
	return a.functionResult(uid, contentType, payload, "1")
}

func (a *API) FunctionResultReject(uid, contentType, payload string) error {
	return a.functionResult(uid, contentType, payload, "0")
}

func (a *API) functionResult(uid, contentType, payload, code string) error {
	var buf bytes.Buffer

	buf.WriteString("FUNCTION_RESULT_BEGIN " + uid + " " + code + " " + contentType + " 0\n")
	if payload != "" {
		buf.WriteString(payload + "\n")
	}
	buf.WriteString("FUNCTION_RESULT_END\n\n")

	_, err := buf.WriteTo(a)
	return err
}
