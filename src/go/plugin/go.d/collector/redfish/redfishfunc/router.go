// SPDX-License-Identifier: GPL-3.0-or-later

package redfishfunc

import (
	"context"
	"fmt"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/redfish/internal/measurement"
)

const (
	SensorsMethod  = "sensors"
	HardwareMethod = "hardware"
)

// Snapshot is a completed collection. Its slices and their contents are immutable
// after publication; an unavailable snapshot deliberately contains no old rows.
type Snapshot struct {
	CollectedAt         time.Time
	Available, Complete bool
	Components          []measurement.Component
	Sensors             []measurement.Sensor
}

type Deps interface {
	CurrentSnapshot() *Snapshot
}

type router struct{ deps Deps }

func NewRouter(deps Deps) funcapi.MethodHandler {
	return &router{
		deps: deps,
	}
}

func Methods(updateEvery int) []funcapi.FunctionConfig {
	return []funcapi.FunctionConfig{
		{
			ID:           SensorsMethod,
			Name:         "Redfish Sensors",
			UpdateEvery:  updateEvery,
			Help:         "Latest collected Redfish sensor readings and health",
			ResponseType: "table",
		},
		{
			ID:           HardwareMethod,
			Name:         "Redfish Hardware",
			UpdateEvery:  updateEvery,
			Help:         "Latest collected Redfish component health and inventory",
			ResponseType: "table",
		},
	}
}

func (r *router) MethodParams(_ context.Context, method string) ([]funcapi.ParamConfig, error) {
	if method != SensorsMethod && method != HardwareMethod {
		return nil, fmt.Errorf("unknown method: %s", method)
	}
	return nil, nil
}

func (r *router) Handle(ctx context.Context, method string, _ funcapi.ResolvedParams) *funcapi.FunctionResponse {
	if method != SensorsMethod && method != HardwareMethod {
		return funcapi.NotFoundResponse(method)
	}
	if ctx.Err() != nil {
		return funcapi.ErrorResponse(499, "Redfish Function request canceled")
	}
	snapshot := r.deps.CurrentSnapshot()
	if snapshot == nil {
		return funcapi.UnavailableResponse("Redfish data is unavailable; waiting for collection")
	}
	if !snapshot.Available {
		return funcapi.ErrorResponse(
			503,
			"Redfish data is unavailable; collection at %s did not produce usable data",
			snapshot.CollectedAt.UTC().Format(time.RFC3339),
		)
	}
	var response *funcapi.FunctionResponse
	if method == SensorsMethod {
		response = sensorsTable(ctx, snapshot.Sensors)
	} else {
		response = hardwareTable(ctx, snapshot.Components)
	}
	if ctx.Err() != nil {
		return funcapi.ErrorResponse(499, "Redfish Function request canceled")
	}
	coverage := "Complete"
	if !snapshot.Complete {
		coverage = "Partial; some resources or readings could not be collected"
	}
	response.Help = fmt.Sprintf(
		"%s collection at %s. Values come from the latest completed poll; opening this table does not query the BMC.",
		coverage,
		snapshot.CollectedAt.UTC().Format(time.RFC3339),
	)
	if len(response.Data.([][]any)) == 0 {
		response.Help += " No rows are available for this view in the current collection."
	}
	return response
}

func (*router) Cleanup(context.Context) {}
