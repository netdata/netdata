// SPDX-License-Identifier: GPL-3.0-or-later

package redfishfunc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	sdkjournal "github.com/netdata/systemd-journal-sdk/go/journal"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
)

type logsFunction struct {
	deps  Deps
	query sdkjournal.NetdataJournalFunction
}

func newLogsFunction(deps Deps) *logsFunction {
	cfg := sdkjournal.SystemdJournalNetdataFunctionConfig()
	cfg.FunctionName = "redfish:logs"
	cfg.SourceSelectorName = "Redfish Log Backends"
	cfg.SourceSelectorHelp = "Select the Redfish log backends to query"
	cfg.DefaultFacets = []string{
		"REDFISH_SEVERITY",
		"REDFISH_MESSAGE_ID",
		"REDFISH_ENTRY_TYPE",
		"REDFISH_RESOURCE_KIND",
		"REDFISH_HOST_NAME",
		"REDFISH_ENDPOINT_JOB",
		"REDFISH_BACKEND",
	}
	cfg.DefaultViewKeys = []string{
		"MESSAGE",
		"REDFISH_HOST_NAME",
		"REDFISH_SEVERITY",
		"REDFISH_MESSAGE_ID",
		"REDFISH_ENTRY_TYPE",
		"REDFISH_RESOURCE_KIND",
		"REDFISH_ENDPOINT_JOB",
		"REDFISH_BACKEND",
		"REDFISH_ENTRY_URI",
		"REDFISH_ENTRY_ID",
		"REDFISH_CREATED",
		"REDFISH_EVENT_TIMESTAMP",
		"REDFISH_JSON",
	}
	cfg.DefaultHistogram = "REDFISH_MESSAGE_ID"
	return &logsFunction{
		deps:  deps,
		query: sdkjournal.NewNetdataJournalFunction(cfg, sdkjournal.SystemdJournalProfile{}),
	}
}

func (f *logsFunction) handle(ctx context.Context, req funcapi.RawMethodRequest) *funcapi.FunctionResponse {
	root, namesToKeys, keysToNames, err := f.queryRoot()
	if err != nil {
		return funcapi.UnavailableResponse(err.Error())
	}
	payload, err := normalizeLogsPayload(req, namesToKeys)
	if err != nil {
		return funcapi.ErrorResponse(400, "Redfish logs query failed: %v", err)
	}
	options := sdkjournal.DefaultNetdataFunctionRunOptions()
	if req.Timeout > 0 {
		timeout := req.Timeout
		options.Timeout = &timeout
	}
	options.CancellationCallback = func() bool {
		return ctx.Err() != nil
	}
	options.State = redfishLogsState{root: root, keysToNames: keysToNames}
	response, err := f.query.RunDirectoryRequestBytesWithOptions(root, payload, options)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return funcapi.ErrorResponse(499, "Redfish logs query canceled")
		}
		return funcapi.ErrorResponse(400, "Redfish logs query failed: %v", err)
	}
	return funcapi.RawResponse(response)
}

func (f *logsFunction) queryRoot() (string, map[string]string, map[string]string, error) {
	roots := f.deps.LogRoots()
	if len(roots) == 0 {
		return "", nil, nil, errors.New("no ready Redfish log backend")
	}
	namesToKeys := make(map[string]string, len(roots))
	keysToNames := make(map[string]string, len(roots))
	var parent string
	for name, root := range roots {
		clean := filepath.Clean(root)
		info, err := os.Stat(clean)
		if err != nil || !info.IsDir() {
			continue
		}
		currentParent := filepath.Dir(clean)
		if parent == "" {
			parent = currentParent
		} else if currentParent != parent {
			return "", nil, nil, errors.New("Redfish log backends do not share one storage root")
		}
		key := filepath.Base(clean)
		namesToKeys[name] = key
		keysToNames[key] = name
	}
	if parent == "" || len(namesToKeys) == 0 {
		return "", nil, nil, errors.New("no queryable Redfish log backend")
	}
	return parent, namesToKeys, keysToNames, nil
}

func normalizeLogsPayload(req funcapi.RawMethodRequest, namesToKeys map[string]string) ([]byte, error) {
	if req.Info {
		return []byte(`{"info":true}`), nil
	}
	payload := bytes.TrimSpace(req.Payload)
	if len(payload) == 0 {
		payload = []byte(`{}`)
	}
	var object map[string]any
	if err := json.Unmarshal(payload, &object); err != nil {
		return nil, fmt.Errorf("invalid JSON request: %w", err)
	}
	if object == nil {
		return nil, errors.New("request must be a JSON object")
	}
	rawSelections, selectionsPresent := object["selections"]
	selections, selectionsObject := rawSelections.(map[string]any)
	if selectionsPresent && rawSelections != nil && !selectionsObject {
		return nil, errors.New("selections must be an object")
	}
	if !selectionsObject {
		selections = make(map[string]any)
		object["selections"] = selections
	}
	if raw, ok := selections["__logs_sources"]; ok {
		values, err := stringSelectionValues(raw)
		if err != nil {
			return nil, fmt.Errorf("__logs_sources: %w", err)
		}
		mapped := make([]string, 0, len(values))
		for _, value := range values {
			if value == "all" {
				mapped = append(mapped, value)
				continue
			}
			if _, ok := namesToKeys[value]; !ok {
				return nil, fmt.Errorf("unknown backend %q", value)
			}
			mapped = append(mapped, value)
		}
		selections["__logs_sources"] = mapped
	}
	if raw, ok := object["facets"]; ok {
		values, err := stringSelectionValues(raw)
		if err == nil && len(values) == 0 {
			object["facets"] = []string{
				"REDFISH_SEVERITY",
				"REDFISH_MESSAGE_ID",
				"REDFISH_ENTRY_TYPE",
				"REDFISH_RESOURCE_KIND",
				"REDFISH_HOST_NAME",
				"REDFISH_ENDPOINT_JOB",
				"REDFISH_BACKEND",
			}
		}
	}
	return json.Marshal(object)
}

func stringSelectionValues(value any) ([]string, error) {
	switch typed := value.(type) {
	case []any:
		result := make([]string, 0, len(typed))
		for _, item := range typed {
			text, ok := item.(string)
			if !ok || strings.TrimSpace(text) == "" {
				return nil, errors.New("values must be non-empty strings")
			}
			result = append(result, text)
		}
		return result, nil
	case []string:
		return typed, nil
	default:
		return nil, errors.New("must be an array")
	}
}

type redfishLogsState struct {
	root        string
	keysToNames map[string]string
}

func (s redfishLogsState) FileMetadata(path string) *sdkjournal.NetdataJournalFileMetadata {
	relative, err := filepath.Rel(s.root, path)
	if err != nil {
		return nil
	}
	relative = filepath.Clean(relative)
	if relative == "." || relative == ".." ||
		strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
		return nil
	}
	parts := strings.Split(filepath.ToSlash(relative), "/")
	if len(parts) == 0 {
		return nil
	}
	name, ok := s.keysToNames[parts[0]]
	if !ok {
		return nil
	}
	sourceType := uint64(sdkjournal.NetdataSourceTypeAll)
	return &sdkjournal.NetdataJournalFileMetadata{
		SourceType: &sourceType,
		SourceName: name,
	}
}

func (redfishLogsState) UpdateFileJournalVsRealtimeDeltaUsec(string, uint64) {
	// The Redfish backend does not publish per-file reader lag.
}
