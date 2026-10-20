// SPDX-License-Identifier: GPL-3.0-or-later

package redfishfunc

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/redfish/internal/acquisition"
)

const logsHelp = "Browse events currently retained by one BMC log service. Each refresh reads the complete log; BMC rotation or clearing can remove events. This is not a local archive or an atomic snapshot."

func logsMethod(updateEvery int) funcapi.FunctionConfig {
	return funcapi.FunctionConfig{
		ID:             LogsMethod,
		Name:           "Redfish Logs",
		UpdateEvery:    updateEvery,
		Help:           logsHelp,
		Tags:           "logs",
		ResponseType:   "table",
		RawRequest:     true,
		ManagedInfo:    true,
		HasHistory:     true,
		AcceptedParams: []string{"after", "before", "query"},
		RequiredParams: []funcapi.ParamConfig{logServiceParam(nil), logSeverityParam()},
	}
}

func logServiceParam(services []acquisition.LogService) funcapi.ParamConfig {
	options := make([]funcapi.ParamOption, 0, len(services))
	for _, s := range services {
		name := s.URI
		if s.Name != "" {
			name = s.Name + " (" + s.URI + ")"
		}
		options = append(options, funcapi.ParamOption{
			ID:   s.URI,
			Name: name,
		})
	}
	return funcapi.ParamConfig{
		ID:         "service",
		Name:       "Log service",
		Help:       "Select a linked BMC log service",
		Options:    options,
		UniqueView: true,
	}
}

func logSeverityParam() funcapi.ParamConfig {
	return funcapi.ParamConfig{
		ID:   "severity",
		Name: "Severity",
		Options: []funcapi.ParamOption{
			{ID: "all", Name: "All severities", Default: true},
			{
				ID:   "Critical",
				Name: "Critical",
			}, {ID: "Warning", Name: "Warning"}, {ID: "OK", Name: "OK"}, {ID: "Unknown", Name: "Unknown"},
		},
	}
}

func (r *router) HandleRaw(ctx context.Context, req funcapi.RawMethodRequest) *funcapi.FunctionResponse {
	if req.Method != LogsMethod {
		return funcapi.NotFoundResponse(req.Method)
	}
	reader := r.deps.Logs()
	if reader == nil {
		return funcapi.UnavailableResponse("Redfish log access is unavailable")
	}
	if req.Info {
		services, err := reader.Services(ctx)
		if err != nil {
			return logError(err)
		}
		help := logsHelp
		if len(services) == 0 {
			help += " No linked log services were advertised by Systems, Managers or Chassis."
		}
		return &funcapi.FunctionResponse{
			Status:         200,
			Help:           help,
			RequiredParams: []funcapi.ParamConfig{logServiceParam(services)},
		}
	}
	query, err := parseLogQuery(req, time.Now())
	if err != nil {
		return funcapi.ErrorResponse(400, "%s", err)
	}
	result, err := reader.Entries(ctx, query.service)
	if err != nil {
		return logError(err)
	}
	response := logsTable(ctx, result.Entries, query)
	response.RequiredParams = []funcapi.ParamConfig{logServiceParam(result.Services)}
	if ctx.Err() != nil {
		return logError(ctx.Err())
	}
	return response
}

func logError(err error) *funcapi.FunctionResponse {
	switch {
	case errors.Is(err, context.Canceled):
		return funcapi.ErrorResponse(499, "Redfish log request canceled")
	case errors.Is(err, context.DeadlineExceeded):
		return funcapi.ErrorResponse(504, "Redfish log request timed out before the complete log could be read")
	case errors.Is(err, acquisition.ErrLogServiceUnavailable), errors.Is(err, acquisition.ErrLogEntriesUnsupported):
		return funcapi.ErrorResponse(400, "%s", err)
	default:
		return funcapi.UnavailableResponse(fmt.Sprintf("Redfish log request failed: %s", err))
	}
}
