// SPDX-License-Identifier: GPL-3.0-or-later

package snmp_traps

import (
	"context"
	"errors"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

const (
	reloadProfilesMethodID = "reload-profiles"
	snmpTrapsLogsMethodID  = "logs"
	snmpTrapsFunctionName  = "snmp:traps"
)

func reloadProfilesMethodConfig() funcapi.MethodConfig {
	return funcapi.MethodConfig{
		ID:           reloadProfilesMethodID,
		Name:         "Reload Trap Profiles",
		UpdateEvery:  0,
		Help:         "Re-parse all profile YAMLs and atomically swap in the new OID index",
		ResponseType: "text",
		AgentWide:    true,
	}
}

func snmpTrapsMethods() []funcapi.MethodConfig {
	return []funcapi.MethodConfig{
		reloadProfilesMethodConfig(),
		snmpTrapsLogsMethodConfig(),
	}
}

func snmpTrapsLogsMethodConfig() funcapi.MethodConfig {
	return funcapi.MethodConfig{
		ID:           snmpTrapsLogsMethodID,
		FunctionName: snmpTrapsFunctionName,
		Name:         "SNMP Trap Logs",
		UpdateEvery:  1,
		Help:         "Query SNMP trap journal entries received by SNMP trap listener jobs",
		RequireCloud: true,
		Tags:         "logs",
		ResponseType: "logs",
		Available:    directJournalLogsAvailable,
		RawRequest:   true,
		AgentWide:    true,
	}
}

func snmpTrapsMethodHandler(job collectorapi.RuntimeJob) funcapi.MethodHandler {
	c, ok := job.Collector().(*Collector)
	if !ok {
		return nil
	}
	return newSNMPTrapsFunctionHandler(c)
}

type profileReloadHandler struct {
	collector *Collector
}

var _ funcapi.MethodHandler = (*profileReloadHandler)(nil)

func (h *profileReloadHandler) MethodParams(_ context.Context, method string) ([]funcapi.ParamConfig, error) {
	if method != reloadProfilesMethodID {
		return nil, nil
	}
	return nil, nil
}

func (h *profileReloadHandler) Handle(_ context.Context, method string, _ funcapi.ResolvedParams) *funcapi.FunctionResponse {
	if method != reloadProfilesMethodID {
		return funcapi.NotFoundResponse(method)
	}

	if err := ReloadProfileCache(); err != nil {
		if h.collector != nil && h.collector.Logger != nil {
			h.collector.Errorf("SNMP trap profile reload failed: %v", err)
		}
		if errors.Is(err, errNoActiveProfileJobs) {
			return funcapi.UnavailableResponse(err.Error())
		}
		return funcapi.ErrorResponse(422, "profile reload failed: %v", err)
	}

	return &funcapi.FunctionResponse{
		Status:  200,
		Message: "profile reload successful",
	}
}

func (h *profileReloadHandler) Cleanup(_ context.Context) {
	// Reload uses the collector-owned profile cache and has no handler-local state.
}
